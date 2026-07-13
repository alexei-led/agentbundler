// Package codex renders normalized packages as Codex plugins.
package codex

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/plugin"
)

const (
	Target         = model.TargetCodex
	FormatRevision = 2
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateUnsupported},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// Adapter renders Codex's lossless native plugin skill subset.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }
func (adapter Adapter) Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if len(packages) != 1 {
		return plugin.Render(adapter.Target(), ".codex-plugin/plugin.json", packages, nil)
	}
	pkg := packages[0]
	manifest := map[string]any{"name": pkg.Identity, "skills": "./skills"}
	if value, ok := pkg.Metadata["version"].(string); ok {
		manifest["version"] = value
	}
	if value, ok := pkg.Metadata["description"].(string); ok {
		manifest["description"] = value
	}
	return plugin.Render(adapter.Target(), ".codex-plugin/plugin.json", packages, manifest)
}
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(packages)
}
