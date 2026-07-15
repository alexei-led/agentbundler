// Package packageoutput renders installable target package roots.
package packageoutput

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/marketplace"
)

// RenderWithCodec renders an explicit target request using a target-owned serialization codec.
func RenderWithCodec(input model.TargetRenderInput, codec Codec) (model.TargetPlan, []model.Diagnostic) {
	target := codec.Target
	input.Packages = append([]model.NormalizedPackage(nil), input.Packages...)
	model.SortTargetRenderInput(&input)
	if diagnostics := validateDuplicateHookIDs(input.Packages); len(diagnostics) != 0 {
		return empty(target), diagnostics
	}
	if diagnostics := model.ValidateTargetRenderInput(input); len(diagnostics) != 0 {
		return empty(target), diagnostics
	}
	if input.PackageMode != model.TargetPackageModeSeparate {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-package-aggregation", fmt.Sprintf("target %q aggregate package rendering is not implemented", target))}
	}
	if diagnostics := validateCodec(codec); len(diagnostics) != 0 {
		return empty(target), diagnostics
	}
	for _, pkg := range input.Packages {
		if pkg.Target != target {
			return empty(target), []model.Diagnostic{diagnostic("target-mismatch", fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, target))}
		}
		if pkg.Profile != model.TargetProfilePackage {
			return empty(target), []model.Diagnostic{diagnostic("invalid-target-profile", fmt.Sprintf("target %q requires package profile", target))}
		}
		if diagnostics := validateCapabilities(pkg, codec); len(diagnostics) != 0 {
			return empty(target), diagnostics
		}
		if codec.ValidatePackage != nil {
			if diagnostics := codec.ValidatePackage(pkg); len(diagnostics) != 0 {
				return empty(target), diagnostics
			}
		}
	}
	var catalog marketplace.Catalog
	if input.Distribution != nil && codec.Catalog != nil {
		var diagnostics []model.Diagnostic
		catalog, diagnostics = marketplace.Build(input)
		if len(diagnostics) != 0 {
			return empty(target), diagnostics
		}
	}

	orderedPackages := append([]model.NormalizedPackage(nil), input.Packages...)
	sort.Slice(orderedPackages, func(left, right int) bool { return orderedPackages[left].Identity < orderedPackages[right].Identity })
	packageIDs := make([]model.PackageID, 0, len(orderedPackages))
	for _, pkg := range orderedPackages {
		packageIDs = append(packageIDs, pkg.Identity)
	}
	plan := model.TargetPlan{Target: target, Packages: packageIDs, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
	paths := make(map[model.RelativePath]outputOwner)
	for _, pkg := range orderedPackages {
		root := packageRoot(len(input.Packages), pkg.Identity)
		hookAssets := make([]model.NormalizedAsset, 0)
		for _, asset := range sortedAssets(pkg.Assets) {
			if asset.Kind == model.AssetKindHook {
				hookAssets = append(hookAssets, asset)
				continue
			}
			if err := renderAsset(&plan.Files, paths, codec, root, asset); err != nil {
				if fieldError, ok := err.(*UnsupportedAgentFieldError); ok {
					return empty(target), []model.Diagnostic{fieldError.Diagnostic()}
				}
				return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
			}
		}
		if len(hookAssets) != 0 {
			if codec.Hooks == nil {
				return empty(target), []model.Diagnostic{diagnostic("invalid-codec", "installable package codec requires a hook renderer for hook assets")}
			}
			hooks, err := renderHookPayloads(&plan.Files, paths, codec, root, hookAssets)
			if err != nil {
				return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
			}
			manifest, err := codec.Hooks(HookRenderInput{packageID: pkg.Identity, hooks: hooks})
			if err != nil {
				return empty(target), []model.Diagnostic{diagnostic("invalid-hook-manifest", err.Error())}
			}
			if err := addGenerated(&plan.Files, paths, rootedPath(root, string(manifest.Path)), manifest.Bytes, hookOrigins(hooks), "native hook manifest"); err != nil {
				return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
			}
		}
		manifestBytes, err := codec.Manifest(pkg)
		if err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-manifest", err.Error())}
		}
		if err := addGenerated(&plan.Files, paths, rootedPath(root, "README.md"), packageReadme(pkg), nil, "generated package README"); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
		if err := addGenerated(&plan.Files, paths, rootedPath(root, codec.ManifestPath), manifestBytes, nil, "generated package manifest"); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
	}
	if input.Distribution != nil && codec.Catalog != nil {
		manifest, err := codec.Catalog(catalog)
		if err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-marketplace-manifest", err.Error())}
		}
		if err := addGenerated(&plan.Files, paths, manifest.Path, manifest.Bytes, nil, "generated marketplace manifest"); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
	}
	sort.Slice(plan.Files, func(left, right int) bool { return plan.Files[left].Path < plan.Files[right].Path })
	return plan, nil
}

