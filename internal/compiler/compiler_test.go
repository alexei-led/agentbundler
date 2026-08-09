package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/antigravity"
	"github.com/alexei-led/agentbundler/internal/target/pi"
)

func TestConsolidateDiagnosticsGroupsUnsupportedAgentFieldTargets(t *testing.T) {
	diagnostics := consolidateDiagnostics([]model.Diagnostic{
		{Code: "unsupported-agent-field", Severity: model.SeverityError, Asset: "agent/demo", Field: "sandbox_mode", Targets: []model.TargetID{model.TargetPi}, Hint: "move it"},
		{Code: "unsupported-agent-field", Severity: model.SeverityError, Asset: "agent/demo", Field: "sandbox_mode", Targets: []model.TargetID{model.TargetAntigravity}, Hint: "move it"},
		{Code: "unsupported-agent-field", Severity: model.SeverityError, Asset: "agent/demo", Field: "sandbox_mode", Targets: []model.TargetID{model.TargetClaude}, Hint: "move it"},
	})
	if len(diagnostics) != 1 {
		t.Fatalf("consolidateDiagnostics() = %#v", diagnostics)
	}
	if !reflect.DeepEqual(diagnostics[0].Targets, []model.TargetID{model.TargetAntigravity, model.TargetClaude, model.TargetPi}) {
		t.Fatalf("targets = %#v", diagnostics[0].Targets)
	}
	want := `agent "agent/demo" field "sandbox_mode" is unsupported by targets: antigravity, claude, pi`
	if diagnostics[0].Message != want || diagnostics[0].Hint != "move it" {
		t.Fatalf("diagnostic = %#v", diagnostics[0])
	}
}

func TestCompileRejectsNativeVerifyForBuild(t *testing.T) {
	result := Compile(CompileRequest{Mode: BuildModeBuild, NativeVerify: true})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-native-verify" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileBuildsMinimalSkillsRepositoryForEveryTarget(t *testing.T) {
	for _, target := range []model.TargetID{model.TargetAntigravity, model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor} {
		t.Run(string(target), func(t *testing.T) {
			workspace := t.TempDir()
			writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
			result := Compile(CompileRequest{
				WorkspaceRoot: filepath.Clean(workspace),
				Manifest:      skillsManifest(target),
				Mode:          BuildModeBuild,
			})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
			}
			if len(result.Plan.Targets) != 1 || result.Plan.Targets[0].Target != target {
				t.Fatalf("Compile() targets = %#v", result.Plan.Targets)
			}
		})
	}
}

func TestCompileRecordsResolvedAdapterRevision(t *testing.T) {
	for _, test := range []struct {
		target   model.TargetID
		revision int
	}{
		{target: model.TargetAntigravity, revision: antigravity.FormatRevision},
		{target: model.TargetPi, revision: pi.FormatRevision},
	} {
		t.Run(string(test.target), func(t *testing.T) {
			workspace := t.TempDir()
			writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
			result := Compile(CompileRequest{
				WorkspaceRoot: filepath.Clean(workspace),
				Manifest:      skillsManifest(test.target),
				Mode:          BuildModeBuild,
			})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
			}

			data, err := os.ReadFile(filepath.Join(workspace, "generated/.agentbundler/build.json"))
			if err != nil {
				t.Fatal(err)
			}
			var provenance struct {
				Outputs []struct {
					Target          model.TargetID `json:"target"`
					AdapterRevision int            `json:"adapterRevision"`
				} `json:"outputs"`
			}
			if err := json.Unmarshal(data, &provenance); err != nil {
				t.Fatal(err)
			}
			if len(provenance.Outputs) != 1 || provenance.Outputs[0].Target != test.target || provenance.Outputs[0].AdapterRevision != test.revision {
				t.Fatalf("provenance outputs = %#v", provenance.Outputs)
			}
		})
	}
}

