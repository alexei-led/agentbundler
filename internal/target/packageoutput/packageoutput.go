// Package packageoutput renders installable target package roots.
package packageoutput

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// RenderWithCodec aggregates package assets using a target-owned serialization codec.
func RenderWithCodec(packages []model.NormalizedPackage, codec Codec) (model.TargetPlan, []model.Diagnostic) {
	target := codec.Target
	if len(packages) == 0 {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-package-aggregation", "installable target output requires at least one package")}
	}
	if diagnostics := validateCodec(codec); len(diagnostics) != 0 {
		return empty(target), diagnostics
	}
	for _, pkg := range packages {
		if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
			return empty(target), diagnostics
		}
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

	orderedPackages := append([]model.NormalizedPackage(nil), packages...)
	sort.Slice(orderedPackages, func(left, right int) bool { return orderedPackages[left].Identity < orderedPackages[right].Identity })
	packageIDs := make([]model.PackageID, 0, len(orderedPackages))
	for _, pkg := range orderedPackages {
		packageIDs = append(packageIDs, pkg.Identity)
	}
	plan := model.TargetPlan{Target: target, Packages: packageIDs, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
	paths := make(map[model.RelativePath]struct{})
	for _, pkg := range orderedPackages {
		root := packageRoot(len(packages), pkg.Identity)
		for _, asset := range sortedAssets(pkg.Assets) {
			if err := renderAsset(&plan.Files, paths, codec, root, asset); err != nil {
				if fieldError, ok := err.(*UnsupportedAgentFieldError); ok {
					return empty(target), []model.Diagnostic{fieldError.Diagnostic()}
				}
				return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
			}
		}
		manifestBytes, err := codec.Manifest(pkg)
		if err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-manifest", err.Error())}
		}
		if err := add(&plan.Files, paths, rootedPath(root, "README.md"), packageReadme(pkg)); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
		if err := add(&plan.Files, paths, rootedPath(root, codec.ManifestPath), manifestBytes); err != nil {
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

func renderAsset(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, codec Codec, root string, asset model.NormalizedAsset) error {
	name := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("asset identity %q cannot be rendered in a package root", asset.Identity)
	}
	switch asset.Kind {
	case model.AssetKindSkill:
		return renderSkill(files, paths, root, asset, "skills/"+name)
	case model.AssetKindResource:
		for path, content := range asset.Content.Files {
			if err := add(files, paths, rootedPath(root, "resources/"+name+"/"+string(path)), content.Bytes); err != nil {
				return err
			}
		}
		return nil
	case model.AssetKindAgent:
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
		return add(files, paths, rootedPath(root, codec.AgentRoot+"/"+name+extension), data)
	default:
		return fmt.Errorf("asset kind %q is not supported in package output", asset.Kind)
	}
}

func renderSkill(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, packageRoot string, asset model.NormalizedAsset, root string) error {
	data, err := markdown(asset.Content.Frontmatter, asset.Content.Body)
	if err != nil {
		return err
	}
	if err := add(files, paths, rootedPath(packageRoot, root+"/SKILL.md"), data); err != nil {
		return err
	}
	for path, content := range asset.Content.Files {
		if err := add(files, paths, rootedPath(packageRoot, root+"/"+string(path)), content.Bytes); err != nil {
			return err
		}
	}
	return nil
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

func add(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, path model.RelativePath, data []byte) error {
	if _, exists := paths[path]; exists {
		return fmt.Errorf("generated output path %q is duplicated", path)
	}
	paths[path] = struct{}{}
	*files = append(*files, model.PlannedFile{Path: path, Bytes: append([]byte(nil), data...)})
	return nil
}

func empty(target model.TargetID) model.TargetPlan {
	return model.TargetPlan{Target: target, Packages: []model.PackageID{}, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
}

func validateCodec(codec Codec) []model.Diagnostic {
	if codec.Manifest == nil || codec.Agent == nil || len(codec.Capabilities) == 0 {
		return []model.Diagnostic{diagnostic("invalid-codec", "installable package codec requires manifest, agent, and capability handlers")}
	}
	if _, err := model.NewRelativePath(codec.ManifestPath); err != nil {
		return []model.Diagnostic{diagnostic("invalid-codec", fmt.Sprintf("manifest path: %v", err))}
	}
	if _, err := model.NewRelativePath(codec.AgentRoot); err != nil {
		return []model.Diagnostic{diagnostic("invalid-codec", fmt.Sprintf("agent root: %v", err))}
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
