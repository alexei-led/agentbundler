// Package copilot renders deterministic baseline plans for the Copilot target.
package copilot

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	// Target is the target identifier rendered by this package.
	Target = model.TargetCopilot

	// FormatRevision identifies the deterministic baseline format.
	FormatRevision = 1
)

// Capabilities returns the Copilot capability profile.
func Capabilities() []model.CapabilityRule {
	return []model.CapabilityRule{
		{Key: "asset.agent", State: model.CapabilityStateNative},
		{Key: "asset.hook", State: model.CapabilityStateNative},
		{Key: "asset.native-resource", State: model.CapabilityStateNative},
		{Key: "asset.skill", State: model.CapabilityStateNative},
	}
}

// Render renders packages to the target-neutral deterministic baseline for Copilot.
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if diagnostics := validatePackages(packages); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}

	plan := model.TargetPlan{
		Target:       Target,
		Packages:     make([]model.PackageID, 0, len(packages)),
		NativeChecks: []model.NativeCheck{},
	}
	files := make([]model.PlannedFile, 0)
	for _, pkg := range packages {
		plan.Packages = append(plan.Packages, pkg.Identity)
		packagePath := "packages/" + encodeSegment(string(pkg.Identity))
		packageJSON, err := marshalLine(struct {
			Identity model.PackageID       `json:"identity"`
			Metadata model.PackageMetadata `json:"metadata"`
			Target   model.TargetID        `json:"target"`
		}{
			Identity: pkg.Identity,
			Metadata: pkg.Metadata,
			Target:   Target,
		})
		if err != nil {
			return emptyPlan(), []model.Diagnostic{renderDiagnostic("invalid-package-metadata", err.Error(), nil)}
		}
		files = append(files, plannedFile(packagePath+"/package.json", packageJSON))

		assets := append([]model.NormalizedAsset(nil), pkg.Assets...)
		sort.Slice(assets, func(i, j int) bool {
			return assets[i].Identity < assets[j].Identity
		})
		for _, asset := range assets {
			assetPath := packagePath + "/assets/" + string(asset.Kind) + "/" + encodeSegment(assetName(asset.Identity))
			capabilityUses := sortedCapabilityUses(asset.CapabilityUses)
			assetJSON, err := marshalLine(struct {
				CapabilityUses []model.CapabilityUse `json:"capabilityUses"`
				Frontmatter    map[string]any        `json:"frontmatter"`
				Identity       model.AssetID         `json:"identity"`
				Kind           model.AssetKind       `json:"kind"`
			}{
				CapabilityUses: capabilityUses,
				Frontmatter:    asset.Content.Frontmatter,
				Identity:       asset.Identity,
				Kind:           asset.Kind,
			})
			if err != nil {
				return emptyPlan(), []model.Diagnostic{renderDiagnostic("invalid-asset-frontmatter", err.Error(), nil)}
			}
			files = append(files,
				plannedFile(assetPath+"/asset.json", assetJSON),
				plannedFile(assetPath+"/content.md", []byte(asset.Content.Body)),
			)

			paths := make([]string, 0, len(asset.Content.Files))
			for path := range asset.Content.Files {
				paths = append(paths, string(path))
			}
			sort.Strings(paths)
			for _, path := range paths {
				files = append(files, plannedFile(assetPath+"/files/"+path, asset.Content.Files[model.RelativePath(path)]))
			}
		}
	}

	sort.Slice(plan.Packages, func(i, j int) bool {
		return plan.Packages[i] < plan.Packages[j]
	})
	index, err := marshalLine(struct {
		Format         string            `json:"format"`
		FormatRevision int               `json:"formatRevision"`
		Packages       []model.PackageID `json:"packages"`
		Target         model.TargetID    `json:"target"`
	}{
		Format:         "agentbundler-target-bundle",
		FormatRevision: FormatRevision,
		Packages:       plan.Packages,
		Target:         Target,
	})
	if err != nil {
		return emptyPlan(), []model.Diagnostic{renderDiagnostic("invalid-package-index", err.Error(), nil)}
	}
	files = append(files, plannedFile("package-index.json", index))

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	if diagnostics := validateUniquePaths(files); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}
	plan.Files = files
	return plan, nil
}

func validatePackages(packages []model.NormalizedPackage) []model.Diagnostic {
	capabilities := make(map[model.CapabilityKey]model.CapabilityState)
	for _, rule := range Capabilities() {
		capabilities[rule.Key] = rule.State
	}

	var diagnostics []model.Diagnostic
	for _, pkg := range packages {
		diagnostics = append(diagnostics, model.ValidateNormalizedPackage(pkg)...)
		if pkg.Target != Target {
			diagnostics = append(diagnostics, renderDiagnostic("target-mismatch", fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, Target), nil))
		}
		for _, asset := range pkg.Assets {
			for _, use := range asset.CapabilityUses {
				state, ok := capabilities[use.Key]
				if ok && (state == model.CapabilityStateNative || state == model.CapabilityStateEquivalent) {
					continue
				}
				location := use.Location
				diagnostics = append(diagnostics, renderDiagnostic("unsupported-capability", fmt.Sprintf("capability %q is not supported by target %q", use.Key, Target), &location))
			}
		}
	}
	return diagnostics
}

func emptyPlan() model.TargetPlan {
	return model.TargetPlan{Target: Target, NativeChecks: []model.NativeCheck{}}
}

func marshalLine(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func plannedFile(path string, bytes []byte) model.PlannedFile {
	return model.PlannedFile{Path: model.RelativePath(path), Bytes: bytes}
}

func sortedCapabilityUses(uses []model.CapabilityUse) []model.CapabilityUse {
	sorted := append([]model.CapabilityUse(nil), uses...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Key != sorted[j].Key {
			return sorted[i].Key < sorted[j].Key
		}
		return compareLocation(sorted[i].Location, sorted[j].Location) < 0
	})
	return sorted
}

func compareLocation(left, right model.SourceLocation) int {
	if left.Path != right.Path {
		return strings.Compare(string(left.Path), string(right.Path))
	}
	if comparison := compareOptionalInt(left.Line, right.Line); comparison != 0 {
		return comparison
	}
	return compareOptionalInt(left.Column, right.Column)
}

func compareOptionalInt(left, right *int) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	case *left < *right:
		return -1
	case *left > *right:
		return 1
	default:
		return 0
	}
}

func encodeSegment(value string) string {
	var encoded strings.Builder
	for _, byte := range []byte(value) {
		if (byte >= 'a' && byte <= 'z') || (byte >= 'A' && byte <= 'Z') || (byte >= '0' && byte <= '9') || byte == '-' || byte == '_' || byte == '.' {
			encoded.WriteByte(byte)
			continue
		}
		fmt.Fprintf(&encoded, "%%%02X", byte)
	}
	return encoded.String()
}

func assetName(identity model.AssetID) string {
	_, name, _ := strings.Cut(string(identity), "/")
	return name
}

func validateUniquePaths(files []model.PlannedFile) []model.Diagnostic {
	for index := 1; index < len(files); index++ {
		if files[index-1].Path == files[index].Path {
			return []model.Diagnostic{renderDiagnostic("duplicate-output-path", fmt.Sprintf("generated output path %q is duplicated", files[index].Path), nil)}
		}
	}
	return nil
}

func renderDiagnostic(code, message string, location *model.SourceLocation) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Location: location, Message: message}
}