func TestCompilePortableCommandForClaudeAndRejectsPi(t *testing.T) {
	newWorkspace := func(t *testing.T) string {
		workspace := t.TempDir()
		writeCompilerFixture(t, workspace, "source/packages/base.json", `{"id":"base","metadata":{"version":"1.0.0"},"assets":["src/commands/resume-from.md"]}`)
		writeCompilerFixture(t, workspace, "source/src/commands/resume-from.md", "---\n{\"description\":\"Resume from a saved handoff.\"}\n---\nResume the session.\n")
		writeCompilerFixture(t, workspace, "source/src/commands/resume-from.md.agentbundler/targets/claude.json", `{"frontmatterPatch":{"description":"Resume a Claude session."},"bodyPatch":{"mode":"replace","text":"Resume Claude.\n"}}`)
		return workspace
	}
	manifest := func(targetID model.TargetID) model.SourceManifest {
		return model.SourceManifest{
			Version: 1, Kind: model.SourceKindBundle, Root: "source", Targets: []model.TargetID{targetID}, Output: "generated",
			Composition: []model.TargetComposition{{Target: targetID, Profile: model.TargetProfilePackage, PackageMode: model.TargetPackageModeSeparate}},
			Bundle:      &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/base.json"}},
		}
	}

	t.Run("Claude", func(t *testing.T) {
		workspace := newWorkspace(t)
		request := CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest(model.TargetClaude), Mode: BuildModeBuild}
		first := Compile(request)
		if len(first.Diagnostics) != 0 || len(first.Plan.Targets) != 1 {
			t.Fatalf("Compile() = (%#v, %#v)", first.Plan, first.Diagnostics)
		}
		var command []byte
		for _, file := range first.Plan.Targets[0].Files {
			if file.Path == "commands/resume-from.md" {
				command = file.Bytes
			}
		}
		if got, want := string(command), "---\n{\"description\":\"Resume a Claude session.\"}\n---\nResume Claude.\n"; got != want {
			t.Fatalf("command = %q, want %q", got, want)
		}
		second := Compile(request)
		if len(second.Diagnostics) != 0 || !reflect.DeepEqual(first.Plan, second.Plan) {
			t.Fatalf("repeat Compile() differs: diagnostics = %#v", second.Diagnostics)
		}
	})

	t.Run("invalid Claude overlay", func(t *testing.T) {
		workspace := newWorkspace(t)
		writeCompilerFixture(t, workspace, "source/src/commands/resume-from.md.agentbundler/targets/claude.json", `{"frontmatterPatch":{"description":false}}`)
		result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest(model.TargetClaude), Mode: BuildModeBuild})
		if len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "description frontmatter") {
			t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
		}
		if len(result.Plan.Targets) != 0 {
			t.Fatalf("invalid command overlay produced target plans: %#v", result.Plan.Targets)
		}
		if _, err := os.Stat(filepath.Join(workspace, "generated")); !os.IsNotExist(err) {
			t.Fatalf("invalid command overlay generated output: %v", err)
		}
	})

	t.Run("Pi", func(t *testing.T) {
		workspace := newWorkspace(t)
		result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest(model.TargetPi), Mode: BuildModeBuild})
		if len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "asset.command") {
			t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
		}
		if len(result.Plan.Targets) != 0 {
			t.Fatalf("unsupported command produced target plans: %#v", result.Plan.Targets)
		}
		if _, err := os.Stat(filepath.Join(workspace, "generated")); !os.IsNotExist(err) {
			t.Fatalf("unsupported command generated output: %v", err)
		}
	})
}

func TestCompileRejectsSourceOutputOverlapBeforeImport(t *testing.T) {
	// Both Root and Output point at the same directory. The layout guard must
	// fail before source.Import runs (no import error, no output written).
	workspace := t.TempDir()
	manifest := skillsManifest(model.TargetClaude)
	manifest.Output = "source" // same as Root
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      manifest,
		Mode:          BuildModeCheck,
	})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-workspace-layout" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	// No output directory should exist: import and write were both blocked.
	if _, err := os.Stat(filepath.Join(workspace, "source", "claude")); !os.IsNotExist(err) {
		t.Fatal("importer or writer ran despite layout conflict")
	}
}

func TestCompileRejectsOutputInsideSourceBeforeImport(t *testing.T) {
	// Output is a subdirectory of source. A write would corrupt source files.
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	manifest := skillsManifest(model.TargetClaude)
	manifest.Output = "source/out" // nested inside source
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      manifest,
		Mode:          BuildModeBuild,
	})
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "invalid-workspace-layout" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	// Generated output must not appear inside the source tree.
	if _, err := os.Stat(filepath.Join(workspace, "source", "out")); !os.IsNotExist(err) {
		t.Fatal("output was written inside source tree")
	}
}

