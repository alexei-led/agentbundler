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
	if len(packages) == 1 && packages[0].Profile == model.TargetProfilePackage {
		return packageoutput.Render(Target, packages)
	}
	return skills.RenderProject(Target, ".github/skills", ".github/resources", packages)
}
