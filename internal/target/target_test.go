package target

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/claude"
)

func TestResolveBuiltInAdapters(t *testing.T) {
	t.Parallel()

	targets := []model.TargetID{
		model.TargetAgentPlugins,
		model.TargetAntigravity,
		model.TargetClaude,
		model.TargetCodex,
		model.TargetPi,
		model.TargetCopilot,
		model.TargetGrok,
		model.TargetCursor,
	}
	for _, targetID := range targets {
		t.Run(string(targetID), func(t *testing.T) {
			adapter, diagnostics := Resolve(targetID)
			if len(diagnostics) != 0 {
				t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
			}
			if adapter.Target != targetID {
				t.Fatalf("Resolve(%q) target = %q", targetID, adapter.Target)
			}
			if adapter.FormatRevision < 1 {
				t.Fatalf("Resolve(%q) format revision = %d", targetID, adapter.FormatRevision)
			}
			if len(Capabilities(adapter)) < 5 {
				t.Fatalf("Capabilities(%q) = %#v, want at least five base rules", targetID, Capabilities(adapter))
			}
		})
	}

	_, diagnostics := Resolve("unknown")
	if !hasDiagnostic(diagnostics, "unknown-target") {
		t.Fatalf("Resolve(unknown) diagnostics = %#v, want unknown-target", diagnostics)
	}
}