func TestCompileRejectsSymlinkedOutputAncestor(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	manifest := skillsManifest(model.TargetClaude)
	manifest.Output = "linked/generated"
	result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-output-root" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsAntigravityProjectProfile(t *testing.T) {
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	manifest := skillsManifest(model.TargetAntigravity)
	manifest.Composition = nil

	result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-target-profile" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsHookCapabilityInPiProjectProfile(t *testing.T) {
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	writeCompilerFixture(t, workspace, "source/.agentbundler/assets/skill/demo/asset.json", `{"capabilities":["asset.hook"]}`)

	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      skillsManifest(model.TargetPi),
		Mode:          BuildModeBuild,
	})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileNativeChecksRunFromTargetRoots(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake native validators require POSIX shell scripts")
	}

	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	manifest := skillsManifest(model.TargetClaude)
	manifest.Targets = []model.TargetID{model.TargetAntigravity, model.TargetClaude, model.TargetGrok}
	manifest.Composition = []model.TargetComposition{
		{Target: model.TargetAntigravity, Profile: model.TargetProfilePackage, PackageMode: model.TargetPackageModeSeparate},
		{Target: model.TargetClaude, Profile: model.TargetProfilePackage},
		{Target: model.TargetGrok, Profile: model.TargetProfilePackage},
	}

	build := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeBuild})
	if len(build.Diagnostics) != 0 {
		t.Fatalf("build diagnostics = %#v", build.Diagnostics)
	}

	bin := t.TempDir()
	logs := t.TempDir()
	writeFakeNativeValidator(t, bin, "agy", filepath.Join(logs, "agy"))
	writeFakeNativeValidator(t, bin, "claude", filepath.Join(logs, "claude"))
	writeFakeNativeValidator(t, bin, "grok", filepath.Join(logs, "grok"))
	t.Setenv("NATIVE_CHECK_LOG", logs)
	t.Setenv("PATH", bin)

	check := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeCheck, NativeVerify: true})
	if len(check.Diagnostics) != 0 || check.NativeVerificationFailed {
		t.Fatalf("native check result = %#v", check)
	}

	outputRoot, err := filepath.EvalSymlinks(filepath.Join(workspace, "generated"))
	if err != nil {
		t.Fatal(err)
	}
	assertNativeValidatorLog(t, filepath.Join(logs, "agy"), filepath.Join(outputRoot, "antigravity")+"\nplugin\nvalidate\n.\n")
	assertNativeValidatorLog(t, filepath.Join(logs, "claude"), filepath.Join(outputRoot, "claude")+"\nplugin\nvalidate\n--strict\n.\n")
	assertNativeValidatorLog(t, filepath.Join(logs, "grok"), filepath.Join(outputRoot, "grok")+"\nplugin\nvalidate\n.\n")
}

func TestNativeChecksPrefixTargetRelativeWorkingDirectories(t *testing.T) {
	packageRoot := model.RelativePath("packages/demo")
	plan := model.BuildPlan{Targets: []model.TargetPlan{
		{Target: model.TargetClaude, NativeChecks: []model.NativeCheck{{Program: "claude"}}},
		{Target: model.TargetGrok, NativeChecks: []model.NativeCheck{{Program: "grok", WorkingDirectory: &packageRoot}}},
	}}

	checks := nativeChecks(plan)
	claudeRoot := model.RelativePath("claude")
	grokPackageRoot := model.RelativePath("grok/packages/demo")
	want := []model.NativeCheck{
		{Program: "claude", WorkingDirectory: &claudeRoot},
		{Program: "grok", WorkingDirectory: &grokPackageRoot},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("nativeChecks() = %#v, want %#v", checks, want)
	}
	if plan.Targets[0].NativeChecks[0].WorkingDirectory != nil || *plan.Targets[1].NativeChecks[0].WorkingDirectory != "packages/demo" {
		t.Fatalf("nativeChecks() mutated target-relative checks: %#v", plan.Targets)
	}
}

func TestDistributionVersionOverridesAllGeneratedPackageAndAggregateVersions(t *testing.T) {
	packages := []model.NormalizedPackage{
		{Identity: "alpha", Metadata: model.PackageMetadata{"version": "1.0.0"}},
		{Identity: "beta", Metadata: model.PackageMetadata{}},
	}
	policy := model.TargetComposition{Aggregate: &model.AggregatePackage{Identity: "all", Metadata: model.PackageMetadata{"version": "2.0.0"}}}
	gotPolicy, gotPackages := applyDistributionVersion(model.DistributionMetadata{"version": "6.8.0"}, policy, packages)
	for _, pkg := range gotPackages {
		if pkg.Metadata["version"] != "6.8.0" {
			t.Fatalf("package %q version = %#v", pkg.Identity, pkg.Metadata["version"])
		}
	}
	if gotPolicy.Aggregate == nil || gotPolicy.Aggregate.Metadata["version"] != "6.8.0" {
		t.Fatalf("aggregate = %#v", gotPolicy.Aggregate)
	}
	if packages[0].Metadata["version"] != "1.0.0" || policy.Aggregate.Metadata["version"] != "2.0.0" {
		t.Fatal("version ownership mutated source input")
	}
}

