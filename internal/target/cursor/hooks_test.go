package cursor

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

func TestCursorCapabilitiesAndFormatRevision(t *testing.T) {
	if formatRevision != 4 {
		t.Fatalf("formatRevision = %d, want 4", formatRevision)
	}
	want := map[model.CapabilityKey]model.CapabilityState{
		"asset.agent": model.CapabilityStateNative, "asset.hook": model.CapabilityStateNative,
		"asset.resource": model.CapabilityStateNative, "asset.native-resource": model.CapabilityStateUnsupported,
		"asset.skill": model.CapabilityStateNative, "hook.async": model.CapabilityStateUnsupported,
		"hook.command.exec": model.CapabilityStateAdvisory, "hook.command.shell": model.CapabilityStateNative,
		"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateNative,
		"hook.event.notification": model.CapabilityStateUnsupported, "hook.event.post-compact": model.CapabilityStateUnsupported,
		"hook.event.post-tool": model.CapabilityStateNative, "hook.event.post-tool-failure": model.CapabilityStateNative,
		"hook.event.pre-compact": model.CapabilityStateNative, "hook.event.pre-tool": model.CapabilityStateNative,
		"hook.event.prompt-submit": model.CapabilityStateNative, "hook.event.session-end": model.CapabilityStateNative,
		"hook.event.session-start": model.CapabilityStateNative, "hook.event.stop": model.CapabilityStateNative,
		"hook.failure.closed": model.CapabilityStateNative, "hook.matcher.tool-category": model.CapabilityStateNative,
	}
	got := make(map[model.CapabilityKey]model.CapabilityState)
	for _, rule := range New().Capabilities {
		if _, exists := got[rule.Key]; exists {
			t.Fatalf("duplicate capability %q", rule.Key)
		}
		got[rule.Key] = rule.State
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities = %#v, want %#v", got, want)
	}
}

