package grok

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestGrokCapabilitiesAndFormatRevision(t *testing.T) {
	if FormatRevision != 7 {
		t.Fatalf("FormatRevision = %d, want 7", FormatRevision)
	}
	want := map[model.CapabilityKey]model.CapabilityState{
		"asset.agent": model.CapabilityStateNative, "asset.hook": model.CapabilityStateNative,
		"asset.resource": model.CapabilityStateNative, "asset.native-resource": model.CapabilityStateUnsupported,
		"asset.skill": model.CapabilityStateNative, "hook.async": model.CapabilityStateUnsupported,
		"hook.command.exec": model.CapabilityStateNative, "hook.command.shell": model.CapabilityStateNative,
		"hook.decision.block": model.CapabilityStateUnsupported, "hook.decision.rewrite-input": model.CapabilityStateUnsupported,
		"hook.event.notification": model.CapabilityStateNative, "hook.event.post-compact": model.CapabilityStateNative,
		"hook.event.post-tool": model.CapabilityStateNative, "hook.event.post-tool-failure": model.CapabilityStateNative,
		"hook.event.pre-compact": model.CapabilityStateNative, "hook.event.pre-tool": model.CapabilityStateNative,
		"hook.event.prompt-submit": model.CapabilityStateNative, "hook.event.session-end": model.CapabilityStateNative,
		"hook.event.session-start": model.CapabilityStateNative, "hook.event.stop": model.CapabilityStateNative,
		"hook.failure.closed": model.CapabilityStateUnsupported, "hook.matcher.tool-category": model.CapabilityStateNative,
	}
	got := make(map[model.CapabilityKey]model.CapabilityState)
	for _, rule := range Capabilities() {
		if _, duplicate := got[rule.Key]; duplicate {
			t.Fatalf("duplicate capability %q", rule.Key)
		}
		got[rule.Key] = rule.State
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities() = %#v, want %#v", got, want)
	}
}

