package pi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderUsesPiNativeSkillRoot(t *testing.T) {
	plan, diagnostics := Render(separate(skillPackage()))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path}, []model.RelativePath{".pi/skills/guide/SKILL.md", ".pi/skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestProjectRenderRejectsAgent(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Assets[0].Kind = model.AssetKindAgent
	pkg.Assets[0].Identity = "agent/reviewer"
	_, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func TestPackageRenderIncludesPiSubagent(t *testing.T) {
	pkg := model.NormalizedPackage{
		Identity: "demo",
		Target:   Target,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{"version": "1.0.0", "dependencies": map[string]any{"pi-subagents": "^1.0.0"}},
		Assets: []model.NormalizedAsset{{
			Identity: "agent/reviewer",
			Kind:     model.AssetKindAgent,
			Content: model.AssetContent{
				Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"},
				Body:        "Review.\n",
				Files:       map[model.RelativePath]model.FileContent{},
			},
		}},
	}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	paths := make(map[model.RelativePath]bool, len(plan.Files))
	for _, file := range plan.Files {
		paths[file.Path] = true
	}
	for _, path := range []model.RelativePath{"agents/reviewer.md", "node_modules/pi-subagents/src/extension/index.ts"} {
		if !paths[path] {
			t.Fatalf("plan files missing %q", path)
		}
	}
}

func TestRuntimeSchemaFixtureMatchesPortableModel(t *testing.T) {
	data, err := os.ReadFile("runtime/testdata/hooks.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version int                    `json:"version"`
		Hooks   []model.HookDescriptor `json:"hooks"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatalf("schema version = %d, want 1", fixture.Version)
	}
	if got, want := len(fixture.Hooks), 2; got != want {
		t.Fatalf("hook count = %d, want %d", got, want)
	}
	if got, want := fixture.Hooks[1].Event, model.HookEventPreTool; got != want {
		t.Fatalf("tool hook event = %q, want %q", got, want)
	}
	if fixture.Hooks[1].Handler.Arguments[1].PackageFile == nil || *fixture.Hooks[1].Handler.Arguments[1].PackageFile != "hooks/payloads/pre-tool/pre-tool.js" {
		t.Fatalf("package-file argument = %#v", fixture.Hooks[1].Handler.Arguments[1])
	}
	assets := make([]model.NormalizedAsset, 0, len(fixture.Hooks))
	for index := range fixture.Hooks {
		descriptor := fixture.Hooks[index]
		files := make(map[model.RelativePath]model.FileContent)
		for _, argument := range descriptor.Handler.Arguments {
			if argument.PackageFile != nil {
				files[*argument.PackageFile] = model.FileContent{Bytes: []byte("fixture\n")}
			}
		}
		assets = append(assets, model.NormalizedAsset{
			Identity: descriptor.Identity,
			Kind:     model.AssetKindHook,
			Content:  model.AssetContent{Files: files},
			Hook:     &descriptor,
		})
	}
	pkg := model.NormalizedPackage{Identity: "fixture", Target: Target, Profile: model.TargetProfilePackage, Assets: assets}
	if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		t.Fatalf("Go rejected shared runtime descriptor fixture: %#v", diagnostics)
	}
}

func TestShellHookFixtureMatchesGoModelAndGeneratedRuntimeBytes(t *testing.T) {
	data, err := os.ReadFile("runtime/testdata/shell-hook.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture hookConfigV1
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 || len(fixture.Hooks) != 1 {
		t.Fatalf("shell fixture = %#v, want one v1 hook", fixture)
	}

	asset := model.NormalizedAsset{
		Identity: fixture.Hooks[0].Identity,
		Kind:     model.AssetKindHook,
		Content:  model.AssetContent{Files: map[model.RelativePath]model.FileContent{}},
		Hook:     &fixture.Hooks[0],
		CapabilityUses: []model.CapabilityUse{
			{Key: "asset.hook", Location: fixture.Hooks[0].Location},
			{Key: "hook.command.shell", Location: fixture.Hooks[0].Location},
			{Key: "hook.event.session-start", Location: fixture.Hooks[0].Location},
		},
	}
	pkg := model.NormalizedPackage{Identity: "fixture", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		t.Fatalf("Go rejected shared shell fixture: %#v", diagnostics)
	}

	pkg.Assets[0].Hook.Handler.Arguments = nil
	plan, diagnostics := Render(model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{pkg},
		PackageMode: model.TargetPackageModeAggregate,
		Aggregate:   &model.AggregatePackage{Identity: "shell-fixture", Metadata: model.PackageMetadata{"version": "1.0.0"}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	for _, file := range plan.Files {
		if file.Path == hookDescriptorPath {
			if !reflect.DeepEqual(file.Bytes, data) {
				t.Fatalf("generated shell descriptor does not match shared fixture\ngot:  %s\nwant: %s", file.Bytes, data)
			}
			return
		}
	}
	t.Fatalf("generated files = %#v, want %q", plan.Files, hookDescriptorPath)
}

func TestRuntimeHookOrderFixtureMatchesPortableModel(t *testing.T) {
	data, err := os.ReadFile("runtime/testdata/hook-order.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Config struct {
			Version int                    `json:"version"`
			Hooks   []model.HookDescriptor `json:"hooks"`
		} `json:"config"`
		ExpectedIdentities []model.AssetID `json:"expectedIdentities"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Config.Version != 1 {
		t.Fatalf("schema version = %d, want 1", fixture.Config.Version)
	}
	model.SortHookDescriptors(fixture.Config.Hooks)
	got := make([]model.AssetID, len(fixture.Config.Hooks))
	for index, hook := range fixture.Config.Hooks {
		got[index] = hook.Identity
	}
	if !reflect.DeepEqual(got, fixture.ExpectedIdentities) {
		t.Fatalf("Go hook order = %#v, want %#v", got, fixture.ExpectedIdentities)
	}
}

func TestCapabilitiesExposeAggregatePiHooksAndSubagents(t *testing.T) {
	if FormatRevision != 7 {
		t.Fatalf("FormatRevision = %d, want 7", FormatRevision)
	}
	rules := make(map[model.CapabilityKey]model.CapabilityState)
	for _, rule := range Capabilities() {
		rules[rule.Key] = rule.State
	}
	want := map[model.CapabilityKey]model.CapabilityState{
		"asset.agent":                 model.CapabilityStateEquivalent,
		"asset.hook":                  model.CapabilityStateNative,
		"asset.native-resource":       model.CapabilityStateNative,
		"hook.command.exec":           model.CapabilityStateNative,
		"hook.command.shell":          model.CapabilityStateNative,
		"hook.decision.block":         model.CapabilityStateNative,
		"hook.decision.rewrite-input": model.CapabilityStateNative,
		"hook.event.notification":     model.CapabilityStateUnsupported,
		"hook.failure.closed":         model.CapabilityStateNative,
	}
	for key, state := range want {
		if rules[key] != state {
			t.Fatalf("capability %q = %q, want %q", key, rules[key], state)
		}
	}
}

func separate(packages []model.NormalizedPackage) model.TargetRenderInput {
	return model.TargetRenderInput{Packages: packages, PackageMode: model.TargetPackageModeSeparate}
}

func skillPackage() []model.NormalizedPackage {
	return []model.NormalizedPackage{{Identity: "demo", Target: Target, Assets: []model.NormalizedAsset{{
		Identity: "skill/guide", Kind: model.AssetKindSkill,
		Content:        model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "# Guide\n", Files: map[model.RelativePath]model.FileContent{"docs/readme.md": {Bytes: []byte("help")}}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/SKILL.md"}}},
	}}}}
}
