// Package target resolves and dispatches built-in target adapters.
package target

import (
	"fmt"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/agentplugins"
	"github.com/alexei-led/agentbundler/internal/target/antigravity"
	"github.com/alexei-led/agentbundler/internal/target/claude"
	"github.com/alexei-led/agentbundler/internal/target/codex"
	"github.com/alexei-led/agentbundler/internal/target/copilot"
	"github.com/alexei-led/agentbundler/internal/target/cursor"
	"github.com/alexei-led/agentbundler/internal/target/grok"
	"github.com/alexei-led/agentbundler/internal/target/pi"
)

// Adapter describes one built-in target format and its renderer.
type Adapter struct {
	Target         model.TargetID
	FormatRevision int
	Capabilities   []model.CapabilityRule

	render func(model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic)
}

type registry struct {
	adapters map[model.TargetID]Adapter
}

var builtInRegistry = mustNewRegistry(
	fromLeaf(agentplugins.New()),
	fromLeaf(antigravity.New()),
	fromLeaf(claude.New()),
	fromLeaf(codex.New()),
	Adapter{
		Target:         copilot.Target,
		FormatRevision: copilot.FormatRevision,
		Capabilities:   copilot.Capabilities(),
		render:         copilot.Render,
	},
	Adapter{
		Target:         cursor.New().Target,
		FormatRevision: cursor.New().FormatRevision,
		Capabilities:   cursor.New().Capabilities,
		render:         cursor.New().Render,
	},
	fromLeaf(grok.New()),
	fromLeaf(pi.New()),
)

type leafAdapter interface {
	Target() model.TargetID
	FormatRevision() int
	Capabilities() []model.CapabilityRule
	Render(model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic)
}

func fromLeaf(leaf leafAdapter) Adapter {
	return Adapter{
		Target:         leaf.Target(),
		FormatRevision: leaf.FormatRevision(),
		Capabilities:   leaf.Capabilities(),
		render:         leaf.Render,
	}
}

func mustNewRegistry(adapters ...Adapter) registry {
	result, diagnostics := newRegistry(adapters...)
	if len(diagnostics) != 0 {
		panic(diagnostics[0].Message)
	}
	return result
}

func newRegistry(adapters ...Adapter) (registry, []model.Diagnostic) {
	result := registry{adapters: make(map[model.TargetID]Adapter, len(adapters))}
	var diagnostics []model.Diagnostic
	for _, adapter := range adapters {
		if adapter.Target == "" {
			diagnostics = append(diagnostics, registryDiagnostic("invalid-adapter", "adapter target must not be empty"))
			continue
		}
		if adapter.FormatRevision < 1 {
			diagnostics = append(diagnostics, registryDiagnostic("invalid-adapter", fmt.Sprintf("adapter %q format revision must be positive", adapter.Target)))
			continue
		}
		if adapter.render == nil {
			diagnostics = append(diagnostics, registryDiagnostic("invalid-adapter", fmt.Sprintf("adapter %q has no renderer", adapter.Target)))
			continue
		}
		if _, exists := result.adapters[adapter.Target]; exists {
			diagnostics = append(diagnostics, registryDiagnostic("duplicate-adapter", fmt.Sprintf("adapter target %q is duplicated", adapter.Target)))
			continue
		}
		adapter.Capabilities = append([]model.CapabilityRule(nil), adapter.Capabilities...)
		result.adapters[adapter.Target] = adapter
	}
	if len(diagnostics) != 0 {
		return registry{}, diagnostics
	}
	return result, nil
}

// portableCapabilityCatalog lists all portable capability keys that every
// registered adapter must declare. Missing keys are normalized to unsupported.
// The catalog grows only when a new portable capability contract is ratified.
var portableCapabilityCatalog = []model.CapabilityKey{
	model.CapabilityKeyAgentPluginSkills,
	model.CapabilityKeyAgentPluginMCPStdio,
	model.CapabilityKeyAgentPluginMCPStreamableHTTP,
	model.CapabilityKeyAgentPluginMCPSSE,
	model.CapabilityKeyAgentPluginExtensions,
	model.CapabilityKeyAgentPluginUnknownJSON,
	model.CapabilityKeyAgentPluginPackageFiles,
}

