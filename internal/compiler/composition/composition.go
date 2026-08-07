// Package composition resolves source inventories for one target.
package composition

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	diagnosticCodeInvalidComposition = "invalid-composition"
	diagnosticInvalidInput           = diagnosticCodeInvalidComposition
	diagnosticMissingCapabilityRule  = diagnosticCodeInvalidComposition
	diagnosticUnusedCapabilityRule   = diagnosticCodeInvalidComposition
	diagnosticMissingAcknowledgment  = diagnosticCodeInvalidComposition
	diagnosticUnusedAcknowledgment   = diagnosticCodeInvalidComposition
	diagnosticNativeGap              = diagnosticCodeInvalidComposition
	diagnosticOverlay                = diagnosticCodeInvalidComposition
)

// Compose resolves source assets, capability rules, acknowledgments, and native
// gaps for target.
func Compose(inventory model.SourceInventory, target model.TargetComposition) ([]model.NormalizedPackage, []model.Diagnostic) {
	diagnostics := append(model.ValidateSourceInventory(inventory), model.ValidateTargetComposition(target)...)
	if hasErrors(diagnostics) {
		return nil, invalidInputDiagnostics(diagnostics)
	}

	rules := make(map[model.CapabilityKey]model.CapabilityRule, len(target.Capabilities))
	for _, rule := range target.Capabilities {
		rules[rule.Key] = rule
	}

	assets := indexAssets(inventory.Packages)
	gapActions, gapDiagnostics := resolveNativeGaps(inventory.NativeGaps, target, assets)
	diagnostics = append(diagnostics, gapDiagnostics...)

	var packages []model.NormalizedPackage
	for _, sourcePackage := range inventory.Packages {
		profile := target.Profile
		if profile == "" {
			profile = model.TargetProfileProject
		}
		pkg := model.NormalizedPackage{
			Identity: sourcePackage.Identity,
			Metadata: cloneMap(sourcePackage.Metadata),
			Target:   target.Target,
			Profile:  profile,
		}
		for _, sourceAsset := range sourcePackage.Assets {
			if !assetSelectedForTarget(sourceAsset, target.Target) {
				continue
			}
			if gapActions[assetKey{packageID: sourcePackage.Identity, assetID: sourceAsset.Identity}] {
				continue
			}

			overlay := overlayForTarget(sourceAsset.Overlays, target.Target)
			content, overlayDiagnostics := applyOverlay(sourceAsset.Base, overlay, sourceAsset.Identity)
			diagnostics = append(diagnostics, overlayDiagnostics...)
			if target.SkillPreamble != nil && sourceAsset.Kind == model.AssetKindSkill {
				content.Body = joinPreamble(*target.SkillPreamble, content.Body)
			}
			command := model.CloneCommandDescriptor(sourceAsset.Command)
			if command != nil {
				description, _ := content.Frontmatter["description"].(string)
				command.Description = description
			}

			acknowledgments := acknowledgmentsForAsset(overlay, sourceAsset.Identity, target.Target)
			for _, use := range sourceAsset.CapabilityUses {
				rule, ok := rules[use.Key]
				if !ok {
					diagnostics = append(diagnostics, diagnostic(diagnosticMissingCapabilityRule, &use.Location, "asset %q uses capability %q without a target rule", sourceAsset.Identity, use.Key))
					continue
				}
				if rule.State == model.CapabilityStateUnsupported {
					diagnostics = append(diagnostics, diagnostic(diagnosticMissingCapabilityRule, &use.Location, "asset %q uses unsupported capability %q for target %q", sourceAsset.Identity, use.Key, target.Target))
					continue
				}
				if rule.State == model.CapabilityStateAdvisory && !hasAcknowledgment(acknowledgments, use.Key) {
					diagnostics = append(diagnostics, diagnostic(diagnosticMissingAcknowledgment, &use.Location, "asset %q uses advisory capability %q without an acknowledgment", sourceAsset.Identity, use.Key))
				}
			}

			seenAcknowledgments := make(map[model.CapabilityKey]bool, len(acknowledgments))
			for _, acknowledgment := range acknowledgments {
				rule, ok := rules[acknowledgment.Key]
				if !ok || !usesCapability(sourceAsset.CapabilityUses, acknowledgment.Key) || rule.State != model.CapabilityStateAdvisory {
					diagnostics = append(diagnostics, diagnostic(diagnosticUnusedAcknowledgment, nil, "acknowledgment for asset %q capability %q does not match an advisory capability use", acknowledgment.Asset, acknowledgment.Key))
					continue
				}
				if seenAcknowledgments[acknowledgment.Key] {
					diagnostics = append(diagnostics, diagnostic(diagnosticUnusedAcknowledgment, nil, "asset %q capability %q has duplicate acknowledgments", acknowledgment.Asset, acknowledgment.Key))
					continue
				}
				seenAcknowledgments[acknowledgment.Key] = true
				pkg.Acknowledgments = append(pkg.Acknowledgments, acknowledgment)
			}

			pkg.Assets = append(pkg.Assets, model.NormalizedAsset{
				Identity:       sourceAsset.Identity,
				Kind:           sourceAsset.Kind,
				Content:        content,
				Hook:           cloneHookDescriptor(sourceAsset.Hook),
				Command:        command,
				Native:         model.CloneNativeResourceOptions(sourceAsset.Native),
				CapabilityUses: cloneCapabilityUses(sourceAsset.CapabilityUses),
			})
		}
		packages = append(packages, pkg)
	}

	if hasErrors(diagnostics) {
		return nil, diagnostics
	}
	canonicalizePackages(packages)
	return packages, diagnostics
}

