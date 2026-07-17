package compiler

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/antigravity"
)

func TestCCThingzDistributionVersionOwnsEveryGeneratedManifestAndCatalog(t *testing.T) {
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
	manifest.Distribution["version"] = "7.2.1"
	result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile cc-thingz acceptance fixture: %#v", result.Diagnostics)
	}
	for _, target := range result.Plan.Targets {
		for _, file := range target.Files {
			if filepath.Ext(string(file.Path)) != ".json" || strings.HasPrefix(string(file.Path), "node_modules/") || strings.Contains(string(file.Path), "/node_modules/") {
				continue
			}
			var document any
			if err := json.Unmarshal(file.Bytes, &document); err != nil {
				t.Fatalf("decode %s %q: %v", target.Target, file.Path, err)
			}
			for _, version := range stringVersionFields(document) {
				if version != "7.2.1" {
					t.Fatalf("%s %q version = %q, want distribution version", target.Target, file.Path, version)
				}
			}
		}
	}
}

func stringVersionFields(value any) []string {
	var versions []string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "version" {
					if version, ok := child.(string); ok {
						versions = append(versions, version)
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(value)
	return versions
}

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
		model.TargetAntigravity,
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
		model.TargetAntigravity: {
			"core-tools/plugin.json",
			"core-tools/skills/review/SKILL.md",
			"core-tools/agents/reviewer.md",
			"core-tools/rules/conductor_antigravity.md",
			"workflow-tools/plugin.json",
			"workflow-tools/skills/release/SKILL.md",
		},
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

	antigravityPlan := acceptanceTargetPlan(t, first.Plan, model.TargetAntigravity)
	wantAntigravityPaths := []model.RelativePath{
		"core-tools/README.md",
		"core-tools/agents/reviewer.md",
		"core-tools/plugin.json",
		"core-tools/rules/conductor_antigravity.md",
		"core-tools/skills/review/SKILL.md",
		"core-tools/skills/review/references/checklist.md",
		"workflow-tools/README.md",
		"workflow-tools/plugin.json",
		"workflow-tools/skills/release/SKILL.md",
	}
	if got := targetPaths(antigravityPlan); !reflect.DeepEqual(got, wantAntigravityPaths) {
		t.Fatalf("Antigravity paths = %#v, want %#v", got, wantAntigravityPaths)
	}
	if len(antigravityPlan.NativeChecks) != 2 {
		t.Fatalf("Antigravity native checks = %#v, want one per package root", antigravityPlan.NativeChecks)
	}
	for index, packageID := range []model.RelativePath{"core-tools", "workflow-tools"} {
		check := antigravityPlan.NativeChecks[index]
		if check.Program != "agy" || !reflect.DeepEqual(check.Arguments, []string{"plugin", "validate", "."}) || check.WorkingDirectory == nil || *check.WorkingDirectory != packageID || check.Location.Path != "internal/target/antigravity/antigravity.go" {
			t.Errorf("Antigravity native check %d = %#v", index, check)
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
		if plan.Target != model.TargetAntigravity && plan.Target != model.TargetClaude && plan.Target != model.TargetGrok && len(plan.NativeChecks) != 0 {
			t.Fatalf("%s unexpectedly declares native checks: %#v", plan.Target, plan.NativeChecks)
		}
	}

	existingTargets := []model.TargetID{
		model.TargetClaude,
		model.TargetCodex,
		model.TargetCopilot,
		model.TargetCursor,
		model.TargetGrok,
		model.TargetPi,
	}
	_, _, existingOnly := compileCCThingzFixture(t, existingTargets, nil)
	for _, targetID := range existingTargets {
		got := acceptanceTargetPlan(t, first.Plan, targetID)
		want := acceptanceTargetPlan(t, existingOnly.Plan, targetID)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s output changed when Antigravity was selected", targetID)
		}
	}
	assertAcceptanceProvenance(t, first.Plan, wantTargets, wantAntigravityPaths)

	outputRoot := filepath.Join(firstRoot, "generated")
	before := snapshotTree(t, outputRoot)
	checked := Compile(CompileRequest{WorkspaceRoot: firstRoot, Manifest: manifest, Mode: BuildModeCheck})
	if len(checked.Diagnostics) != 0 || checked.Drift || checked.NativeVerificationFailed {
		t.Fatalf("current acceptance check = %#v", checked)
	}
	if after := snapshotTree(t, outputRoot); !reflect.DeepEqual(after, before) {
		t.Fatal("acceptance check changed generated output")
	}

	driftPath := filepath.Join(outputRoot, "antigravity", "core-tools", "plugin.json")
	original, err := os.ReadFile(driftPath)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(driftPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	restoredBefore := snapshotTree(t, outputRoot)
	restored := Compile(CompileRequest{WorkspaceRoot: firstRoot, Manifest: manifest, Mode: BuildModeCheck})
	if len(restored.Diagnostics) != 0 || restored.Drift {
		t.Fatalf("restored acceptance check = %#v", restored)
	}
	if after := snapshotTree(t, outputRoot); !reflect.DeepEqual(after, restoredBefore) {
		t.Fatal("restored acceptance check changed generated output")
	}

	_, _, selected := compileCCThingzFixture(t, []model.TargetID{model.TargetAntigravity}, []model.PackageID{"core-tools"})
	if len(selected.Plan.Targets) != 1 || selected.Plan.Targets[0].Target != model.TargetAntigravity || !reflect.DeepEqual(selected.Plan.Targets[0].Packages, []model.PackageID{"core-tools"}) {
		t.Fatalf("selected acceptance plan = %#v", selected.Plan.Targets)
	}
	selectedPlan := selected.Plan.Targets[0]
	if !planHasPath(selectedPlan, "plugin.json") || planHasPath(selectedPlan, "core-tools/plugin.json") || planHasPath(selectedPlan, "workflow-tools/plugin.json") {
		t.Fatalf("single-package selector did not use the flat Antigravity root: %#v", targetPaths(selectedPlan))
	}
	if len(selectedPlan.NativeChecks) != 1 || selectedPlan.NativeChecks[0].WorkingDirectory != nil {
		t.Fatalf("selected Antigravity native checks = %#v", selectedPlan.NativeChecks)
	}
}

func assertAcceptanceProvenance(t *testing.T, plan model.BuildPlan, wantTargets []model.TargetID, wantAntigravityPaths []model.RelativePath) {
	t.Helper()
	if len(plan.CompilerFiles) != 1 || plan.CompilerFiles[0].Path != ".agentbundler/build.json" {
		t.Fatalf("compiler files = %#v", plan.CompilerFiles)
	}
	var document struct {
		Outputs []struct {
			Target          model.TargetID `json:"target"`
			AdapterRevision int            `json:"adapterRevision"`
			Files           []struct {
				Path model.RelativePath `json:"path"`
			} `json:"files"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(plan.CompilerFiles[0].Bytes, &document); err != nil {
		t.Fatal(err)
	}
	gotTargets := make([]model.TargetID, len(document.Outputs))
	for index, output := range document.Outputs {
		gotTargets[index] = output.Target
		if output.Target != model.TargetAntigravity {
			continue
		}
		if output.AdapterRevision != antigravity.FormatRevision {
			t.Errorf("Antigravity adapter revision = %d, want %d", output.AdapterRevision, antigravity.FormatRevision)
		}
		paths := make([]model.RelativePath, len(output.Files))
		for fileIndex, file := range output.Files {
			paths[fileIndex] = file.Path
		}
		if !reflect.DeepEqual(paths, wantAntigravityPaths) {
			t.Errorf("Antigravity provenance paths = %#v, want %#v", paths, wantAntigravityPaths)
		}
	}
	if !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("provenance targets = %#v, want %#v", gotTargets, wantTargets)
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
