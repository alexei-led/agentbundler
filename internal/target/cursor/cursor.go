// Package cursor renders deterministic Cursor target plans.
package cursor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const formatRevision = 1

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "asset.agent", State: model.CapabilityStateNative},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
}

// Adapter describes the Cursor target capability profile and renderer revision.
type Adapter struct {
	Target         model.TargetID
	FormatRevision int
	Capabilities   []model.CapabilityRule
}

// New returns the Cursor adapter at its current format revision.
func New() Adapter {
	return Adapter{
		Target:         model.TargetCursor,
		FormatRevision: formatRevision,
		Capabilities:   append([]model.CapabilityRule(nil), capabilityRules...),
	}
}

// Render renders packages with the Cursor deterministic interchange baseline.
func Render(adapter Adapter, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if diagnostics := validateAdapter(adapter); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}
	if diagnostics := validatePackages(packages); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}

	return renderBaseline(packages), nil
}

// Render renders packages with this adapter.
func (adapter Adapter) Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return Render(adapter, packages)
}

func emptyPlan() model.TargetPlan {
	return model.TargetPlan{Target: model.TargetCursor}
}

func validateAdapter(adapter Adapter) []model.Diagnostic {
	if adapter.Target != model.TargetCursor || adapter.FormatRevision != formatRevision || !sameCapabilityRules(adapter.Capabilities, capabilityRules) {
		return []model.Diagnostic{diagnostic("invalid-adapter", "adapter is not the Cursor format revision 1 capability profile")}
	}
	return nil
}

func validatePackages(packages []model.NormalizedPackage) []model.Diagnostic {
	packageIDs := make(map[model.PackageID]struct{}, len(packages))
	for _, pkg := range packages {
		if pkg.Target != model.TargetCursor {
			return []model.Diagnostic{diagnostic("target-mismatch", fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, model.TargetCursor))}
		}
		if _, exists := packageIDs[pkg.Identity]; exists {
			return []model.Diagnostic{diagnostic("duplicate-package", fmt.Sprintf("package %q is duplicated", pkg.Identity))}
		}
		packageIDs[pkg.Identity] = struct{}{}
		if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
			return diagnostics
		}
		for _, asset := range pkg.Assets {
			if state := capabilityState(asset.Kind); state != model.CapabilityStateNative {
				return []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("target %q does not support %q", model.TargetCursor, capabilityKey(asset.Kind)))}
			}
		}
	}
	return nil
}

func renderBaseline(packages []model.NormalizedPackage) model.TargetPlan {
	sortedPackages := append([]model.NormalizedPackage(nil), packages...)
	sort.Slice(sortedPackages, func(left, right int) bool {
		return sortedPackages[left].Identity < sortedPackages[right].Identity
	})

	plan := model.TargetPlan{
		Target:   model.TargetCursor,
		Packages: make([]model.PackageID, len(sortedPackages)),
	}
	files := make([]model.PlannedFile, 0, 1+len(sortedPackages))
	for packageIndex, pkg := range sortedPackages {
		plan.Packages[packageIndex] = pkg.Identity
		packageRoot := "packages/" + segment(string(pkg.Identity))
		files = append(files, plannedJSON(packageRoot+"/package.json", packageDocument{
			Identity: pkg.Identity,
			Metadata: pkg.Metadata,
			Target:   model.TargetCursor,
		}))

		sortedAssets := append([]model.NormalizedAsset(nil), pkg.Assets...)
		sort.Slice(sortedAssets, func(left, right int) bool {
			return sortedAssets[left].Identity < sortedAssets[right].Identity
		})
		for _, asset := range sortedAssets {
			assetRoot := packageRoot + "/assets/" + string(asset.Kind) + "/" + segment(assetName(asset.Identity))
			files = append(files,
				plannedJSON(assetRoot+"/asset.json", assetDocument{
					CapabilityUses: sortedCapabilityUses(asset.CapabilityUses),
					Frontmatter:    asset.Content.Frontmatter,
					Identity:       asset.Identity,
					Kind:           asset.Kind,
				}),
				model.PlannedFile{Path: model.RelativePath(assetRoot + "/content.md"), Bytes: []byte(asset.Content.Body)},
			)
			filePaths := make([]string, 0, len(asset.Content.Files))
			for filePath := range asset.Content.Files {
				filePaths = append(filePaths, string(filePath))
			}
			sort.Strings(filePaths)
			for _, filePath := range filePaths {
				files = append(files, model.PlannedFile{
					Path:  model.RelativePath(assetRoot + "/files/" + filePath),
					Bytes: append([]byte(nil), asset.Content.Files[model.RelativePath(filePath)]...),
				})
			}
		}
	}
	files = append(files, plannedJSON("package-index.json", packageIndexDocument{
		Format:         "agentbundler-target-bundle",
		FormatRevision: formatRevision,
		Packages:       plan.Packages,
		Target:         model.TargetCursor,
	}))
	sort.Slice(files, func(left, right int) bool {
		return files[left].Path < files[right].Path
	})
	plan.Files = files
	return plan
}

