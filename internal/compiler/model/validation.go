package model

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const diagnosticCodeInvalidModel = "invalid-model"

// NewRelativePath validates value as a normalized path below its declared root.
func NewRelativePath(value string) (RelativePath, error) {
	if err := validateRelativePath(value); err != nil {
		return "", err
	}
	return RelativePath(value), nil
}

// NewPackageID validates value as a stable package identity.
func NewPackageID(value string) (PackageID, error) {
	if err := validateIdentifier(value, "package ID"); err != nil {
		return "", err
	}
	return PackageID(value), nil
}

// NewAssetID validates value as a kind/name asset identity.
func NewAssetID(value string) (AssetID, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("asset ID must have kind/name form")
	}
	if !validAssetKind(AssetKind(parts[0])) {
		return "", fmt.Errorf("asset ID has invalid kind %q", parts[0])
	}
	if err := validateIdentifier(parts[1], "asset name"); err != nil {
		return "", err
	}
	return AssetID(value), nil
}

// NewCapabilityKey validates value as a canonical capability identifier.
func NewCapabilityKey(value string) (CapabilityKey, error) {
	if err := validateIdentifier(value, "capability key"); err != nil {
		return "", err
	}
	return CapabilityKey(value), nil
}

// ValidateSourceManifest validates a source declaration without accessing the filesystem.
func ValidateSourceManifest(manifest SourceManifest) []Diagnostic {
	var diagnostics []Diagnostic
	if !validSourceKind(manifest.Kind) {
		diagnostics = appendInvalid(diagnostics, "manifest kind is invalid")
	}
	if err := validateRelativePath(string(manifest.Root)); err != nil {
		diagnostics = appendInvalid(diagnostics, "manifest root: "+err.Error())
	}
	if err := validateRelativePath(string(manifest.Output)); err != nil {
		diagnostics = appendInvalid(diagnostics, "manifest output: "+err.Error())
	}
	if len(manifest.Targets) == 0 {
		diagnostics = appendInvalid(diagnostics, "manifest targets must not be empty")
	}
	targets := make(map[TargetID]struct{}, len(manifest.Targets))
	for _, target := range manifest.Targets {
		if !validTargetID(target) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("manifest target %q is invalid", target))
		}
		if _, ok := targets[target]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("manifest target %q is duplicated", target))
		}
		targets[target] = struct{}{}
	}
	compositions := make(map[TargetID]struct{}, len(manifest.Composition))
	for _, composition := range manifest.Composition {
		diagnostics = append(diagnostics, ValidateTargetComposition(composition)...)
		if _, ok := compositions[composition.Target]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target composition %q is duplicated", composition.Target))
		}
		compositions[composition.Target] = struct{}{}
		if _, ok := targets[composition.Target]; !ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target composition %q is not selected", composition.Target))
		}
	}

	switch manifest.Kind {
	case SourceKindBundle:
		if manifest.Bundle == nil || manifest.ClaudePlugin != nil || manifest.SkillsRepository != nil {
			diagnostics = appendInvalid(diagnostics, "bundle manifest must contain only bundle configuration")
		} else {
			diagnostics = append(diagnostics, validateBundleSourceConfig(*manifest.Bundle)...)
		}
	case SourceKindClaudePlugin:
		if manifest.Bundle != nil || manifest.ClaudePlugin == nil || manifest.SkillsRepository != nil {
			diagnostics = appendInvalid(diagnostics, "claude-plugin manifest must contain only claudePlugin configuration")
		} else if err := validateRelativePath(string(manifest.ClaudePlugin.PluginRoot)); err != nil {
			diagnostics = appendInvalid(diagnostics, "claudePlugin pluginRoot: "+err.Error())
		}
	case SourceKindSkillsRepository:
		if manifest.Bundle != nil || manifest.ClaudePlugin != nil || manifest.SkillsRepository == nil {
			diagnostics = appendInvalid(diagnostics, "skills-repository manifest must contain only skillsRepository configuration")
		} else {
			diagnostics = append(diagnostics, validateSkillsRepositorySourceConfig(*manifest.SkillsRepository)...)
		}
	}
	return diagnostics
}

