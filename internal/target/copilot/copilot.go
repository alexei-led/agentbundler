// Package copilot renders normalized packages for GitHub Copilot.
package copilot

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetCopilot
	FormatRevision = 4
)

// Capabilities returns Copilot's supported package asset capabilities.
func Capabilities() []model.CapabilityRule {
	return []model.CapabilityRule{
		{Key: "asset.agent", State: model.CapabilityStateNative},
		{Key: "asset.hook", State: model.CapabilityStateUnsupported},
		{Key: "asset.resource", State: model.CapabilityStateNative},
		{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
		{Key: "asset.skill", State: model.CapabilityStateNative},
	}
}

// Render emits either an installable Copilot plugin or project-local skills.
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if packagesHaveProfile(packages, model.TargetProfilePackage) {
		return packageoutput.Render(Target, packages)
	}
	return skills.RenderProject(Target, ".github/skills", ".github/resources", packages)
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
