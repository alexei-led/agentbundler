package packageoutput_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

func TestRenderWithCodecRendersSortedImmutableHookInputsAndPayloadMetadata(t *testing.T) {
	alpha := hookPackage("alpha", hookFixture("only", 2, "scripts/alpha.sh", false, "src/alpha/only.sh"))
	alpha.Assets = append(alpha.Assets, skillFixture())
	zeta := hookPackage("zeta",
		hookFixture("late", 20, "scripts/late.sh", false, "src/zeta/late.sh"),
		hookFixture("early", 10, "scripts/early.sh", true, "src/zeta/early.sh"),
	)
	zeta.Assets = append([]model.NormalizedAsset{skillFixture()}, zeta.Assets...)

	var callbackPackages []model.PackageID
	manifestBytes := []byte("target-owned\n")
	codec := hookCodec(func(input packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
		callbackPackages = append(callbackPackages, input.PackageID())
		hooks := input.Hooks()
		if input.PackageID() == "zeta" {
			got := []model.AssetID{hooks[0].Descriptor().Identity, hooks[1].Descriptor().Identity}
			if want := []model.AssetID{"hook/early", "hook/late"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("hook callback order = %#v, want %#v", got, want)
			}

			descriptor := hooks[0].Descriptor()
			*descriptor.Handler.Program = "changed"
			*descriptor.Handler.Arguments[0].PackageFile = "changed.sh"
			payload := hooks[0].PayloadFiles()
			payloadBytes := payload[0].Bytes()
			payloadBytes[0] = 'X'
			origin := payload[0].Origin()
			origin[0].Path = "changed"

			fresh := input.Hooks()[0]
			freshDescriptor := fresh.Descriptor()
			if *freshDescriptor.Handler.Program != "sh" || *freshDescriptor.Handler.Arguments[0].PackageFile != "scripts/early.sh" {
				t.Fatalf("callback mutated descriptor view: %#v", freshDescriptor)
			}
			freshPayload := fresh.PayloadFiles()[0]
			if string(freshPayload.Bytes()) != "hook early\n" || freshPayload.Origin()[0].Path != "src/zeta/early.sh" {
				t.Fatalf("callback mutated payload view: bytes=%q origin=%#v", freshPayload.Bytes(), freshPayload.Origin())
			}
			if fresh.PayloadRoot() != "hook-payload/early" || freshPayload.Path() != "scripts/early.sh" || freshPayload.PackagePath() != "hook-payload/early/scripts/early.sh" {
				t.Fatalf("payload paths = (%q, %q, %q)", fresh.PayloadRoot(), freshPayload.Path(), freshPayload.PackagePath())
			}
			if !freshPayload.Executable() {
				t.Fatal("callback payload lost executable intent")
			}
		}
		return packageoutput.HookManifest{Path: "native-hooks.bin", Bytes: manifestBytes}, nil
	})

	plan, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{zeta, alpha}), codec)
	if len(diagnostics) != 0 {
		t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(callbackPackages, []model.PackageID{"alpha", "zeta"}) {
		t.Fatalf("callback package order = %#v", callbackPackages)
	}

	if file := plannedFile(t, plan, "alpha/hook-payload/only/scripts/alpha.sh"); string(file.Bytes) != "hook only\n" {
		t.Fatalf("alpha payload = %#v", file)
	}
	executable := plannedFile(t, plan, "zeta/hook-payload/early/scripts/early.sh")
	if !executable.Executable || string(executable.Bytes) != "hook early\n" || !reflect.DeepEqual(executable.Origin, []model.SourceLocation{{Path: "src/zeta/early.sh"}}) {
		t.Fatalf("executable payload = %#v", executable)
	}
	nonExecutable := plannedFile(t, plan, "zeta/hook-payload/late/scripts/late.sh")
	if nonExecutable.Executable {
		t.Fatalf("non-executable payload = %#v", nonExecutable)
	}
	if file := plannedFile(t, plan, "zeta/native-hooks.bin"); string(file.Bytes) != "target-owned\n" || file.Executable {
		t.Fatalf("target-owned hook manifest = %#v", file)
	}
	if _, ok := plannedFiles(plan)["zeta/skills/demo/SKILL.md"]; !ok {
		t.Fatal("mixed skill output is missing")
	}

	manifestBytes[0] = 'X'
	if got := string(plannedFile(t, plan, "zeta/native-hooks.bin").Bytes); got != "target-owned\n" {
		t.Fatalf("caller mutated planned manifest bytes: %q", got)
	}
	if got := string(zeta.Assets[2].Content.Files["scripts/early.sh"].Bytes); got != "hook early\n" {
		t.Fatalf("callback mutated normalized package: %q", got)
	}
}