// ValidateSourceInventory validates discovered source values without accessing the filesystem.
func ValidateSourceInventory(inventory SourceInventory) []Diagnostic {
	var diagnostics []Diagnostic
	packages := make(map[PackageID]struct{}, len(inventory.Packages))
	for _, pkg := range inventory.Packages {
		if err := validateIdentifier(string(pkg.Identity), "package ID"); err != nil {
			diagnostics = appendInvalid(diagnostics, "source package: "+err.Error())
		}
		if _, ok := packages[pkg.Identity]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("source package %q is duplicated", pkg.Identity))
		}
		packages[pkg.Identity] = struct{}{}
		if err := validateJSONValue(pkg.Metadata); err != nil {
			diagnostics = appendInvalid(diagnostics, "source package metadata: "+err.Error())
		}
		assets := make(map[AssetID]struct{}, len(pkg.Assets))
		for _, asset := range pkg.Assets {
			diagnostics = append(diagnostics, validateSourceAsset(asset)...)
			if _, ok := assets[asset.Identity]; ok {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("source asset %q is duplicated", asset.Identity))
			}
			assets[asset.Identity] = struct{}{}
		}
	}
	for _, gap := range inventory.NativeGaps {
		diagnostics = append(diagnostics, validateNativeGap(gap)...)
	}
	inputs := make(map[RelativePath]struct{}, len(inventory.Inputs))
	for _, input := range inventory.Inputs {
		if err := validateRelativePath(string(input.Path)); err != nil {
			diagnostics = appendInvalid(diagnostics, "input path: "+err.Error())
		}
		if _, ok := inputs[input.Path]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("input path %q is duplicated", input.Path))
		}
		inputs[input.Path] = struct{}{}
		if len(input.SHA256) != 64 || !isLowerHex(input.SHA256) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("input %q has invalid SHA-256", input.Path))
		}
	}
	return diagnostics
}

// ValidateTargetComposition validates target-specific composition policy.
func ValidateTargetComposition(input TargetComposition) []Diagnostic {
	var diagnostics []Diagnostic
	if !validTargetID(input.Target) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("composition target %q is invalid", input.Target))
	}
	capabilities := make(map[CapabilityKey]struct{}, len(input.Capabilities))
	for _, rule := range input.Capabilities {
		if err := validateIdentifier(string(rule.Key), "capability key"); err != nil {
			diagnostics = appendInvalid(diagnostics, "capability rule: "+err.Error())
		}
		if !validCapabilityState(rule.State) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("capability %q has invalid state %q", rule.Key, rule.State))
		}
		if rule.State == CapabilityStateUnsupported {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("capability %q is unsupported for target %q", rule.Key, input.Target))
		}
		if _, ok := capabilities[rule.Key]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("capability %q is duplicated", rule.Key))
		}
		capabilities[rule.Key] = struct{}{}
	}
	components := make(map[string]struct{}, len(input.NativeGaps))
	for _, policy := range input.NativeGaps {
		if err := validateComponent(policy.Component); err != nil {
			diagnostics = appendInvalid(diagnostics, "native gap policy: "+err.Error())
		}
		if !validNativeGapAction(policy.Action) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("native gap policy %q has invalid action %q", policy.Component, policy.Action))
		}
		if policy.Action == NativeGapActionReplace {
			if policy.Replacement == nil {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("native gap policy %q requires a replacement", policy.Component))
			} else if _, err := NewAssetID(string(*policy.Replacement)); err != nil {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("native gap policy %q replacement: %v", policy.Component, err))
			}
		} else if policy.Replacement != nil {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("native gap policy %q forbids a replacement", policy.Component))
		}
		if _, ok := components[policy.Component]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("native gap policy %q is duplicated", policy.Component))
		}
		components[policy.Component] = struct{}{}
	}
	return diagnostics
}

