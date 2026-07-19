// Package pi renders normalized packages for Pi.
package pi

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetPi
	FormatRevision = 8
)

// Adapter renders Pi's lossless native skill subset.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }
func (adapter Adapter) Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if input.PackageMode == model.TargetPackageModeAggregate {
		return renderAggregate(input)
	}
	if packagesHaveProfile(input.Packages, model.TargetProfilePackage) {
		plan, diagnostics := packageoutput.RenderWithCodec(input, PackageCodec())
		if len(diagnostics) != 0 {
			return plan, diagnostics
		}
		return plan, nil
	}
	return skills.Render(adapter.Target(), ".pi/skills", input.Packages)
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
