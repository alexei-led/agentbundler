// Package bundle imports Agent Bundler's canonical bundle source layout.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/compiler/source/frontmatter"
)

const diagnosticCodeInvalidBundle = "invalid-bundle-source"

type inspector struct {
	workspaceRoot string
	filesystem    *os.Root
	root          string
	inputs        map[model.RelativePath]string
	nativeGaps    map[model.RelativePath]model.NativeGap
	diagnostics   []model.Diagnostic
}

type packageManifest struct {
	ID       *string                `json:"id"`
	Metadata *model.PackageMetadata `json:"metadata"`
	Assets   *[]assetEntry          `json:"assets"`
}

type assetEntry struct {
	Path    *string          `json:"path"`
	Targets []model.TargetID `json:"targets"`
}

func (entry *assetEntry) UnmarshalJSON(data []byte) error {
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		entry.Path = &path
		return nil
	}
	var value struct {
		Path    *string          `json:"path"`
		Targets []model.TargetID `json:"targets"`
	}
	if err := decodeStrictJSON(data, &value); err != nil {
		return err
	}
	entry.Path = value.Path
	entry.Targets = value.Targets
	return nil
}

type assetSidecar struct {
	Capabilities *[]string `json:"capabilities"`
	PiExtensions *[]string `json:"piExtensions"`
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
	workspace, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", "workspace root: "+err.Error())}
	}
	defer func() { _ = workspace.Close() }()
	return InspectBundleRoot(manifest, workspaceRoot, workspace)
}

// InspectBundleRoot imports a bundle using a workspace-bounded filesystem.
func InspectBundleRoot(manifest model.SourceManifest, workspaceRoot string, workspace *os.Root) (model.SourceInventory, []model.Diagnostic) {
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
	inspector := inspector{
		workspaceRoot: workspaceRoot,
		filesystem:    workspace,
		root:          root,
		inputs:        make(map[model.RelativePath]string),
		nativeGaps:    make(map[model.RelativePath]model.NativeGap),
	}
	info, err := inspector.lstat(root)
	if err != nil {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", fmt.Sprintf("source root: %v", err))}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", "source root must be a non-symlink directory")}
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
	for _, entry := range *manifest.Assets {
		if entry.Path == nil {
			i.addDiagnostic(packagePath, "asset entry requires path")
			continue
		}
		assetPath, err := model.NewRelativePath(*entry.Path)
		if err != nil {
			i.addDiagnostic(packagePath, "asset path %q: %v", *entry.Path, err)
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
		asset.Targets = append([]model.TargetID(nil), entry.Targets...)
		if len(asset.Targets) > 0 {
			sort.Slice(asset.Targets, func(left, right int) bool { return asset.Targets[left] < asset.Targets[right] })
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
	kind, name, mainFile, assetDir, markdown, hookDirectory, err := classifyAsset(string(assetPath))
	if err != nil {
		i.addDiagnostic(assetPath, "%v", err)
		return model.SourceAsset{}, nil, false
	}
	identity, err := model.NewAssetID(string(kind) + "/" + name)
	if err != nil {
		i.addDiagnostic(assetPath, "asset identity: %v", err)
		return model.SourceAsset{}, nil, false
	}

	content := model.AssetContent{Frontmatter: map[string]any{}, Files: make(map[model.RelativePath]model.FileContent)}
	metadataDir := assetDir
	switch kind {
	case model.AssetKindResource:
		i.readSupportFiles(assetDir, &content)
	case model.AssetKindNativeResource:
		mainPath := model.RelativePath(mainFile)
		info, err := i.lstat(filepath.Join(i.root, filepath.FromSlash(mainFile)))
		if err != nil {
			i.addDiagnostic(mainPath, "inspect native resource: %v", err)
			return model.SourceAsset{}, nil, false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			i.addDiagnostic(mainPath, "native resource must not be a symlink")
			return model.SourceAsset{}, nil, false
		}
		if info.IsDir() {
			i.readNativeResourceFiles(assetDir, &content)
		} else {
			metadataDir = filepath.ToSlash(filepath.Dir(string(assetPath)))
			data, ok := i.readRegular(mainPath)
			if !ok {
				return model.SourceAsset{}, nil, false
			}
			content.Files[model.RelativePath(filepath.Base(mainFile))] = model.FileContent{
				Bytes:      append([]byte(nil), data...),
				Executable: info.Mode().Perm()&0o111 != 0,
				Origin:     []model.SourceLocation{{Path: mainPath}},
			}
		}
	default:
		mainPath := model.RelativePath(mainFile)
		data, ok := i.readRegular(mainPath)
		if !ok {
			return model.SourceAsset{}, nil, false
		}
		if markdown {
			frontmatter, body, err := parseMarkdown(data)
			if err != nil {
				i.addDiagnostic(mainPath, "%v", err)
				return model.SourceAsset{}, nil, false
			}
			content.Frontmatter = frontmatter
			content.Body = body
		} else {
			content.Body = string(data)
		}
		if kind == model.AssetKindSkill {
			i.readSupportFiles(assetDir, &content)
		}
		if hookDirectory {
			i.readHookPayloads(assetDir, &content)
		}
	}

	overlayDir := metadataDir
	if kind == model.AssetKindAgent && strings.HasSuffix(string(assetPath), ".md") {
		overlayDir = string(assetPath)
	}
	capabilities, native := i.readAssetMetadata(metadataDir, kind)
	var hook *model.HookDescriptor
	if kind == model.AssetKindHook {
		descriptorPath := model.RelativePath(mainFile)
		descriptor, err := model.DecodeHookDescriptorJSON([]byte(content.Body), identity, model.SourceLocation{Path: descriptorPath})
		if err != nil {
			i.addDiagnostic(descriptorPath, "hook descriptor: %v", err)
			return model.SourceAsset{}, nil, false
		}
		i.validateHookPayloadReferences(descriptor, content.Files)
		hook = &descriptor
		capabilities = mergeCapabilityUses(hookCapabilityUses(descriptor), capabilities)
	}

	asset := model.SourceAsset{
		Identity:       identity,
		Kind:           kind,
		Base:           content,
		Hook:           hook,
		Native:         native,
		CapabilityUses: capabilities,
		Overlays:       i.readOverlays(overlayDir, identity),
	}
	if kind != model.AssetKindNativeResource {
		return asset, nil, true
	}
	parts := strings.Split(string(assetPath), "/")
	targetIndex := 1
	if len(parts) > 0 && parts[0] == "src" {
		targetIndex = 2
	}
	if len(parts) <= targetIndex {
		i.addDiagnostic(assetPath, "native resource path is missing its target")
		return model.SourceAsset{}, nil, false
	}
	target := model.TargetID(parts[targetIndex])
	if target == model.TargetAntigravity {
		declared := false
		for _, capability := range capabilities {
			if capability.Key == "asset.native-resource" {
				declared = true
				break
			}
		}
		if !declared {
			i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(metadataDir, ".agentbundler/asset.json"))), "Antigravity native resource must explicitly declare capability %q", "asset.native-resource")
		}
	}
	return asset, &model.NativeGap{
		Component: name,
		Asset:     &identity,
		Location:  model.SourceLocation{Path: assetPath},
		Target:    &target,
	}, true
}

