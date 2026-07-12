// Package bundle imports Agentbundler's canonical bundle source layout.
package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const diagnosticCodeInvalidBundle = "invalid-bundle-source"

type inspector struct {
	root        string
	inputs      map[model.RelativePath]string
	nativeGaps  map[model.RelativePath]model.NativeGap
	diagnostics []model.Diagnostic
}

type packageManifest struct {
	ID       *string                `json:"id"`
	Metadata *model.PackageMetadata `json:"metadata"`
	Assets   *[]string              `json:"assets"`
}

type assetSidecar struct {
	Capabilities *[]string `json:"capabilities"`
}

type overlaySidecar struct {
	FrontmatterPatch *map[string]any            `json:"frontmatterPatch"`
	BodyPatch        *json.RawMessage           `json:"bodyPatch"`
	Files            map[string]json.RawMessage `json:"files"`
	DeletedFiles     *[]string                  `json:"deletedFiles"`
	Acknowledgments  *[]acknowledgmentSidecar   `json:"acknowledgments"`
}

type acknowledgmentSidecar struct {
	Asset  *string `json:"asset"`
	Target *string `json:"target"`
	Key    *string `json:"key"`
	Reason *string `json:"reason"`
}

type bodyPatchSidecar struct {
	Mode     *model.BodyMode   `json:"mode"`
	Text     *string           `json:"text"`
	Sections *[]sectionSidecar `json:"sections"`
}

type sectionSidecar struct {
	HeadingPath *[]string `json:"headingPath"`
	Body        *string   `json:"body"`
}

// InspectBundle imports a canonical bundle rooted below workspaceRoot.
func InspectBundle(manifest model.SourceManifest, workspaceRoot string) (model.SourceInventory, []model.Diagnostic) {
	if manifest.Kind != model.SourceKindBundle {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", "bundle importer requires kind bundle")}
	}
	if diagnostics := model.ValidateSourceManifest(manifest); hasErrors(diagnostics) {
		return model.SourceInventory{}, diagnostics
	}
	if !filepath.IsAbs(workspaceRoot) || filepath.Clean(workspaceRoot) != workspaceRoot {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", "workspace root must be a cleaned absolute path")}
	}

	root := filepath.Join(workspaceRoot, filepath.FromSlash(string(manifest.Root)))
	info, err := os.Lstat(root)
	if err != nil {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", fmt.Sprintf("source root: %v", err))}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", "source root must be a non-symlink directory")}
	}

	inspector := inspector{
		root:       root,
		inputs:     make(map[model.RelativePath]string),
		nativeGaps: make(map[model.RelativePath]model.NativeGap),
	}
	packagePaths := append([]model.RelativePath(nil), manifest.Bundle.Packages...)
	sort.Slice(packagePaths, func(i, j int) bool { return packagePaths[i] < packagePaths[j] })

	packages := make([]model.SourcePackage, 0, len(packagePaths))
	seenPackages := make(map[model.PackageID]model.RelativePath)
	for _, packagePath := range packagePaths {
		pkg, ok := inspector.inspectPackage(packagePath)
		if !ok {
			continue
		}
		if previous, exists := seenPackages[pkg.Identity]; exists {
			inspector.addDiagnostic(packagePath, "package identity %q duplicates %q", pkg.Identity, previous)
			continue
		}
		seenPackages[pkg.Identity] = packagePath
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Identity < packages[j].Identity })

	if len(inspector.diagnostics) != 0 {
		return model.SourceInventory{}, inspector.diagnostics
	}
	nativeGaps := make([]model.NativeGap, 0, len(inspector.nativeGaps))
	for _, gap := range inspector.nativeGaps {
		nativeGaps = append(nativeGaps, gap)
	}
	sort.Slice(nativeGaps, func(i, j int) bool {
		return nativeGaps[i].Location.Path < nativeGaps[j].Location.Path
	})
	inputs := make([]model.InputFile, 0, len(inspector.inputs))
	for path, digest := range inspector.inputs {
		inputs = append(inputs, model.InputFile{Path: path, SHA256: digest})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })
	inventory := model.SourceInventory{Packages: packages, NativeGaps: nativeGaps, Inputs: inputs}
	if diagnostics := model.ValidateSourceInventory(inventory); hasErrors(diagnostics) {
		return model.SourceInventory{}, diagnostics
	}
	return inventory, nil
}