func packageRoot(packageCount int, packageID model.PackageID) string {
	if packageCount == 1 {
		return ""
	}
	return string(packageID)
}

func rootedPath(root, value string) model.RelativePath {
	if root == "" {
		return model.RelativePath(value)
	}
	return model.RelativePath(root + "/" + value)
}

func renderAsset(files *[]model.PlannedFile, paths map[model.RelativePath]outputOwner, codec Codec, root string, asset model.NormalizedAsset) error {
	name := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("asset identity %q cannot be rendered in a package root", asset.Identity)
	}
	switch asset.Kind {
	case model.AssetKindSkill:
		return renderSkill(files, paths, root, asset, "skills/"+name)
	case model.AssetKindResource:
		for _, path := range sortedFilePaths(asset.Content.Files) {
			content := asset.Content.Files[path]
			if err := addContent(files, paths, rootedPath(root, "resources/"+name+"/"+string(path)), content, fmt.Sprintf("resource %q payload %q", asset.Identity, path)); err != nil {
				return err
			}
		}
		return nil
	case model.AssetKindAgent:
		if codec.Agent == nil || codec.AgentRoot == "" {
			return fmt.Errorf("target %q does not support agents in installable packages", codec.Target)
		}
		data, extension, err := codec.Agent(asset)
		if err != nil {
			if fieldError, ok := err.(*UnsupportedAgentFieldError); ok && fieldError.Asset == "" {
				fieldError.Asset = asset.Identity
			}
			return err
		}
		if err := validateAgentSuffix(extension); err != nil {
			return err
		}
		return addGenerated(files, paths, rootedPath(root, codec.AgentRoot+"/"+name+extension), data, nil, fmt.Sprintf("agent %q", asset.Identity))
	default:
		return fmt.Errorf("asset kind %q is not supported in package output", asset.Kind)
	}
}

func renderSkill(files *[]model.PlannedFile, paths map[model.RelativePath]outputOwner, packageRoot string, asset model.NormalizedAsset, root string) error {
	data, err := markdown(asset.Content.Frontmatter, asset.Content.Body)
	if err != nil {
		return err
	}
	if err := addGenerated(files, paths, rootedPath(packageRoot, root+"/SKILL.md"), data, nil, fmt.Sprintf("skill %q", asset.Identity)); err != nil {
		return err
	}
	for _, path := range sortedFilePaths(asset.Content.Files) {
		content := asset.Content.Files[path]
		if err := addContent(files, paths, rootedPath(packageRoot, root+"/"+string(path)), content, fmt.Sprintf("skill %q payload %q", asset.Identity, path)); err != nil {
			return err
		}
	}
	return nil
}

