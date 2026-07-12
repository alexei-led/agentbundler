// Package codex renders normalized packages for the Codex target.
package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	// Target is the target identifier rendered by this package.
	Target = model.TargetCodex

	// FormatRevision identifies the deterministic baseline format.
	FormatRevision = 1
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateEquivalent},
	{Key: "asset.hook", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateNative},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// Adapter renders packages using the Codex target-neutral baseline.
type Adapter struct{}

// New returns a Codex adapter.
func New() Adapter {
	return Adapter{}
}

// Target returns the target rendered by Adapter.
func (Adapter) Target() model.TargetID {
	return Target
}

// FormatRevision returns the target format revision.
func (Adapter) FormatRevision() int {
	return FormatRevision
}

// Capabilities returns the Codex capability rules.
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}

// Capabilities returns the Codex capability rules.
func (Adapter) Capabilities() []model.CapabilityRule {
	return Capabilities()
}

// Render renders packages into one deterministic Codex target plan.
func (adapter Adapter) Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return render(adapter, packages)
}

// Render renders packages using a new Codex adapter.
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(packages)
}

func render(adapter Adapter, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	var diagnostics []model.Diagnostic
	seenPackages := make(map[model.PackageID]struct{}, len(packages))
	for _, pkg := range packages {
		diagnostics = append(diagnostics, model.ValidateNormalizedPackage(pkg)...)
		if pkg.Target != adapter.Target() {
			diagnostics = append(diagnostics, diagnostic(
				"target-mismatch",
				fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, adapter.Target()),
				nil,
			))
		}
		if _, exists := seenPackages[pkg.Identity]; exists {
			diagnostics = append(diagnostics, diagnostic(
				"duplicate-package",
				fmt.Sprintf("package %q is duplicated", pkg.Identity),
				nil,
			))
		}
		seenPackages[pkg.Identity] = struct{}{}
		diagnostics = append(diagnostics, capabilityDiagnostics(adapter, pkg)...)
	}
	if len(diagnostics) != 0 {
		return emptyPlan(adapter), diagnostics
	}

	sortedPackages := append([]model.NormalizedPackage(nil), packages...)
	sort.Slice(sortedPackages, func(i, j int) bool {
		return sortedPackages[i].Identity < sortedPackages[j].Identity
	})

	plan := model.TargetPlan{
		Target:       adapter.Target(),
		Packages:     make([]model.PackageID, 0, len(sortedPackages)),
		NativeChecks: []model.NativeCheck{},
	}
	paths := make(map[model.RelativePath]struct{})
	for _, pkg := range sortedPackages {
		plan.Packages = append(plan.Packages, pkg.Identity)
		packageSegment := encodeSegment(string(pkg.Identity))
		packagePath := model.RelativePath("packages/" + packageSegment + "/package.json")
		packageBytes, err := canonicalJSON(map[string]any{
			"identity": pkg.Identity,
			"metadata": pkg.Metadata,
			"target":   adapter.Target(),
		})
		if err != nil {
			return emptyPlan(adapter), []model.Diagnostic{diagnostic("invalid-json", err.Error(), nil)}
		}
		if duplicatePath(paths, packagePath) {
			return emptyPlan(adapter), []model.Diagnostic{duplicatePathDiagnostic(packagePath)}
		}
		plan.Files = append(plan.Files, model.PlannedFile{Path: packagePath, Bytes: packageBytes})

		assets := append([]model.NormalizedAsset(nil), pkg.Assets...)
		sort.Slice(assets, func(i, j int) bool {
			return assets[i].Identity < assets[j].Identity
		})
		for _, asset := range assets {
			assetName := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
			assetRoot := "packages/" + packageSegment + "/assets/" + string(asset.Kind) + "/" + encodeSegment(assetName)
			assetJSON, err := canonicalJSON(map[string]any{
				"capabilityUses": capabilityUsesJSON(asset.CapabilityUses),
				"frontmatter":    asset.Content.Frontmatter,
				"identity":       asset.Identity,
				"kind":           asset.Kind,
			})
			if err != nil {
				return emptyPlan(adapter), []model.Diagnostic{diagnostic("invalid-json", err.Error(), nil)}
			}
			assetPath := model.RelativePath(assetRoot + "/asset.json")
			if duplicatePath(paths, assetPath) {
				return emptyPlan(adapter), []model.Diagnostic{duplicatePathDiagnostic(assetPath)}
			}
			plan.Files = append(plan.Files, model.PlannedFile{Path: assetPath, Bytes: assetJSON})

			contentPath := model.RelativePath(assetRoot + "/content.md")
			if duplicatePath(paths, contentPath) {
				return emptyPlan(adapter), []model.Diagnostic{duplicatePathDiagnostic(contentPath)}
			}
			plan.Files = append(plan.Files, model.PlannedFile{Path: contentPath, Bytes: []byte(asset.Content.Body)})

			filePaths := make([]model.RelativePath, 0, len(asset.Content.Files))
			for filePath := range asset.Content.Files {
				filePaths = append(filePaths, filePath)
			}
			sort.Slice(filePaths, func(i, j int) bool { return filePaths[i] < filePaths[j] })
			for _, filePath := range filePaths {
				outputPath := model.RelativePath(assetRoot + "/files/" + string(filePath))
				if duplicatePath(paths, outputPath) {
					return emptyPlan(adapter), []model.Diagnostic{duplicatePathDiagnostic(outputPath)}
				}
				plan.Files = append(plan.Files, model.PlannedFile{
					Path:  outputPath,
					Bytes: append([]byte(nil), asset.Content.Files[filePath]...),
				})
			}
		}
	}

	indexBytes, err := canonicalJSON(map[string]any{
		"format":         "agentbundler-target-bundle",
		"formatRevision": adapter.FormatRevision(),
		"packages":       plan.Packages,
		"target":         adapter.Target(),
	})
	if err != nil {
		return emptyPlan(adapter), []model.Diagnostic{diagnostic("invalid-json", err.Error(), nil)}
	}
	indexPath := model.RelativePath("package-index.json")
	if duplicatePath(paths, indexPath) {
		return emptyPlan(adapter), []model.Diagnostic{duplicatePathDiagnostic(indexPath)}
	}
	plan.Files = append(plan.Files, model.PlannedFile{Path: indexPath, Bytes: indexBytes})
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