func TestRenderCursorHookGolden(t *testing.T) {
	plan, diagnostics := New().Render(separate([]model.NormalizedPackage{cursorGoldenPackage()}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	assertCursorGoldenTree(t, plan, "testdata/plugin-golden")
	if len(plan.NativeChecks) != 0 {
		t.Fatalf("Cursor must not declare a production native check: %#v", plan.NativeChecks)
	}
	files := cursorPlannedFiles(plan)
	manifest := cursorDecodeObject(t, files[".cursor-plugin/plugin.json"].Bytes)
	if manifest["skills"] != "./skills/" || manifest["agents"] != "./agents/" || manifest["hooks"] != "./hooks/hooks.json" {
		t.Fatalf("plugin manifest components = %#v", manifest)
	}
	hooks := cursorDecodeObject(t, files["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)
	pre := hooks["preToolUse"].([]any)[0].(map[string]any)
	if pre["matcher"] != "Shell" || pre["timeout"] != 5.0 || pre["failClosed"] != true {
		t.Fatalf("pre-tool hook = %#v", pre)
	}
	if got, want := pre["command"], `'bash' '-eu' './hooks/guard/scripts/guard.sh'`; got != want {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
	if !files["hooks/guard/scripts/guard.sh"].Executable {
		t.Fatal("hook payload lost executable intent")
	}
}

func TestRenderCursorHookFreePackageRegression(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Profile = model.TargetProfilePackage
	plan, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := cursorPlannedPaths(plan), []model.RelativePath{".cursor-plugin/plugin.json", "README.md", "skills/guide/SKILL.md", "skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if strings.Contains(string(cursorPlannedFiles(plan)[".cursor-plugin/plugin.json"].Bytes), `"hooks"`) {
		t.Fatal("hook-free manifest declares hooks")
	}
}

func TestRenderCursorMapsDocumentedCamelCaseEvents(t *testing.T) {
	events := []struct {
		portable model.HookEvent
		native   string
	}{
		{model.HookEventSessionStart, "sessionStart"}, {model.HookEventSessionEnd, "sessionEnd"},
		{model.HookEventPromptSubmit, "beforeSubmitPrompt"}, {model.HookEventPreTool, "preToolUse"},
		{model.HookEventPostTool, "postToolUse"}, {model.HookEventPostToolFailure, "postToolUseFailure"},
		{model.HookEventStop, "stop"}, {model.HookEventPreCompact, "preCompact"},
	}
	assets := make([]model.NormalizedAsset, 0, len(events))
	for index, event := range events {
		assets = append(assets, cursorShellHook(string(event.portable), event.portable, nil, 1_000, index, "true"))
	}
	pkg := model.NormalizedPackage{Identity: "events", Target: model.TargetCursor, Profile: model.TargetProfilePackage, Assets: assets}
	plan, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	native := cursorDecodeObject(t, cursorPlannedFiles(plan)["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)
	for _, event := range events {
		if _, exists := native[event.native]; !exists {
			t.Errorf("native event %q is missing", event.native)
		}
	}
}

func TestRenderCursorRequiresExecAdvisoryAcknowledgment(t *testing.T) {
	asset := cursorExecHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 2_000, 0, "bash", nil)
	pkg := model.NormalizedPackage{Identity: "demo", Target: model.TargetCursor, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	_, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "missing-capability-acknowledgment" || !strings.Contains(diagnostics[0].Message, "hook.command.exec") {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	pkg.Acknowledgments = cursorAcknowledge(asset, "hook.command.exec")
	if _, diagnostics = New().Render(separate([]model.NormalizedPackage{pkg})); len(diagnostics) != 0 {
		t.Fatalf("acknowledged Render() diagnostics = %#v", diagnostics)
	}
}

func TestRenderCursorRejectsDecisionAndFailureGaps(t *testing.T) {
	for _, test := range []struct {
		name               string
		asset              model.NormalizedAsset
		mutate             func(*model.NormalizedAsset)
		wantCode, wantText string
	}{
		{name: "notification", asset: cursorShellHook("notify", model.HookEventNotification, nil, 1_000, 0, "true"), mutate: func(*model.NormalizedAsset) {}, wantCode: "unsupported-capability", wantText: "hook.event.notification"},
		{name: "post compact", asset: cursorShellHook("compact", model.HookEventPostCompact, nil, 1_000, 0, "true"), mutate: func(*model.NormalizedAsset) {}, wantCode: "unsupported-capability", wantText: "hook.event.post-compact"},
		{name: "async", asset: cursorShellHook("async", model.HookEventPostTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.Hook.Asynchronous = true
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.async", Location: a.Hook.Location})
		}, wantCode: "unsupported-capability", wantText: "hook.async"},
		{name: "closed passive", asset: cursorShellHook("closed", model.HookEventPostTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.Hook.FailurePolicy = model.HookFailurePolicyClosed
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.failure.closed", Location: a.Hook.Location})
		}, wantCode: "unsupported-hook-semantics", wantText: "pre-tool and prompt-submit"},
		{name: "block after tool", asset: cursorShellHook("block", model.HookEventPostTool, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.decision.block", Location: a.Hook.Location})
		}, wantCode: "unsupported-hook-semantics", wantText: "pre-tool and prompt-submit"},
		{name: "rewrite prompt", asset: cursorShellHook("rewrite", model.HookEventPromptSubmit, nil, 1_000, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.decision.rewrite-input", Location: a.Hook.Location})
		}, wantCode: "unsupported-hook-semantics", wantText: "pre-tool hooks"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.mutate(&test.asset)
			pkg := model.NormalizedPackage{Identity: "demo", Target: model.TargetCursor, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{test.asset}}
			plan, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg}))
			if len(diagnostics) != 1 || diagnostics[0].Code != test.wantCode || !strings.Contains(diagnostics[0].Message, test.wantText) {
				t.Fatalf("Render() = (%#v, %#v), want %q containing %q", plan, diagnostics, test.wantCode, test.wantText)
			}
			if len(plan.Files) != 0 {
				t.Fatalf("rejected hook produced partial output: %#v", plan.Files)
			}
		})
	}
}

func TestRenderCursorMatcherTimeoutCollisionAndDeterminism(t *testing.T) {
	asset := cursorShellHook("match", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryCommand, model.HookToolCategoryRead, model.HookToolCategoryWrite, model.HookToolCategoryTask, model.HookToolCategoryMCP}, 1_250, 0, `printf "done"`)
	pkg := model.NormalizedPackage{Identity: "demo", Target: model.TargetCursor, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	plan, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	handler := cursorDecodeObject(t, cursorPlannedFiles(plan)["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)["postToolUse"].([]any)[0].(map[string]any)
	if handler["matcher"] != "Shell|Read|Write|Task|^MCP:.*$" || handler["timeout"] != 1.25 {
		t.Fatalf("handler = %#v", handler)
	}

	unsupported := cursorShellHook("search", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategorySearch}, 1_000, 0, "true")
	pkg.Assets = []model.NormalizedAsset{unsupported}
	if rejected, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg})); len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-hook-semantics" || !strings.Contains(diagnostics[0].Message, "no lossless Cursor matcher") || len(rejected.Files) != 0 {
		t.Fatalf("unsupported matcher Render() = (%#v, %#v)", rejected, diagnostics)
	}

	pkg.Assets = []model.NormalizedAsset{asset, asset}
	pkg.Assets[1].Hook = cursorDescriptorPointer(*asset.Hook)
	pkg.Assets[1].Hook.Location.Path = "source/hooks/copy/hook.json"
	if collision, diagnostics := New().Render(separate([]model.NormalizedPackage{pkg})); len(diagnostics) == 0 || diagnostics[0].Code != "duplicate-hook-id" || len(collision.Files) != 0 {
		t.Fatalf("collision Render() = (%#v, %#v)", collision, diagnostics)
	}

	zeta := cursorGoldenPackage()
	zeta.Identity = "zeta"
	alpha := cursorGoldenPackage()
	alpha.Identity = "alpha"
	first, diagnostics := New().Render(separate([]model.NormalizedPackage{zeta, alpha}))
	if len(diagnostics) != 0 {
		t.Fatalf("first diagnostics = %#v", diagnostics)
	}
	second, diagnostics := New().Render(separate([]model.NormalizedPackage{alpha, zeta}))
	if len(diagnostics) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatal("multi-package rendering is not deterministic")
	}
}

