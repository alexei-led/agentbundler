package target

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/claude"
)

func TestResolveBuiltInAdapters(t *testing.T) {
	t.Parallel()

	targets := []model.TargetID{
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
			if len(Capabilities(adapter)) != 4 {
				t.Fatalf("Capabilities(%q) = %#v, want four rules", targetID, Capabilities(adapter))
			}
		})
	}

	_, diagnostics := Resolve("unknown")
	if !hasDiagnostic(diagnostics, "unknown-target") {
		t.Fatalf("Resolve(unknown) diagnostics = %#v, want unknown-target", diagnostics)
	}
}

func TestCapabilitiesReturnsIndependentRules(t *testing.T) {
	t.Parallel()

	adapter, diagnostics := Resolve(model.TargetClaude)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve() diagnostics = %#v", diagnostics)
	}
	capabilities := Capabilities(adapter)
	capabilities[0].State = model.CapabilityStateUnsupported
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
	got, diagnostics := Render(adapter, packages)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	want, diagnostics := claude.Render(packages)
	if len(diagnostics) != 0 {
		t.Fatalf("claude.Render() diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Render() plan = %#v, want %#v", got, want)
	}
}

func TestRenderRejectsTargetMismatch(t *testing.T) {
	t.Parallel()

	adapter, diagnostics := Resolve(model.TargetClaude)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve() diagnostics = %#v", diagnostics)
	}
	plan, diagnostics := Render(adapter, []model.NormalizedPackage{{
		Identity: "example",
		Target:   model.TargetCodex,
	}})
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

func hasDiagnostic(diagnostics []model.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
