package compiler

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestCompilePiAggregateHooksEndToEndIsDeterministicAndCheckIsReadOnly(t *testing.T) {
	firstRoot, manifest, first := compilePiHookFixture(t)
	secondRoot, _, second := compilePiHookFixture(t)
	if !reflect.DeepEqual(first.Plan, second.Plan) {
		t.Fatal("Compile() Pi plans differ across absolute workspace roots")
	}
	for _, file := range first.Plan.CompilerFiles {
		if bytes.Contains(file.Bytes, []byte(firstRoot)) || bytes.Contains(file.Bytes, []byte(secondRoot)) {
			t.Fatal("Pi provenance contains an absolute workspace root")
		}
	}
	if len(first.Plan.Targets) != 1 || first.Plan.Targets[0].Target != model.TargetPi {
		t.Fatalf("Compile() target plan = %#v", first.Plan.Targets)
	}
	plan := first.Plan.Targets[0]
	for _, path := range []model.RelativePath{
		"agents/reviewer.md",
		"extensions/_agentbundler-hooks/index.ts",
		"extensions/agentbundler-hooks.ts",
		"hooks/hooks.v1.json",
		"hooks/payloads/deny/decision.mjs",
		"hooks/payloads/post-tool/record.mjs",
		"hooks/payloads/rewrite/decision.mjs",
		"hooks/payloads/timeout/wait.mjs",
		"package.json",
		"skills/safety/SKILL.md",
	} {
		if !planHasPath(plan, path) {
			t.Errorf("generated plan is missing %q", path)
		}
	}
	manifestFile := plannedFile(t, plan, "package.json")
	var packageManifest struct {
		Name string `json:"name"`
		Pi   struct {
			Extensions []string `json:"extensions"`
			Skills     []string `json:"skills"`
			Subagents  struct {
				Agents []string `json:"agents"`
			} `json:"subagents"`
		} `json:"pi"`
	}
	if err := json.Unmarshal(manifestFile.Bytes, &packageManifest); err != nil {
		t.Fatal(err)
	}
	if packageManifest.Name != "pi-hook-suite" || !reflect.DeepEqual(packageManifest.Pi.Extensions, []string{"./extensions/agentbundler-hooks.ts"}) || !reflect.DeepEqual(packageManifest.Pi.Skills, []string{"./skills"}) || !reflect.DeepEqual(packageManifest.Pi.Subagents.Agents, []string{"./agents"}) {
		t.Fatalf("generated package.json = %s", manifestFile.Bytes)
	}
	if bytes.Contains(manifestFile.Bytes, []byte("pi-subagents")) || bytes.Contains(manifestFile.Bytes, []byte("bundledDependencies")) {
		t.Fatalf("generated package.json contains implicit third-party runtime metadata: %s", manifestFile.Bytes)
	}
	for _, file := range plan.Files {
		if strings.HasPrefix(string(file.Path), "node_modules/") {
			t.Fatalf("generated plan bundles third-party file %q", file.Path)
		}
	}

	descriptor := plannedFile(t, plan, "hooks/hooks.v1.json")
	fixture, err := os.ReadFile(filepath.Join("..", "target", "pi", "runtime", "testdata", "hooks-pi.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(descriptor.Bytes, fixture) {
		t.Fatalf("generated descriptor differs from TypeScript fixture:\ngot  %s\nwant %s", descriptor.Bytes, fixture)
	}

	outputRoot := filepath.Join(firstRoot, "generated")
	before := snapshotTree(t, outputRoot)
	time.Sleep(10 * time.Millisecond)
	checked := Compile(CompileRequest{WorkspaceRoot: firstRoot, Manifest: manifest, Mode: BuildModeCheck})
	if len(checked.Diagnostics) != 0 || checked.Drift {
		t.Fatalf("check result = %#v", checked)
	}
	if after := snapshotTree(t, outputRoot); !reflect.DeepEqual(after, before) {
		t.Fatalf("check changed generated output:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestCompilePiAggregateReplacesV051BundledRuntimeOutput(t *testing.T) {
	workspace := copyPiHookFixture(t)
	stale := filepath.Join(workspace, "generated", "pi", "node_modules", "pi-subagents", "src", "extension", "index.ts")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := decodePiHookManifest(t, workspace)
	result := Compile(CompileRequest{WorkspaceRoot: workspace, Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated", "pi", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("stale third-party runtime remains after regeneration: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "generated", "pi", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"pi-subagents", "bundledDependencies", "node_modules/"} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("regenerated package contains %q: %s", forbidden, data)
		}
	}
	if !bytes.Contains(data, []byte(`"subagents":{"agents":["./agents"]}`)) {
		t.Fatalf("regenerated package lost pi.subagents.agents: %s", data)
	}
}

func TestCompilePiAggregateFixtureRejectsConflictingDependencies(t *testing.T) {
	workspace := copyPiHookFixture(t)
	path := filepath.Join(workspace, "source", "packages", "observability.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"shared-runtime": "1.0.0"`), []byte(`"shared-runtime": "2.0.0"`), 1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := decodePiHookManifest(t, workspace)
	result := Compile(CompileRequest{WorkspaceRoot: workspace, Manifest: manifest, Mode: BuildModeBuild})
	if !diagnosticContainsText(result.Diagnostics, `aggregate dependency "shared-runtime" conflicts between package "core" ("1.0.0") and package "observability" ("2.0.0")`) {
		t.Fatalf("Compile() diagnostics = %#v, want dependency conflict", result.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated")); !os.IsNotExist(err) {
		t.Fatalf("conflicting aggregate wrote output: %v", err)
	}
}

func compilePiHookFixture(t *testing.T) (string, model.SourceManifest, CompilationResult) {
	t.Helper()
	workspace := copyPiHookFixture(t)
	manifest := decodePiHookManifest(t, workspace)
	result := Compile(CompileRequest{WorkspaceRoot: workspace, Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	return workspace, manifest, result
}

func copyPiHookFixture(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.CopyFS(workspace, os.DirFS(filepath.Join("testdata", "hooks-pi"))); err != nil {
		t.Fatalf("copy Pi fixture: %v", err)
	}
	return filepath.Clean(workspace)
}

func decodePiHookManifest(t *testing.T, workspace string) model.SourceManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "agentbundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, diagnostics := model.DecodeSourceManifestJSON(data)
	if len(diagnostics) != 0 {
		t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v", diagnostics)
	}
	return manifest
}

func plannedFile(t *testing.T, plan model.TargetPlan, path model.RelativePath) model.PlannedFile {
	t.Helper()
	for _, file := range plan.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("planned file %q not found", path)
	return model.PlannedFile{}
}

func planHasPath(plan model.TargetPlan, path model.RelativePath) bool {
	for _, file := range plan.Files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func diagnosticContainsText(diagnostics []model.Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

type treeEntry struct {
	Path    string
	Mode    os.FileMode
	Bytes   string
	ModTime time.Time
}

func snapshotTree(t *testing.T, root string) []treeEntry {
	t.Helper()
	var entries []treeEntry
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, treeEntry{Path: filepath.ToSlash(relative), Mode: info.Mode(), Bytes: string(data), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}
