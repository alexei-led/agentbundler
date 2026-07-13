// Package cursor renders normalized packages as Cursor plugins.
package cursor

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/plugin"
)

const formatRevision = 2

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "asset.agent", State: model.CapabilityStateUnsupported},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
}

// Adapter describes Cursor's lossless native plugin skill subset.
type Adapter struct {
	Target         model.TargetID
	FormatRevision int
	Capabilities   []model.CapabilityRule
}

func New() Adapter {
	return Adapter{Target: model.TargetCursor, FormatRevision: formatRevision, Capabilities: append([]model.CapabilityRule(nil), capabilityRules...)}
}

func Render(adapter Adapter, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if adapter.Target != model.TargetCursor || adapter.FormatRevision != formatRevision || !sameCapabilityRules(adapter.Capabilities, capabilityRules) {
		return model.TargetPlan{Target: model.TargetCursor}, []model.Diagnostic{{Code: "invalid-adapter", Severity: model.SeverityError, Message: "adapter is not the Cursor format revision 2 capability profile"}}
	}
	if len(packages) != 1 {
		return plugin.Render(adapter.Target, ".cursor-plugin/plugin.json", packages, nil)
	}
	pkg := packages[0]
	manifest := map[string]any{"name": pkg.Identity, "skills": "./skills/"}
	for _, key := range []string{"displayName", "description", "version", "homepage", "repository", "license", "publisher"} {
		if value, ok := pkg.Metadata[key].(string); ok {
			manifest[key] = value
		}
	}
	return plugin.Render(adapter.Target, ".cursor-plugin/plugin.json", packages, manifest)
}

func (adapter Adapter) Render(packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return Render(adapter, packages)
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
