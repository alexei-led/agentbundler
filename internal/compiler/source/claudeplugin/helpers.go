package claudeplugin

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/compiler/source/frontmatter"
)

type claudeInspector struct {
	workspaceRoot string
	filesystem    *os.Root
	sourceRoot    string
	inputs        map[model.RelativePath]string
	diagnostics   []model.Diagnostic
	native        []model.NativeGap
}

func (i *claudeInspector) findSkillFiles(root string) []string {
	var found []string
	var walk func(string)
	walk = func(directory string) {
		entries, err := i.readDir(directory)
		if err != nil {
			i.error(i.relativePath(directory), "read directory: "+err.Error())
			return
		}
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				i.error(i.relativePath(path), "source symlinks are not allowed")
				continue
			}
			if entry.IsDir() {
				if entry.Name() == ".agentbundler" {
					continue
				}
				walk(path)
				continue
			}
			if !entry.Type().IsRegular() {
				i.error(i.relativePath(path), "source entries must be regular files or directories")
				continue
			}
			if entry.Name() == "SKILL.md" {
				found = append(found, path)
			}
		}
	}
	walk(root)
	sort.Strings(found)
	return found
}

func (i *claudeInspector) inspectSkill(skillFile string) (model.SourceAsset, bool) {
	assetRoot := filepath.Dir(skillFile)
	name := filepath.Base(assetRoot)
	identity, err := model.NewAssetID("skill/" + name)
	if err != nil {
		i.error(i.relativePath(skillFile), "skill identity: "+err.Error())
		return model.SourceAsset{}, false
	}
	markdown, ok := i.readInput(skillFile)
	if !ok {
		return model.SourceAsset{}, false
	}
	frontmatter, body, err := parseFrontmatter(markdown)
	if err != nil {
		i.error(i.relativePath(skillFile), "skill frontmatter: "+err.Error())
		return model.SourceAsset{}, false
	}
	files := i.supportFiles(assetRoot, skillFile)
	capabilities, overlays := i.sidecars(identity, model.AssetKindSkill, name)
	return model.SourceAsset{
		Identity: identity,
		Kind:     model.AssetKindSkill,
		Base: model.AssetContent{
			Frontmatter: frontmatter,
			Body:        body,
			Files:       files,
		},
		CapabilityUses: capabilities,
		Overlays:       overlays,
	}, true
}

func (i *claudeInspector) supportFiles(assetRoot, skillFile string) map[model.RelativePath]model.FileContent {
	files := make(map[model.RelativePath]model.FileContent)
	var walk func(string)
	walk = func(directory string) {
		entries, err := i.readDir(directory)
		if err != nil {
			i.error(i.relativePath(directory), "read support directory: "+err.Error())
			return
		}
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				i.error(i.relativePath(path), "source symlinks are not allowed")
				continue
			}
			if entry.IsDir() {
				if entry.Name() != ".agentbundler" {
					walk(path)
				}
				continue
			}
			if !entry.Type().IsRegular() {
				i.error(i.relativePath(path), "support entries must be regular files or directories")
				continue
			}
			if path == skillFile {
				continue
			}
			content, ok := i.readInput(path)
			if !ok {
				continue
			}
			relative, err := filepath.Rel(assetRoot, path)
			if err != nil {
				i.error(i.relativePath(path), "resolve support file: "+err.Error())
				continue
			}
			normalized, err := model.NewRelativePath(filepath.ToSlash(relative))
			if err != nil {
				i.error(i.relativePath(path), "support file path: "+err.Error())
				continue
			}
			files[normalized] = model.FileContent{Bytes: content}
		}
	}
	walk(assetRoot)
	return files
}

