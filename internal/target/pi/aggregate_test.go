package pi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderAggregateEmitsOneInstallablePiPackage(t *testing.T) {
	input := aggregateFixture()
	plan, diagnostics := Render(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := plan.Packages, []model.PackageID{"team-tools"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}

	for _, path := range []model.RelativePath{
		"README.md", "agents/reviewer.md", "extensions/_agentbundler-hooks/index.ts",
		"extensions/agentbundler-hooks.ts", "hooks/hooks.v1.json",
		"hooks/payloads/pre-tool/pre-tool.js", "package.json", "skills/guide/SKILL.md",
	} {
		_ = aggregateFile(t, plan, path)
	}

	var manifest map[string]any
	if err := json.Unmarshal(aggregateFile(t, plan, "package.json").Bytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["name"] != "team-tools" || manifest["version"] != "2.0.0" || manifest["description"] != "Explicit aggregate metadata" {
		t.Fatalf("aggregate manifest metadata = %#v", manifest)
	}
	dependencies, ok := manifest["dependencies"].(map[string]any)
	if !ok || !reflect.DeepEqual(dependencies, map[string]any{"aggregate-runtime": "1.0.0", "author-runtime": "^1.0.0", "shared": "2.0.0"}) {
		t.Fatalf("dependencies = %#v", manifest["dependencies"])
	}
	piManifest, ok := manifest["pi"].(map[string]any)
	if !ok {
		t.Fatalf("pi manifest = %#v", manifest["pi"])
	}
	if got, want := piManifest["extensions"], []any{"./extensions/agentbundler-hooks.ts"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pi.extensions = %#v, want %#v", got, want)
	}
	if got, want := piManifest["skills"], []any{"./skills"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pi.skills = %#v, want %#v", got, want)
	}
	if got, want := piManifest["subagents"], map[string]any{"agents": []any{"./agents"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pi.subagents = %#v, want %#v", got, want)
	}

	fixture := readTestFile(t, "runtime/testdata/hooks.v1.json")
	if got := aggregateFile(t, plan, hookDescriptorPath).Bytes; !reflect.DeepEqual(got, fixture) {
		t.Fatalf("generated descriptor does not match shared fixture\ngot:  %s\nwant: %s", got, fixture)
	}
	adapter := string(aggregateFile(t, plan, hookAdapterPath).Bytes)
	for _, text := range []string{"./_agentbundler-hooks/index.js", "hooks/hooks.v1.json", "createPiHookRuntime(pi, config, { packageRoot })"} {
		if !strings.Contains(adapter, text) {
			t.Fatalf("thin adapter does not contain %q:\n%s", text, adapter)
		}
	}
}

func TestRenderAggregateRegistersDeclarativeNativeExtension(t *testing.T) {
	input := aggregateFixture()
	input.Packages[0].Assets = append(input.Packages[0].Assets, model.NormalizedAsset{
		Identity: "native-resource/custom-extension", Kind: model.AssetKindNativeResource,
		Content: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{
			"extensions/custom.ts": {Bytes: []byte("export default function custom() {}\n")},
		}},
		Native:         &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extensions/custom.ts"}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.native-resource", Location: model.SourceLocation{Path: "source/plugins/pi/custom-extension/.agentbundler/asset.json"}}},
	})
	plan, diagnostics := Render(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got := string(aggregateFile(t, plan, "extensions/custom.ts").Bytes); got != "export default function custom() {}\n" {
		t.Fatalf("native extension = %q", got)
	}
	var manifest map[string]any
	if err := json.Unmarshal(aggregateFile(t, plan, "package.json").Bytes, &manifest); err != nil {
		t.Fatal(err)
	}
	piManifest := manifest["pi"].(map[string]any)
	if got, want := piManifest["extensions"], []any{"./extensions/custom.ts", "./extensions/agentbundler-hooks.ts"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pi.extensions = %#v, want %#v", got, want)
	}
}

func TestRenderAggregateRejectsNativeExtensionNotInResourceTree(t *testing.T) {
	input := aggregateFixture()
	input.Packages[0].Assets = append(input.Packages[0].Assets, model.NormalizedAsset{
		Identity: "native-resource/custom-extension", Kind: model.AssetKindNativeResource,
		Content:        model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{"extensions/custom.ts": {Bytes: []byte("export default function custom() {}\n")}}},
		Native:         &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extensions/missing.ts"}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.native-resource", Location: model.SourceLocation{Path: "source/plugins/pi/custom-extension/.agentbundler/asset.json"}}},
	})
	plan, diagnostics := Render(input)
	if len(diagnostics) != 1 || diagnostics[0].Code != "invalid-package-output" || !strings.Contains(diagnostics[0].Message, "does not name a resource file") {
		t.Fatalf("Render() = (%#v, %#v), want invalid native extension diagnostic", plan, diagnostics)
	}
}