func TestTargetsAdvertisePortableDecisionSupport(t *testing.T) {
	t.Parallel()

	want := map[model.TargetID]map[model.CapabilityKey]model.CapabilityState{
		model.TargetAntigravity: {"hook.decision.block": model.CapabilityStateUnsupported, "hook.decision.rewrite-input": model.CapabilityStateUnsupported},
		model.TargetClaude:      {"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateNative},
		model.TargetCodex:       {"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateUnsupported},
		model.TargetCopilot:     {"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateNative},
		model.TargetCursor:      {"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateNative},
		model.TargetGrok:        {"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateUnsupported},
		model.TargetPi:          {"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateNative},
	}
	for targetID, statesWant := range want {
		adapter, diagnostics := Resolve(targetID)
		if len(diagnostics) != 0 {
			t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
		}
		states := make(map[model.CapabilityKey]model.CapabilityState)
		for _, rule := range Capabilities(adapter) {
			states[rule.Key] = rule.State
		}
		for key, stateWant := range statesWant {
			if states[key] != stateWant {
				t.Errorf("Capabilities(%q)[%q] = %q, want %q", targetID, key, states[key], stateWant)
			}
		}
	}
}

func TestTargetsRejectUnsupportedDecisionCellsBeforeOutput(t *testing.T) {
	t.Parallel()

	wantUnsupported := map[model.TargetID][]model.CapabilityKey{
		model.TargetAntigravity: {"hook.decision.block", "hook.decision.rewrite-input"},
		model.TargetCodex:       {"hook.decision.rewrite-input"},
		model.TargetGrok:        {"hook.decision.rewrite-input"},
	}
	for targetID, keys := range wantUnsupported {
		for _, key := range keys {
			t.Run(string(targetID)+"/"+string(key), func(t *testing.T) {
				command := "true"
				location := model.SourceLocation{Path: "source/hooks/decision/hook.json"}
				identity := model.AssetID("hook/decision")
				pkg := model.NormalizedPackage{
					Identity: "demo", Target: targetID, Profile: model.TargetProfilePackage,
					Assets: []model.NormalizedAsset{{
						Identity: identity, Kind: model.AssetKindHook,
						Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}},
						Hook: &model.HookDescriptor{
							Identity: identity, Location: location, Event: model.HookEventPreTool,
							Matcher:             &model.HookMatcher{Tools: []model.HookToolCategory{model.HookToolCategoryCommand}},
							Handler:             model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command},
							TimeoutMilliseconds: 1_000, FailurePolicy: model.HookFailurePolicyOpen,
						},
						CapabilityUses: []model.CapabilityUse{
							{Key: "asset.hook", Location: location},
							{Key: "hook.command.shell", Location: location},
							{Key: "hook.event.pre-tool", Location: location},
							{Key: "hook.matcher.tool-category", Location: location},
							{Key: key, Location: location},
						},
					}},
				}
				adapter, diagnostics := Resolve(targetID)
				if len(diagnostics) != 0 {
					t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
				}
				plan, diagnostics := Render(adapter, model.TargetRenderInput{Packages: []model.NormalizedPackage{pkg}, PackageMode: model.TargetPackageModeSeparate})
				if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
					t.Fatalf("Render(%q, %q) = (%#v, %#v), want unsupported-capability", targetID, key, plan, diagnostics)
				}
				if len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
					t.Fatalf("rejected decision hook produced partial plan: %#v", plan)
				}
			})
		}
	}
}

func TestTargetsRejectUnsupportedCommandsBeforeOutput(t *testing.T) {
	t.Parallel()

	for _, targetID := range []model.TargetID{model.TargetAntigravity, model.TargetCodex, model.TargetCopilot, model.TargetCursor, model.TargetGrok, model.TargetPi} {
		profiles := []model.TargetProfile{model.TargetProfileProject, model.TargetProfilePackage}
		if targetID == model.TargetAntigravity {
			profiles = []model.TargetProfile{model.TargetProfilePackage}
		}
		for _, profile := range profiles {
			targetID, profile := targetID, profile
			t.Run(string(targetID)+"/"+string(profile), func(t *testing.T) {
				location := model.SourceLocation{Path: "source/commands/resume-from.md"}
				identity := model.AssetID("command/resume-from")
				pkg := model.NormalizedPackage{
					Identity: "demo", Target: targetID, Profile: profile,
					Assets: []model.NormalizedAsset{{
						Identity: identity, Kind: model.AssetKindCommand,
						Content:        model.AssetContent{Frontmatter: map[string]any{"description": "Resume from a saved handoff."}, Body: "Resume the session.\n", Files: map[model.RelativePath]model.FileContent{}},
						Command:        &model.CommandDescriptor{Identity: identity, Location: location, Name: "resume-from", Description: "Resume from a saved handoff."},
						CapabilityUses: []model.CapabilityUse{{Key: "asset.command", Location: location}},
					}},
				}
				adapter, diagnostics := Resolve(targetID)
				if len(diagnostics) != 0 {
					t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
				}
				plan, diagnostics := Render(adapter, model.TargetRenderInput{Packages: []model.NormalizedPackage{pkg}, PackageMode: model.TargetPackageModeSeparate})
				if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || !strings.Contains(diagnostics[0].Message, "asset.command") {
					t.Fatalf("Render(%q) = (%#v, %#v), want unsupported-capability", targetID, plan, diagnostics)
				}
				if len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
					t.Fatalf("rejected command produced partial plan: %#v", plan)
				}
			})
		}
	}
}

func TestCapabilitiesReturnsIndependentRules(t *testing.T) {
	t.Parallel()

	adapter, diagnostics := Resolve(model.TargetClaude)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve() diagnostics = %#v", diagnostics)
	}
	capabilities := Capabilities(adapter)
	capabilities[0].State = model.CapabilityStateAdvisory
	if reflect.DeepEqual(capabilities, Capabilities(adapter)) {
		t.Fatal("Capabilities() returned mutable adapter state")
	}
}

func TestRenderDispatchesToResolvedAdapter(t *testing.T) {
	t.Parallel()

	packages := []model.NormalizedPackage{{
		Identity: "example",
		Metadata: model.PackageMetadata{"name": "Example"},
		Target:   model.TargetClaude,
	}}
	adapter, diagnostics := Resolve(model.TargetClaude)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve() diagnostics = %#v", diagnostics)
	}
	input := model.TargetRenderInput{Packages: packages, PackageMode: model.TargetPackageModeSeparate}
	got, diagnostics := Render(adapter, input)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	want, diagnostics := claude.Render(input)
	if len(diagnostics) != 0 {
		t.Fatalf("claude.Render() diagnostics = %#v", diagnostics)
	}
	// target.Render applies the central archive-unit default; claude.Render does
	// not, so we add the expected unit to the direct-render plan for comparison.
	want.ArchiveUnits = []model.ArchiveUnit{{Root: ".", Stem: "claude", Suffix: ".tar.gz"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Render() plan = %#v, want %#v", got, want)
	}
}