func TestTargetRenderInputUsesManifestDistributionAndExplicitPackageMode(t *testing.T) {
	packages := []model.NormalizedPackage{
		{Identity: "zeta", Target: model.TargetPi, Profile: model.TargetProfilePackage},
		{Identity: "alpha", Target: model.TargetPi, Profile: model.TargetProfilePackage},
	}
	aggregate := &model.AggregatePackage{Identity: "team-tools", Metadata: model.PackageMetadata{"version": "1.0.0"}}
	manifest := model.SourceManifest{Distribution: model.DistributionMetadata{"name": "Team tools"}}
	policy := model.TargetComposition{
		Target:      model.TargetPi,
		Profile:     model.TargetProfilePackage,
		PackageMode: model.TargetPackageModeAggregate,
		Aggregate:   aggregate,
	}

	input := targetRenderInput(manifest, policy, packages)
	if got := []model.PackageID{input.Packages[0].Identity, input.Packages[1].Identity}; !reflect.DeepEqual(got, []model.PackageID{"alpha", "zeta"}) {
		t.Fatalf("package order = %#v", got)
	}
	if input.PackageMode != model.TargetPackageModeAggregate || input.Aggregate != aggregate || !reflect.DeepEqual(input.Distribution, manifest.Distribution) {
		t.Fatalf("targetRenderInput() = %#v", input)
	}

	compatibility := targetRenderInput(model.SourceManifest{}, model.TargetComposition{}, packages)
	if compatibility.PackageMode != model.TargetPackageModeSeparate || compatibility.Aggregate != nil {
		t.Fatalf("compatibility targetRenderInput() = %#v", compatibility)
	}
}

func TestSelectPackagesFiltersOnlyPackageOwnedNativeGaps(t *testing.T) {
	ownedAsset := model.AssetID("native-resource/core")
	missingAsset := model.AssetID("native-resource/missing")
	inventory := model.SourceInventory{
		Packages: []model.SourcePackage{
			{Identity: "core", Assets: []model.SourceAsset{{Identity: ownedAsset}}},
			{Identity: "workflow", Assets: []model.SourceAsset{{Identity: "skill/release"}}},
		},
		NativeGaps: []model.NativeGap{
			{Package: "core", Component: "core", Asset: &ownedAsset},
			{Package: "workflow", Component: "missing", Asset: &missingAsset},
			{Package: "workflow", Component: "assetless"},
		},
	}
	var diagnostics []model.Diagnostic

	selected := selectPackages(inventory, []model.PackageID{"workflow"}, &diagnostics)

	if len(diagnostics) != 0 || len(selected.Packages) != 1 || selected.Packages[0].Identity != "workflow" {
		t.Fatalf("selectPackages() = (%#v, %#v)", selected, diagnostics)
	}
	if want := inventory.NativeGaps[1:]; !reflect.DeepEqual(selected.NativeGaps, want) {
		t.Fatalf("native gaps = %#v, want %#v", selected.NativeGaps, want)
	}
}

