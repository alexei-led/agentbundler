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

	assets := make(map[model.AssetID]model.SourceAsset)
	for _, pkg := range inventory.Packages {
		for _, asset := range pkg.Assets {
			assets[asset.Identity] = asset
		}
	}

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
			Metadata: sourcePackage.Metadata,
			Target:   target.Target,
			Profile:  profile,
		}
		for _, sourceAsset := range sourcePackage.Assets {
			if !assetSelectedForTarget(sourceAsset, target.Target) {
				continue
			}
			if gapActions[sourceAsset.Identity] {
				continue
			}

			overlay := overlayForTarget(sourceAsset.Overlays, target.Target)
			content, overlayDiagnostics := applyOverlay(sourceAsset.Base, overlay, sourceAsset.Identity)
			diagnostics = append(diagnostics, overlayDiagnostics...)
			if target.SkillPreamble != nil && sourceAsset.Kind == model.AssetKindSkill {
				content.Body = joinPreamble(*target.SkillPreamble, content.Body)
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
				CapabilityUses: sourceAsset.CapabilityUses,
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

func resolveNativeGaps(gaps []model.NativeGap, target model.TargetComposition, assets map[model.AssetID]model.SourceAsset) (map[model.AssetID]bool, []model.Diagnostic) {
	policies := make(map[string]model.NativeGapPolicy, len(target.NativeGaps))
	for _, policy := range target.NativeGaps {
		policies[policy.Component] = policy
	}
	usedPolicies := make(map[string]bool, len(policies))
	excluded := make(map[model.AssetID]bool)
	var diagnostics []model.Diagnostic
	for _, gap := range gaps {
		if gap.Target != nil && *gap.Target != target.Target {
			continue
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
			} else if replacement, ok := assets[*policy.Replacement]; !ok {
				diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q replacement asset %q does not exist", gap.Component, *policy.Replacement))
			} else if !assetSelectedForTarget(replacement, target.Target) {
				diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, &gap.Location, "native gap %q replacement asset %q is unavailable for target %q", gap.Component, *policy.Replacement, target.Target))
			}
		}
		if gap.Asset != nil && (policy.Action == model.NativeGapActionReplace || policy.Action == model.NativeGapActionExclude || policy.Action == model.NativeGapActionSourceOnly) {
			excluded[*gap.Asset] = true
		}
	}
	for _, policy := range target.NativeGaps {
		if !usedPolicies[policy.Component] {
			diagnostics = append(diagnostics, diagnostic(diagnosticNativeGap, nil, "native gap policy %q has no matching source gap", policy.Component))
		}
	}
	return excluded, diagnostics
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
		content.Files[file.Path] = append([]byte(nil), file.Bytes...)
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
		Files:       make(map[model.RelativePath][]byte, len(content.Files)),
	}
	for path, bytes := range content.Files {
		clone.Files[path] = append([]byte(nil), bytes...)
	}
	return clone
}

func cloneMap(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		if nested, ok := value.(map[string]any); ok {
			clone[key] = cloneMap(nested)
			continue
		}
		clone[key] = value
	}
	return clone
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
		base[key] = value
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