// ValidateNormalizedPackage validates a composed package before an adapter consumes it.
func ValidateNormalizedPackage(pkg NormalizedPackage) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateIdentifier(string(pkg.Identity), "package ID"); err != nil {
		diagnostics = appendInvalid(diagnostics, "normalized package: "+err.Error())
	}
	if !validTargetID(pkg.Target) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized package target %q is invalid", pkg.Target))
	}
	if err := validateJSONValue(pkg.Metadata); err != nil {
		diagnostics = appendInvalid(diagnostics, "normalized package metadata: "+err.Error())
	}
	assets := make(map[AssetID]struct{}, len(pkg.Assets))
	for _, asset := range pkg.Assets {
		diagnostics = append(diagnostics, validateNormalizedAsset(asset)...)
		if _, ok := assets[asset.Identity]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized asset %q is duplicated", asset.Identity))
		}
		assets[asset.Identity] = struct{}{}
	}
	for _, acknowledgment := range pkg.Acknowledgments {
		diagnostics = append(diagnostics, validateAcknowledgment(acknowledgment)...)
		if acknowledgment.Target != pkg.Target {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("acknowledgment for asset %q has target %q, not package target %q", acknowledgment.Asset, acknowledgment.Target, pkg.Target))
		}
		if _, ok := assets[acknowledgment.Asset]; !ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("acknowledgment references unknown asset %q", acknowledgment.Asset))
		}
	}
	return diagnostics
}

// ValidateBuildPlan validates the complete artifact transaction without writing files or running checks.
func ValidateBuildPlan(plan BuildPlan) []Diagnostic {
	var diagnostics []Diagnostic
	targets := make(map[TargetID]struct{}, len(plan.Targets))
	destinations := make(map[string]struct{})
	for _, targetPlan := range plan.Targets {
		if !validTargetID(targetPlan.Target) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target plan target %q is invalid", targetPlan.Target))
		}
		if _, ok := targets[targetPlan.Target]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target plan %q is duplicated", targetPlan.Target))
		}
		targets[targetPlan.Target] = struct{}{}
		packages := make(map[PackageID]struct{}, len(targetPlan.Packages))
		for _, packageID := range targetPlan.Packages {
			if err := validateIdentifier(string(packageID), "package ID"); err != nil {
				diagnostics = appendInvalid(diagnostics, "target plan: "+err.Error())
			}
			if _, ok := packages[packageID]; ok {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target plan package %q is duplicated", packageID))
			}
			packages[packageID] = struct{}{}
		}
		for _, file := range targetPlan.Files {
			diagnostics = append(diagnostics, validatePlannedFile(file)...)
			diagnostics = appendDestination(diagnostics, destinations, string(targetPlan.Target)+"/"+string(file.Path))
		}
		for _, check := range targetPlan.NativeChecks {
			diagnostics = append(diagnostics, validateNativeCheck(check)...)
		}
	}
	for _, file := range plan.CompilerFiles {
		diagnostics = append(diagnostics, validatePlannedFile(file)...)
		diagnostics = appendDestination(diagnostics, destinations, string(file.Path))
	}
	return diagnostics
}

func validateBundleSourceConfig(config BundleSourceConfig) []Diagnostic {
	var diagnostics []Diagnostic
	if len(config.Packages) == 0 {
		diagnostics = appendInvalid(diagnostics, "bundle packages must not be empty")
	}
	paths := make(map[RelativePath]struct{}, len(config.Packages))
	for _, packagePath := range config.Packages {
		if err := validateRelativePath(string(packagePath)); err != nil {
			diagnostics = appendInvalid(diagnostics, "bundle package path: "+err.Error())
		}
		if _, ok := paths[packagePath]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("bundle package path %q is duplicated", packagePath))
		}
		paths[packagePath] = struct{}{}
	}
	return diagnostics
}

