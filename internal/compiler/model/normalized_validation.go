package model

import "fmt"

// ValidateNormalizedPackage validates a composed package before an adapter consumes it.
func ValidateNormalizedPackage(pkg NormalizedPackage) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateIdentifier(string(pkg.Identity), "package ID"); err != nil {
		diagnostics = appendInvalid(diagnostics, "normalized package: "+err.Error())
	}
	if !validTargetID(pkg.Target) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized package target %q is invalid", pkg.Target))
	}
	if pkg.Profile != "" && pkg.Profile != TargetProfileProject && pkg.Profile != TargetProfilePackage {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized package profile %q is invalid", pkg.Profile))
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