func TestCompilePackageSelectorKeepsSelectedNativeResourceWithCrossTargetDuplicateID(t *testing.T) {
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/packages/pi.json", `{"id":"pi-only","metadata":{},"assets":[{"path":"src/plugins/pi/shared","targets":["pi"]}]}`)
	writeCompilerFixture(t, workspace, "source/packages/antigravity.json", `{"id":"antigravity-only","metadata":{},"assets":[{"path":"src/plugins/antigravity/shared","targets":["antigravity"]}]}`)
	writeCompilerFixture(t, workspace, "source/src/plugins/pi/shared/.agentbundler/asset.json", `{"capabilities":["asset.native-resource"],"piExtensions":["extensions/shared.ts"]}`)
	writeCompilerFixture(t, workspace, "source/src/plugins/pi/shared/extensions/shared.ts", "export default function shared() {}\n")
	writeCompilerFixture(t, workspace, "source/src/plugins/antigravity/shared/.agentbundler/asset.json", `{"capabilities":["asset.native-resource"]}`)
	writeCompilerFixture(t, workspace, "source/src/plugins/antigravity/shared/rules/shared.md", "# Shared\n")
	manifest := model.SourceManifest{
		Version: 1,
		Kind:    model.SourceKindBundle,
		Root:    "source",
		Targets: []model.TargetID{model.TargetPi, model.TargetAntigravity},
		Output:  "generated",
		Composition: []model.TargetComposition{{
			Target: model.TargetPi, Profile: model.TargetProfilePackage, PackageMode: model.TargetPackageModeAggregate,
			Aggregate: &model.AggregatePackage{Identity: "selected", Metadata: model.PackageMetadata{"version": "1.0.0"}},
		}},
		Bundle: &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/pi.json", "packages/antigravity.json"}},
	}

	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      manifest,
		Targets:       []model.TargetID{model.TargetPi},
		Packages:      []model.PackageID{"pi-only"},
		Mode:          BuildModeBuild,
	})
	if len(result.Diagnostics) != 0 || len(result.Plan.Targets) != 1 {
		t.Fatalf("Compile() = (%#v, %#v)", result.Plan, result.Diagnostics)
	}
	for _, file := range result.Plan.Targets[0].Files {
		if file.Path == "extensions/shared.ts" && string(file.Bytes) == "export default function shared() {}\n" {
			return
		}
	}
	t.Fatalf("selected Pi native resource missing from files %#v", result.Plan.Targets[0].Files)
}

func TestCompileRejectsPackageSelectorWithRepositoryRootCompatibility(t *testing.T) {
	workspace := t.TempDir()
	manifest := model.SourceManifest{
		Version: 1, Kind: model.SourceKindBundle, Root: "source", Output: "dist",
		Targets:       []model.TargetID{model.TargetClaude},
		Distribution:  model.DistributionMetadata{"name": "tools"},
		Compatibility: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude}},
		Composition: []model.TargetComposition{{
			Target: model.TargetClaude, Profile: model.TargetProfilePackage, PackageMode: model.TargetPackageModeSeparate,
		}},
		Bundle: &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/base.json"}},
	}
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest,
		Packages: []model.PackageID{"base"}, Mode: BuildModeBuild,
	})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "compatibility-incomplete-selection" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsUndeclaredTargetBeforeFilesystemWork(t *testing.T) {
	root := t.TempDir()
	manifest := model.SourceManifest{
		Version: 1,
		Kind:    model.SourceKindBundle,
		Root:    "source",
		Targets: []model.TargetID{model.TargetClaude},
		Output:  "generated",
		Bundle:  &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/base.json"}},
	}
	for _, targetID := range []model.TargetID{model.TargetAntigravity, model.TargetCodex} {
		result := Compile(CompileRequest{
			WorkspaceRoot: filepath.Clean(root),
			Manifest:      manifest,
			Targets:       []model.TargetID{targetID},
			Mode:          BuildModeCheck,
		})
		if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "invalid-target-selector" {
			t.Fatalf("Compile(%q) diagnostics = %#v", targetID, result.Diagnostics)
		}
	}
}

func skillsManifest(target model.TargetID) model.SourceManifest {
	manifest := model.SourceManifest{
		Version: 1,
		Kind:    model.SourceKindSkillsRepository,
		Root:    "source",
		Targets: []model.TargetID{target},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package:  "demo",
			Roots:    []model.RelativePath{"skills"},
			Metadata: model.PackageMetadata{},
		},
	}
	if target == model.TargetAntigravity {
		manifest.Composition = []model.TargetComposition{{
			Target: target, Profile: model.TargetProfilePackage, PackageMode: model.TargetPackageModeSeparate,
		}}
	}
	return manifest
}

func writeCompilerFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func writeFakeNativeValidator(t *testing.T, root, name, logPath string) {
	t.Helper()
	content := "#!/bin/sh\nif [ \"${NATIVE_CHECK_LOG+x}\" = x ]; then exit 90; fi\n{\n  pwd -P\n  printf '%s\\n' \"$@\"\n} > " + shellTestQuote(logPath) + "\n"
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func shellTestQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func assertNativeValidatorLog(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("native validator log = %q, want %q", data, want)
	}
}