func plannedJSON(path string, value any) model.PlannedFile {
	bytes, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("validated model JSON failed to marshal: %v", err))
	}
	return model.PlannedFile{Path: model.RelativePath(path), Bytes: append(bytes, '\n')}
}

func sortedCapabilityUses(uses []model.CapabilityUse) []model.CapabilityUse {
	result := append([]model.CapabilityUse(nil), uses...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Key != result[right].Key {
			return result[left].Key < result[right].Key
		}
		return compareLocation(result[left].Location, result[right].Location) < 0
	})
	return result
}

func compareLocation(left, right model.SourceLocation) int {
	if left.Path < right.Path {
		return -1
	}
	if left.Path > right.Path {
		return 1
	}
	if compared := compareOptionalInt(left.Line, right.Line); compared != 0 {
		return compared
	}
	return compareOptionalInt(left.Column, right.Column)
}

func compareOptionalInt(left, right *int) int {
	if left == nil {
		if right == nil {
			return 0
		}
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

func capabilityState(kind model.AssetKind) model.CapabilityState {
	for _, rule := range capabilityRules {
		if rule.Key == model.CapabilityKey(capabilityKey(kind)) {
			return rule.State
		}
	}
	return model.CapabilityStateUnsupported
}

func capabilityKey(kind model.AssetKind) string {
	return "asset." + string(kind)
}

func sameCapabilityRules(left, right []model.CapabilityRule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assetName(identity model.AssetID) string {
	_, name, _ := strings.Cut(string(identity), "/")
	return name
}

func segment(value string) string {
	const hexadecimal = "0123456789ABCDEF"

	var builder strings.Builder
	for _, byteValue := range []byte(value) {
		if byteValue >= 'a' && byteValue <= 'z' || byteValue >= 'A' && byteValue <= 'Z' || byteValue >= '0' && byteValue <= '9' || byteValue == '-' || byteValue == '_' || byteValue == '.' {
			builder.WriteByte(byteValue)
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(hexadecimal[byteValue>>4])
		builder.WriteByte(hexadecimal[byteValue&0x0f])
	}
	return builder.String()
}

func diagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}

type packageDocument struct {
	Identity model.PackageID       `json:"identity"`
	Metadata model.PackageMetadata `json:"metadata"`
	Target   model.TargetID        `json:"target"`
}

type assetDocument struct {
	CapabilityUses []model.CapabilityUse `json:"capabilityUses"`
	Frontmatter    map[string]any        `json:"frontmatter"`
	Identity       model.AssetID         `json:"identity"`
	Kind           model.AssetKind       `json:"kind"`
}

type packageIndexDocument struct {
	Format         string            `json:"format"`
	FormatRevision int               `json:"formatRevision"`
	Packages       []model.PackageID `json:"packages"`
	Target         model.TargetID    `json:"target"`
}