func validateSkillsRepositorySourceConfig(config SkillsRepositorySourceConfig) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateIdentifier(string(config.Package), "package ID"); err != nil {
		diagnostics = appendInvalid(diagnostics, "skillsRepository package: "+err.Error())
	}
	if len(config.Roots) == 0 {
		diagnostics = appendInvalid(diagnostics, "skillsRepository roots must not be empty")
	}
	roots := make(map[RelativePath]struct{}, len(config.Roots))
	for _, root := range config.Roots {
		if err := validateRelativePath(string(root)); err != nil {
			diagnostics = appendInvalid(diagnostics, "skillsRepository root: "+err.Error())
		}
		if _, ok := roots[root]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("skillsRepository root %q is duplicated", root))
		}
		roots[root] = struct{}{}
	}
	if err := validateJSONValue(config.Metadata); err != nil {
		diagnostics = appendInvalid(diagnostics, "skillsRepository metadata: "+err.Error())
	}
	return diagnostics
}

func validateSourceAsset(asset SourceAsset) []Diagnostic {
	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, validateAssetIdentity(asset.Identity, asset.Kind)...)
	diagnostics = append(diagnostics, validateAssetContent(asset.Base)...)
	overlays := make(map[TargetID]struct{}, len(asset.Overlays))
	for _, overlay := range asset.Overlays {
		if !validTargetID(overlay.Target) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay target %q is invalid", overlay.Target))
		}
		if _, ok := overlays[overlay.Target]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay target %q is duplicated", overlay.Target))
		}
		overlays[overlay.Target] = struct{}{}
		if overlay.FrontmatterPatch != nil {
			if err := validateJSONValue(*overlay.FrontmatterPatch); err != nil {
				diagnostics = appendInvalid(diagnostics, "overlay frontmatter patch: "+err.Error())
			}
		}
		if overlay.BodyPatch != nil {
			diagnostics = append(diagnostics, validateBodyPatch(*overlay.BodyPatch)...)
		}
		files := make(map[RelativePath]struct{}, len(overlay.Files))
		for _, file := range overlay.Files {
			if err := validateRelativePath(string(file.Path)); err != nil {
				diagnostics = appendInvalid(diagnostics, "overlay file path: "+err.Error())
			}
			if _, ok := files[file.Path]; ok {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay file path %q is duplicated", file.Path))
			}
			files[file.Path] = struct{}{}
		}
		deleted := make(map[RelativePath]struct{}, len(overlay.DeletedFiles))
		for _, deletedPath := range overlay.DeletedFiles {
			if err := validateRelativePath(string(deletedPath)); err != nil {
				diagnostics = appendInvalid(diagnostics, "overlay deleted file path: "+err.Error())
			}
			if _, ok := deleted[deletedPath]; ok {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay deleted file path %q is duplicated", deletedPath))
			}
			if _, ok := files[deletedPath]; ok {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay file path %q is both replaced and deleted", deletedPath))
			}
			deleted[deletedPath] = struct{}{}
		}
		for _, acknowledgment := range overlay.Acknowledgments {
			diagnostics = append(diagnostics, validateAcknowledgment(acknowledgment)...)
			if acknowledgment.Asset != asset.Identity {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay acknowledgment asset %q does not match %q", acknowledgment.Asset, asset.Identity))
			}
			if acknowledgment.Target != overlay.Target {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("overlay acknowledgment target %q does not match %q", acknowledgment.Target, overlay.Target))
			}
		}
	}
	return diagnostics
}

func validateNormalizedAsset(asset NormalizedAsset) []Diagnostic {
	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, validateAssetIdentity(asset.Identity, asset.Kind)...)
	diagnostics = append(diagnostics, validateAssetContent(asset.Content)...)
	for _, capability := range asset.CapabilityUses {
		if err := validateIdentifier(string(capability.Key), "capability key"); err != nil {
			diagnostics = appendInvalid(diagnostics, "capability use: "+err.Error())
		}
		diagnostics = append(diagnostics, validateSourceLocation(capability.Location)...)
	}
	return diagnostics
}