func renderHookPayloads(files *[]model.PlannedFile, paths map[model.RelativePath]outputOwner, codec Codec, packageRoot string, assets []model.NormalizedAsset) ([]HookInput, error) {
	hooksByIdentity := make(map[model.AssetID]HookInput, len(assets))
	descriptors := make([]model.HookDescriptor, 0, len(assets))
	for _, asset := range assets {
		if asset.Hook == nil {
			return nil, fmt.Errorf("hook asset %q has no descriptor", asset.Identity)
		}
		name := strings.TrimPrefix(string(asset.Identity), string(model.AssetKindHook)+"/")
		payloadRoot := model.RelativePath(codec.HookPayloadRoot + "/" + name)
		payload := make([]HookPayloadFile, 0, len(asset.Content.Files))
		for _, path := range sortedFilePaths(asset.Content.Files) {
			content := asset.Content.Files[path]
			packagePath := model.RelativePath(string(payloadRoot) + "/" + string(path))
			if err := addContent(files, paths, rootedPath(packageRoot, string(packagePath)), content, fmt.Sprintf("hook %q payload %q", asset.Identity, path)); err != nil {
				return nil, err
			}
			payload = append(payload, HookPayloadFile{
				path:        path,
				packagePath: packagePath,
				bytes:       append([]byte(nil), content.Bytes...),
				executable:  content.Executable,
				origin:      cloneSourceLocations(content.Origin),
			})
		}
		descriptor := cloneHookDescriptor(*asset.Hook)
		if previous, exists := hooksByIdentity[descriptor.Identity]; exists {
			return nil, fmt.Errorf("hook ID %q is duplicated between %s and %s", descriptor.Identity, formatLocation(previous.descriptor.Location), formatLocation(descriptor.Location))
		}
		hooksByIdentity[descriptor.Identity] = HookInput{descriptor: descriptor, payloadRoot: payloadRoot, payload: payload}
		descriptors = append(descriptors, descriptor)
	}
	model.SortHookDescriptors(descriptors)
	hooks := make([]HookInput, 0, len(descriptors))
	for _, descriptor := range descriptors {
		hooks = append(hooks, hooksByIdentity[descriptor.Identity])
	}
	return hooks, nil
}

func hookOrigins(hooks []HookInput) []model.SourceLocation {
	origins := make([]model.SourceLocation, 0, len(hooks))
	for _, hook := range hooks {
		origins = append(origins, cloneSourceLocation(hook.descriptor.Location))
	}
	return origins
}

func packageReadme(pkg model.NormalizedPackage) []byte {
	title := string(pkg.Identity)
	if value, ok := pkg.Metadata["displayName"].(string); ok && value != "" {
		title = value
	}
	lines := []string{"# " + title, ""}
	if description, ok := pkg.Metadata["description"].(string); ok && description != "" {
		lines = append(lines, description, "")
	}
	lines = append(lines, "Generated by **Agent Bundler**.")
	if homepage, ok := pkg.Metadata["homepage"].(string); ok && homepage != "" {
		lines = append(lines, "", "More information: "+homepage)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// ManifestBase returns the common installable package manifest fields.
func ManifestBase(pkg model.NormalizedPackage) map[string]any {
	return map[string]any{"name": pkg.Identity}
}

// CopyMetadata copies selected package metadata fields into a manifest.
func CopyMetadata(destination map[string]any, metadata model.PackageMetadata, keys ...string) {
	copyMetadata(destination, metadata, keys...)
}

// PersonMetadata normalizes a manifest author value.
func PersonMetadata(value any) any { return personMetadata(value) }

// ManifestJSON serializes a package manifest deterministically.
func ManifestJSON(values map[string]any) ([]byte, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// PackageHasAsset reports whether a package contains an asset kind.
func PackageHasAsset(pkg model.NormalizedPackage, kind model.AssetKind) bool {
	return packageHasAsset(pkg, kind)
}

// Markdown serializes markdown frontmatter and body.
func Markdown(frontmatter map[string]any, body string) ([]byte, error) {
	return markdown(frontmatter, body)
}

func copyMetadata(destination map[string]any, metadata model.PackageMetadata, keys ...string) {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			destination[key] = value
		}
	}
}

func personMetadata(value any) any {
	switch value := value.(type) {
	case string:
		if value != "" {
			return map[string]string{"name": value}
		}
	case map[string]any:
		return value
	}
	return value
}

func packageHasAsset(pkg model.NormalizedPackage, kind model.AssetKind) bool {
	for _, asset := range pkg.Assets {
		if asset.Kind == kind {
			return true
		}
	}
	return false
}

func markdown(frontmatter map[string]any, body string) ([]byte, error) {
	if len(frontmatter) == 0 {
		return []byte(body), nil
	}
	encoded, err := json.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(encoded) + "\n---\n" + body), nil
}