func (i *inspector) inspectPackage(packagePath model.RelativePath) (model.SourcePackage, bool) {
	data, ok := i.readRegular(packagePath)
	if !ok {
		return model.SourcePackage{}, false
	}
	var manifest packageManifest
	if err := decodeStrictJSON(data, &manifest); err != nil {
		i.addDiagnostic(packagePath, "package manifest: %v", err)
		return model.SourcePackage{}, false
	}
	if manifest.ID == nil || manifest.Metadata == nil || manifest.Assets == nil {
		i.addDiagnostic(packagePath, "package manifest requires id, metadata, and assets")
		return model.SourcePackage{}, false
	}
	packageID, err := model.NewPackageID(*manifest.ID)
	if err != nil {
		i.addDiagnostic(packagePath, "package manifest id: %v", err)
		return model.SourcePackage{}, false
	}

	assets := make([]model.SourceAsset, 0, len(*manifest.Assets))
	seenPaths := make(map[model.RelativePath]struct{}, len(*manifest.Assets))
	seenAssets := make(map[model.AssetID]struct{}, len(*manifest.Assets))
	for _, value := range *manifest.Assets {
		assetPath, err := model.NewRelativePath(value)
		if err != nil {
			i.addDiagnostic(packagePath, "asset path %q: %v", value, err)
			continue
		}
		if _, exists := seenPaths[assetPath]; exists {
			i.addDiagnostic(packagePath, "asset path %q is duplicated", assetPath)
			continue
		}
		seenPaths[assetPath] = struct{}{}
		asset, nativeGap, ok := i.inspectAsset(assetPath)
		if !ok {
			continue
		}
		if nativeGap != nil {
			i.nativeGaps[assetPath] = *nativeGap
		}
		if _, exists := seenAssets[asset.Identity]; exists {
			i.addDiagnostic(packagePath, "asset identity %q is duplicated", asset.Identity)
			continue
		}
		seenAssets[asset.Identity] = struct{}{}
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(a, b int) bool { return assets[a].Identity < assets[b].Identity })
	return model.SourcePackage{Identity: packageID, Metadata: *manifest.Metadata, Assets: assets}, true
}

func (i *inspector) inspectAsset(assetPath model.RelativePath) (model.SourceAsset, *model.NativeGap, bool) {
	kind, name, mainFile, assetDir, markdown, err := classifyAsset(string(assetPath))
	if err != nil {
		i.addDiagnostic(assetPath, "%v", err)
		return model.SourceAsset{}, nil, false
	}
	identity, err := model.NewAssetID(string(kind) + "/" + name)
	if err != nil {
		i.addDiagnostic(assetPath, "asset identity: %v", err)
		return model.SourceAsset{}, nil, false
	}

	mainPath := model.RelativePath(mainFile)
	data, ok := i.readRegular(mainPath)
	if !ok {
		return model.SourceAsset{}, nil, false
	}
	content := model.AssetContent{Frontmatter: map[string]any{}, Files: make(map[model.RelativePath][]byte)}
	if markdown {
		frontmatter, body, err := parseMarkdown(data)
		if err != nil {
			i.addDiagnostic(mainPath, "%v", err)
			return model.SourceAsset{}, nil, false
		}
		content.Frontmatter = frontmatter
		content.Body = body
	} else if kind == model.AssetKindNativeResource {
		content.Files[model.RelativePath(filepath.Base(mainFile))] = append([]byte(nil), data...)
	} else {
		content.Body = string(data)
	}
	if kind == model.AssetKindSkill {
		i.readSupportFiles(assetDir, &content)
	}

	asset := model.SourceAsset{
		Identity:       identity,
		Kind:           kind,
		Base:           content,
		CapabilityUses: i.readCapabilities(assetDir),
		Overlays:       i.readOverlays(assetDir, identity),
	}
	if kind != model.AssetKindNativeResource {
		return asset, nil, true
	}
	parts := strings.Split(string(assetPath), "/")
	target := model.TargetID(parts[2])
	return asset, &model.NativeGap{
		Component: name,
		Asset:     &identity,
		Location:  model.SourceLocation{Path: assetPath},
		Target:    &target,
	}, true
}

