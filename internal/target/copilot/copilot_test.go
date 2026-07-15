package copilot

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

func TestRenderUsesCopilotNativeSkillRoot(t *testing.T) {
	plan, diagnostics := Render(separate(skillPackage()))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := plannedPaths(plan), []model.RelativePath{".github/resources/templates/report.md", ".github/skills/guide/SKILL.md", ".github/skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestCopilotCapabilitiesAndFormatRevision(t *testing.T) {
	if FormatRevision != 5 {
		t.Fatalf("FormatRevision = %d, want 5", FormatRevision)
	}
	want := map[model.CapabilityKey]model.CapabilityState{
		"asset.agent": model.CapabilityStateNative, "asset.hook": model.CapabilityStateNative,
		"asset.resource": model.CapabilityStateNative, "asset.native-resource": model.CapabilityStateUnsupported,
		"asset.skill": model.CapabilityStateNative, "hook.async": model.CapabilityStateNative,
		"hook.command.exec": model.CapabilityStateAdvisory, "hook.command.shell": model.CapabilityStateNative,
		"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateNative,
		"hook.event.notification": model.CapabilityStateNative, "hook.event.post-compact": model.CapabilityStateUnsupported,
		"hook.event.post-tool": model.CapabilityStateNative, "hook.event.post-tool-failure": model.CapabilityStateNative,
		"hook.event.pre-compact": model.CapabilityStateNative, "hook.event.pre-tool": model.CapabilityStateAdvisory,
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

func TestRenderCopilotHookGolden(t *testing.T) {
	pkg := goldenHookPackage()
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	assertGoldenTree(t, plan, "testdata/plugin-golden")
	if len(plan.NativeChecks) != 0 {
		t.Fatalf("Copilot must not declare a mutating native check: %#v", plan.NativeChecks)
	}
	files := plannedFiles(plan)
	manifest := decodeObject(t, files["plugin.json"].Bytes)
	if manifest["hooks"] != "hooks.json" || manifest["agents"] != "agents/" || !reflect.DeepEqual(manifest["skills"], []any{"skills/"}) {
		t.Fatalf("plugin manifest components = %#v", manifest)
	}
	hooks := decodeObject(t, files["hooks.json"].Bytes)["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)[0].(map[string]any)
	if pre["matcher"] != "Bash" || pre["timeoutSec"] != 5.0 {
		t.Fatalf("pre-tool hook = %#v", pre)
	}
	if got, want := pre["command"], `'bash' '-eu' "${PLUGIN_ROOT}/hooks/guard/scripts/guard.sh"`; got != want {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
	post := hooks["PostToolUse"].([]any)[0].(map[string]any)
	if post["timeoutSec"] != 7.25 || post["command"] != `printf done | cat` {
		t.Fatalf("post-tool hook = %#v", post)
	}
	if !files["hooks/guard/scripts/guard.sh"].Executable {
		t.Fatal("hook payload lost executable intent")
	}
}

func TestRenderCopilotHookFreePackageRegression(t *testing.T) {
	pkg := model.NormalizedPackage{Identity: "plain", Target: Target, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{"version": "1.0.0"}, Assets: []model.NormalizedAsset{{
		Identity: "skill/guide", Kind: model.AssetKindSkill,
		Content:        model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "Guide.\n", Files: map[model.RelativePath]model.FileContent{}},
		CapabilityUses: uses("source/guide", "asset.skill"),
	}}}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := plannedPaths(plan), []model.RelativePath{"README.md", "plugin.json", "skills/guide/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if strings.Contains(string(plannedFiles(plan)["plugin.json"].Bytes), `"hooks"`) {
		t.Fatal("hook-free manifest declares hooks")
	}
}

func TestRenderCopilotMapsVerifiedPascalCaseEvents(t *testing.T) {
	events := []struct {
		portable model.HookEvent
		native   string
		async    bool
	}{
		{model.HookEventSessionStart, "SessionStart", false}, {model.HookEventSessionEnd, "SessionEnd", false},
		{model.HookEventPromptSubmit, "UserPromptSubmit", false}, {model.HookEventPostTool, "PostToolUse", false},
		{model.HookEventPostToolFailure, "PostToolUseFailure", false}, {model.HookEventStop, "Stop", false},
		{model.HookEventNotification, "Notification", true}, {model.HookEventPreCompact, "PreCompact", false},
	}
	assets := make([]model.NormalizedAsset, 0, len(events))
	for index, event := range events {
		assets = append(assets, shellHook(string(event.portable), event.portable, nil, 1_000, event.async, index, "true"))
	}
	pkg := model.NormalizedPackage{Identity: "events", Target: Target, Profile: model.TargetProfilePackage, Assets: assets}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	native := decodeObject(t, plannedFiles(plan)["hooks.json"].Bytes)["hooks"].(map[string]any)
	for _, event := range events {
		if _, exists := native[event.native]; !exists {
			t.Errorf("native event %q is missing", event.native)
		}
	}
}

func TestRenderCopilotRequiresExactAdvisoryAcknowledgments(t *testing.T) {
	asset := execHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 2_000, false, 0, "bash", nil)
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	_, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "missing-capability-acknowledgment" || !strings.Contains(diagnostics[0].Message, "hook.command.exec") {
		t.Fatalf("missing exec acknowledgment diagnostics = %#v", diagnostics)
	}
	pkg.Acknowledgments = acknowledge(asset, "hook.command.exec")
	_, diagnostics = Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "missing-capability-acknowledgment" || !strings.Contains(diagnostics[0].Message, "hook.event.pre-tool") {
		t.Fatalf("missing pre-tool acknowledgment diagnostics = %#v", diagnostics)
	}
	pkg.Acknowledgments = append(pkg.Acknowledgments, acknowledge(asset, "hook.event.pre-tool")...)
	if _, diagnostics = Render(separate([]model.NormalizedPackage{pkg})); len(diagnostics) != 0 {
		t.Fatalf("acknowledged Render() diagnostics = %#v", diagnostics)
	}
}

func TestRenderCopilotRejectsFailureTimeoutAndDecisionMismatches(t *testing.T) {
	for _, test := range []struct {
		name               string
		asset              model.NormalizedAsset
		mutate             func(*model.NormalizedAsset)
		wantCode, wantText string
	}{
		{name: "closed policy", asset: shellHook("closed", model.HookEventPostTool, nil, 1_000, false, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.Hook.FailurePolicy = model.HookFailurePolicyClosed
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.failure.closed", Location: a.Hook.Location})
		}, wantCode: "unsupported-capability", wantText: "hook.failure.closed"},
		{name: "async post tool", asset: shellHook("async", model.HookEventPostTool, nil, 1_000, false, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.Hook.Asynchronous = true
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.async", Location: a.Hook.Location})
		}, wantCode: "unsupported-hook-semantics", wantText: "hook.async"},
		{name: "synchronous notification", asset: shellHook("notify", model.HookEventNotification, nil, 1_000, false, 0, "true"), mutate: func(*model.NormalizedAsset) {}, wantCode: "unsupported-hook-semantics", wantText: "inherently asynchronous"},
		{name: "rewrite on stop", asset: shellHook("stop", model.HookEventStop, nil, 1_000, false, 0, "true"), mutate: func(a *model.NormalizedAsset) {
			a.CapabilityUses = append(a.CapabilityUses, model.CapabilityUse{Key: "hook.decision.rewrite-input", Location: a.Hook.Location})
		}, wantCode: "unsupported-hook-semantics", wantText: "pre-tool"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.mutate(&test.asset)
			pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{test.asset}}
			plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
			if len(diagnostics) != 1 || diagnostics[0].Code != test.wantCode || !strings.Contains(diagnostics[0].Message, test.wantText) {
				t.Fatalf("Render() = (%#v, %#v), want %q containing %q", plan, diagnostics, test.wantCode, test.wantText)
			}
			if len(plan.Files) != 0 {
				t.Fatalf("rejected hook produced partial output: %#v", plan)
			}
		})
	}
}

func TestRenderCopilotMatcherTimeoutCollisionAndDeterminism(t *testing.T) {
	allTools := []model.HookToolCategory{model.HookToolCategoryCommand, model.HookToolCategoryRead, model.HookToolCategoryWrite, model.HookToolCategoryEdit, model.HookToolCategorySearch, model.HookToolCategoryWeb, model.HookToolCategoryTask, model.HookToolCategoryMCP}
	asset := shellHook("match", model.HookEventPostTool, allTools, 1_250, false, 0, `printf "it's safe"`)
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	handler := decodeObject(t, plannedFiles(plan)["hooks.json"].Bytes)["hooks"].(map[string]any)["PostToolUse"].([]any)[0].(map[string]any)
	if got, want := handler["matcher"], "Bash|Read|Write|Edit|Glob|Grep|WebFetch|WebSearch|Agent|Task|^mcp__.*$"; got != want {
		t.Fatalf("matcher = %#v, want %#v", got, want)
	}
	if handler["timeoutSec"] != 1.25 {
		t.Fatalf("timeoutSec = %#v, want 1.25", handler["timeoutSec"])
	}
	second := asset
	second.Hook = pointerDescriptor(*asset.Hook)
	second.Hook.Location.Path = "source/hooks/copy/hook.json"
	pkg.Assets = append(pkg.Assets, second)
	collisionPlan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) == 0 || diagnostics[0].Code != "duplicate-hook-id" || len(collisionPlan.Files) != 0 {
		t.Fatalf("collision Render() = (%#v, %#v)", collisionPlan, diagnostics)
	}

	zeta := goldenHookPackage()
	zeta.Identity = "zeta"
	alpha := goldenHookPackage()
	alpha.Identity = "alpha"
	first, diagnostics := Render(separate([]model.NormalizedPackage{zeta, alpha}))
	if len(diagnostics) != 0 {
		t.Fatalf("first diagnostics = %#v", diagnostics)
	}
	secondPlan, diagnostics := Render(separate([]model.NormalizedPackage{alpha, zeta}))
	if len(diagnostics) != 0 || !reflect.DeepEqual(first, secondPlan) {
		t.Fatalf("multi-package rendering is not deterministic")
	}
}