func sortedAssets(assets []model.NormalizedAsset) []model.NormalizedAsset {
	result := append([]model.NormalizedAsset(nil), assets...)
	sort.Slice(result, func(left, right int) bool { return result[left].Identity < result[right].Identity })
	return result
}

func sortedFilePaths(files map[model.RelativePath]model.FileContent) []model.RelativePath {
	paths := make([]model.RelativePath, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(left, right int) bool { return paths[left] < paths[right] })
	return paths
}

type outputOwner struct {
	description string
	origin      []model.SourceLocation
}

func addContent(files *[]model.PlannedFile, paths map[model.RelativePath]outputOwner, path model.RelativePath, content model.FileContent, description string) error {
	return addPlanned(files, paths, model.PlannedFile{
		Path:       path,
		Bytes:      append([]byte(nil), content.Bytes...),
		Executable: content.Executable,
		Origin:     cloneSourceLocations(content.Origin),
	}, outputOwner{description: description, origin: cloneSourceLocations(content.Origin)})
}

func addGenerated(files *[]model.PlannedFile, paths map[model.RelativePath]outputOwner, path model.RelativePath, data []byte, origin []model.SourceLocation, description string) error {
	return addPlanned(files, paths, model.PlannedFile{
		Path:   path,
		Bytes:  append([]byte(nil), data...),
		Origin: cloneSourceLocations(origin),
	}, outputOwner{description: description, origin: cloneSourceLocations(origin)})
}

func addPlanned(files *[]model.PlannedFile, paths map[model.RelativePath]outputOwner, file model.PlannedFile, owner outputOwner) error {
	if _, err := model.NewRelativePath(string(file.Path)); err != nil {
		return fmt.Errorf("generated output path: %w", err)
	}
	if previous, exists := paths[file.Path]; exists {
		return fmt.Errorf("generated output path %q is duplicated between %s%s and %s%s", file.Path, previous.description, formatOrigins(previous.origin), owner.description, formatOrigins(owner.origin))
	}
	paths[file.Path] = owner
	*files = append(*files, file)
	return nil
}

func formatOrigins(origins []model.SourceLocation) string {
	if len(origins) == 0 {
		return ""
	}
	values := make([]string, len(origins))
	for index, origin := range origins {
		values[index] = formatLocation(origin)
	}
	return " (source " + strings.Join(values, ", ") + ")"
}

func formatLocation(location model.SourceLocation) string {
	value := string(location.Path)
	if location.Line != nil {
		value += fmt.Sprintf(":%d", *location.Line)
		if location.Column != nil {
			value += fmt.Sprintf(":%d", *location.Column)
		}
	}
	return value
}