func TestRenderWithCodecRejectsUnsupportedHookSemanticsBeforeCallback(t *testing.T) {
	pkg := hookPackage("demo", hookFixture("check", 0, "run.sh", false, "src/hooks/check/run.sh"))
	pkg.Assets[0].CapabilityUses = append(pkg.Assets[0].CapabilityUses, model.CapabilityUse{
		Key:      "hook.event.stop",
		Location: pkg.Assets[0].Hook.Location,
	})
	called := false
	codec := hookCodec(func(packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
		called = true
		return packageoutput.HookManifest{Path: "native-hooks.bin", Bytes: []byte("native\n")}, nil
	})

	_, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{pkg}), codec)
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || !contains(diagnostics[0].Message, "hook.event.stop") {
		t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
	}
	if called {
		t.Fatal("hook callback ran for unsupported semantics")
	}
}

func TestRenderWithCodecRejectsHookPayloadEscapes(t *testing.T) {
	validPackage := hookPackage("demo", hookFixture("check", 0, "run.sh", false, "src/hooks/check/run.sh"))

	t.Run("payload root", func(t *testing.T) {
		codec := hookCodec(staticHookManifest("native-hooks.bin", []byte("native\n")))
		codec.HookPayloadRoot = "../outside"
		_, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{validPackage}), codec)
		if len(diagnostics) != 1 || diagnostics[0].Code != "invalid-codec" || !contains(diagnostics[0].Message, "hook payload root") {
			t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
		}
	})

	t.Run("payload file", func(t *testing.T) {
		pkg := hookPackage("demo", hookFixture("check", 0, "run.sh", false, "src/hooks/check/run.sh"))
		content := pkg.Assets[0].Content.Files["run.sh"]
		delete(pkg.Assets[0].Content.Files, "run.sh")
		pkg.Assets[0].Content.Files["../run.sh"] = content
		pkg.Assets[0].Hook.Handler.Arguments[0].PackageFile = relativePathPointer("../run.sh")
		_, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{pkg}), hookCodec(staticHookManifest("native-hooks.bin", []byte("native\n"))))
		if len(diagnostics) == 0 || diagnostics[0].Code != "invalid-model" || !contains(diagnostics[0].Message, "asset file path") {
			t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
		}
	})

	t.Run("native manifest", func(t *testing.T) {
		_, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{validPackage}), hookCodec(staticHookManifest("../native-hooks.bin", []byte("native\n"))))
		if len(diagnostics) != 1 || diagnostics[0].Code != "invalid-package-output" || !contains(diagnostics[0].Message, "generated output path") {
			t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
		}
	})
}

func TestRenderWithCodecReportsHookPayloadCollisionsWithOrigins(t *testing.T) {
	pkg := hookPackage("demo", hookFixture("templates", 0, "design.md", false, "src/hooks/templates/design.md"))
	pkg.Assets = append(pkg.Assets, model.NormalizedAsset{
		Identity: "resource/templates",
		Kind:     model.AssetKindResource,
		Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
			"design.md": {Bytes: []byte("resource\n"), Origin: []model.SourceLocation{{Path: "src/resources/templates/design.md"}}},
		}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.resource", Location: model.SourceLocation{Path: "src/resources/templates"}}},
	})
	codec := hookCodec(staticHookManifest("native-hooks.bin", []byte("native\n")))
	codec.HookPayloadRoot = "resources"

	_, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{pkg}), codec)
	if len(diagnostics) != 1 || diagnostics[0].Code != "invalid-package-output" {
		t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
	}
	for _, value := range []string{"resources/templates/design.md", "src/resources/templates/design.md", "src/hooks/templates/design.md"} {
		if !contains(diagnostics[0].Message, value) {
			t.Fatalf("collision diagnostic %q does not contain %q", diagnostics[0].Message, value)
		}
	}
}

func TestRenderWithCodecRejectsDuplicateHookIDs(t *testing.T) {
	first := hookFixture("check", 0, "first.sh", false, "src/hooks/first.sh")
	second := hookFixture("check", 1, "second.sh", false, "src/hooks/second.sh")
	second.Hook.Location.Path = "src/hooks/check-copy/hook.json"
	pkg := hookPackage("demo", first, second)

	_, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{pkg}), hookCodec(staticHookManifest("native-hooks.bin", []byte("native\n"))))
	if len(diagnostics) != 1 || diagnostics[0].Code != "duplicate-hook-id" {
		t.Fatalf("RenderWithCodec() diagnostics = %#v", diagnostics)
	}
	for _, value := range []string{"src/hooks/check/hook.json", "src/hooks/check-copy/hook.json", "hook/check"} {
		if !contains(diagnostics[0].Message, value) {
			t.Fatalf("duplicate hook diagnostic %q does not contain %q", diagnostics[0].Message, value)
		}
	}
}