func classifyAsset(assetPath string) (model.AssetKind, string, string, string, bool, bool, error) {
	parts := strings.Split(assetPath, "/")
	if len(parts) >= 2 && parts[0] == "src" {
		parts = parts[1:]
	}
	switch {
	case len(parts) == 2 && parts[0] == "skills":
		return model.AssetKindSkill, parts[1], assetPath + "/SKILL.md", assetPath, true, false, nil
	case len(parts) == 2 && parts[0] == "agents" && strings.HasSuffix(parts[1], ".md") && strings.TrimSuffix(parts[1], ".md") != "":
		return model.AssetKindAgent, strings.TrimSuffix(parts[1], ".md"), assetPath, filepath.ToSlash(filepath.Dir(assetPath)), true, false, nil
	case len(parts) == 2 && parts[0] == "resources" && parts[1] != "":
		return model.AssetKindResource, parts[1], "", assetPath, false, false, nil
	case len(parts) == 2 && parts[0] == "hooks" && strings.HasSuffix(parts[1], ".json") && strings.TrimSuffix(parts[1], ".json") != "":
		return model.AssetKindHook, strings.TrimSuffix(parts[1], ".json"), assetPath, filepath.ToSlash(filepath.Dir(assetPath)), false, false, nil
	case len(parts) == 2 && parts[0] == "hooks" && parts[1] != "":
		return model.AssetKindHook, parts[1], assetPath + "/hook.json", assetPath, false, true, nil
	case len(parts) == 3 && parts[0] == "plugins" && parts[1] != "" && parts[2] != "":
		if _, err := parseTarget(parts[1]); err != nil {
			return "", "", "", "", false, false, err
		}
		return model.AssetKindNativeResource, parts[2], assetPath, assetPath, false, false, nil
	default:
		return "", "", "", "", false, false, fmt.Errorf("asset path %q is not a canonical skill, agent, resource, hook, or native resource", assetPath)
	}
}

