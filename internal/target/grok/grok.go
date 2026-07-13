// Package grok renders normalized packages for Grok Build.
package grok

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetGrok
	FormatRevision = 2
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateUnsupported},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// Adapter renders Grok's lossless native skill subset.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }
func (adapter Adapter) Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return skills.Render(adapter.Target(), ".grok/skills", packages)
}
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(packages)
}