func (i *claudeInspector) sidecars(identity model.AssetID, kind model.AssetKind, name string) ([]model.CapabilityUse, []model.TargetOverlay) {
	sidecarRoot := filepath.Join(i.sourceRoot, ".agentbundler", "assets", string(kind), name)
	info, err := i.lstat(sidecarRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		i.error(i.relativePath(sidecarRoot), "inspect sidecar: "+err.Error())
		return nil, nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		i.error(i.relativePath(sidecarRoot), "asset sidecar root must be a directory and not a symlink")
		return nil, nil
	}

	var capabilities []model.CapabilityUse
	assetConfig := filepath.Join(sidecarRoot, "asset.json")
	if data, exists := i.optionalRegularInput(assetConfig); exists {
		parsed, err := parseAssetSidecar(data)
		if err != nil {
			i.error(i.relativePath(assetConfig), "asset sidecar: "+err.Error())
		} else {
			for _, capability := range parsed {
				key, err := model.NewCapabilityKey(capability)
				if err != nil {
					i.error(i.relativePath(assetConfig), "capability: "+err.Error())
					continue
				}
				capabilities = append(capabilities, model.CapabilityUse{
					Key:      key,
					Location: model.SourceLocation{Path: model.RelativePath(i.relativePath(assetConfig))},
				})
			}
		}
	}

	targetsRoot := filepath.Join(sidecarRoot, "targets")
	return capabilities, i.targetSidecars(identity, targetsRoot)
}

func (i *claudeInspector) targetSidecars(identity model.AssetID, targetsRoot string) []model.TargetOverlay {
	entries, err := i.readDir(targetsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		i.error(i.relativePath(targetsRoot), "read target sidecars: "+err.Error())
		return nil
	}
	targets := make(map[model.TargetID]targetFiles)
	for _, entry := range entries {
		path := filepath.Join(targetsRoot, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			i.error(i.relativePath(path), "sidecar symlinks are not allowed")
			continue
		}
		if entry.IsDir() {
			target, ok := parseTargetID(entry.Name())
			if !ok {
				i.error(i.relativePath(path), "target sidecar directory has an invalid target")
				continue
			}
			config := targets[target]
			config.treePath = filepath.Join(path, "files")
			targets[target] = config
			continue
		}
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			i.error(i.relativePath(path), "target sidecar must be a <target>.json file or target directory")
			continue
		}
		target, ok := parseTargetID(strings.TrimSuffix(entry.Name(), ".json"))
		if !ok {
			i.error(i.relativePath(path), "target sidecar has an invalid target")
			continue
		}
		config := targets[target]
		config.jsonPath = path
		targets[target] = config
	}

	orderedTargets := make([]model.TargetID, 0, len(targets))
	for target := range targets {
		orderedTargets = append(orderedTargets, target)
	}
	sort.Slice(orderedTargets, func(i, j int) bool { return orderedTargets[i] < orderedTargets[j] })
	overlays := make([]model.TargetOverlay, 0, len(orderedTargets))
	for _, target := range orderedTargets {
		config := targets[target]
		overlay := model.TargetOverlay{Target: target}
		fileEntries := make(map[model.RelativePath][]byte)
		if config.jsonPath != "" {
			data, ok := i.readInput(config.jsonPath)
			if !ok {
				continue
			}
			parsed, err := parseTargetSidecar(data)
			if err != nil {
				i.error(i.relativePath(config.jsonPath), "target sidecar: "+err.Error())
			} else {
				overlay.FrontmatterPatch = parsed.FrontmatterPatch
				overlay.BodyPatch = parsed.BodyPatch
				overlay.DeletedFiles = parsed.DeletedFiles
				overlay.Acknowledgments = parsed.Acknowledgments
				for path, content := range parsed.Files {
					fileEntries[path] = content
				}
			}
		}
		if config.treePath != "" {
			for path, content := range i.filesTree(config.treePath) {
				fileEntries[path] = content
			}
		}
		paths := make([]model.RelativePath, 0, len(fileEntries))
		for path := range fileEntries {
			paths = append(paths, path)
		}
		sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
		for _, path := range paths {
			overlay.Files = append(overlay.Files, model.FilePatch{Path: path, Content: model.FileContent{Bytes: fileEntries[path]}})
		}
		for index := range overlay.Acknowledgments {
			overlay.Acknowledgments[index].Asset = identity
			overlay.Acknowledgments[index].Target = target
		}
		overlays = append(overlays, overlay)
	}
	return overlays
}

type targetFiles struct {
	jsonPath string
	treePath string
}