func capabilityDiagnostics(adapter Adapter, pkg model.NormalizedPackage) []model.Diagnostic {
	rules := make(map[model.CapabilityKey]model.CapabilityState, len(capabilityRules))
	for _, rule := range adapter.Capabilities() {
		rules[rule.Key] = rule.State
	}

	var diagnostics []model.Diagnostic
	for _, asset := range pkg.Assets {
		key := model.CapabilityKey("asset." + string(asset.Kind))
		if rules[key] != model.CapabilityStateNative {
			diagnostics = append(diagnostics, diagnostic(
				"unsupported-capability",
				fmt.Sprintf("target %q does not natively support capability %q", adapter.Target(), key),
				nil,
			))
		}
		for _, use := range asset.CapabilityUses {
			if rules[use.Key] == model.CapabilityStateNative {
				continue
			}
			location := use.Location
			diagnostics = append(diagnostics, diagnostic(
				"unsupported-capability",
				fmt.Sprintf("target %q does not natively support capability %q", adapter.Target(), use.Key),
				&location,
			))
		}
	}
	return diagnostics
}

func capabilityUsesJSON(uses []model.CapabilityUse) []any {
	sortedUses := append([]model.CapabilityUse(nil), uses...)
	sort.Slice(sortedUses, func(i, j int) bool {
		if sortedUses[i].Key != sortedUses[j].Key {
			return sortedUses[i].Key < sortedUses[j].Key
		}
		return compareLocation(sortedUses[i].Location, sortedUses[j].Location) < 0
	})

	result := make([]any, 0, len(sortedUses))
	for _, use := range sortedUses {
		result = append(result, map[string]any{
			"key":      use.Key,
			"location": sourceLocationJSON(use.Location),
		})
	}
	return result
}

func sourceLocationJSON(location model.SourceLocation) map[string]any {
	result := map[string]any{"path": location.Path}
	if location.Line != nil {
		result["line"] = *location.Line
	}
	if location.Column != nil {
		result["column"] = *location.Column
	}
	return result
}

func compareLocation(left, right model.SourceLocation) int {
	if left.Path < right.Path {
		return -1
	}
	if left.Path > right.Path {
		return 1
	}
	if result := compareOptionalInt(left.Line, right.Line); result != 0 {
		return result
	}
	return compareOptionalInt(left.Column, right.Column)
}

func compareOptionalInt(left, right *int) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(normalized); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func encodeSegment(value string) string {
	const hex = "0123456789ABCDEF"

	var output strings.Builder
	for _, byteValue := range []byte(value) {
		if (byteValue >= 'a' && byteValue <= 'z') ||
			(byteValue >= 'A' && byteValue <= 'Z') ||
			(byteValue >= '0' && byteValue <= '9') ||
			byteValue == '-' || byteValue == '_' || byteValue == '.' {
			output.WriteByte(byteValue)
			continue
		}
		output.WriteByte('%')
		output.WriteByte(hex[byteValue>>4])
		output.WriteByte(hex[byteValue&0x0f])
	}
	return output.String()
}

func duplicatePath(paths map[model.RelativePath]struct{}, outputPath model.RelativePath) bool {
	if _, exists := paths[outputPath]; exists {
		return true
	}
	paths[outputPath] = struct{}{}
	return false
}

func duplicatePathDiagnostic(path model.RelativePath) model.Diagnostic {
	return diagnostic("duplicate-output-path", fmt.Sprintf("output path %q is duplicated", path), nil)
}

func emptyPlan(adapter Adapter) model.TargetPlan {
	return model.TargetPlan{
		Target:       adapter.Target(),
		Packages:     []model.PackageID{},
		Files:        []model.PlannedFile{},
		NativeChecks: []model.NativeCheck{},
	}
}

func diagnostic(code, message string, location *model.SourceLocation) model.Diagnostic {
	return model.Diagnostic{
		Code:     code,
		Severity: model.SeverityError,
		Location: location,
		Message:  message,
	}
}