func classifyAsset(assetPath string) (model.AssetKind, string, string, string, bool, error) {
	parts := strings.Split(assetPath, "/")
	switch {
	case len(parts) == 3 && parts[0] == "src" && parts[1] == "skills":
		return model.AssetKindSkill, parts[2], assetPath + "/SKILL.md", assetPath, true, nil
	case len(parts) == 3 && parts[0] == "src" && parts[1] == "agents" && strings.HasSuffix(parts[2], ".md") && strings.TrimSuffix(parts[2], ".md") != "":
		return model.AssetKindAgent, strings.TrimSuffix(parts[2], ".md"), assetPath, filepath.ToSlash(filepath.Dir(assetPath)), true, nil
	case len(parts) == 3 && parts[0] == "src" && parts[1] == "hooks" && strings.HasSuffix(parts[2], ".json") && strings.TrimSuffix(parts[2], ".json") != "":
		return model.AssetKindHook, strings.TrimSuffix(parts[2], ".json"), assetPath, filepath.ToSlash(filepath.Dir(assetPath)), false, nil
	case len(parts) == 4 && parts[0] == "src" && parts[1] == "plugins" && parts[2] != "" && parts[3] != "":
		if _, err := parseTarget(parts[2]); err != nil {
			return "", "", "", "", false, err
		}
		return model.AssetKindNativeResource, parts[3], assetPath, filepath.ToSlash(filepath.Dir(assetPath)), false, nil
	default:
		return "", "", "", "", false, fmt.Errorf("asset path %q is not a canonical skill, agent, hook, or native resource", assetPath)
	}
}

func (i *inspector) readSupportFiles(assetDir string, content *model.AssetContent) {
	root := filepath.Join(i.root, filepath.FromSlash(assetDir))
	err := filepath.WalkDir(root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".agentbundler" {
			if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				return fmt.Errorf("sidecar path must be a non-symlink directory")
			}
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("support file %q is a symlink", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("support path %q is not a regular file", rel)
		}
		if rel == "SKILL.md" {
			return nil
		}
		path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, rel)))
		data, ok := i.readRegular(path)
		if ok {
			content.Files[model.RelativePath(rel)] = data
		}
		return nil
	})
	if err != nil {
		i.addDiagnostic(model.RelativePath(assetDir), "%v", err)
	}
}

func (i *inspector) readCapabilities(assetDir string) []model.CapabilityUse {
	path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/asset.json")))
	data, exists, ok := i.readOptionalRegular(path)
	if !exists {
		return nil
	}
	if !ok {
		return nil
	}
	var sidecar assetSidecar
	if err := decodeStrictJSON(data, &sidecar); err != nil {
		i.addDiagnostic(path, "asset sidecar: %v", err)
		return nil
	}
	if sidecar.Capabilities == nil {
		i.addDiagnostic(path, "asset sidecar requires capabilities")
		return nil
	}
	uses := make([]model.CapabilityUse, 0, len(*sidecar.Capabilities))
	seen := make(map[model.CapabilityKey]struct{}, len(*sidecar.Capabilities))
	for _, value := range *sidecar.Capabilities {
		key, err := model.NewCapabilityKey(value)
		if err != nil {
			i.addDiagnostic(path, "capability %q: %v", value, err)
			continue
		}
		if _, exists := seen[key]; exists {
			i.addDiagnostic(path, "capability %q is duplicated", key)
			continue
		}
		seen[key] = struct{}{}
		uses = append(uses, model.CapabilityUse{Key: key, Location: model.SourceLocation{Path: path}})
	}
	sort.Slice(uses, func(a, b int) bool { return uses[a].Key < uses[b].Key })
	return uses
}