func cursorGoldenPackage() model.NormalizedPackage {
	path := model.RelativePath("scripts/guard.sh")
	guard := cursorExecHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 5_000, 1, "bash", []model.HookArgument{{Literal: cursorStringPointer("-eu")}, {PackageFile: &path}})
	guard.Content.Files[path] = model.FileContent{Bytes: []byte("#!/usr/bin/env bash\ncat >/dev/null\n"), Executable: true}
	guard.Hook.FailurePolicy = model.HookFailurePolicyClosed
	guard.CapabilityUses = append(guard.CapabilityUses,
		model.CapabilityUse{Key: "hook.failure.closed", Location: guard.Hook.Location},
		model.CapabilityUse{Key: "hook.decision.block", Location: guard.Hook.Location},
		model.CapabilityUse{Key: "hook.decision.rewrite-input", Location: guard.Hook.Location})
	post := cursorShellHook("report", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryWrite}, 7_250, 2, "printf done | cat")
	end := cursorShellHook("audit", model.HookEventSessionEnd, nil, 3_000, 3, "./hooks/audit/audit.sh")
	end.Content.Files["audit.sh"] = model.FileContent{Bytes: []byte("#!/bin/sh\nexit 0\n"), Executable: true}
	agent := model.NormalizedAsset{Identity: "agent/reviewer", Kind: model.AssetKindAgent, Content: model.AssetContent{Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"}, Body: "Review.\n", Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: cursorUses("source/agent", "asset.agent")}
	skill := model.NormalizedAsset{Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "Guide.\n", Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: cursorUses("source/skill", "asset.skill")}
	pkg := model.NormalizedPackage{Identity: "demo", Target: model.TargetCursor, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{"version": "1.2.3", "description": "Demo hooks", "displayName": "Demo Hooks", "author": "Agent Bundler"}, Assets: []model.NormalizedAsset{post, skill, guard, agent, end}}
	pkg.Acknowledgments = cursorAcknowledge(guard, "hook.command.exec")
	return pkg
}

func cursorExecHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout, order int, program string, arguments []model.HookArgument) model.NormalizedAsset {
	asset := cursorShellHook(name, event, tools, timeout, order, "unused")
	asset.Hook.Handler = model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: arguments}
	for index := range asset.CapabilityUses {
		if asset.CapabilityUses[index].Key == "hook.command.shell" {
			asset.CapabilityUses[index].Key = "hook.command.exec"
		}
	}
	return asset
}

func cursorShellHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout, order int, command string) model.NormalizedAsset {
	identity := model.AssetID("hook/" + name)
	location := model.SourceLocation{Path: model.RelativePath("source/hooks/" + name + "/hook.json")}
	var matcher *model.HookMatcher
	if tools != nil {
		matcher = &model.HookMatcher{Tools: append([]model.HookToolCategory(nil), tools...)}
	}
	capabilities := cursorUses(location.Path, "asset.hook", "hook.command.shell", model.CapabilityKey("hook.event."+string(event)))
	if matcher != nil {
		capabilities = append(capabilities, model.CapabilityUse{Key: "hook.matcher.tool-category", Location: location})
	}
	return model.NormalizedAsset{Identity: identity, Kind: model.AssetKindHook, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}}, Hook: &model.HookDescriptor{Identity: identity, Location: location, Event: event, Matcher: matcher, Handler: model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}, TimeoutMilliseconds: timeout, FailurePolicy: model.HookFailurePolicyOpen, Order: order}, CapabilityUses: capabilities}
}

func cursorAcknowledge(asset model.NormalizedAsset, keys ...model.CapabilityKey) []model.Acknowledgment {
	result := make([]model.Acknowledgment, len(keys))
	for index, key := range keys {
		result[index] = model.Acknowledgment{Asset: asset.Identity, Target: model.TargetCursor, Key: key, Reason: "accepted Cursor command-string behavior"}
	}
	return result
}

func cursorUses(path model.RelativePath, keys ...model.CapabilityKey) []model.CapabilityUse {
	result := make([]model.CapabilityUse, len(keys))
	for index, key := range keys {
		result[index] = model.CapabilityUse{Key: key, Location: model.SourceLocation{Path: path}}
	}
	return result
}

func cursorDescriptorPointer(value model.HookDescriptor) *model.HookDescriptor { return &value }
func cursorStringPointer(value string) *string                                 { return &value }

func cursorPlannedFiles(plan model.TargetPlan) map[model.RelativePath]model.PlannedFile {
	result := make(map[model.RelativePath]model.PlannedFile, len(plan.Files))
	for _, file := range plan.Files {
		result[file.Path] = file
	}
	return result
}

func cursorPlannedPaths(plan model.TargetPlan) []model.RelativePath {
	result := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		result[index] = file.Path
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func cursorDecodeObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return value
}

func assertCursorGoldenTree(t *testing.T, plan model.TargetPlan, root string) {
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
	actual := cursorPlannedFiles(plan)
	if len(actual) != len(expected) {
		t.Fatalf("paths = %#v, want %d files", cursorPlannedPaths(plan), len(expected))
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok || !reflect.DeepEqual(got.Bytes, want.Bytes) || got.Executable != want.Executable {
			t.Errorf("file %q = %#v, want %#v", path, got, want)
		}
	}
}