func (i *inspector) readNativeResourceFiles(assetDir string, content *model.AssetContent) {
	root := filepath.Join(i.root, filepath.FromSlash(assetDir))
	err := i.walkDir(root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
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
			return fmt.Errorf("native resource path %q is a symlink", rel)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect native resource path %q: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("native resource path %q is not a regular file", rel)
		}
		path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, rel)))
		data, ok := i.readRegular(path)
		if ok {
			content.Files[model.RelativePath(rel)] = model.FileContent{
				Bytes:      data,
				Executable: info.Mode().Perm()&0o111 != 0,
				Origin:     []model.SourceLocation{{Path: path}},
			}
		}
		return nil
	})
	if err != nil {
		i.addDiagnostic(model.RelativePath(assetDir), "%v", err)
	}
}

func (i *inspector) readSupportFiles(assetDir string, content *model.AssetContent) {
	root := filepath.Join(i.root, filepath.FromSlash(assetDir))
	err := i.walkDir(root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
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
		if ignoredSupportPath(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("support file %q is a symlink", rel)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect support file %q: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("support path %q is not a regular file", rel)
		}
		if rel == "SKILL.md" {
			return nil
		}
		path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, rel)))
		data, ok := i.readRegular(path)
		if ok {
			content.Files[model.RelativePath(rel)] = model.FileContent{
				Bytes:      data,
				Executable: info.Mode().Perm()&0o111 != 0,
				Origin:     []model.SourceLocation{{Path: path}},
			}
		}
		return nil
	})
	if err != nil {
		i.addDiagnostic(model.RelativePath(assetDir), "%v", err)
	}
}

func (i *inspector) readHookPayloads(assetDir string, content *model.AssetContent) {
	root := filepath.Join(i.root, filepath.FromSlash(assetDir))
	err := i.walkDir(root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		if rel == "." {
			if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				return fmt.Errorf("hook path must be a non-symlink directory")
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".agentbundler" {
			if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				return fmt.Errorf("sidecar path must be a non-symlink directory")
			}
			return filepath.SkipDir
		}
		if ignoredSupportPath(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("hook payload %q is a symlink", rel)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect hook payload %q: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("hook payload %q is not a regular file", rel)
		}
		if rel == "hook.json" {
			return nil
		}
		path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, rel)))
		data, ok := i.readRegular(path)
		if ok {
			content.Files[model.RelativePath(rel)] = model.FileContent{
				Bytes:      data,
				Executable: info.Mode().Perm()&0o111 != 0,
				Origin:     []model.SourceLocation{{Path: path}},
			}
		}
		return nil
	})
	if err != nil {
		i.addDiagnostic(model.RelativePath(assetDir), "%v", err)
	}
}

func ignoredSupportPath(path string, directory bool) bool {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if name == ".git" || name == "__pycache__" || name == ".DS_Store" {
		return true
	}
	if directory {
		return strings.HasPrefix(name, ".cache") || name == "node_modules"
	}
	return strings.HasSuffix(name, ".pyc") || strings.HasSuffix(name, "~") ||
		strings.HasPrefix(name, ".#") || strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".swo") || strings.HasSuffix(name, ".bak")
}

func (i *inspector) validateHookPayloadReferences(descriptor model.HookDescriptor, files map[model.RelativePath]model.FileContent) {
	for index, argument := range descriptor.Handler.Arguments {
		if argument.PackageFile == nil {
			continue
		}
		path, err := model.NewRelativePath(string(*argument.PackageFile))
		if err != nil {
			i.addDiagnostic(descriptor.Location.Path, "hook descriptor argument %d packageFile: %v", index, err)
			continue
		}
		if _, exists := files[path]; !exists {
			i.addDiagnostic(descriptor.Location.Path, "hook descriptor argument %d packageFile %q does not exist in the hook payload", index, path)
		}
	}
}

