// Package copilot renders normalized packages for GitHub Copilot.
package copilot

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetCopilot
	FormatRevision = 2
)

// Capabilities returns Copilot's lossless native skill subset.
func Capabilities() []model.CapabilityRule {
	return []model.CapabilityRule{
		{Key: "asset.agent", State: model.CapabilityStateUnsupported},
		{Key: "asset.hook", State: model.CapabilityStateUnsupported},
		{Key: "asset.resource", State: model.CapabilityStateUnsupported},
		{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
		{Key: "asset.skill", State: model.CapabilityStateNative},
	}
}

// Render emits skills in Copilot's project skill root.
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return skills.Render(Target, ".github/skills", packages)
}
