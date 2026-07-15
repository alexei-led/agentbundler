package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestCCThingzAcceptanceMatrixBuildCheckSelectorsDriftAndDeterminism(t *testing.T) {
	firstRoot, manifest, first := compileCCThingzFixture(t, nil, nil)
	secondRoot, _, second := compileCCThingzFixture(t, nil, nil)
	if !reflect.DeepEqual(first.Plan, second.Plan) {
		t.Fatal("cc-thingz acceptance plans differ across absolute workspace roots")
	}
	for _, file := range first.Plan.CompilerFiles {
		if bytes.Contains(file.Bytes, []byte(firstRoot)) || bytes.Contains(file.Bytes, []byte(secondRoot)) {
			t.Fatal("acceptance provenance contains an absolute workspace root")
		}
	}

	wantTargets := []model.TargetID{
		model.TargetClaude,
		model.TargetCodex,
		model.TargetCopilot,
		model.TargetCursor,
		model.TargetGrok,
		model.TargetPi,
	}
	gotTargets := make([]model.TargetID, len(first.Plan.Targets))
	for index, plan := range first.Plan.Targets {
		gotTargets[index] = plan.Target
	}
	if !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("acceptance targets = %#v, want %#v", gotTargets, wantTargets)
	}

	expectedPaths := map[model.TargetID][]model.RelativePath{
		model.TargetClaude: {
			".claude-plugin/marketplace.json",
			"core-tools/hooks/command-guard/check.sh",
			"core-tools/hooks/command-guard/rules.json",
			"workflow-tools/hooks/file-audit/audit.sh",
		},
		model.TargetCodex: {
			".agents/plugins/marketplace.json",
			"core-tools/assets/hooks/command-guard/check.sh",
			"core-tools/hooks/hooks.json",
		},
		model.TargetCopilot: {
			".github/plugin/marketplace.json",
			"core-tools/hooks/command-guard/check.sh",
			"workflow-tools/hooks/file-audit/audit.sh",
		},
		model.TargetCursor: {
			".cursor-plugin/marketplace.json",
			"core-tools/hooks/command-guard/check.sh",
			"workflow-tools/hooks/file-audit/audit.sh",
		},
		model.TargetGrok: {
			".claude-plugin/marketplace.json",
			"core-tools/hooks/command-guard/check.sh",
			"workflow-tools/hooks/file-audit/audit.sh",
		},
		model.TargetPi: {
			"extensions/agentbundler-hooks.ts",
			"extensions/_agentbundler-hooks/runtime.ts",
			"hooks/hooks.v1.json",
			"hooks/payloads/command-guard/check.sh",
			"hooks/payloads/file-audit/audit.sh",
			"skills/review/references/pi.md",
		},
	}
	for _, plan := range first.Plan.Targets {
		for _, path := range expectedPaths[plan.Target] {
			if !planHasPath(plan, path) {
				t.Errorf("%s acceptance output is missing %q", plan.Target, path)
			}
		}
	}

	claudePlan := acceptanceTargetPlan(t, first.Plan, model.TargetClaude)
	if len(claudePlan.NativeChecks) != 1 || claudePlan.NativeChecks[0].Program != "claude" {
		t.Fatalf("Claude native checks = %#v, want one strict catalog validation", claudePlan.NativeChecks)
	}
	grokPlan := acceptanceTargetPlan(t, first.Plan, model.TargetGrok)
	if len(grokPlan.NativeChecks) != 2 {
		t.Fatalf("Grok native checks = %#v, want one per package root", grokPlan.NativeChecks)
	}
	for _, plan := range first.Plan.Targets {
		if plan.Target != model.TargetClaude && plan.Target != model.TargetGrok && len(plan.NativeChecks) != 0 {
			t.Fatalf("%s unexpectedly declares native checks: %#v", plan.Target, plan.NativeChecks)
		}
	}

	outputRoot := filepath.Join(firstRoot, "generated")
	before := snapshotTree(t, outputRoot)
	checked := Compile(CompileRequest{WorkspaceRoot: firstRoot, Manifest: manifest, Mode: BuildModeCheck})
	if len(checked.Diagnostics) != 0 || checked.Drift || checked.NativeVerificationFailed {
		t.Fatalf("current acceptance check = %#v", checked)
	}
	if after := snapshotTree(t, outputRoot); !reflect.DeepEqual(after, before) {
		t.Fatal("acceptance check changed generated output")
	}

	driftPath := filepath.Join(outputRoot, "cursor", "core-tools", "hooks", "hooks.json")
	if err := os.WriteFile(driftPath, []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drifted := Compile(CompileRequest{WorkspaceRoot: firstRoot, Manifest: manifest, Mode: BuildModeCheck})
	if !drifted.Drift || !diagnosticCode(drifted.Diagnostics, "DRIFT_CHANGED") {
		t.Fatalf("drift acceptance check = %#v", drifted)
	}
	data, err := os.ReadFile(driftPath)
	if err != nil || string(data) != "drift\n" {
		t.Fatalf("drift check rewrote %q: data=%q err=%v", driftPath, data, err)
	}

	_, _, selected := compileCCThingzFixture(t, []model.TargetID{model.TargetCodex}, []model.PackageID{"core-tools"})
	if len(selected.Plan.Targets) != 1 || selected.Plan.Targets[0].Target != model.TargetCodex || !reflect.DeepEqual(selected.Plan.Targets[0].Packages, []model.PackageID{"core-tools"}) {
		t.Fatalf("selected acceptance plan = %#v", selected.Plan.Targets)
	}
	if !planHasPath(selected.Plan.Targets[0], ".codex-plugin/plugin.json") || planHasPath(selected.Plan.Targets[0], "core-tools/.codex-plugin/plugin.json") {
		t.Fatalf("single-package selector did not use the flat Codex root: %#v", targetPaths(selected.Plan.Targets[0]))
	}
}

func compileCCThingzFixture(t *testing.T, targets []model.TargetID, packages []model.PackageID) (string, model.SourceManifest, CompilationResult) {
	t.Helper()
	workspace := t.TempDir()
	fixture := filepath.Join("..", "..", "testdata", "cc-thingz-hooks")
	if err := os.CopyFS(workspace, os.DirFS(fixture)); err != nil {
		t.Fatalf("copy cc-thingz acceptance fixture: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "agentbundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, diagnostics := model.DecodeSourceManifestJSON(data)
	if len(diagnostics) != 0 {
		t.Fatalf("decode cc-thingz acceptance manifest: %#v", diagnostics)
	}
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      manifest,
		Targets:       targets,
		Packages:      packages,
		Mode:          BuildModeBuild,
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile cc-thingz acceptance fixture: %#v", result.Diagnostics)
	}
	return workspace, manifest, result
}

func acceptanceTargetPlan(t *testing.T, plan model.BuildPlan, target model.TargetID) model.TargetPlan {
	t.Helper()
	for _, targetPlan := range plan.Targets {
		if targetPlan.Target == target {
			return targetPlan
		}
	}
	t.Fatalf("acceptance target %q is missing", target)
	return model.TargetPlan{}
}

func diagnosticCode(diagnostics []model.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