func TestHookRenderPlansAreDeterministic(t *testing.T) {
	first := hookPackage("zeta",
		hookFixture("late", 9, "z.sh", false, "src/zeta/z.sh"),
		hookFixture("early", 1, "a.sh", true, "src/zeta/a.sh"),
	)
	second := hookPackage("alpha", hookFixture("only", 4, "run.sh", false, "src/alpha/run.sh"))
	codec := hookCodec(func(input packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
		data := make([]byte, 0)
		for _, hook := range input.Hooks() {
			descriptor := hook.Descriptor()
			data = append(data, string(descriptor.Identity)...)
			data = append(data, ':')
			data = append(data, string(descriptor.Event)...)
			data = append(data, '\n')
		}
		return packageoutput.HookManifest{Path: "native-hooks.bin", Bytes: data}, nil
	})

	plan, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{first, second}), codec)
	if len(diagnostics) != 0 {
		t.Fatalf("first RenderWithCodec() diagnostics = %#v", diagnostics)
	}
	first.Assets[0], first.Assets[1] = first.Assets[1], first.Assets[0]
	reversed, diagnostics := packageoutput.RenderWithCodec(separate([]model.NormalizedPackage{second, first}), codec)
	if len(diagnostics) != 0 {
		t.Fatalf("reversed RenderWithCodec() diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(plan, reversed) {
		t.Fatalf("reordered input changed plan\nfirst: %#v\nsecond: %#v", plan, reversed)
	}
	if got := string(plannedFile(t, plan, "zeta/native-hooks.bin").Bytes); got != "hook/early:stop\nhook/late:stop\n" {
		t.Fatalf("native manifest bytes = %q", got)
	}
}

func hookCodec(render func(packageoutput.HookRenderInput) (packageoutput.HookManifest, error)) packageoutput.Codec {
	return packageoutput.Codec{
		Target:          model.TargetClaude,
		ManifestPath:    "plugin.json",
		AgentRoot:       "agents",
		HookPayloadRoot: "hook-payload",
		Capabilities: []model.CapabilityRule{
			{Key: "asset.agent", State: model.CapabilityStateNative},
			{Key: "asset.hook", State: model.CapabilityStateNative},
			{Key: "asset.resource", State: model.CapabilityStateNative},
			{Key: "asset.skill", State: model.CapabilityStateNative},
		},
		Manifest: func(model.NormalizedPackage) ([]byte, error) { return []byte("{}\n"), nil },
		Agent:    func(model.NormalizedAsset) ([]byte, string, error) { return []byte("agent\n"), ".md", nil },
		Hooks:    render,
	}
}

func staticHookManifest(path model.RelativePath, data []byte) func(packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
	return func(packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
		return packageoutput.HookManifest{Path: path, Bytes: data}, nil
	}
}

func hookPackage(identity model.PackageID, hooks ...model.NormalizedAsset) model.NormalizedPackage {
	return model.NormalizedPackage{
		Identity: identity,
		Target:   model.TargetClaude,
		Profile:  model.TargetProfilePackage,
		Assets:   hooks,
	}
}

func hookFixture(name string, order int, payloadPath model.RelativePath, executable bool, origin model.RelativePath) model.NormalizedAsset {
	identity := model.AssetID("hook/" + name)
	program := "sh"
	return model.NormalizedAsset{
		Identity: identity,
		Kind:     model.AssetKindHook,
		Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
			payloadPath: {Bytes: []byte("hook " + name + "\n"), Executable: executable, Origin: []model.SourceLocation{{Path: origin}}},
		}},
		Hook: &model.HookDescriptor{
			Identity:            identity,
			Location:            model.SourceLocation{Path: model.RelativePath("src/hooks/" + name + "/hook.json")},
			Event:               model.HookEventStop,
			Handler:             model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: []model.HookArgument{{PackageFile: &payloadPath}}},
			TimeoutMilliseconds: 1_000,
			FailurePolicy:       model.HookFailurePolicyOpen,
			Order:               order,
		},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.hook", Location: model.SourceLocation{Path: model.RelativePath("src/hooks/" + name + "/hook.json")}}},
	}
}

func skillFixture() model.NormalizedAsset {
	return model.NormalizedAsset{
		Identity:       "skill/demo",
		Kind:           model.AssetKindSkill,
		Content:        model.AssetContent{Body: "Demo.\n", Files: map[model.RelativePath]model.FileContent{}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "src/skills/demo/SKILL.md"}}},
	}
}

func plannedFile(t *testing.T, plan model.TargetPlan, path model.RelativePath) model.PlannedFile {
	t.Helper()
	for _, file := range plan.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("planned file %q is missing", path)
	return model.PlannedFile{}
}

func relativePathPointer(value model.RelativePath) *model.RelativePath { return &value }