func (i *claudeInspector) filesTree(root string) map[model.RelativePath][]byte {
	files := make(map[model.RelativePath][]byte)
	info, err := i.lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return files
		}
		i.error(i.relativePath(root), "inspect target files tree: "+err.Error())
		return files
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		i.error(i.relativePath(root), "target files tree must be a directory and not a symlink")
		return files
	}
	var walk func(string)
	walk = func(directory string) {
		entries, err := i.readDir(directory)
		if err != nil {
			i.error(i.relativePath(directory), "read target files tree: "+err.Error())
			return
		}
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				i.error(i.relativePath(path), "sidecar symlinks are not allowed")
				continue
			}
			if entry.IsDir() {
				walk(path)
				continue
			}
			if !entry.Type().IsRegular() {
				i.error(i.relativePath(path), "target files entries must be regular files or directories")
				continue
			}
			content, ok := i.readInput(path)
			if !ok {
				continue
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				i.error(i.relativePath(path), "resolve target file: "+err.Error())
				continue
			}
			normalized, err := model.NewRelativePath(filepath.ToSlash(relative))
			if err != nil {
				i.error(i.relativePath(path), "target file path: "+err.Error())
				continue
			}
			files[normalized] = content
		}
	}
	walk(root)
	return files
}

func (i *claudeInspector) optionalRegularInput(path string) ([]byte, bool) {
	info, err := i.lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		i.error(i.relativePath(path), "inspect sidecar: "+err.Error())
		return nil, false
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		i.error(i.relativePath(path), "sidecar must be a regular file and not a symlink")
		return nil, false
	}
	return i.readInput(path)
}

func (i *claudeInspector) readInput(path string) ([]byte, bool) {
	content, err := i.readFile(path)
	if err != nil {
		i.error(i.relativePath(path), "read input: "+err.Error())
		return nil, false
	}
	relative := model.RelativePath(i.relativePath(path))
	digest := sha256.Sum256(content)
	i.inputs[relative] = hex.EncodeToString(digest[:])
	return content, true
}

func (i *claudeInspector) relativeToWorkspace(path string) (string, error) {
	relative, err := filepath.Rel(i.workspaceRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return filepath.ToSlash(relative), nil
}

func (i *claudeInspector) lstat(path string) (os.FileInfo, error) {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return nil, err
	}
	return i.filesystem.Lstat(relative)
}

func (i *claudeInspector) readDir(path string) ([]os.DirEntry, error) {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return nil, err
	}
	directory, err := i.filesystem.Open(relative)
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	return directory.ReadDir(-1)
}

func (i *claudeInspector) readFile(path string) ([]byte, error) {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return nil, err
	}
	return i.filesystem.ReadFile(relative)
}

func (i *claudeInspector) walkDir(path string, walk fs.WalkDirFunc) error {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return err
	}
	return fs.WalkDir(i.filesystem.FS(), relative, func(relativePath string, entry fs.DirEntry, walkErr error) error {
		return walk(filepath.Join(i.workspaceRoot, filepath.FromSlash(relativePath)), entry, walkErr)
	})
}

func (i *claudeInspector) requireDirectory(path string) error {
	info, err := i.lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("must be a directory")
	}
	return nil
}

func (i *claudeInspector) noSymlinkComponents(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes its declared root")
	}
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "." || segment == "" {
			continue
		}
		root = filepath.Join(root, segment)
		info, err := i.lstat(root)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlinks are not allowed")
		}
	}
	return nil
}