func validateAssetIdentity(identity AssetID, kind AssetKind) []Diagnostic {
	var diagnostics []Diagnostic
	if _, err := NewAssetID(string(identity)); err != nil {
		diagnostics = appendInvalid(diagnostics, "asset identity: "+err.Error())
	}
	if !validAssetKind(kind) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("asset kind %q is invalid", kind))
	}
	parts := strings.Split(string(identity), "/")
	if len(parts) == 2 && string(kind) != parts[0] {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("asset identity %q does not match kind %q", identity, kind))
	}
	return diagnostics
}

func validateAssetContent(content AssetContent) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateJSONValue(content.Frontmatter); err != nil {
		diagnostics = appendInvalid(diagnostics, "asset frontmatter: "+err.Error())
	}
	filePaths := make([]string, 0, len(content.Files))
	for filePath := range content.Files {
		filePaths = append(filePaths, string(filePath))
	}
	sort.Strings(filePaths)
	for _, filePath := range filePaths {
		if err := validateRelativePath(filePath); err != nil {
			diagnostics = appendInvalid(diagnostics, "asset file path: "+err.Error())
		}
	}
	return diagnostics
}

func validateBodyPatch(patch BodyPatch) []Diagnostic {
	var diagnostics []Diagnostic
	switch patch.Mode {
	case BodyModeReplace:
		if patch.Text == nil {
			diagnostics = appendInvalid(diagnostics, "replace body patch requires text")
		}
		if len(patch.Sections) != 0 {
			diagnostics = appendInvalid(diagnostics, "replace body patch forbids sections")
		}
	case BodyModeSections:
		if patch.Text != nil {
			diagnostics = appendInvalid(diagnostics, "sections body patch forbids text")
		}
		if len(patch.Sections) == 0 {
			diagnostics = appendInvalid(diagnostics, "sections body patch requires sections")
		}
		paths := make(map[string]struct{}, len(patch.Sections))
		for _, section := range patch.Sections {
			if len(section.HeadingPath) == 0 {
				diagnostics = appendInvalid(diagnostics, "section heading path must not be empty")
			}
			for _, heading := range section.HeadingPath {
				if strings.TrimSpace(heading) == "" || strings.ContainsRune(heading, '\x00') {
					diagnostics = appendInvalid(diagnostics, "section heading must not be empty or contain NUL")
				}
			}
			key := strings.Join(section.HeadingPath, "\x00")
			if _, ok := paths[key]; ok {
				diagnostics = appendInvalid(diagnostics, "section heading path is duplicated")
			}
			paths[key] = struct{}{}
		}
	default:
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("body patch mode %q is invalid", patch.Mode))
	}
	return diagnostics
}

func validateNativeGap(gap NativeGap) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateComponent(gap.Component); err != nil {
		diagnostics = appendInvalid(diagnostics, "native gap: "+err.Error())
	}
	diagnostics = append(diagnostics, validateSourceLocation(gap.Location)...)
	if gap.Target != nil && !validTargetID(*gap.Target) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("native gap target %q is invalid", *gap.Target))
	}
	return diagnostics
}

func validateAcknowledgment(acknowledgment Acknowledgment) []Diagnostic {
	var diagnostics []Diagnostic
	if _, err := NewAssetID(string(acknowledgment.Asset)); err != nil {
		diagnostics = appendInvalid(diagnostics, "acknowledgment asset: "+err.Error())
	}
	if !validTargetID(acknowledgment.Target) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("acknowledgment target %q is invalid", acknowledgment.Target))
	}
	if err := validateIdentifier(string(acknowledgment.Key), "capability key"); err != nil {
		diagnostics = appendInvalid(diagnostics, "acknowledgment: "+err.Error())
	}
	if strings.TrimSpace(acknowledgment.Reason) == "" || strings.ContainsRune(acknowledgment.Reason, '\x00') {
		diagnostics = appendInvalid(diagnostics, "acknowledgment reason must not be empty or contain NUL")
	}
	return diagnostics
}

func validatePlannedFile(file PlannedFile) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateRelativePath(string(file.Path)); err != nil {
		diagnostics = appendInvalid(diagnostics, "planned file path: "+err.Error())
	}
	for _, origin := range file.Origin {
		diagnostics = append(diagnostics, validateSourceLocation(origin)...)
	}
	return diagnostics
}

