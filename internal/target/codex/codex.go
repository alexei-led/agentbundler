// Package codex renders normalized packages as Codex plugins.
package codex

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/plugin"
)

const (
	Target         = model.TargetCodex
	FormatRevision = 2
)

// Adapter renders Codex's lossless native plugin skill subset.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }
func (adapter Adapter) Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if packagesHaveProfile(input.Packages, model.TargetProfilePackage) {
		return packageoutput.RenderWithCodec(input, PackageCodec())
	}
	if len(input.Packages) != 1 {
		return plugin.Render(adapter.Target(), ".codex-plugin/plugin.json", input.Packages, nil)
	}
	pkg := input.Packages[0]
	manifest := map[string]any{"name": pkg.Identity, "skills": "./skills"}
	if value, ok := pkg.Metadata["version"].(string); ok {
		manifest["version"] = value
	}
	if value, ok := pkg.Metadata["description"].(string); ok {
		manifest["description"] = value
	}
	return plugin.Render(adapter.Target(), ".codex-plugin/plugin.json", input.Packages, manifest)
}
func Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(input)
}

func packagesHaveProfile(packages []model.NormalizedPackage, profile model.TargetProfile) bool {
	if len(packages) == 0 {
		return false
	}
	for _, pkg := range packages {
		if pkg.Profile != profile {
			return false
		}
	}
	return true
}