func (i *inspector) readOverlays(assetDir string, identity model.AssetID) []model.TargetOverlay {
	targetsPath := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets")))
	targetsRoot := filepath.Join(i.root, filepath.FromSlash(string(targetsPath)))
	info, err := os.Lstat(targetsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		i.addDiagnostic(targetsPath, "inspect target sidecars: %v", err)
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		i.addDiagnostic(targetsPath, "target sidecars must be a non-symlink directory")
		return nil
	}
	if err := noSymlinkComponents(i.root, targetsRoot); err != nil {
		i.addDiagnostic(targetsPath, "%v", err)
		return nil
	}
	entries, err := os.ReadDir(targetsRoot)
	if err != nil {
		i.addDiagnostic(targetsPath, "read target sidecars: %v", err)
		return nil
	}

	targets := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", entry.Name()))), "target sidecar path is a symlink")
			continue
		}
		if entry.IsDir() {
			targets[entry.Name()] = struct{}{}
			continue
		}
		if strings.HasSuffix(entry.Name(), ".json") {
			targets[strings.TrimSuffix(entry.Name(), ".json")] = struct{}{}
			continue
		}
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", entry.Name()))), "target sidecar path must be a target JSON file or directory")
	}
	names := make([]string, 0, len(targets))
	for target := range targets {
		names = append(names, target)
	}
	sort.Strings(names)
	overlays := make([]model.TargetOverlay, 0, len(names))
	for _, name := range names {
		target, err := parseTarget(name)
		if err != nil {
			i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", name))), "%v", err)
			continue
		}
		overlay, ok := i.readOverlay(assetDir, target, identity)
		if ok {
			overlays = append(overlays, overlay)
		}
	}
	return overlays
}

func (i *inspector) readOverlay(assetDir string, target model.TargetID, identity model.AssetID) (model.TargetOverlay, bool) {
	jsonPath := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", string(target)+".json")))
	data, jsonExists, jsonOK := i.readOptionalRegular(jsonPath)
	if jsonExists && !jsonOK {
		return model.TargetOverlay{}, false
	}

	overlay := model.TargetOverlay{Target: target}
	files := make(map[model.RelativePath][]byte)
	if jsonExists {
		var sidecar overlaySidecar
		if err := decodeStrictJSON(data, &sidecar); err != nil {
			i.addDiagnostic(jsonPath, "target sidecar: %v", err)
			return model.TargetOverlay{}, false
		}
		if sidecar.FrontmatterPatch != nil {
			overlay.FrontmatterPatch = sidecar.FrontmatterPatch
		}
		if sidecar.BodyPatch != nil {
			patch, err := decodeBodyPatch(*sidecar.BodyPatch)
			if err != nil {
				i.addDiagnostic(jsonPath, "body patch: %v", err)
				return model.TargetOverlay{}, false
			}
			overlay.BodyPatch = &patch
		}
		for path, raw := range sidecar.Files {
			rel, err := model.NewRelativePath(path)
			if err != nil {
				i.addDiagnostic(jsonPath, "overlay file %q: %v", path, err)
				continue
			}
			value, err := decodeFileValue(raw)
			if err != nil {
				i.addDiagnostic(jsonPath, "overlay file %q: %v", path, err)
				continue
			}
			files[rel] = value
		}
		if sidecar.DeletedFiles != nil {
			for _, value := range *sidecar.DeletedFiles {
				rel, err := model.NewRelativePath(value)
				if err != nil {
					i.addDiagnostic(jsonPath, "deleted file %q: %v", value, err)
					continue
				}
				overlay.DeletedFiles = append(overlay.DeletedFiles, rel)
			}
		}
		if sidecar.Acknowledgments != nil {
			for _, value := range *sidecar.Acknowledgments {
				acknowledgment, err := decodeAcknowledgment(value)
				if err != nil {
					i.addDiagnostic(jsonPath, "acknowledgment: %v", err)
					continue
				}
				overlay.Acknowledgments = append(overlay.Acknowledgments, acknowledgment)
			}
		}
	}

	filesDir := filepath.Join(i.root, filepath.FromSlash(filepath.Join(assetDir, ".agentbundler/targets", string(target), "files")))
	if !i.readOverlayFiles(filesDir, assetDir, target, files) {
		return model.TargetOverlay{}, false
	}
	for path, data := range files {
		overlay.Files = append(overlay.Files, model.FilePatch{Path: path, Bytes: data})
	}
	sort.Slice(overlay.Files, func(a, b int) bool { return overlay.Files[a].Path < overlay.Files[b].Path })
	sort.Slice(overlay.DeletedFiles, func(a, b int) bool { return overlay.DeletedFiles[a] < overlay.DeletedFiles[b] })
	sort.Slice(overlay.Acknowledgments, func(a, b int) bool {
		return overlay.Acknowledgments[a].Key < overlay.Acknowledgments[b].Key
	})
	return overlay, true
}