func TestRenderProjectProfileRejectsAgent(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Assets[0].Kind = model.AssetKindAgent
	pkg.Assets[0].Identity = "agent/reviewer"
	_, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func separate(packages []model.NormalizedPackage) model.TargetRenderInput {
	return model.TargetRenderInput{Packages: packages, PackageMode: model.TargetPackageModeSeparate}
}

func skillPackage() []model.NormalizedPackage {
	return []model.NormalizedPackage{{Identity: "demo", Target: Target, Assets: []model.NormalizedAsset{
		{Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "# Guide\n", Files: map[model.RelativePath]model.FileContent{"docs/readme.md": {Bytes: []byte("help")}}}, CapabilityUses: uses("source/SKILL.md", "asset.skill")},
		{Identity: "resource/templates", Kind: model.AssetKindResource, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{"report.md": {Bytes: []byte("# Report\n")}}}},
	}}}
}

func goldenHookPackage() model.NormalizedPackage {
	path := model.RelativePath("scripts/guard.sh")
	guard := execHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 5_000, false, 1, "bash", []model.HookArgument{{Literal: stringPointer("-eu")}, {PackageFile: &path}})
	guard.Content.Files[path] = model.FileContent{Bytes: []byte("#!/usr/bin/env bash\ncat >/dev/null\n"), Executable: true}
	guard.CapabilityUses = append(guard.CapabilityUses, model.CapabilityUse{Key: "hook.decision.block", Location: guard.Hook.Location}, model.CapabilityUse{Key: "hook.decision.rewrite-input", Location: guard.Hook.Location})
	post := shellHook("report", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryWrite}, 7_250, false, 2, "printf done | cat")
	agent := model.NormalizedAsset{Identity: "agent/reviewer", Kind: model.AssetKindAgent, Content: model.AssetContent{Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"}, Body: "Review.\n", Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: uses("source/agent", "asset.agent")}
	skill := model.NormalizedAsset{Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "Guide.\n", Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: uses("source/skill", "asset.skill")}
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{"version": "1.2.3", "description": "Demo hooks", "author": "Agent Bundler"}, Assets: []model.NormalizedAsset{post, skill, guard, agent}}
	pkg.Acknowledgments = append(acknowledge(guard, "hook.command.exec"), acknowledge(guard, "hook.event.pre-tool")...)
	return pkg
}

func execHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout int, asynchronous bool, order int, program string, arguments []model.HookArgument) model.NormalizedAsset {
	asset := shellHook(name, event, tools, timeout, asynchronous, order, "unused")
	asset.Hook.Handler = model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: arguments}
	asset.CapabilityUses = replaceUse(asset.CapabilityUses, "hook.command.shell", "hook.command.exec")
	return asset
}

func shellHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout int, asynchronous bool, order int, command string) model.NormalizedAsset {
	identity := model.AssetID("hook/" + name)
	location := model.SourceLocation{Path: model.RelativePath("source/hooks/" + name + "/hook.json")}
	var matcher *model.HookMatcher
	if tools != nil {
		matcher = &model.HookMatcher{Tools: append([]model.HookToolCategory(nil), tools...)}
	}
	capabilities := uses(location.Path, "asset.hook", "hook.command.shell", model.CapabilityKey("hook.event."+string(event)))
	if matcher != nil {
		capabilities = append(capabilities, model.CapabilityUse{Key: "hook.matcher.tool-category", Location: location})
	}
	if asynchronous {
		capabilities = append(capabilities, model.CapabilityUse{Key: "hook.async", Location: location})
	}
	return model.NormalizedAsset{Identity: identity, Kind: model.AssetKindHook, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}}, Hook: &model.HookDescriptor{Identity: identity, Location: location, Event: event, Matcher: matcher, Handler: model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}, TimeoutMilliseconds: timeout, Asynchronous: asynchronous, FailurePolicy: model.HookFailurePolicyOpen, Order: order}, CapabilityUses: capabilities}
}