type assetKey struct {
	packageID model.PackageID
	assetID   model.AssetID
}

type assetReference struct {
	packageID model.PackageID
	asset     model.SourceAsset
}

func indexAssets(packages []model.SourcePackage) map[model.AssetID][]assetReference {
	assets := make(map[model.AssetID][]assetReference)
	for _, pkg := range packages {
		for _, asset := range pkg.Assets {
			assets[asset.Identity] = append(assets[asset.Identity], assetReference{packageID: pkg.Identity, asset: asset})
		}
	}
	return assets
}

func resolveNativeGaps(gaps []model.NativeGap, target model.TargetComposition, assets map[model.AssetID][]assetReference) (map[assetKey]bool, []model.Diagnostic) {
	policies := make(map[string]model.NativeGapPolicy, len(target.NativeGaps))
	for _, policy := range target.NativeGaps {
		policies[policy.Component] = policy
	}
	usedPolicies := make(map[string]bool, len(policies))
	excluded := make(map[assetKey]bool)
	var diagnostics []model.Diagnostic
	for _, gap := range gaps {
		if gap.Target != nil && *gap.Target != target.Target {
			if gap.Asset != nil {
				excluded[assetKey{packageID: gap.Package, assetID: *gap.Asset}] = true
			}
			continue
		}
		if gap.Asset != nil {
			if reference, ok := gapAssetReference(gap, assets); ok && nativeAssetSupportedByTarget(reference.asset, target.Target) {
				continue
			}
		}
		if gap.Asset != nil {
			reference, ok := gapAssetReference(gap, assets)
			if !ok {
				usedPolicies[gap.Component] = policyExists(policies, gap.Component)
				diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q references missing asset %q in package %q", gap.Component, *gap.Asset, gap.Package))
				continue
			}
			if !assetSelectedForTarget(reference.asset, target.Target) {
				usedPolicies[gap.Component] = policyExists(policies, gap.Component)
				continue
			}
		}

		policy, ok := policies[gap.Component]
		if !ok {
			diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q has no policy for target %q", gap.Component, target.Target))
			continue
		}
		usedPolicies[gap.Component] = true
		if policy.Action == model.NativeGapActionReplace {
			if policy.Replacement == nil {
				diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q replacement is missing", gap.Component))
			} else if gap.Asset != nil && *policy.Replacement == *gap.Asset {
				diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q cannot replace asset %q with itself", gap.Component, *gap.Asset))
			} else {
				references := assets[*policy.Replacement]
				switch {
				case len(references) == 0:
					diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q replacement asset %q does not exist", gap.Component, *policy.Replacement))
				case len(references) > 1:
					diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q replacement asset %q is ambiguous across packages %s", gap.Component, *policy.Replacement, packageList(references)))
				case !assetSelectedForTarget(references[0].asset, target.Target):
					diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q replacement asset %q is unavailable for target %q", gap.Component, *policy.Replacement, target.Target))
				}
			}
		}
		if gap.Asset != nil && (policy.Action == model.NativeGapActionReplace || policy.Action == model.NativeGapActionExclude || policy.Action == model.NativeGapActionSourceOnly) {
			excluded[assetKey{packageID: gap.Package, assetID: *gap.Asset}] = true
		}
	}
	for _, policy := range target.NativeGaps {
		if !usedPolicies[policy.Component] {
			diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, nil, "native gap policy %q has no matching source gap", policy.Component))
		}
	}
	return excluded, diagnostics
}

func gapAssetReference(gap model.NativeGap, assets map[model.AssetID][]assetReference) (assetReference, bool) {
	if gap.Asset == nil {
		return assetReference{}, false
	}
	for _, reference := range assets[*gap.Asset] {
		if reference.packageID == gap.Package {
			return reference, true
		}
	}
	return assetReference{}, false
}