func (i *inspector) readOverlayFiles(filesDir, assetDir string, target model.TargetID, files map[model.RelativePath][]byte) bool {
	info, err := os.Lstat(filesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", string(target), "files"))), "overlay files: %v", err)
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", string(target), "files"))), "overlay files must be a non-symlink directory")
		return false
	}
	valid := true
	err = filepath.WalkDir(filesDir, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(filesDir, fullPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("overlay file %q is a symlink", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("overlay file %q is not regular", rel)
		}
		path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", string(target), "files", rel)))
		data, ok := i.readRegular(path)
		if !ok {
			valid = false
			return nil
		}
		files[model.RelativePath(rel)] = data
		return nil
	})
	if err != nil {
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/targets", string(target), "files"))), "%v", err)
		return false
	}
	return valid
}

func (i *inspector) readRegular(path model.RelativePath) ([]byte, bool) {
	data, exists, ok := i.readOptionalRegular(path)
	if !exists {
		i.addDiagnostic(path, "required file does not exist")
	}
	return data, ok
}

func (i *inspector) readOptionalRegular(path model.RelativePath) ([]byte, bool, bool) {
	fullPath := filepath.Join(i.root, filepath.FromSlash(string(path)))
	if err := noSymlinkComponents(i.root, fullPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, false
		}
		i.addDiagnostic(path, "%v", err)
		return nil, true, false
	}
	info, err := os.Lstat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, false
		}
		i.addDiagnostic(path, "inspect file: %v", err)
		return nil, true, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		i.addDiagnostic(path, "source file is a symlink")
		return nil, true, false
	}
	if !info.Mode().IsRegular() {
		i.addDiagnostic(path, "source file is not regular")
		return nil, true, false
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		i.addDiagnostic(path, "read file: %v", err)
		return nil, true, false
	}
	digest := sha256.Sum256(data)
	i.inputs[path] = hex.EncodeToString(digest[:])
	return data, true, true
}

func (i *inspector) addDiagnostic(path model.RelativePath, format string, arguments ...any) {
	i.diagnostics = append(i.diagnostics, diagnostic(path, fmt.Sprintf(format, arguments...)))
}

func diagnostic(path model.RelativePath, message string) model.Diagnostic {
	diagnostic := model.Diagnostic{Code: diagnosticCodeInvalidBundle, Severity: model.SeverityError, Message: message}
	if path != "" {
		diagnostic.Location = &model.SourceLocation{Path: path}
	}
	return diagnostic
}