// normalizeCapabilities fills in any missing portable capability keys as
// unsupported, returning a new slice. Existing rules are preserved as-is.
func normalizeCapabilities(rules []model.CapabilityRule) []model.CapabilityRule {
	declared := make(map[model.CapabilityKey]bool, len(rules))
	for _, r := range rules {
		declared[r.Key] = true
	}
	result := append([]model.CapabilityRule(nil), rules...)
	for _, key := range portableCapabilityCatalog {
		if !declared[key] {
			result = append(result, model.CapabilityRule{
				Key:   key,
				State: model.CapabilityStateUnsupported,
			})
		}
	}
	return result
}

// Resolve returns the built-in adapter for target.
func Resolve(target model.TargetID) (Adapter, []model.Diagnostic) {
	adapter, exists := builtInRegistry.adapters[target]
	if !exists {
		return Adapter{}, []model.Diagnostic{registryDiagnostic("unknown-target", fmt.Sprintf("target %q is not registered", target))}
	}
	cloned := cloneAdapter(adapter)
	cloned.Capabilities = normalizeCapabilities(cloned.Capabilities)
	return cloned, nil
}

// Capabilities returns an adapter's capability rules.
func Capabilities(adapter Adapter) []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), adapter.Capabilities...)
}

// Render renders one explicit target request through adapter after validation.
func Render(adapter Adapter, input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if adapter.render == nil {
		return emptyPlan(adapter.Target), []model.Diagnostic{registryDiagnostic("invalid-adapter", fmt.Sprintf("adapter %q has no renderer", adapter.Target))}
	}

	input.Packages = append([]model.NormalizedPackage(nil), input.Packages...)
	model.SortTargetRenderInput(&input)
	if diagnostics := model.ValidateTargetRenderInput(input); len(diagnostics) != 0 {
		return emptyPlan(adapter.Target), diagnostics
	}

	var diagnostics []model.Diagnostic
	for _, pkg := range input.Packages {
		if pkg.Target != adapter.Target {
			diagnostics = append(diagnostics, registryDiagnostic(
				"target-mismatch",
				fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, adapter.Target),
			))
		}
	}
	if len(diagnostics) != 0 {
		return emptyPlan(adapter.Target), diagnostics
	}
	plan, renderDiagnostics := adapter.render(input)
	// Centrally default empty archive-unit lists from existing renderers to one
	// dot archive containing all target files. The Pi target uses .tgz; all
	// others use .tar.gz. Adapters that set their own units (e.g. agent-plugins)
	// are not affected.
	if len(plan.ArchiveUnits) == 0 && len(renderDiagnostics) == 0 {
		plan.ArchiveUnits = []model.ArchiveUnit{{
			Root:   ".",
			Stem:   string(adapter.Target),
			Suffix: defaultArchiveSuffix(adapter.Target),
		}}
	}
	return plan, renderDiagnostics
}

func cloneAdapter(adapter Adapter) Adapter {
	adapter.Capabilities = Capabilities(adapter)
	return adapter
}

func emptyPlan(target model.TargetID) model.TargetPlan {
	return model.TargetPlan{
		Target:       target,
		Packages:     []model.PackageID{},
		Files:        []model.PlannedFile{},
		NativeChecks: []model.NativeCheck{},
	}
}

// defaultArchiveSuffix returns the standard archive file suffix for a target.
// Pi uses .tgz for compatibility with npm conventions; all others use .tar.gz.
func defaultArchiveSuffix(target model.TargetID) string {
	if target == model.TargetPi {
		return ".tgz"
	}
	return ".tar.gz"
}

func registryDiagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{
		Code:     code,
		Severity: model.SeverityError,
		Message:  message,
	}
}
