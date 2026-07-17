// Package skillrepo imports explicit roots from a generic skills repository.
package skillrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/compiler/source/frontmatter"
)

const diagnosticCode = "invalid-skills-repository"

// InspectSkillRepo discovers skills and their optional derived-target sidecars.
func InspectSkillRepo(manifest model.SourceManifest, workspaceRoot string) (model.SourceInventory, []model.Diagnostic) {
	workspace, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return model.SourceInventory{}, []model.Diagnostic{{Code: diagnosticCode, Severity: model.SeverityError, Message: "workspace root: " + err.Error()}}
	}
	defer func() { _ = workspace.Close() }()
	return InspectSkillRepoRoot(manifest, workspaceRoot, workspace)
}

// InspectSkillRepoRoot imports a skills repository using a workspace-bounded filesystem.
func InspectSkillRepoRoot(manifest model.SourceManifest, workspaceRoot string, workspace *os.Root) (model.SourceInventory, []model.Diagnostic) {
	inspector := inspector{
		workspaceRoot: workspaceRoot,
		filesystem:    workspace,
		inputs:        make(map[model.RelativePath]string),
	}
	inspector.diagnostics = append(inspector.diagnostics, model.ValidateSourceManifest(manifest)...)
	if manifest.Kind != model.SourceKindSkillsRepository {
		inspector.error("", "manifest kind must be skills-repository")
	}
	if hasErrors(inspector.diagnostics) {
		return model.SourceInventory{}, inspector.diagnostics
	}
	if !filepath.IsAbs(workspaceRoot) || filepath.Clean(workspaceRoot) != workspaceRoot {
		inspector.error("", "workspace root must be a cleaned absolute path")
		return model.SourceInventory{}, inspector.diagnostics
	}
	var err error
	inspector.sourceRoot, err = containedPath(workspaceRoot, string(manifest.Root))
	if err != nil {
		inspector.error(string(manifest.Root), "manifest root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	if err := inspector.noSymlinkComponents(inspector.sourceRoot); err != nil {
		inspector.error(string(manifest.Root), "manifest root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	if err := inspector.requireDirectory(inspector.sourceRoot); err != nil {
		inspector.error(string(manifest.Root), "manifest root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}

	roots := append([]model.RelativePath(nil), manifest.SkillsRepository.Roots...)
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	var skillRoots []string
	for _, root := range roots {
		absoluteRoot, err := containedPath(inspector.sourceRoot, string(root))
		if err != nil {
			inspector.error(string(root), "skill root: "+err.Error())
			continue
		}
		if err := inspector.noSymlinkComponents(absoluteRoot); err != nil {
			inspector.error(inspector.relativePath(absoluteRoot), "skill root: "+err.Error())
			continue
		}
		if err := inspector.requireDirectory(absoluteRoot); err != nil {
			inspector.error(inspector.relativePath(absoluteRoot), "skill root: "+err.Error())
			continue
		}
		skillRoots = append(skillRoots, absoluteRoot)
	}

	assets := make(map[model.AssetID]model.SourceAsset)
	for _, root := range skillRoots {
		skillFiles := inspector.findSkillFiles(root)
		if len(skillFiles) == 0 {
			inspector.error(inspector.relativePath(root), "declared skill root contains no SKILL.md")
		}
		for _, skillFile := range skillFiles {
			asset, ok := inspector.inspectSkill(skillFile)
			if !ok {
				continue
			}
			if _, exists := assets[asset.Identity]; exists {
				inspector.error(inspector.relativePath(skillFile), fmt.Sprintf("duplicate skill identity %q", asset.Identity))
				continue
			}
			assets[asset.Identity] = asset
		}
	}
	if hasErrors(inspector.diagnostics) {
		return model.SourceInventory{}, inspector.diagnostics
	}

	assetList := make([]model.SourceAsset, 0, len(assets))
	for _, asset := range assets {
		assetList = append(assetList, asset)
	}
	sort.Slice(assetList, func(i, j int) bool { return assetList[i].Identity < assetList[j].Identity })
	inputs := make([]model.InputFile, 0, len(inspector.inputs))
	for path, digest := range inspector.inputs {
		inputs = append(inputs, model.InputFile{Path: path, SHA256: digest})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })

	inventory := model.SourceInventory{
		Packages: []model.SourcePackage{{
			Identity: manifest.SkillsRepository.Package,
			Metadata: manifest.SkillsRepository.Metadata,
			Assets:   assetList,
		}},
		Inputs: inputs,
	}
	if diagnostics := model.ValidateSourceInventory(inventory); len(diagnostics) != 0 {
		return model.SourceInventory{}, append(inspector.diagnostics, diagnostics...)
	}
	return inventory, inspector.diagnostics
}

type inspector struct {
	workspaceRoot string
	filesystem    *os.Root
	sourceRoot    string
	inputs        map[model.RelativePath]string
	diagnostics   []model.Diagnostic
}

func (i *inspector) findSkillFiles(root string) []string {
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

func (i *inspector) inspectSkill(skillFile string) (model.SourceAsset, bool) {
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
	capabilities, overlays := i.sidecars(identity, name)
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

func (i *inspector) supportFiles(assetRoot, skillFile string) map[model.RelativePath]model.FileContent {
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
			info, err := entry.Info()
			if err != nil {
				i.error(i.relativePath(path), "inspect support file: "+err.Error())
				continue
			}
			if !info.Mode().IsRegular() {
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
			files[normalized] = model.FileContent{
				Bytes:      content,
				Executable: info.Mode().Perm()&0o111 != 0,
				Origin:     []model.SourceLocation{{Path: model.RelativePath(i.relativePath(path))}},
			}
		}
	}
	walk(assetRoot)
	return files
}

func (i *inspector) sidecars(identity model.AssetID, name string) ([]model.CapabilityUse, []model.TargetOverlay) {
	sidecarRoot := filepath.Join(i.sourceRoot, ".agentbundler", "assets", "skill", name)
	if err := i.noSymlinkComponents(sidecarRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		i.error(i.relativePath(sidecarRoot), "inspect sidecar: "+err.Error())
		return nil, nil
	}
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

func (i *inspector) targetSidecars(identity model.AssetID, targetsRoot string) []model.TargetOverlay {
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
		fileEntries := make(map[model.RelativePath]model.FileContent)
		if config.jsonPath != "" {
			data, ok := i.readInput(config.jsonPath)
			if !ok {
				continue
			}
			parsed, err := parseTargetSidecar(data, model.SourceLocation{Path: model.RelativePath(i.relativePath(config.jsonPath))})
			if err != nil {
				i.error(i.relativePath(config.jsonPath), "target sidecar: "+err.Error())
			} else {
				overlay.FrontmatterPatch = parsed.FrontmatterPatch
				overlay.BodyPatch = parsed.BodyPatch
				overlay.DeletedFiles = parsed.DeletedFiles
				for _, acknowledgment := range parsed.Acknowledgments {
					overlay.Acknowledgments = append(overlay.Acknowledgments, model.Acknowledgment{
						Asset:  identity,
						Target: target,
						Key:    acknowledgment.Key,
						Reason: acknowledgment.Reason,
					})
				}
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
			overlay.Files = append(overlay.Files, model.FilePatch{Path: path, Content: fileEntries[path]})
		}
		overlays = append(overlays, overlay)
	}
	return overlays
}

type targetFiles struct {
	jsonPath string
	treePath string
}

func (i *inspector) filesTree(root string) map[model.RelativePath]model.FileContent {
	files := make(map[model.RelativePath]model.FileContent)
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
			info, err := entry.Info()
			if err != nil {
				i.error(i.relativePath(path), "inspect target file: "+err.Error())
				continue
			}
			if !info.Mode().IsRegular() {
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
			files[normalized] = model.FileContent{
				Bytes:      content,
				Executable: info.Mode().Perm()&0o111 != 0,
				Origin:     []model.SourceLocation{{Path: model.RelativePath(i.relativePath(path))}},
			}
		}
	}
	walk(root)
	return files
}

func (i *inspector) optionalRegularInput(path string) ([]byte, bool) {
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

func (i *inspector) readInput(path string) ([]byte, bool) {
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

func containedPath(root, relative string) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	path, err := filepath.Rel(root, candidate)
	if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes its declared root")
	}
	return candidate, nil
}

func (i *inspector) relativeToWorkspace(path string) (string, error) {
	relative, err := filepath.Rel(i.workspaceRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return filepath.ToSlash(relative), nil
}

func (i *inspector) noSymlinkComponents(path string) error {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return err
	}
	current := "."
	for _, segment := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		if segment == "." || segment == "" {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := i.filesystem.Lstat(filepath.ToSlash(current))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlinks are not allowed")
		}
	}
	return nil
}

func (i *inspector) lstat(path string) (os.FileInfo, error) {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return nil, err
	}
	return i.filesystem.Lstat(relative)
}

func (i *inspector) readDir(path string) ([]os.DirEntry, error) {
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

func (i *inspector) readFile(path string) ([]byte, error) {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return nil, err
	}
	return i.filesystem.ReadFile(relative)
}

func (i *inspector) requireDirectory(path string) error {
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

func (i *inspector) relativePath(path string) string {
	relative, err := filepath.Rel(i.workspaceRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func (i *inspector) error(path, message string) {
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
	Files            map[model.RelativePath]model.FileContent
	DeletedFiles     []model.RelativePath
	Acknowledgments  []sidecarAcknowledgment
}

type sidecarAcknowledgment struct {
	Key    model.CapabilityKey `json:"key"`
	Reason string              `json:"reason"`
}

func parseTargetSidecar(data []byte, location model.SourceLocation) (parsedTargetSidecar, error) {
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
	result := parsedTargetSidecar{Files: make(map[model.RelativePath]model.FileContent)}
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
		files, err := parseJSONFiles(raw.Files, location)
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

func parseJSONFiles(data []byte, location model.SourceLocation) (map[model.RelativePath]model.FileContent, error) {
	var raw map[string]json.RawMessage
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(raw))
	for path := range raw {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files := make(map[model.RelativePath]model.FileContent, len(raw))
	for _, path := range paths {
		normalized, err := model.NewRelativePath(path)
		if err != nil {
			return nil, fmt.Errorf("file path %q: %w", path, err)
		}
		content, err := model.DecodeOverlayFileContentJSON(raw[path], location)
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", path, err)
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
	case model.TargetAntigravity, model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor:
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