func validateNativeCheck(check NativeCheck) []Diagnostic {
	var diagnostics []Diagnostic
	if check.Program == "" || strings.ContainsAny(check.Program, "\x00/\\") {
		diagnostics = appendInvalid(diagnostics, "native check program must be a non-empty executable name")
	}
	for _, argument := range check.Arguments {
		if strings.ContainsRune(argument, '\x00') {
			diagnostics = appendInvalid(diagnostics, "native check argument contains NUL")
		}
	}
	if check.WorkingDirectory != nil {
		if err := validateRelativePath(string(*check.WorkingDirectory)); err != nil {
			diagnostics = appendInvalid(diagnostics, "native check working directory: "+err.Error())
		}
	}
	diagnostics = append(diagnostics, validateSourceLocation(check.Location)...)
	return diagnostics
}

func validateSourceLocation(location SourceLocation) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateRelativePath(string(location.Path)); err != nil {
		diagnostics = appendInvalid(diagnostics, "source location path: "+err.Error())
	}
	if location.Line != nil && *location.Line < 1 {
		diagnostics = appendInvalid(diagnostics, "source location line must be positive")
	}
	if location.Column != nil && *location.Column < 1 {
		diagnostics = appendInvalid(diagnostics, "source location column must be positive")
	}
	return diagnostics
}

func appendDestination(diagnostics []Diagnostic, destinations map[string]struct{}, destination string) []Diagnostic {
	if _, ok := destinations[destination]; ok {
		return appendInvalid(diagnostics, fmt.Sprintf("planned destination %q is duplicated", destination))
	}
	destinations[destination] = struct{}{}
	return diagnostics
}

func appendInvalid(diagnostics []Diagnostic, message string) []Diagnostic {
	return append(diagnostics, Diagnostic{Code: diagnosticCodeInvalidModel, Severity: SeverityError, Message: message})
}

func validateRelativePath(value string) error {
	if value == "" {
		return fmt.Errorf("relative path must not be empty")
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("relative path must be valid UTF-8")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("relative path contains NUL")
	}
	if strings.Contains(value, "\\") || path.IsAbs(value) || strings.HasPrefix(value, "/") {
		return fmt.Errorf("relative path must not be absolute or use backslashes")
	}
	if strings.Contains(value, ":") {
		return fmt.Errorf("relative path must not contain a volume separator")
	}
	if path.Clean(value) != value {
		return fmt.Errorf("relative path must be normalized")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("relative path contains an empty or escaping segment")
		}
	}
	return nil
}

func validateIdentifier(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if strings.ContainsAny(value, "\x00/\\") || strings.TrimSpace(value) != value || value == "." || value == ".." {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func validateComponent(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("component must not be empty or contain NUL")
	}
	return nil
}

func validateJSONValue(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("must contain JSON values: %w", err)
	}
	if !json.Valid(data) {
		return fmt.Errorf("must contain JSON values")
	}
	return nil
}

func isLowerHex(value string) bool {
	if strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validSourceKind(kind SourceKind) bool {
	return kind == SourceKindBundle || kind == SourceKindClaudePlugin || kind == SourceKindSkillsRepository
}

func validTargetID(target TargetID) bool {
	switch target {
	case TargetClaude, TargetCodex, TargetPi, TargetCopilot, TargetGrok, TargetCursor:
		return true
	default:
		return false
	}
}

func validAssetKind(kind AssetKind) bool {
	switch kind {
	case AssetKindSkill, AssetKindAgent, AssetKindHook, AssetKindNativeResource:
		return true
	default:
		return false
	}
}

func validCapabilityState(state CapabilityState) bool {
	switch state {
	case CapabilityStateNative, CapabilityStateEquivalent, CapabilityStateAdvisory, CapabilityStateUnsupported:
		return true
	default:
		return false
	}
}

func validNativeGapAction(action NativeGapAction) bool {
	return action == NativeGapActionReplace || action == NativeGapActionExclude || action == NativeGapActionSourceOnly
}