func TestAggregateRuntimeBytesAreEmbeddedDeterministically(t *testing.T) {
	plan, diagnostics := Render(aggregateFixture())
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	runtime, err := runtimeFiles()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	for _, source := range runtime {
		path := model.RelativePath(embeddedRuntimeRoot + "/" + source.name)
		file := aggregateFile(t, plan, path)
		if !reflect.DeepEqual(file.Bytes, source.bytes) {
			t.Fatalf("embedded runtime %q differs from generated bytes", path)
		}
		hash.Write([]byte(source.name))
		hash.Write([]byte{0})
		hash.Write(source.bytes)
	}
	if got, want := hex.EncodeToString(hash.Sum(nil)), "4c7b39d79ed4a61b87ad4f5a10888e99187bc5bf034b41fbaedfa03015ccffa9"; got != want {
		t.Fatalf("embedded runtime hash = %q, want %q", got, want)
	}

	reversed := aggregateFixture()
	reversed.Packages[0], reversed.Packages[1] = reversed.Packages[1], reversed.Packages[0]
	reversed.Packages[0].Assets[0], reversed.Packages[0].Assets[1] = reversed.Packages[0].Assets[1], reversed.Packages[0].Assets[0]
	reversedPlan, diagnostics := Render(reversed)
	if len(diagnostics) != 0 {
		t.Fatalf("Render(reversed) diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(plan, reversedPlan) {
		t.Fatalf("reordered aggregate input changed plan\nfirst: %#v\nsecond: %#v", plan, reversedPlan)
	}
}

func TestAggregatePreservesExplicitDependenciesWhenAgentsAreAbsent(t *testing.T) {
	input := aggregateFixture()
	input.Packages[1].Assets = input.Packages[1].Assets[:1]
	plan, diagnostics := Render(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	var manifest map[string]any
	if err := json.Unmarshal(aggregateFile(t, plan, "package.json").Bytes, &manifest); err != nil {
		t.Fatal(err)
	}
	dependencies := manifest["dependencies"].(map[string]any)
	if dependencies["author-runtime"] != "^1.0.0" {
		t.Fatalf("agent-free dependencies = %#v, want explicit dependency preserved", dependencies)
	}
	piManifest := manifest["pi"].(map[string]any)
	if _, exists := piManifest["subagents"]; exists {
		t.Fatalf("agent-free pi manifest = %#v, want subagents omitted", piManifest)
	}
	var config hookConfigV1
	if err := json.Unmarshal(aggregateFile(t, plan, hookDescriptorPath).Bytes, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Hooks) != 2 {
		t.Fatalf("agent-free hook count = %d, want 2", len(config.Hooks))
	}
}

func TestAggregateWithoutHooksStillRegistersOneThinAdapterAndEmptyDescriptor(t *testing.T) {
	input := aggregateFixture()
	input.Packages[0].Assets = input.Packages[0].Assets[:1]
	input.Packages[1].Assets = input.Packages[1].Assets[1:]
	plan, diagnostics := Render(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := string(aggregateFile(t, plan, hookDescriptorPath).Bytes), "{\"version\":1,\"hooks\":[]}\n"; got != want {
		t.Fatalf("empty descriptor = %q, want %q", got, want)
	}
	var manifest map[string]any
	if err := json.Unmarshal(aggregateFile(t, plan, "package.json").Bytes, &manifest); err != nil {
		t.Fatal(err)
	}
	piManifest := manifest["pi"].(map[string]any)
	if got, want := piManifest["extensions"], []any{"./extensions/agentbundler-hooks.ts"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pi.extensions = %#v, want %#v", got, want)
	}
}

func TestAggregateRejectsFailClosedOutsidePreTool(t *testing.T) {
	for _, test := range []struct {
		event      model.HookEvent
		capability model.CapabilityKey
	}{
		{event: model.HookEventSessionStart, capability: "hook.event.session-start"},
		{event: model.HookEventPostTool, capability: "hook.event.post-tool"},
	} {
		t.Run(string(test.event), func(t *testing.T) {
			input := aggregateFixture()
			asset := &input.Packages[0].Assets[1]
			asset.Hook.Event = test.event
			asset.Hook.FailurePolicy = model.HookFailurePolicyClosed
			asset.CapabilityUses[2].Key = test.capability
			asset.CapabilityUses = append(asset.CapabilityUses, model.CapabilityUse{
				Key:      "hook.failure.closed",
				Location: asset.Hook.Location,
			})

			_, diagnostics := Render(input)
			assertDiagnosticContains(t, diagnostics, "unsupported-hook-semantics",
				"hook.failure.closed is enforceable only for Pi pre-tool hooks", string(test.event))
			if diagnostics[0].Location == nil || diagnostics[0].Location.Path != asset.Hook.Location.Path {
				t.Fatalf("diagnostic location = %#v, want %q", diagnostics[0].Location, asset.Hook.Location.Path)
			}
		})
	}
}

func TestAggregateConflictDiagnosticsNameBothOwners(t *testing.T) {
	t.Run("dependency", func(t *testing.T) {
		input := aggregateFixture()
		input.Packages[1].Metadata["dependencies"] = map[string]any{"shared": "3.0.0"}
		_, diagnostics := Render(input)
		assertDiagnosticContains(t, diagnostics, "invalid-model", `aggregate dependency "shared" conflicts between package "alpha" ("2.0.0") and package "zeta" ("3.0.0")`)
	})

	t.Run("asset name", func(t *testing.T) {
		input := aggregateFixture()
		duplicate := input.Packages[0].Assets[0]
		duplicate.CapabilityUses = append([]model.CapabilityUse(nil), duplicate.CapabilityUses...)
		duplicate.CapabilityUses[0].Location.Path = "source/zeta/skills/guide/SKILL.md"
		input.Packages[1].Assets = append(input.Packages[1].Assets, duplicate)
		_, diagnostics := Render(input)
		assertDiagnosticContains(t, diagnostics, "aggregate-asset-conflict", "source/alpha/skills/guide/SKILL.md", "source/zeta/skills/guide/SKILL.md", "skill/guide")
	})

	t.Run("output path", func(t *testing.T) {
		input := aggregateFixture()
		content := input.Packages[0].Assets[0].Content
		content.Files["SKILL.md"] = model.FileContent{Bytes: []byte("collision\n"), Origin: []model.SourceLocation{{Path: "source/alpha/skills/guide/SKILL.md.payload"}}}
		input.Packages[0].Assets[0].Content = content
		_, diagnostics := Render(input)
		assertDiagnosticContains(t, diagnostics, "aggregate-path-conflict", "skills/guide/SKILL.md", "source/alpha/skills/guide/SKILL.md", "source/alpha/skills/guide/SKILL.md.payload")
	})

	t.Run("identity", func(t *testing.T) {
		input := aggregateFixture()
		input.Packages[1].Identity = input.Packages[0].Identity
		_, diagnostics := Render(input)
		assertDiagnosticContains(t, diagnostics, "invalid-model", `target render input package "alpha" is duplicated`)
	})

	t.Run("metadata", func(t *testing.T) {
		input := aggregateFixture()
		input.Aggregate.Metadata = nil
		_, diagnostics := Render(input)
		assertDiagnosticContains(t, diagnostics, "invalid-model", "aggregate package metadata must be explicitly provided")
	})
}

func aggregateFixture() model.TargetRenderInput {
	program := "node"
	literalDashE := "-e"
	literalProgram := "process.stdin.resume()"
	payloadPath := model.RelativePath("pre-tool.js")
	alpha := model.NormalizedPackage{
		Identity: "alpha",
		Metadata: model.PackageMetadata{
			"version":      "source-version-is-not-aggregate-metadata",
			"dependencies": map[string]any{"shared": "2.0.0"},
		},
		Target:  Target,
		Profile: model.TargetProfilePackage,
		Assets: []model.NormalizedAsset{
			{
				Identity: "skill/guide",
				Kind:     model.AssetKindSkill,
				Content: model.AssetContent{
					Frontmatter: map[string]any{"name": "guide"},
					Body:        "# Guide\n",
					Files: map[model.RelativePath]model.FileContent{
						"docs/readme.md": {Bytes: []byte("Guide help.\n"), Origin: []model.SourceLocation{{Path: "source/alpha/skills/guide/docs/readme.md"}}},
					},
				},
				CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/alpha/skills/guide/SKILL.md"}}},
			},
			{
				Identity: "hook/session",
				Kind:     model.AssetKindHook,
				Content:  model.AssetContent{Files: map[model.RelativePath]model.FileContent{}},
				Hook: &model.HookDescriptor{
					Identity:            "hook/session",
					Location:            model.SourceLocation{Path: "src/hooks/session/hook.json"},
					Event:               model.HookEventSessionStart,
					Handler:             model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: []model.HookArgument{{Literal: &literalDashE}, {Literal: &literalProgram}}},
					TimeoutMilliseconds: 10_000,
					FailurePolicy:       model.HookFailurePolicyOpen,
					Order:               10,
				},
				CapabilityUses: []model.CapabilityUse{
					{Key: "asset.hook", Location: model.SourceLocation{Path: "src/hooks/session/hook.json"}},
					{Key: "hook.command.exec", Location: model.SourceLocation{Path: "src/hooks/session/hook.json"}},
					{Key: "hook.event.session-start", Location: model.SourceLocation{Path: "src/hooks/session/hook.json"}},
				},
			},
		},
	}
	zeta := model.NormalizedPackage{
		Identity: "zeta",
		Metadata: model.PackageMetadata{
			"description":  "source description is ignored",
			"dependencies": map[string]any{"shared": "2.0.0"},
		},
		Target:  Target,
		Profile: model.TargetProfilePackage,
		Assets: []model.NormalizedAsset{
			{
				Identity: "hook/pre-tool",
				Kind:     model.AssetKindHook,
				Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
					payloadPath: {Bytes: []byte("process.stdin.resume();\n"), Origin: []model.SourceLocation{{Path: "src/hooks/pre-tool/pre-tool.js"}}},
				}},
				Hook: &model.HookDescriptor{
					Identity:            "hook/pre-tool",
					Location:            model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"},
					Event:               model.HookEventPreTool,
					Matcher:             &model.HookMatcher{Tools: []model.HookToolCategory{model.HookToolCategoryCommand}},
					Handler:             model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: []model.HookArgument{{Literal: &literalDashE}, {PackageFile: &payloadPath}}},
					TimeoutMilliseconds: 5_000,
					FailurePolicy:       model.HookFailurePolicyClosed,
					Order:               20,
				},
				CapabilityUses: []model.CapabilityUse{
					{Key: "asset.hook", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
					{Key: "hook.command.exec", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
					{Key: "hook.decision.block", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
					{Key: "hook.decision.rewrite-input", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
					{Key: "hook.event.pre-tool", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
					{Key: "hook.failure.closed", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
					{Key: "hook.matcher.tool-category", Location: model.SourceLocation{Path: "src/hooks/pre-tool/hook.json"}},
				},
			},
			{
				Identity: "agent/reviewer",
				Kind:     model.AssetKindAgent,
				Content: model.AssetContent{
					Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"},
					Body:        "Review.\n",
					Files:       map[model.RelativePath]model.FileContent{},
				},
				CapabilityUses: []model.CapabilityUse{{Key: "asset.agent", Location: model.SourceLocation{Path: "source/zeta/agents/reviewer.md"}}},
			},
		},
	}
	return model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{alpha, zeta},
		PackageMode: model.TargetPackageModeAggregate,
		Aggregate: &model.AggregatePackage{
			Identity: "team-tools",
			Metadata: model.PackageMetadata{
				"version":     "2.0.0",
				"description": "Explicit aggregate metadata",
				"dependencies": map[string]any{
					"aggregate-runtime": "1.0.0",
					"author-runtime":    "^1.0.0",
				},
			},
		},
	}
}

func aggregateFile(t *testing.T, plan model.TargetPlan, path model.RelativePath) model.PlannedFile {
	t.Helper()
	for _, file := range plan.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("planned file %q is missing", path)
	return model.PlannedFile{}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertDiagnosticContains(t *testing.T, diagnostics []model.Diagnostic, code string, texts ...string) {
	t.Helper()
	if len(diagnostics) == 0 || diagnostics[0].Code != code {
		t.Fatalf("diagnostics = %#v, want first code %q", diagnostics, code)
	}
	for _, text := range texts {
		if !strings.Contains(diagnostics[0].Message, text) {
			t.Fatalf("diagnostic %q does not contain %q", diagnostics[0].Message, text)
		}
	}
}
