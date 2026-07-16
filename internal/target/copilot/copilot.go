// Package copilot renders normalized packages for GitHub Copilot.
package copilot

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetCopilot
	FormatRevision = 7
)

// Capabilities returns Copilot's supported package asset capabilities.
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}

// Render emits either an installable Copilot plugin or project-local skills.
func Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if packagesHaveProfile(input.Packages, model.TargetProfilePackage) {
		return packageoutput.RenderWithCodec(input, PackageCodec())
	}
	return skills.RenderProject(Target, ".github/skills", ".github/resources", input.Packages)
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
