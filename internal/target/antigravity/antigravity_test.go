package antigravity

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestAdapterIdentityCapabilitiesAndPackageOnlyDispatch(t *testing.T) {
	adapter := New()
	if adapter.Target() != Target || adapter.FormatRevision() != 1 {
		t.Fatalf("adapter = target %q revision %d", adapter.Target(), adapter.FormatRevision())
	}
	first := adapter.Capabilities()
	first[0].State = model.CapabilityStateAdvisory
	if reflect.DeepEqual(first, adapter.Capabilities()) {
		t.Fatal("Capabilities() returned mutable state")
	}

	pkg := packageFixture()
	pkg.Profile = model.TargetProfileProject
	if _, diagnostics := adapter.Render(separate(pkg)); len(diagnostics) != 1 || diagnostics[0].Code != "invalid-target-profile" {
		t.Fatalf("project diagnostics = %#v", diagnostics)
	}
	pkg.Profile = model.TargetProfilePackage
	input := separate(pkg)
	input.PackageMode = model.TargetPackageModeAggregate
	if plan, diagnostics := adapter.Render(input); len(diagnostics) == 0 || len(plan.Files) != 0 {
		t.Fatalf("aggregate render = (%#v, %#v)", plan, diagnostics)
	}
}

func TestCapabilitiesRejectEveryPortableHookCell(t *testing.T) {
	rules := make(map[model.CapabilityKey]model.CapabilityState)
	for _, rule := range Capabilities() {
		rules[rule.Key] = rule.State
	}
	for _, key := range []model.CapabilityKey{
		"asset.hook", "hook.async", "hook.command.exec", "hook.command.shell", "hook.decision.block", "hook.decision.rewrite-input",
		"hook.event.notification", "hook.event.post-compact", "hook.event.post-tool", "hook.event.post-tool-failure", "hook.event.pre-compact",
		"hook.event.pre-tool", "hook.event.prompt-submit", "hook.event.session-end", "hook.event.session-start", "hook.event.stop",
		"hook.failure.closed", "hook.matcher.tool-category",
	} {
		if rules[key] != model.CapabilityStateUnsupported {
			t.Errorf("capability %q = %q, want unsupported", key, rules[key])
		}
		pkg := packageFixture()
		pkg.Assets[0].CapabilityUses = []model.CapabilityUse{{Key: key, Location: model.SourceLocation{Path: "source/hook.json"}}}
		plan, diagnostics := Render(separate(pkg))
		if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
			t.Errorf("Render(%q) = (%#v, %#v)", key, plan, diagnostics)
		}
	}

	pkg := packageFixture()
	command := "true"
	pkg.Assets = []model.NormalizedAsset{{
		Identity: "hook/raw", Kind: model.AssetKindHook, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}},
		Hook: &model.HookDescriptor{
			Identity: "hook/raw", Location: model.SourceLocation{Path: "source/hook.json"}, Event: model.HookEventSessionStart,
			Handler: model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}, TimeoutMilliseconds: 1_000,
			FailurePolicy: model.HookFailurePolicyOpen,
		},
	}}
	if plan, diagnostics := Render(separate(pkg)); len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || len(plan.Files) != 0 {
		t.Fatalf("portable hook without capability uses = (%#v, %#v)", plan, diagnostics)
	}
}

func TestNativeChecksFlatAndMultipleRoots(t *testing.T) {
	flat, diagnostics := Render(separate(packageFixture()))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	wantFlat := []model.NativeCheck{{Program: "agy", Arguments: []string{"plugin", "validate", "."}, Location: model.SourceLocation{Path: "internal/target/antigravity/antigravity.go"}}}
	if !reflect.DeepEqual(flat.NativeChecks, wantFlat) {
		t.Fatalf("flat checks = %#v, want %#v", flat.NativeChecks, wantFlat)
	}

	second := packageFixture()
	second.Identity = "alpha"
	input := separate(packageFixture())
	input.Packages = append(input.Packages, second)
	multi, diagnostics := Render(input)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if got := multi.Packages; !reflect.DeepEqual(got, []model.PackageID{"alpha", "demo"}) {
		t.Fatalf("packages = %#v", got)
	}
	if len(multi.NativeChecks) != 2 || multi.NativeChecks[0].WorkingDirectory == nil || *multi.NativeChecks[0].WorkingDirectory != "alpha" || multi.NativeChecks[1].WorkingDirectory == nil || *multi.NativeChecks[1].WorkingDirectory != "demo" {
		t.Fatalf("multi checks = %#v", multi.NativeChecks)
	}
	for _, check := range multi.NativeChecks {
		if check.Program != "agy" || !reflect.DeepEqual(check.Arguments, []string{"plugin", "validate", "."}) || check.Location.Path != "internal/target/antigravity/antigravity.go" {
			t.Errorf("check = %#v", check)
		}
	}
}

func TestGoldenPluginTree(t *testing.T) {
	plan, diagnostics := Render(separate(packageFixture()))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	goldenRoot := "testdata/plugin-golden"
	entries := make(map[model.RelativePath][]byte)
	err := filepath.Walk(goldenRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relative, err := filepath.Rel(goldenRoot, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries[model.RelativePath(filepath.ToSlash(relative))] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != len(entries) {
		t.Fatalf("planned file count = %d, golden count = %d\npaths: %v", len(plan.Files), len(entries), plannedPaths(plan))
	}
	for _, file := range plan.Files {
		want, ok := entries[file.Path]
		if !ok || !reflect.DeepEqual(file.Bytes, want) {
			t.Errorf("file %q mismatch\ngot:  %q\nwant: %q", file.Path, file.Bytes, want)
		}
	}

	reversed := packageFixture()
	sort.Slice(reversed.Assets, func(left, right int) bool { return reversed.Assets[left].Identity > reversed.Assets[right].Identity })
	again, diagnostics := Render(separate(reversed))
	if len(diagnostics) != 0 || !reflect.DeepEqual(plan, again) {
		t.Fatalf("reordered render differs: diagnostics=%#v", diagnostics)
	}
}

func plannedPaths(plan model.TargetPlan) string {
	paths := make([]string, len(plan.Files))
	for index, file := range plan.Files {
		paths[index] = string(file.Path)
	}
	return strings.Join(paths, ", ")
}

func packageFixture() model.NormalizedPackage {
	return model.NormalizedPackage{
		Identity: "demo", Target: Target, Profile: model.TargetProfilePackage,
		Metadata: model.PackageMetadata{"description": "Demo plugin", "version": "ignored"},
		Assets: []model.NormalizedAsset{
			{Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Frontmatter: map[string]any{"name": "guide", "description": "Guide users", "metadata": map[string]any{"version": "1.0.0"}}, Body: "Use this guide.\n", Files: map[model.RelativePath]model.FileContent{"references/notes.md": {Bytes: []byte("# Notes\n")}}}},
			{Identity: "agent/reviewer", Kind: model.AssetKindAgent, Content: model.AssetContent{Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"}, Body: "Review carefully.\n", Files: map[model.RelativePath]model.FileContent{}}},
			{Identity: "resource/templates", Kind: model.AssetKindResource, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{"prompt.txt": {Bytes: []byte("Prompt template.\n")}}}},
			{Identity: "native-resource/conductor", Kind: model.AssetKindNativeResource, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
				"rules/conductor.md": {Bytes: []byte("# Conductor rule\n")}, "mcp_config.json": {Bytes: []byte("{\"mcpServers\":{}}\n")},
			}}},
		},
	}
}