func hookCapabilityUses(descriptor model.HookDescriptor) []model.CapabilityUse {
	location := descriptor.Location
	keys := []model.CapabilityKey{
		"asset.hook",
		model.CapabilityKey("hook.command." + string(descriptor.Handler.Mode)),
		model.CapabilityKey("hook.event." + string(descriptor.Event)),
	}
	if descriptor.Matcher != nil {
		keys = append(keys, "hook.matcher.tool-category")
	}
	if descriptor.Asynchronous {
		keys = append(keys, "hook.async")
	}
	if descriptor.FailurePolicy == model.HookFailurePolicyClosed {
		keys = append(keys, "hook.failure.closed")
	}
	uses := make([]model.CapabilityUse, len(keys))
	for index, key := range keys {
		uses[index] = model.CapabilityUse{Key: key, Location: location}
	}
	return uses
}

func mergeCapabilityUses(groups ...[]model.CapabilityUse) []model.CapabilityUse {
	byKey := make(map[model.CapabilityKey]model.CapabilityUse)
	for _, group := range groups {
		for _, use := range group {
			if _, exists := byKey[use.Key]; !exists {
				byKey[use.Key] = use
			}
		}
	}
	keys := make([]model.CapabilityKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	uses := make([]model.CapabilityUse, 0, len(keys))
	for _, key := range keys {
		uses = append(uses, byKey[key])
	}
	return uses
}

func (i *inspector) readAssetMetadata(assetDir string, kind model.AssetKind) ([]model.CapabilityUse, *model.NativeResourceOptions) {
	path := model.RelativePath(filepath.ToSlash(filepath.Join(assetDir, ".agentbundler/asset.json")))
	data, exists, ok := i.readOptionalRegular(path)
	if !exists {
		return nil, nil
	}
	if !ok {
		return nil, nil
	}
	var sidecar assetSidecar
	if err := decodeStrictJSON(data, &sidecar); err != nil {
		i.addDiagnostic(path, "asset sidecar: %v", err)
		return nil, nil
	}
	if sidecar.Capabilities == nil {
		i.addDiagnostic(path, "asset sidecar requires capabilities")
		return nil, nil
	}
	if kind != model.AssetKindNativeResource && sidecar.PiExtensions != nil {
		i.addDiagnostic(path, "piExtensions is valid only for native resources")
	}
	var native *model.NativeResourceOptions
	if sidecar.PiExtensions != nil {
		native = &model.NativeResourceOptions{PiExtensions: make([]model.RelativePath, 0, len(*sidecar.PiExtensions))}
		seen := make(map[model.RelativePath]struct{}, len(*sidecar.PiExtensions))
		for _, value := range *sidecar.PiExtensions {
			resourcePath, err := model.NewRelativePath(value)
			if err != nil {
				i.addDiagnostic(path, "Pi extension path %q: %v", value, err)
				continue
			}
			if _, exists := seen[resourcePath]; exists {
				i.addDiagnostic(path, "Pi extension path %q is duplicated", resourcePath)
				continue
			}
			seen[resourcePath] = struct{}{}
			native.PiExtensions = append(native.PiExtensions, resourcePath)
		}
		sort.Slice(native.PiExtensions, func(left, right int) bool { return native.PiExtensions[left] < native.PiExtensions[right] })
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
	return uses, native
}

func overlaySidecarRoot(assetDir string) string {
	if strings.HasSuffix(assetDir, ".md") {
		return assetDir + ".agentbundler"
	}
	return filepath.ToSlash(filepath.Join(assetDir, ".agentbundler"))
}

func (i *inspector) readOverlays(assetDir string, identity model.AssetID) []model.TargetOverlay {
	sidecarRoot := overlaySidecarRoot(assetDir)
	targetsPath := model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets")))
	targetsRoot := filepath.Join(i.root, filepath.FromSlash(string(targetsPath)))
	info, err := i.lstat(targetsRoot)
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
	if err := i.noSymlinkComponents(i.root, targetsRoot); err != nil {
		i.addDiagnostic(targetsPath, "%v", err)
		return nil
	}
	entries, err := i.readDir(targetsRoot)
	if err != nil {
		i.addDiagnostic(targetsPath, "read target sidecars: %v", err)
		return nil
	}

	targets := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", entry.Name()))), "target sidecar path is a symlink")
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
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", entry.Name()))), "target sidecar path must be a target JSON file or directory")
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
			i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", name))), "%v", err)
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
	sidecarRoot := overlaySidecarRoot(assetDir)
	jsonPath := model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", string(target)+".json")))
	data, jsonExists, jsonOK := i.readOptionalRegular(jsonPath)
	if jsonExists && !jsonOK {
		return model.TargetOverlay{}, false
	}

	overlay := model.TargetOverlay{Target: target}
	files := make(map[model.RelativePath]model.FileContent)
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
		filePaths := make([]string, 0, len(sidecar.Files))
		for path := range sidecar.Files {
			filePaths = append(filePaths, path)
		}
		sort.Strings(filePaths)
		for _, path := range filePaths {
			rel, err := model.NewRelativePath(path)
			if err != nil {
				i.addDiagnostic(jsonPath, "overlay file %q: %v", path, err)
				continue
			}
			value, err := model.DecodeOverlayFileContentJSON(sidecar.Files[path], model.SourceLocation{Path: jsonPath})
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

	filesDir := filepath.Join(i.root, filepath.FromSlash(filepath.Join(sidecarRoot, "targets", string(target), "files")))
	if !i.readOverlayFiles(filesDir, assetDir, target, files) {
		return model.TargetOverlay{}, false
	}
	for path, content := range files {
		overlay.Files = append(overlay.Files, model.FilePatch{Path: path, Content: content})
	}
	sort.Slice(overlay.Files, func(a, b int) bool { return overlay.Files[a].Path < overlay.Files[b].Path })
	sort.Slice(overlay.DeletedFiles, func(a, b int) bool { return overlay.DeletedFiles[a] < overlay.DeletedFiles[b] })
	sort.Slice(overlay.Acknowledgments, func(a, b int) bool {
		return overlay.Acknowledgments[a].Key < overlay.Acknowledgments[b].Key
	})
	return overlay, true
}

func (i *inspector) readOverlayFiles(filesDir, assetDir string, target model.TargetID, files map[model.RelativePath]model.FileContent) bool {
	sidecarRoot := overlaySidecarRoot(assetDir)
	info, err := i.lstat(filesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", string(target), "files"))), "overlay files: %v", err)
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", string(target), "files"))), "overlay files must be a non-symlink directory")
		return false
	}
	valid := true
	err = i.walkDir(filesDir, func(fullPath string, entry fs.DirEntry, walkErr error) error {
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
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect overlay file %q: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("overlay file %q is not regular", rel)
		}
		path := model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", string(target), "files", rel)))
		data, ok := i.readRegular(path)
		if !ok {
			valid = false
			return nil
		}
		files[model.RelativePath(rel)] = model.FileContent{
			Bytes:      data,
			Executable: info.Mode().Perm()&0o111 != 0,
			Origin:     []model.SourceLocation{{Path: path}},
		}
		return nil
	})
	if err != nil {
		i.addDiagnostic(model.RelativePath(filepath.ToSlash(filepath.Join(sidecarRoot, "targets", string(target), "files"))), "%v", err)
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
	if err := i.noSymlinkComponents(i.root, fullPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, false
		}
		i.addDiagnostic(path, "%v", err)
		return nil, true, false
	}
	info, err := i.lstat(fullPath)
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
	data, err := i.readFile(fullPath)
	if err != nil {
		i.addDiagnostic(path, "read file: %v", err)
		return nil, true, false
	}
	digest := sha256.Sum256(data)
	i.inputs[path] = hex.EncodeToString(digest[:])
	return data, true, true
}

func (i *inspector) relativeToWorkspace(path string) (string, error) {
	relative, err := filepath.Rel(i.workspaceRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return filepath.ToSlash(relative), nil
}

func (i *inspector) noSymlinkComponents(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("source path leaves source root")
	}
	current := root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "." || segment == "" {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := i.lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source path crosses a symlink")
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

func (i *inspector) walkDir(path string, walk fs.WalkDirFunc) error {
	relative, err := i.relativeToWorkspace(path)
	if err != nil {
		return err
	}
	return fs.WalkDir(i.filesystem.FS(), relative, func(relativePath string, entry fs.DirEntry, walkErr error) error {
		return walk(filepath.Join(i.workspaceRoot, filepath.FromSlash(relativePath)), entry, walkErr)
	})
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
	return model.DecodeStrictJSON(data, destination)
}

func parseMarkdown(data []byte) (map[string]any, string, error) {
	return frontmatter.Parse(data)
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
	case model.TargetAntigravity, model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor:
		return target, nil
	default:
		return "", fmt.Errorf("target %q is invalid", value)
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
