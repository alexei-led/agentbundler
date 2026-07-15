// Package grok renders normalized packages for Grok Build.
package grok

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetGrok
	FormatRevision = 4
)

// Adapter renders Grok project skills and installable plugins.
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
		plan, diagnostics := packageoutput.RenderWithCodec(input, PackageCodec())
		if len(diagnostics) != 0 {
			return plan, diagnostics
		}
		plan.NativeChecks = nativeChecks(plan.Packages)
		return plan, nil
	}
	return skills.RenderProject(adapter.Target(), ".grok/skills", ".grok/resources", input.Packages)
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

func nativeChecks(packages []model.PackageID) []model.NativeCheck {
	checks := make([]model.NativeCheck, 0, len(packages))
	for _, identity := range packages {
		root := "."
		if len(packages) > 1 {
			root = string(identity)
		}
		checks = append(checks, model.NativeCheck{
			Program:   "grok",
			Arguments: []string{"plugin", "validate", root},
			Location:  model.SourceLocation{Path: "internal/target/grok/codec.go"},
		})
	}
	return checks
}