func TestRenderCanonicalizesExplicitTargetInputBeforeDispatch(t *testing.T) {
	t.Parallel()

	var received model.TargetRenderInput
	adapter := Adapter{
		Target:         model.TargetClaude,
		FormatRevision: 1,
		render: func(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
			received = input
			return emptyPlan(model.TargetClaude), nil
		},
	}
	input := model.TargetRenderInput{
		Packages: []model.NormalizedPackage{
			{Identity: "zeta", Target: model.TargetClaude},
			{Identity: "alpha", Target: model.TargetClaude},
		},
		Distribution: model.DistributionMetadata{"name": "Team tools"},
		PackageMode:  model.TargetPackageModeSeparate,
	}
	if _, diagnostics := Render(adapter, input); len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got := []model.PackageID{received.Packages[0].Identity, received.Packages[1].Identity}; !reflect.DeepEqual(got, []model.PackageID{"alpha", "zeta"}) {
		t.Fatalf("received package order = %#v", got)
	}
	if received.PackageMode != model.TargetPackageModeSeparate || !reflect.DeepEqual(received.Distribution, input.Distribution) {
		t.Fatalf("received render input = %#v", received)
	}
}

func TestRenderRejectsTargetMismatch(t *testing.T) {
	t.Parallel()

	adapter, diagnostics := Resolve(model.TargetClaude)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve() diagnostics = %#v", diagnostics)
	}
	plan, diagnostics := Render(adapter, model.TargetRenderInput{
		Packages: []model.NormalizedPackage{{
			Identity: "example",
			Target:   model.TargetCodex,
		}},
		PackageMode: model.TargetPackageModeSeparate,
	})
	if !hasDiagnostic(diagnostics, "target-mismatch") {
		t.Fatalf("Render() diagnostics = %#v, want target-mismatch", diagnostics)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("Render() files = %#v, want none", plan.Files)
	}
}

func TestNewRegistryRejectsDuplicateTarget(t *testing.T) {
	t.Parallel()

	adapter, diagnostics := Resolve(model.TargetClaude)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve() diagnostics = %#v", diagnostics)
	}
	_, diagnostics = newRegistry(adapter, adapter)
	if !hasDiagnostic(diagnostics, "duplicate-adapter") {
		t.Fatalf("newRegistry() diagnostics = %#v, want duplicate-adapter", diagnostics)
	}
}

func TestResolveMapsAgentPluginSkillsAndRejectsUnmappedComponents(t *testing.T) {
	t.Parallel()

	unsupportedAgentPluginKeys := []model.CapabilityKey{
		model.CapabilityKeyAgentPluginMCPStdio,
		model.CapabilityKeyAgentPluginMCPStreamableHTTP,
		model.CapabilityKeyAgentPluginMCPSSE,
		model.CapabilityKeyAgentPluginExtensions,
		model.CapabilityKeyAgentPluginUnknownJSON,
		model.CapabilityKeyAgentPluginPackageFiles,
	}

	// Existing targets reuse their verified asset.skill support. Other Agent
	// Plugin components remain unsupported without dedicated codecs.
	existingTargets := []model.TargetID{
		model.TargetAntigravity,
		model.TargetClaude,
		model.TargetCodex,
		model.TargetPi,
		model.TargetCopilot,
		model.TargetGrok,
		model.TargetCursor,
	}

	for _, targetID := range existingTargets {
		t.Run(string(targetID), func(t *testing.T) {
			adapter, diagnostics := Resolve(targetID)
			if len(diagnostics) != 0 {
				t.Fatalf("Resolve(%q) diagnostics = %v", targetID, diagnostics)
			}
			states := make(map[model.CapabilityKey]model.CapabilityState, len(adapter.Capabilities))
			for _, r := range Capabilities(adapter) {
				states[r.Key] = r.State
			}
			if states[model.CapabilityKeyAgentPluginSkills] != states["asset.skill"] {
				t.Errorf("Capabilities(%q)[%q] = %q, want mapped asset.skill state %q", targetID, model.CapabilityKeyAgentPluginSkills, states[model.CapabilityKeyAgentPluginSkills], states["asset.skill"])
			}
			for _, key := range unsupportedAgentPluginKeys {
				if states[key] != model.CapabilityStateUnsupported {
					t.Errorf("Capabilities(%q)[%q] = %q, want unsupported", targetID, key, states[key])
				}
			}
		})
	}
}

