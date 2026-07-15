// Package cursor renders normalized packages as Cursor plugins.
package cursor

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/plugin"
)

const formatRevision = 3

// Adapter describes Cursor's native plugin asset subset.
type Adapter struct {
	Target         model.TargetID
	FormatRevision int
	Capabilities   []model.CapabilityRule
}

func New() Adapter {
	return Adapter{Target: model.TargetCursor, FormatRevision: formatRevision, Capabilities: append([]model.CapabilityRule(nil), capabilityRules...)}
}

func Render(adapter Adapter, input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if adapter.Target != model.TargetCursor || adapter.FormatRevision != formatRevision || !sameCapabilityRules(adapter.Capabilities, capabilityRules) {
		return model.TargetPlan{Target: model.TargetCursor}, []model.Diagnostic{{Code: "invalid-adapter", Severity: model.SeverityError, Message: "adapter is not the Cursor format revision 3 capability profile"}}
	}
	if packagesHaveProfile(input.Packages, model.TargetProfilePackage) {
		return packageoutput.RenderWithCodec(input, PackageCodec())
	}
	if len(input.Packages) != 1 {
		return plugin.Render(adapter.Target, ".cursor-plugin/plugin.json", input.Packages, nil)
	}
	pkg := input.Packages[0]
	manifest := map[string]any{"name": pkg.Identity, "skills": "./skills/"}
	for _, key := range []string{"displayName", "description", "version", "homepage", "repository", "license", "publisher"} {
		if value, ok := pkg.Metadata[key].(string); ok {
			manifest[key] = value
		}
	}
	return plugin.Render(adapter.Target, ".cursor-plugin/plugin.json", input.Packages, manifest)
}

func (adapter Adapter) Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	return Render(adapter, input)
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

func sameCapabilityRules(left, right []model.CapabilityRule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