func TestCapabilityCeilingRejectsUpgradeFromUnsupported(t *testing.T) {
	t.Parallel()
	// Build a skills-repository workspace.
	root := t.TempDir()
	writeCompilerFixture(t, root, "source/skills/example/SKILL.md", "Example skill.")
	manifest := model.SourceManifest{
		Kind:    model.SourceKindSkillsRepository,
		Root:    "source",
		Targets: []model.TargetID{model.TargetClaude},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package: "pkg", Roots: []model.RelativePath{"skills"}, Metadata: model.PackageMetadata{},
		},
		// Manifest tries to upgrade an unsupported capability to native.
		Composition: []model.TargetComposition{{
			Target: model.TargetClaude,
			Capabilities: []model.CapabilityRule{
				{Key: model.CapabilityKeyAgentPluginMCPStdio, State: model.CapabilityStateNative},
			},
		}},
	}
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(root),
		Manifest:      manifest,
		Mode:          BuildModeCheck,
	})
	hasCeiling := false
	for _, d := range result.Diagnostics {
		if d.Code == "capability-ceiling-upgrade" {
			hasCeiling = true
		}
	}
	if !hasCeiling {
		t.Fatalf("expected capability-ceiling-upgrade diagnostic, got: %v", result.Diagnostics)
	}
}

func TestCompileRejectsAgentPluginMCPForVendorTarget(t *testing.T) {
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/plugin/plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"plugin"}`)
	writeCompilerFixture(t, workspace, "source/plugin/mcp.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"srv":{"type":"stdio","command":"node"}}}`)
	manifest := model.SourceManifest{
		Version:     1,
		Kind:        model.SourceKindAgentPlugin,
		Root:        "source",
		Targets:     []model.TargetID{model.TargetClaude},
		Output:      "generated",
		AgentPlugin: &model.AgentPluginSourceConfig{Plugins: []model.RelativePath{"plugin"}},
	}

	result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0].Message, "uses unsupported capability \"agent-plugin.mcp.stdio\"") {
		t.Fatalf("Compile() diagnostics = %#v; want unsupported MCP capability", result.Diagnostics)
	}
	if len(result.Plan.Targets) != 0 {
		t.Fatalf("Compile() rendered vendor target despite unsupported MCP: %#v", result.Plan.Targets)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated")); !os.IsNotExist(err) {
		t.Fatalf("Compile() mutated output: %v", err)
	}
}

func TestCapabilityCeilingRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCompilerFixture(t, root, "source/skills/example/SKILL.md", "Example skill.")
	manifest := model.SourceManifest{
		Kind:    model.SourceKindSkillsRepository,
		Root:    "source",
		Targets: []model.TargetID{model.TargetClaude},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package: "pkg", Roots: []model.RelativePath{"skills"}, Metadata: model.PackageMetadata{},
		},
		// Manifest references a key that the adapter does not declare.
		Composition: []model.TargetComposition{{
			Target: model.TargetClaude,
			Capabilities: []model.CapabilityRule{
				{Key: "unknown.capability.key", State: model.CapabilityStateUnsupported},
			},
		}},
	}
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(root),
		Manifest:      manifest,
		Mode:          BuildModeCheck,
	})
	hasUnknown := false
	for _, d := range result.Diagnostics {
		if d.Code == "unknown-capability-key" {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Fatalf("expected unknown-capability-key diagnostic, got: %v", result.Diagnostics)
	}
}

func TestCapabilityCeilingRejectsNativeEquivalentSubstitution(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCompilerFixture(t, root, "source/skills/example/SKILL.md", "Example skill.")
	manifest := model.SourceManifest{
		Kind:    model.SourceKindSkillsRepository,
		Root:    "source",
		Targets: []model.TargetID{model.TargetClaude},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package: "pkg", Roots: []model.RelativePath{"skills"}, Metadata: model.PackageMetadata{},
		},
		// Manifest tries to substitute native→equivalent for a native capability.
		Composition: []model.TargetComposition{{
			Target: model.TargetClaude,
			Capabilities: []model.CapabilityRule{
				{Key: "asset.skill", State: model.CapabilityStateEquivalent},
			},
		}},
	}
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(root),
		Manifest:      manifest,
		Mode:          BuildModeCheck,
	})
	hasSubstitution := false
	for _, d := range result.Diagnostics {
		if d.Code == "capability-ceiling-substitution" {
			hasSubstitution = true
		}
	}
	if !hasSubstitution {
		t.Fatalf("expected capability-ceiling-substitution diagnostic, got: %v", result.Diagnostics)
	}
}