func nativeAssetSupportedByTarget(asset model.SourceAsset, target model.TargetID) bool {
	if asset.Kind != model.AssetKindNativeResource {
		return false
	}
	switch target {
	case model.TargetPi:
		return asset.Native != nil && len(asset.Native.PiExtensions) != 0
	case model.TargetAntigravity:
		return asset.Native == nil
	default:
		return false
	}
}

func policyExists(policies map[string]model.NativeGapPolicy, component string) bool {
	_, ok := policies[component]
	return ok
}

func packageList(references []assetReference) string {
	packages := make([]string, len(references))
	for index, reference := range references {
		packages[index] = fmt.Sprintf("%q", reference.packageID)
	}
	sort.Strings(packages)
	return strings.Join(packages, ", ")
}

func assetSelectedForTarget(asset model.SourceAsset, target model.TargetID) bool {
	if len(asset.Targets) == 0 {
		return true
	}
	for _, selected := range asset.Targets {
		if selected == target {
			return true
		}
	}
	return false
}

func overlayForTarget(overlays []model.TargetOverlay, target model.TargetID) *model.TargetOverlay {
	for index := range overlays {
		if overlays[index].Target == target {
			return &overlays[index]
		}
	}
	return nil
}

func acknowledgmentsForAsset(overlay *model.TargetOverlay, asset model.AssetID, target model.TargetID) []model.Acknowledgment {
	if overlay == nil {
		return nil
	}
	var acknowledgments []model.Acknowledgment
	for _, acknowledgment := range overlay.Acknowledgments {
		if acknowledgment.Asset == asset && acknowledgment.Target == target {
			acknowledgments = append(acknowledgments, acknowledgment)
		}
	}
	return acknowledgments
}

func hasAcknowledgment(acknowledgments []model.Acknowledgment, key model.CapabilityKey) bool {
	for _, acknowledgment := range acknowledgments {
		if acknowledgment.Key == key {
			return true
		}
	}
	return false
}

func usesCapability(uses []model.CapabilityUse, key model.CapabilityKey) bool {
	for _, use := range uses {
		if use.Key == key {
			return true
		}
	}
	return false
}

func applyOverlay(base model.AssetContent, overlay *model.TargetOverlay, asset model.AssetID) (model.AssetContent, []model.Diagnostic) {
	content := cloneContent(base)
	if overlay == nil {
		return content, nil
	}
	if overlay.FrontmatterPatch != nil {
		applyFrontmatterPatch(content.Frontmatter, *overlay.FrontmatterPatch)
	}
	for _, path := range overlay.DeletedFiles {
		delete(content.Files, path)
	}
	for _, file := range overlay.Files {
		content.Files[file.Path] = cloneFileContent(file.Content)
	}
	if overlay.BodyPatch == nil {
		return content, nil
	}
	body, err := applyBodyPatch(content.Body, *overlay.BodyPatch)
	if err != nil {
		return content, []model.Diagnostic{diagnostic(diagnosticOverlay, nil, "asset %q: %v", asset, err)}
	}
	content.Body = body
	return content, nil
}

func cloneContent(content model.AssetContent) model.AssetContent {
	clone := model.AssetContent{
		Frontmatter: cloneMap(content.Frontmatter),
		Body:        content.Body,
		Files:       make(map[model.RelativePath]model.FileContent, len(content.Files)),
	}
	for path, content := range content.Files {
		clone.Files[path] = cloneFileContent(content)
	}
	return clone
}

func cloneFileContent(content model.FileContent) model.FileContent {
	return model.FileContent{
		Bytes:      append([]byte(nil), content.Bytes...),
		Executable: content.Executable,
		Origin:     model.CloneSourceLocations(content.Origin),
	}
}

func cloneHookDescriptor(descriptor *model.HookDescriptor) *model.HookDescriptor {
	if descriptor == nil {
		return nil
	}
	clone := model.CloneHookDescriptor(*descriptor)
	return &clone
}

func cloneCapabilityUses(uses []model.CapabilityUse) []model.CapabilityUse {
	if uses == nil {
		return nil
	}
	clone := make([]model.CapabilityUse, len(uses))
	for index, use := range uses {
		clone[index] = model.CapabilityUse{Key: use.Key, Location: model.CloneSourceLocation(use.Location)}
	}
	return clone
}

func cloneMap(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneJSONValue(value)
	}
	return clone
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneJSONValue(item)
		}
		return clone
	default:
		return value
	}
}

func applyFrontmatterPatch(base, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(base, key)
			continue
		}
		patchMap, patchIsMap := value.(map[string]any)
		baseMap, baseIsMap := base[key].(map[string]any)
		if patchIsMap && baseIsMap {
			applyFrontmatterPatch(baseMap, patchMap)
			continue
		}
		if patchIsMap {
			base[key] = cloneMap(patchMap)
			continue
		}
		base[key] = cloneJSONValue(value)
	}
}