func empty(target model.TargetID) model.TargetPlan {
	return model.TargetPlan{Target: target, Packages: []model.PackageID{}, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
}

func validateDuplicateHookIDs(packages []model.NormalizedPackage) []model.Diagnostic {
	for _, pkg := range packages {
		locations := make(map[model.AssetID]model.SourceLocation)
		for _, asset := range pkg.Assets {
			if asset.Kind != model.AssetKindHook || asset.Hook == nil {
				continue
			}
			identity := asset.Hook.Identity
			if previous, exists := locations[identity]; exists {
				return []model.Diagnostic{diagnostic("duplicate-hook-id", fmt.Sprintf("package %q hook ID %q is duplicated between %s and %s", pkg.Identity, identity, formatLocation(previous), formatLocation(asset.Hook.Location)))}
			}
			locations[identity] = asset.Hook.Location
		}
	}
	return nil
}

func validateCodec(codec Codec) []model.Diagnostic {
	if codec.Manifest == nil || len(codec.Capabilities) == 0 {
		return []model.Diagnostic{diagnostic("invalid-codec", "installable package codec requires manifest and capability handlers")}
	}
	if _, err := model.NewRelativePath(codec.ManifestPath); err != nil {
		return []model.Diagnostic{diagnostic("invalid-codec", fmt.Sprintf("manifest path: %v", err))}
	}
	if (codec.Agent == nil) != (codec.AgentRoot == "") {
		return []model.Diagnostic{diagnostic("invalid-codec", "agent serializer and root must be configured together")}
	}
	if codec.AgentRoot != "" {
		if _, err := model.NewRelativePath(codec.AgentRoot); err != nil {
			return []model.Diagnostic{diagnostic("invalid-codec", fmt.Sprintf("agent root: %v", err))}
		}
	}
	if (codec.Hooks == nil) != (codec.HookPayloadRoot == "") {
		return []model.Diagnostic{diagnostic("invalid-codec", "hook renderer and hook payload root must be configured together")}
	}
	if codec.HookPayloadRoot != "" {
		if _, err := model.NewRelativePath(codec.HookPayloadRoot); err != nil {
			return []model.Diagnostic{diagnostic("invalid-codec", fmt.Sprintf("hook payload root: %v", err))}
		}
	}
	if diagnostics := model.ValidateTargetComposition(model.TargetComposition{Target: codec.Target, Capabilities: codec.Capabilities}); len(diagnostics) != 0 {
		return []model.Diagnostic{diagnostic("invalid-codec", diagnostics[0].Message)}
	}
	return nil
}

func validateAgentSuffix(suffix string) error {
	if len(suffix) < 2 || suffix[0] != '.' || strings.ContainsAny(suffix, "/\\\x00") {
		return fmt.Errorf("agent suffix %q must start with a dot and contain no path separators", suffix)
	}
	return nil
}

func validateCapabilities(pkg model.NormalizedPackage, codec Codec) []model.Diagnostic {
	rules := make(map[model.CapabilityKey]model.CapabilityRule, len(codec.Capabilities))
	for _, rule := range codec.Capabilities {
		if _, exists := rules[rule.Key]; exists {
			return []model.Diagnostic{diagnostic("invalid-codec", fmt.Sprintf("capability %q is duplicated", rule.Key))}
		}
		rules[rule.Key] = rule
	}

	assets := make(map[model.AssetID]model.NormalizedAsset, len(pkg.Assets))
	for _, asset := range pkg.Assets {
		assets[asset.Identity] = asset
	}
	acknowledgments := make(map[model.AssetID]map[model.CapabilityKey]struct{}, len(pkg.Acknowledgments))
	for _, acknowledgment := range pkg.Acknowledgments {
		asset, exists := assets[acknowledgment.Asset]
		rule, known := rules[acknowledgment.Key]
		if !exists || acknowledgment.Target != codec.Target || !known || rule.State != model.CapabilityStateAdvisory || !usesCapability(asset.CapabilityUses, acknowledgment.Key) {
			return []model.Diagnostic{diagnostic("invalid-capability-acknowledgment", fmt.Sprintf("acknowledgment for asset %q capability %q does not match an advisory capability use", acknowledgment.Asset, acknowledgment.Key))}
		}
		keys := acknowledgments[acknowledgment.Asset]
		if keys == nil {
			keys = make(map[model.CapabilityKey]struct{})
			acknowledgments[acknowledgment.Asset] = keys
		}
		if _, duplicate := keys[acknowledgment.Key]; duplicate {
			return []model.Diagnostic{diagnostic("invalid-capability-acknowledgment", fmt.Sprintf("asset %q capability %q has duplicate acknowledgments", acknowledgment.Asset, acknowledgment.Key))}
		}
		keys[acknowledgment.Key] = struct{}{}
	}
	for _, asset := range pkg.Assets {
		for _, use := range asset.CapabilityUses {
			rule, known := rules[use.Key]
			if !known || rule.State == model.CapabilityStateUnsupported {
				return []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("package target %q cannot render capability %q for asset %q", codec.Target, use.Key, asset.Identity))}
			}
			if rule.State == model.CapabilityStateAdvisory {
				if _, acknowledged := acknowledgments[asset.Identity][use.Key]; !acknowledged {
					return []model.Diagnostic{diagnostic("missing-capability-acknowledgment", fmt.Sprintf("asset %q capability %q requires an acknowledgment for target %q", asset.Identity, use.Key, codec.Target))}
				}
			}
		}
	}
	return nil
}

func usesCapability(uses []model.CapabilityUse, key model.CapabilityKey) bool {
	for _, use := range uses {
		if use.Key == key {
			return true
		}
	}
	return false
}

func diagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}