func decodeStrictJSON(data []byte, destination any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON must be valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is invalid")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("invalid JSON delimiter %q", delimiter)
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func parseMarkdown(data []byte) (map[string]any, string, error) {
	if !utf8.Valid(data) {
		return nil, "", fmt.Errorf("Markdown must be valid UTF-8")
	}
	frontmatter := map[string]any{}
	lines := strings.SplitAfter(string(data), "\n")
	if len(lines) == 0 || trimLineEnding(lines[0]) != "---" {
		return frontmatter, string(data), nil
	}
	for index := 1; index < len(lines); index++ {
		if trimLineEnding(lines[index]) != "---" {
			continue
		}
		decoded := make(map[string]any)
		if err := decodeStrictJSON([]byte(strings.Join(lines[1:index], "")), &decoded); err != nil {
			return nil, "", fmt.Errorf("frontmatter: %w", err)
		}
		return decoded, strings.Join(lines[index+1:], ""), nil
	}
	return nil, "", fmt.Errorf("frontmatter opening delimiter has no closing delimiter")
}

func trimLineEnding(value string) string {
	return strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r")
}

func decodeBodyPatch(raw json.RawMessage) (model.BodyPatch, error) {
	var sidecar bodyPatchSidecar
	if err := decodeStrictJSON(raw, &sidecar); err != nil {
		return model.BodyPatch{}, err
	}
	if sidecar.Mode == nil {
		return model.BodyPatch{}, fmt.Errorf("requires mode")
	}
	patch := model.BodyPatch{Mode: *sidecar.Mode, Text: sidecar.Text}
	if sidecar.Sections != nil {
		for _, section := range *sidecar.Sections {
			if section.HeadingPath == nil || section.Body == nil {
				return model.BodyPatch{}, fmt.Errorf("section requires headingPath and body")
			}
			patch.Sections = append(patch.Sections, model.SectionPatch{HeadingPath: *section.HeadingPath, Body: *section.Body})
		}
	}
	return patch, nil
}

func decodeFileValue(raw json.RawMessage) ([]byte, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("must be a UTF-8 string or {base64: String}")
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		return []byte(stringValue), nil
	}
	var encoded struct {
		Base64 *string `json:"base64"`
	}
	if err := decodeStrictJSON(raw, &encoded); err != nil {
		return nil, fmt.Errorf("must be a UTF-8 string or {base64: String}: %w", err)
	}
	if encoded.Base64 == nil {
		return nil, fmt.Errorf("base64 object requires base64")
	}
	value, err := base64.StdEncoding.DecodeString(*encoded.Base64)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	return value, nil
}

func decodeAcknowledgment(value acknowledgmentSidecar) (model.Acknowledgment, error) {
	if value.Asset == nil || value.Target == nil || value.Key == nil || value.Reason == nil {
		return model.Acknowledgment{}, fmt.Errorf("requires asset, target, key, and reason")
	}
	asset, err := model.NewAssetID(*value.Asset)
	if err != nil {
		return model.Acknowledgment{}, fmt.Errorf("asset: %w", err)
	}
	target, err := parseTarget(*value.Target)
	if err != nil {
		return model.Acknowledgment{}, err
	}
	key, err := model.NewCapabilityKey(*value.Key)
	if err != nil {
		return model.Acknowledgment{}, fmt.Errorf("key: %w", err)
	}
	if strings.TrimSpace(*value.Reason) == "" || strings.ContainsRune(*value.Reason, '\x00') {
		return model.Acknowledgment{}, fmt.Errorf("reason must not be empty or contain NUL")
	}
	return model.Acknowledgment{Asset: asset, Target: target, Key: key, Reason: *value.Reason}, nil
}

func parseTarget(value string) (model.TargetID, error) {
	target := model.TargetID(value)
	switch target {
	case model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor:
		return target, nil
	default:
		return "", fmt.Errorf("target %q is invalid", value)
	}
}

func noSymlinkComponents(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("source path leaves source root")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator))[:len(strings.Split(rel, string(filepath.Separator)))-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect source path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source path crosses a symlink")
		}
	}
	return nil
}

func hasErrors(diagnostics []model.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