func applyBodyPatch(body string, patch model.BodyPatch) (string, error) {
	switch patch.Mode {
	case model.BodyModeReplace:
		return *patch.Text, nil
	case model.BodyModeSections:
		return applySectionPatches(body, patch.Sections)
	default:
		return "", fmt.Errorf("unsupported body patch mode %q", patch.Mode)
	}
}

func applySectionPatches(body string, patches []model.SectionPatch) (string, error) {
	lines := strings.SplitAfter(body, "\n")
	type replacement struct {
		start, end int
		body       string
	}
	var replacements []replacement
	for _, patch := range patches {
		start, level, err := findHeading(lines, patch.HeadingPath)
		if err != nil {
			return "", err
		}
		end := len(lines)
		for index := start + 1; index < len(lines); index++ {
			if nextLevel, _, ok := heading(lines[index]); ok && nextLevel <= level {
				end = index
				break
			}
		}
		replacements = append(replacements, replacement{start: start + 1, end: end, body: patch.Body})
	}
	sort.Slice(replacements, func(left, right int) bool { return replacements[left].start > replacements[right].start })
	for index := 1; index < len(replacements); index++ {
		if replacements[index].end > replacements[index-1].start {
			return "", fmt.Errorf("section patches overlap")
		}
	}
	for _, replacement := range replacements {
		insert := splitBody(replacement.body)
		lines = append(lines[:replacement.start], append(insert, lines[replacement.end:]...)...)
	}
	return strings.Join(lines, ""), nil
}

func splitBody(body string) []string {
	if body == "" {
		return nil
	}
	return strings.SplitAfter(body, "\n")
}

func findHeading(lines []string, path []string) (int, int, error) {
	ancestors := make([]string, 7)
	fence := ""
	match := -1
	matchLevel := 0
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			switch fence {
			case "":
				fence = marker
			case marker:
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		level, text, ok := heading(line)
		if !ok {
			continue
		}
		ancestors[level] = text
		for child := level + 1; child < len(ancestors); child++ {
			ancestors[child] = ""
		}
		if level != len(path) {
			continue
		}
		matched := true
		for parent := 1; parent <= level; parent++ {
			if ancestors[parent] != path[parent-1] {
				matched = false
				break
			}
		}
		if matched {
			if match >= 0 {
				return 0, 0, fmt.Errorf("section %q is ambiguous", strings.Join(path, " / "))
			}
			match = index
			matchLevel = level
		}
	}
	if match < 0 {
		return 0, 0, fmt.Errorf("section %q does not exist", strings.Join(path, " / "))
	}
	return match, matchLevel, nil
}

func canonicalizePackages(packages []model.NormalizedPackage) {
	for index := range packages {
		pkg := &packages[index]
		sort.Slice(pkg.Assets, func(i, j int) bool { return pkg.Assets[i].Identity < pkg.Assets[j].Identity })
		sort.Slice(pkg.Acknowledgments, func(i, j int) bool {
			left, right := pkg.Acknowledgments[i], pkg.Acknowledgments[j]
			if left.Asset != right.Asset {
				return left.Asset < right.Asset
			}
			if left.Target != right.Target {
				return left.Target < right.Target
			}
			if left.Key != right.Key {
				return left.Key < right.Key
			}
			return left.Reason < right.Reason
		})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Identity < packages[j].Identity })
}

func heading(line string) (int, string, bool) {
	line = strings.TrimRight(line, "\r\n")
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || len(line) == level || (line[level] != ' ' && line[level] != '\t') {
		return 0, "", false
	}
	text := strings.TrimSpace(line[level:])
	text = strings.TrimSpace(strings.TrimRight(text, "#"))
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}

func joinPreamble(preamble, body string) string {
	if preamble == "" {
		return body
	}
	if body == "" {
		return preamble
	}
	return preamble + "\n\n" + body
}

func invalidInputDiagnostics(diagnostics []model.Diagnostic) []model.Diagnostic {
	result := make([]model.Diagnostic, 0, len(diagnostics))
	for _, item := range diagnostics {
		result = append(result, model.Diagnostic{
			Code:     diagnosticCodeInvalidComposition,
			Severity: item.Severity,
			Location: item.Location,
			Message:  item.Message,
		})
	}
	return result
}

func hasErrors(diagnostics []model.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == model.SeverityError {
			return true
		}
	}
	return false
}

func diagnostic(code string, location *model.SourceLocation, format string, args ...any) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Location: location, Message: fmt.Sprintf(format, args...)}
}