func TestRenderGrokPluginGolden(t *testing.T) {
	plan, diagnostics := Render(separate([]model.NormalizedPackage{grokGoldenPackage()}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	assertGrokGoldenTree(t, plan, "testdata/plugin-golden")
	if got, want := plan.NativeChecks, []model.NativeCheck{{Program: "grok", Arguments: []string{"plugin", "validate", "."}, Location: model.SourceLocation{Path: "internal/target/grok/codec.go"}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeChecks = %#v, want %#v", got, want)
	}
	files := grokPlannedFiles(plan)
	manifest := grokDecodeObject(t, files[".claude-plugin/plugin.json"].Bytes)
	if manifest["hooks"] != "./hooks/hooks.json" {
		t.Fatalf("plugin hooks = %#v", manifest["hooks"])
	}
	hooks := grokDecodeObject(t, files["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)[0].(map[string]any)
	if pre["matcher"] != "Bash" {
		t.Fatalf("pre-tool matcher = %#v", pre)
	}
	handler := pre["hooks"].([]any)[0].(map[string]any)
	if handler["command"] != "bash" || handler["timeout"] != 5.0 {
		t.Fatalf("pre-tool handler = %#v", handler)
	}
	if got, want := handler["args"], []any{"-eu", "${GROK_PLUGIN_ROOT}/hooks/guard/scripts/guard.sh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	if !files["hooks/guard/scripts/guard.sh"].Executable {
		t.Fatal("hook payload lost executable intent")
	}
}

func TestRenderGrokHookFreePackageRegression(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Profile = model.TargetProfilePackage
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := grokPlannedPaths(plan), []model.RelativePath{".claude-plugin/plugin.json", "README.md", "resources/templates/report.md", "skills/guide/SKILL.md", "skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if strings.Contains(string(grokPlannedFiles(plan)[".claude-plugin/plugin.json"].Bytes), `"hooks"`) {
		t.Fatal("hook-free manifest declares hooks")
	}
}

func TestRenderGrokMapsDocumentedEvents(t *testing.T) {
	events := []struct {
		portable model.HookEvent
		native   string
	}{
		{model.HookEventSessionStart, "SessionStart"}, {model.HookEventSessionEnd, "SessionEnd"},
		{model.HookEventPromptSubmit, "UserPromptSubmit"}, {model.HookEventPreTool, "PreToolUse"},
		{model.HookEventPostTool, "PostToolUse"}, {model.HookEventPostToolFailure, "PostToolUseFailure"},
		{model.HookEventStop, "Stop"}, {model.HookEventNotification, "Notification"},
		{model.HookEventPreCompact, "PreCompact"}, {model.HookEventPostCompact, "PostCompact"},
	}
	assets := make([]model.NormalizedAsset, 0, len(events))
	for index, event := range events {
		assets = append(assets, grokShellHook(string(event.portable), event.portable, nil, 1_000, index, "true"))
	}
	pkg := model.NormalizedPackage{Identity: "events", Target: Target, Profile: model.TargetProfilePackage, Assets: assets}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	native := grokDecodeObject(t, grokPlannedFiles(plan)["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)
	for _, event := range events {
		if _, exists := native[event.native]; !exists {
			t.Errorf("native event %q is missing", event.native)
		}
	}
}

func TestRenderGrokRejectsUnsupportedSemantics(t *testing.T) {
	for _, test := range []struct {
		name               string
		asset              model.NormalizedAsset
		mutate             func(*model.NormalizedAsset)
		wantCode, wantText string
	}{
		{name: "closed failure", asset: grokShellHook("closed", model.HookEventPreTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.Hook.FailurePolicy = model.HookFailurePolicyClosed
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.failure.closed", Location: a.Hook.Location})
		}, wantCode: "unsupported-capability", wantText: "hook.failure.closed"},
		{name: "async", asset: grokShellHook("async", model.HookEventPostTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.Hook.Asynchronous = true
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.async", Location: a.Hook.Location})
		}, wantCode: "unsupported-capability", wantText: "hook.async"},
		{name: "rewrite", asset: grokShellHook("rewrite", model.HookEventPreTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.decision.rewrite-input", Location: a.Hook.Location})
		}, wantCode: "unsupported-capability", wantText: "hook.decision.rewrite-input"},
		{name: "block decision", asset: grokShellHook("block", model.HookEventPreTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.decision.block", Location: a.Hook.Location})
		}, wantCode: "unsupported-capability", wantText: "hook.decision.block"},
		{name: "unknown event", asset: grokShellHook("unknown", model.HookEvent("future"), nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.CapabilityUses = grokUses(a.Hook.Location.Path, "asset.hook", "hook.command.shell")
		}, wantCode: "invalid-model", wantText: "event \"future\" is invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.mutate(&test.asset)
			pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{test.asset}}
			plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
			if len(diagnostics) != 1 || diagnostics[0].Code != test.wantCode || !strings.Contains(diagnostics[0].Message, test.wantText) {
				t.Fatalf("Render() = (%#v, %#v), want %q containing %q", plan, diagnostics, test.wantCode, test.wantText)
			}
			if len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
				t.Fatalf("rejected hook produced partial output: %#v", plan)
			}
		})
	}
}

func TestRenderGrokRejectsUnsupportedAgentField(t *testing.T) {
	pkg := grokGoldenPackage()
	for index := range pkg.Assets {
		if pkg.Assets[index].Kind == model.AssetKindAgent {
			pkg.Assets[index].Content.Frontmatter["sandbox_mode"] = "read-only"
		}
	}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-agent-field" || len(plan.Files) != 0 {
		t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
	}
}

func TestRenderGrokMatcherTimeoutCollisionSeparateAndDeterministic(t *testing.T) {
	asset := grokShellHook("match", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand, model.HookToolCategoryRead, model.HookToolCategoryWrite, model.HookToolCategoryEdit, model.HookToolCategorySearch, model.HookToolCategoryWeb, model.HookToolCategoryTask, model.HookToolCategoryMCP}, 1_250, 0, "true")
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	group := grokDecodeObject(t, grokPlannedFiles(plan)["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if group["matcher"] != "Bash|Read|Write|Edit|NotebookEdit|Glob|Grep|WebFetch|WebSearch|Task|Agent|^mcp__.*$" || group["hooks"].([]any)[0].(map[string]any)["timeout"] != 1.25 {
		t.Fatalf("group = %#v", group)
	}

	pkg.Assets = []model.NormalizedAsset{asset, asset}
	pkg.Assets[1].Hook = grokDescriptorPointer(*asset.Hook)
	pkg.Assets[1].Hook.Location.Path = "source/hooks/copy/hook.json"
	if collision, diagnostics := Render(separate([]model.NormalizedPackage{pkg})); len(diagnostics) == 0 || diagnostics[0].Code != "duplicate-hook-id" || len(collision.Files) != 0 {
		t.Fatalf("collision Render() = (%#v, %#v)", collision, diagnostics)
	}

	zeta := grokGoldenPackage()
	zeta.Identity = "zeta"
	alpha := grokGoldenPackage()
	alpha.Identity = "alpha"
	first, diagnostics := Render(separate([]model.NormalizedPackage{zeta, alpha}))
	if len(diagnostics) != 0 {
		t.Fatalf("first diagnostics = %#v", diagnostics)
	}
	second, diagnostics := Render(separate([]model.NormalizedPackage{alpha, zeta}))
	if len(diagnostics) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatal("multi-package rendering is not deterministic")
	}
	files := grokPlannedFiles(first)
	if _, ok := files["alpha/.claude-plugin/plugin.json"]; !ok {
		t.Fatal("alpha separate package root is missing")
	}
	if _, ok := files["zeta/.claude-plugin/plugin.json"]; !ok {
		t.Fatal("zeta separate package root is missing")
	}
	if got, want := first.NativeChecks, []model.NativeCheck{
		{Program: "grok", Arguments: []string{"plugin", "validate", "./alpha"}, Location: model.SourceLocation{Path: "internal/target/grok/codec.go"}},
		{Program: "grok", Arguments: []string{"plugin", "validate", "./zeta"}, Location: model.SourceLocation{Path: "internal/target/grok/codec.go"}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeChecks = %#v, want %#v", got, want)
	}
}

func TestNativeChecksAnchorOptionLikePluginRoots(t *testing.T) {
	checks := nativeChecks([]model.PackageID{"--help", "-version"})
	want := [][]string{
		{"plugin", "validate", "./--help"},
		{"plugin", "validate", "./-version"},
	}
	for index := range checks {
		if !reflect.DeepEqual(checks[index].Arguments, want[index]) {
			t.Fatalf("NativeChecks[%d].Arguments = %#v, want %#v", index, checks[index].Arguments, want[index])
		}
	}
}

func grokGoldenPackage() model.NormalizedPackage {
	path := model.RelativePath("scripts/guard.sh")
	guard := grokExecHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 5_000, 1, "bash", []model.HookArgument{{Literal: grokStringPointer("-eu")}, {PackageFile: &path}})
	guard.Content.Files[path] = model.FileContent{Bytes: []byte("#!/usr/bin/env bash\ncat >/dev/null\n"), Executable: true}
	post := grokShellHook("audit", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryWrite}, 7_250, 2, `cd "${GROK_PLUGIN_ROOT}" && printf done`)
	agent := model.NormalizedAsset{Identity: "agent/reviewer", Kind: model.AssetKindAgent, Content: model.AssetContent{Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"}, Body: "Review.\n", Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: grokUses("source/agent", "asset.agent")}
	skill := model.NormalizedAsset{Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Frontmatter: map[string]any{"name": "guide", "description": "Guide"}, Body: "Guide.\n", Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: grokUses("source/skill", "asset.skill")}
	return model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{"version": "1.2.3", "description": "Demo hooks", "author": "Agent Bundler"}, Assets: []model.NormalizedAsset{post, skill, guard, agent}}
}

func grokExecHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout, order int, program string, arguments []model.HookArgument) model.NormalizedAsset {
	asset := grokShellHook(name, event, tools, timeout, order, "unused")
	asset.Hook.Handler = model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: arguments}
	for index := range asset.CapabilityUses {
		if asset.CapabilityUses[index].Key == "hook.command.shell" {
			asset.CapabilityUses[index].Key = "hook.command.exec"
		}
	}
	return asset
}

func grokShellHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout, order int, command string) model.NormalizedAsset {
	identity := model.AssetID("hook/" + name)
	location := model.SourceLocation{Path: model.RelativePath("source/hooks/" + name + "/hook.json")}
	var matcher *model.HookMatcher
	if tools != nil {
		matcher = &model.HookMatcher{Tools: append([]model.HookToolCategory(nil), tools...)}
	}
	capabilities := grokUses(location.Path, "asset.hook", "hook.command.shell", model.CapabilityKey("hook.event."+string(event)))
	if matcher != nil {
		capabilities = append(capabilities, model.CapabilityUse{Key: "hook.matcher.tool-category", Location: location})
	}
	return model.NormalizedAsset{Identity: identity, Kind: model.AssetKindHook, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}}, Hook: &model.HookDescriptor{Identity: identity, Location: location, Event: event, Matcher: matcher, Handler: model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}, TimeoutMilliseconds: timeout, FailurePolicy: model.HookFailurePolicyOpen, Order: order}, CapabilityUses: capabilities}
}

func grokUses(path model.RelativePath, keys ...model.CapabilityKey) []model.CapabilityUse {
	result := make([]model.CapabilityUse, len(keys))
	for index, key := range keys {
		result[index] = model.CapabilityUse{Key: key, Location: model.SourceLocation{Path: path}}
	}
	return result
}

func grokDescriptorPointer(value model.HookDescriptor) *model.HookDescriptor { return &value }
func grokStringPointer(value string) *string                                 { return &value }

func grokPlannedFiles(plan model.TargetPlan) map[model.RelativePath]model.PlannedFile {
	result := make(map[model.RelativePath]model.PlannedFile, len(plan.Files))
	for _, file := range plan.Files {
		result[file.Path] = file
	}
	return result
}

func grokPlannedPaths(plan model.TargetPlan) []model.RelativePath {
	result := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		result[index] = file.Path
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func grokDecodeObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return value
}

func assertGrokGoldenTree(t *testing.T, plan model.TargetPlan, root string) {
	t.Helper()
	expected := map[model.RelativePath]model.PlannedFile{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		expected[model.RelativePath(filepath.ToSlash(relative))] = model.PlannedFile{Bytes: data, Executable: info.Mode()&0o111 != 0}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	actual := grokPlannedFiles(plan)
	if len(actual) != len(expected) {
		t.Fatalf("paths = %#v, want %d files", grokPlannedPaths(plan), len(expected))
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok || !reflect.DeepEqual(got.Bytes, want.Bytes) || got.Executable != want.Executable {
			t.Errorf("file %q = %#v, want %#v", path, got, want)
		}
	}
}