func TestRenderSetsDefaultArchiveUnitForExistingTargets(t *testing.T) {
	// Every existing target should get a central default archive unit when its
	// renderer returns an empty ArchiveUnits list.
	t.Parallel()
	for _, targetID := range []model.TargetID{
		model.TargetAntigravity, model.TargetClaude, model.TargetCodex,
		model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor,
	} {
		targetID := targetID
		t.Run(string(targetID), func(t *testing.T) {
			t.Parallel()
			adapter, diagnostics := Resolve(targetID)
			if len(diagnostics) != 0 {
				t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
			}
			pkg := model.NormalizedPackage{
				Identity: "demo",
				Target:   targetID,
				Metadata: model.PackageMetadata{},
				Profile:  model.TargetProfilePackage,
			}
			plan, diags := Render(adapter, model.TargetRenderInput{
				Packages:    []model.NormalizedPackage{pkg},
				PackageMode: model.TargetPackageModeSeparate,
			})
			if len(diags) != 0 {
				t.Fatalf("Render(%q) diagnostics = %#v", targetID, diags)
			}
			if len(plan.ArchiveUnits) != 1 {
				t.Fatalf("Render(%q) ArchiveUnits = %#v, want exactly 1 default unit", targetID, plan.ArchiveUnits)
			}
			unit := plan.ArchiveUnits[0]
			if unit.Root != "." {
				t.Errorf("Render(%q) ArchiveUnit.Root = %q, want .", targetID, unit.Root)
			}
			if unit.Stem != string(targetID) {
				t.Errorf("Render(%q) ArchiveUnit.Stem = %q, want %q", targetID, unit.Stem, string(targetID))
			}
			wantSuffix := ".tar.gz"
			if targetID == model.TargetPi {
				wantSuffix = ".tgz"
			}
			if unit.Suffix != wantSuffix {
				t.Errorf("Render(%q) ArchiveUnit.Suffix = %q, want %q", targetID, unit.Suffix, wantSuffix)
			}
		})
	}
}

func TestAgentPluginsAdapterRegisteredAndCapable(t *testing.T) {
	t.Parallel()
	adapter, diagnostics := Resolve(model.TargetAgentPlugins)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve(agent-plugins) diagnostics = %#v", diagnostics)
	}
	if adapter.Target != model.TargetAgentPlugins {
		t.Fatalf("adapter.Target = %q", adapter.Target)
	}
	if adapter.FormatRevision < 1 {
		t.Fatalf("adapter.FormatRevision = %d", adapter.FormatRevision)
	}
	// All portable agent-plugin keys must be native.
	portableKeys := []model.CapabilityKey{
		model.CapabilityKeyAgentPluginSkills,
		model.CapabilityKeyAgentPluginMCPStdio,
		model.CapabilityKeyAgentPluginMCPStreamableHTTP,
		model.CapabilityKeyAgentPluginMCPSSE,
		model.CapabilityKeyAgentPluginExtensions,
		model.CapabilityKeyAgentPluginUnknownJSON,
		model.CapabilityKeyAgentPluginPackageFiles,
	}
	states := make(map[model.CapabilityKey]model.CapabilityState)
	for _, r := range Capabilities(adapter) {
		states[r.Key] = r.State
	}
	for _, key := range portableKeys {
		if states[key] != model.CapabilityStateNative {
			t.Errorf("Capabilities(agent-plugins)[%q] = %q, want native", key, states[key])
		}
	}
}

func hasDiagnostic(diagnostics []model.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
