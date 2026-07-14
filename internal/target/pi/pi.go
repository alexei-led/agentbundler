// Package pi renders normalized packages for Pi.
package pi

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetPi
	FormatRevision = 3
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateEquivalent},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// Adapter renders Pi's lossless native skill subset.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }
func (adapter Adapter) Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if packagesHaveProfile(packages, model.TargetProfilePackage) {
		return packageoutput.Render(adapter.Target(), packages)
	}
	return skills.Render(adapter.Target(), ".pi/skills", packages)
}
func Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(packages)
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