func (i *claudeInspector) relativePath(path string) string {
	relative, err := filepath.Rel(i.workspaceRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func (i *claudeInspector) error(path, message string) {
	diagnostic := model.Diagnostic{Code: diagnosticCode, Severity: model.SeverityError, Message: message}
	if path != "" {
		diagnostic.Location = &model.SourceLocation{Path: model.RelativePath(path)}
	}
	i.diagnostics = append(i.diagnostics, diagnostic)
}

func parseFrontmatter(markdown []byte) (map[string]any, string, error) {
	return frontmatter.Parse(markdown)
}

func parseAssetSidecar(data []byte) ([]string, error) {
	var sidecar struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := decodeStrictJSONObject(data, &sidecar); err != nil {
		return nil, err
	}
	if sidecar.Capabilities == nil {
		return nil, fmt.Errorf("capabilities is required")
	}
	return sidecar.Capabilities, nil
}

type parsedTargetSidecar struct {
	FrontmatterPatch *map[string]any
	BodyPatch        *model.BodyPatch
	Files            map[model.RelativePath][]byte
	DeletedFiles     []model.RelativePath
	Acknowledgments  []model.Acknowledgment
}

func parseTargetSidecar(data []byte) (parsedTargetSidecar, error) {
	var raw struct {
		FrontmatterPatch json.RawMessage `json:"frontmatterPatch"`
		BodyPatch        json.RawMessage `json:"bodyPatch"`
		Files            json.RawMessage `json:"files"`
		DeletedFiles     json.RawMessage `json:"deletedFiles"`
		Acknowledgments  json.RawMessage `json:"acknowledgments"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return parsedTargetSidecar{}, err
	}
	result := parsedTargetSidecar{Files: make(map[model.RelativePath][]byte)}
	if raw.FrontmatterPatch != nil {
		var patch map[string]any
		if err := decodeStrictJSONObject(raw.FrontmatterPatch, &patch); err != nil {
			return result, fmt.Errorf("frontmatterPatch: %w", err)
		}
		result.FrontmatterPatch = &patch
	}
	if raw.BodyPatch != nil {
		patch, err := parseBodyPatch(raw.BodyPatch)
		if err != nil {
			return result, fmt.Errorf("bodyPatch: %w", err)
		}
		result.BodyPatch = &patch
	}
	if raw.Files != nil {
		files, err := parseJSONFiles(raw.Files)
		if err != nil {
			return result, fmt.Errorf("files: %w", err)
		}
		result.Files = files
	}
	if raw.DeletedFiles != nil {
		var paths []string
		if err := decodeStrictJSON(raw.DeletedFiles, &paths); err != nil {
			return result, fmt.Errorf("deletedFiles: %w", err)
		}
		for _, path := range paths {
			normalized, err := model.NewRelativePath(path)
			if err != nil {
				return result, fmt.Errorf("deletedFiles: %w", err)
			}
			result.DeletedFiles = append(result.DeletedFiles, normalized)
		}
	}
	if raw.Acknowledgments != nil {
		if err := decodeStrictJSON(raw.Acknowledgments, &result.Acknowledgments); err != nil {
			return result, fmt.Errorf("acknowledgments: %w", err)
		}
	}
	return result, nil
}

func parseBodyPatch(data []byte) (model.BodyPatch, error) {
	var raw struct {
		Mode     model.BodyMode `json:"mode"`
		Text     *string        `json:"text"`
		Sections []struct {
			HeadingPath []string `json:"headingPath"`
			Body        string   `json:"body"`
		} `json:"sections"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return model.BodyPatch{}, err
	}
	patch := model.BodyPatch{Mode: raw.Mode, Text: raw.Text}
	for _, section := range raw.Sections {
		patch.Sections = append(patch.Sections, model.SectionPatch{HeadingPath: section.HeadingPath, Body: section.Body})
	}
	return patch, nil
}

func parseJSONFiles(data []byte) (map[model.RelativePath][]byte, error) {
	var raw map[string]json.RawMessage
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return nil, err
	}
	files := make(map[model.RelativePath][]byte, len(raw))
	for path, value := range raw {
		normalized, err := model.NewRelativePath(path)
		if err != nil {
			return nil, fmt.Errorf("file path %q: %w", path, err)
		}
		var text string
		if err := decodeStrictJSON(value, &text); err == nil {
			files[normalized] = []byte(text)
			continue
		}
		var encoded struct {
			Base64 *string `json:"base64"`
		}
		if err := decodeStrictJSONObject(value, &encoded); err != nil || encoded.Base64 == nil {
			return nil, fmt.Errorf("file %q must be a UTF-8 string or {\"base64\": String}", path)
		}
		content, err := base64.StdEncoding.DecodeString(*encoded.Base64)
		if err != nil {
			return nil, fmt.Errorf("file %q base64: %w", path, err)
		}
		files[normalized] = content
	}
	return files, nil
}

func decodeStrictJSONObject(data []byte, destination any) error {
	return model.DecodeStrictJSONObject(data, destination)
}

func decodeStrictJSON(data []byte, destination any) error {
	return model.DecodeStrictJSON(data, destination)
}

func parseTargetID(value string) (model.TargetID, bool) {
	target := model.TargetID(value)
	switch target {
	case model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor:
		return target, true
	default:
		return "", false
	}
}

func hasErrors(diagnostics []model.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