func acknowledge(asset model.NormalizedAsset, keys ...model.CapabilityKey) []model.Acknowledgment {
	result := make([]model.Acknowledgment, len(keys))
	for index, key := range keys {
		result[index] = model.Acknowledgment{Asset: asset.Identity, Target: Target, Key: key, Reason: "accepted Copilot CLI behavior"}
	}
	return result
}

func uses(path model.RelativePath, keys ...model.CapabilityKey) []model.CapabilityUse {
	result := make([]model.CapabilityUse, len(keys))
	for index, key := range keys {
		result[index] = model.CapabilityUse{Key: key, Location: model.SourceLocation{Path: path}}
	}
	return result
}

func replaceUse(values []model.CapabilityUse, oldKey, newKey model.CapabilityKey) []model.CapabilityUse {
	result := append([]model.CapabilityUse(nil), values...)
	for index := range result {
		if result[index].Key == oldKey {
			result[index].Key = newKey
		}
	}
	return result
}

func pointerDescriptor(value model.HookDescriptor) *model.HookDescriptor { return &value }
func stringPointer(value string) *string                                 { return &value }

func plannedFiles(plan model.TargetPlan) map[model.RelativePath]model.PlannedFile {
	result := make(map[model.RelativePath]model.PlannedFile, len(plan.Files))
	for _, file := range plan.Files {
		result[file.Path] = file
	}
	return result
}

func plannedPaths(plan model.TargetPlan) []model.RelativePath {
	result := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		result[index] = file.Path
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func decodeObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return value
}

func assertGoldenTree(t *testing.T, plan model.TargetPlan, root string) {
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
	actual := plannedFiles(plan)
	if len(actual) != len(expected) {
		t.Fatalf("paths = %#v, want %d files", plannedPaths(plan), len(expected))
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok || !reflect.DeepEqual(got.Bytes, want.Bytes) || got.Executable != want.Executable {
			t.Errorf("file %q = %#v, want %#v", path, got, want)
		}
	}
}
