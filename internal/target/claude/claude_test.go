package claude

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

func TestRenderUsesClaudeNativeSkillRoot(t *testing.T) {
	plan, diagnostics := Render(separate(skillPackage()))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path}, []model.RelativePath{".claude/skills/guide/SKILL.md", ".claude/skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestRenderCommandsInProjectAndPackageLayouts(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile model.TargetProfile
		path    model.RelativePath
	}{
		{name: "project", profile: model.TargetProfileProject, path: ".claude/commands/resume-from.md"},
		{name: "package", profile: model.TargetProfilePackage, path: "commands/resume-from.md"},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: test.profile, Metadata: model.PackageMetadata{"version": "1.0.0"}, Assets: []model.NormalizedAsset{commandAsset("resume-from")}}
			plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
			if len(diagnostics) != 0 {
				t.Fatalf("Render() diagnostics = %#v", diagnostics)
			}
			file, exists := plannedFiles(plan)[test.path]
			if !exists || string(file.Bytes) != "---\n{\"description\":\"Resume from a saved handoff.\"}\n---\nResume the session.\n" {
				t.Fatalf("command file = %#v; paths = %#v", file, sortedPlannedPaths(plan))
			}
			reversed := pkg
			reversed.Assets = []model.NormalizedAsset{commandAsset("z-last"), commandAsset("resume-from")}
			first, diagnostics := Render(separate([]model.NormalizedPackage{reversed}))
			if len(diagnostics) != 0 {
				t.Fatalf("first Render() diagnostics = %#v", diagnostics)
			}
			reversed.Assets[0], reversed.Assets[1] = reversed.Assets[1], reversed.Assets[0]
			second, diagnostics := Render(separate([]model.NormalizedPackage{reversed}))
			if len(diagnostics) != 0 || !reflect.DeepEqual(first, second) {
				t.Fatalf("command rendering is not deterministic: diagnostics = %#v", diagnostics)
			}
		})
	}
}

func TestRenderRejectsCommandSupportFilesWithoutPartialOutput(t *testing.T) {
	asset := commandAsset("resume-from")
	asset.Content.Files["script.sh"] = model.FileContent{Bytes: []byte("exit 0\n")}
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "invalid-package-output" || !strings.Contains(diagnostics[0].Message, "cannot contain support files") {
		t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
	}
	if len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
		t.Fatalf("rejected command produced partial output: %#v", plan)
	}
}

func TestRenderRejectsNonSkillAssets(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Assets[0].Kind = model.AssetKindAgent
	pkg.Assets[0].Identity = "agent/reviewer"
	_, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func TestClaudeCapabilitiesAndFormatRevision(t *testing.T) {
	if FormatRevision != 7 {
		t.Fatalf("FormatRevision = %d, want 7", FormatRevision)
	}
	want := map[model.CapabilityKey]model.CapabilityState{
		"asset.agent":                  model.CapabilityStateNative,
		"asset.hook":                   model.CapabilityStateNative,
		"asset.command":                model.CapabilityStateNative,
		"asset.resource":               model.CapabilityStateNative,
		"asset.native-resource":        model.CapabilityStateUnsupported,
		"asset.skill":                  model.CapabilityStateNative,
		"hook.async":                   model.CapabilityStateNative,
		"hook.command.exec":            model.CapabilityStateNative,
		"hook.command.shell":           model.CapabilityStateNative,
		"hook.decision.block":          model.CapabilityStateNative,
		"hook.decision.rewrite-input":  model.CapabilityStateNative,
		"hook.event.notification":      model.CapabilityStateNative,
		"hook.event.post-compact":      model.CapabilityStateNative,
		"hook.event.post-tool":         model.CapabilityStateNative,
		"hook.event.post-tool-failure": model.CapabilityStateNative,
		"hook.event.pre-compact":       model.CapabilityStateNative,
		"hook.event.pre-tool":          model.CapabilityStateNative,
		"hook.event.prompt-submit":     model.CapabilityStateNative,
		"hook.event.session-end":       model.CapabilityStateNative,
		"hook.event.session-start":     model.CapabilityStateNative,
		"hook.event.stop":              model.CapabilityStateNative,
		"hook.failure.closed":          model.CapabilityStateAdvisory,
		"hook.matcher.tool-category":   model.CapabilityStateNative,
	}
	got := make(map[model.CapabilityKey]model.CapabilityState)
	for _, rule := range Capabilities() {
		if _, duplicate := got[rule.Key]; duplicate {
			t.Fatalf("capability %q is duplicated", rule.Key)
		}
		got[rule.Key] = rule.State
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities() = %#v, want %#v", got, want)
	}
}

func TestRenderClaudeHookGolden(t *testing.T) {
	pkg := goldenHookPackage()
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	assertGoldenTree(t, plan, "testdata/plugin-golden")
	if got, want := plan.NativeChecks, []model.NativeCheck{{
		Program:   "claude",
		Arguments: []string{"plugin", "validate", "--strict", "."},
		Location:  model.SourceLocation{Path: "internal/target/claude/codec.go"},
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeChecks = %#v, want %#v", got, want)
	}

	files := plannedFiles(plan)
	manifest := decodeJSONObject(t, files[".claude-plugin/plugin.json"].Bytes)
	if _, exists := manifest["hooks"]; exists {
		t.Fatalf("plugin manifest declares hooks: %#v", manifest)
	}
	hookFile, exists := files["hooks/hooks.json"]
	if !exists {
		t.Fatal("hook package did not emit hooks/hooks.json")
	}
	hooks := decodeJSONObject(t, hookFile.Bytes)
	events := hooks["hooks"].(map[string]any)
	preTool := events["PreToolUse"].([]any)
	if got := preTool[0].(map[string]any)["matcher"]; got != "Read" {
		t.Fatalf("first PreToolUse matcher = %#v, want Read", got)
	}
	if got := preTool[1].(map[string]any)["matcher"]; got != "Bash" {
		t.Fatalf("second PreToolUse matcher = %#v, want Bash", got)
	}
	postHandler := events["PostToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if postHandler["async"] != true || postHandler["timeout"] != 7.25 {
		t.Fatalf("async PostToolUse handler = %#v", postHandler)
	}
	if got, want := postHandler["args"], []any{"${CLAUDE_PLUGIN_ROOT}/hooks/async/scripts/check.js", "--fix"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("exec args = %#v, want %#v", got, want)
	}
	preHandler := preTool[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if preHandler["command"] != `cd "${CLAUDE_PLUGIN_ROOT}" && printf ready | cat` {
		t.Fatalf("adopted shell command = %#v", preHandler)
	}
	if _, hasArgs := preHandler["args"]; hasArgs {
		t.Fatalf("shell handler unexpectedly has args: %#v", preHandler)
	}
	if !files["hooks/guard/scripts/guard.sh"].Executable || files["hooks/async/scripts/check.js"].Executable {
		t.Fatalf("payload executable intent = (%t, %t)", files["hooks/guard/scripts/guard.sh"].Executable, files["hooks/async/scripts/check.js"].Executable)
	}

	reversed := pkg
	reversed.Assets = append([]model.NormalizedAsset(nil), pkg.Assets...)
	for left, right := 0, len(reversed.Assets)-1; left < right; left, right = left+1, right-1 {
		reversed.Assets[left], reversed.Assets[right] = reversed.Assets[right], reversed.Assets[left]
	}
	reordered, diagnostics := Render(separate([]model.NormalizedPackage{reversed}))
	if len(diagnostics) != 0 || !reflect.DeepEqual(plan, reordered) {
		t.Fatalf("reordered render = (%#v, %#v), want identical plan", reordered, diagnostics)
	}
}

func TestRenderClaudeHookFreePackageRegression(t *testing.T) {
	pkg := model.NormalizedPackage{
		Identity: "plain",
		Target:   Target,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{"version": "1.0.0", "description": "Plain plugin", "author": "Agent Bundler"},
		Assets: []model.NormalizedAsset{{
			Identity:       "skill/guide",
			Kind:           model.AssetKindSkill,
			Content:        model.AssetContent{Frontmatter: map[string]any{"description": "Guide", "name": "guide"}, Body: "Guide.\n", Files: map[model.RelativePath]model.FileContent{}},
			CapabilityUses: capabilityUses("source/skills/guide/SKILL.md", "asset.skill"),
		}},
	}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	files := plannedFiles(plan)
	if got, want := sortedPlannedPaths(plan), []model.RelativePath{".claude-plugin/plugin.json", "README.md", "skills/guide/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hook-free paths = %#v, want %#v", got, want)
	}
	if strings.Contains(string(files[".claude-plugin/plugin.json"].Bytes), `"hooks"`) {
		t.Fatalf("hook-free manifest declares hooks: %s", files[".claude-plugin/plugin.json"].Bytes)
	}
	if _, exists := files["hooks/hooks.json"]; exists {
		t.Fatal("hook-free package emitted hooks/hooks.json")
	}
}

func TestRenderClaudeRejectsUnsupportedHookCells(t *testing.T) {
	for _, test := range []struct {
		name     string
		mutate   func(*model.NormalizedAsset)
		wantCode string
		wantText string
	}{
		{
			name: "closed failure policy",
			mutate: func(asset *model.NormalizedAsset) {
				asset.Hook.FailurePolicy = model.HookFailurePolicyClosed
				asset.CapabilityUses = append(asset.CapabilityUses, model.CapabilityUse{Key: "hook.failure.closed", Location: asset.Hook.Location})
			},
			wantCode: "missing-capability-acknowledgment", wantText: "hook.failure.closed",
		},
		{
			name: "other tool category",
			mutate: func(asset *model.NormalizedAsset) {
				asset.Hook.Matcher.Tools = []model.HookToolCategory{model.HookToolCategoryOther}
			},
			wantCode: "unsupported-hook-semantics", wantText: "no lossless Claude matcher",
		},
		{
			name: "unknown handler cell",
			mutate: func(asset *model.NormalizedAsset) {
				asset.CapabilityUses = append(asset.CapabilityUses, model.CapabilityUse{Key: "hook.command.http", Location: asset.Hook.Location})
			},
			wantCode: "unsupported-capability", wantText: "hook.command.http",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{
				execHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 1_000, false, 1, "bash", nil),
			}}
			test.mutate(&pkg.Assets[0])
			plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
			if len(diagnostics) != 1 || diagnostics[0].Code != test.wantCode || !strings.Contains(diagnostics[0].Message, test.wantText) {
				t.Fatalf("Render() = (%#v, %#v), want %q containing %q", plan, diagnostics, test.wantCode, test.wantText)
			}
			if len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
				t.Fatalf("unsupported hook produced a partial plan: %#v", plan)
			}
		})
	}
}

func TestRenderClaudeTranslatesAcknowledgedPreToolDecisions(t *testing.T) {
	asset := execHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 1_000, false, 1, "bash", nil)
	asset.Hook.FailurePolicy = model.HookFailurePolicyClosed
	asset.CapabilityUses = append(asset.CapabilityUses, model.CapabilityUse{Key: "hook.failure.closed", Location: asset.Hook.Location}, model.CapabilityUse{Key: "hook.decision.block", Location: asset.Hook.Location})
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{asset}, Acknowledgments: []model.Acknowledgment{{Asset: asset.Identity, Target: Target, Key: "hook.failure.closed", Reason: "timeouts fail open but explicit canonical deny is translated"}}}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	command := decodeJSONObject(t, plannedFiles(plan)["hooks/hooks.json"].Bytes)["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	for _, value := range []string{"node", "claude", "hook/guard"} {
		if !strings.Contains(command, value) {
			t.Fatalf("translated command %q is missing %q", command, value)
		}
	}
}

func TestRenderClaudeMapsEveryPortableEvent(t *testing.T) {
	events := []struct {
		portable model.HookEvent
		native   string
	}{
		{model.HookEventSessionStart, "SessionStart"},
		{model.HookEventSessionEnd, "SessionEnd"},
		{model.HookEventPromptSubmit, "UserPromptSubmit"},
		{model.HookEventPreTool, "PreToolUse"},
		{model.HookEventPostTool, "PostToolUse"},
		{model.HookEventPostToolFailure, "PostToolUseFailure"},
		{model.HookEventStop, "Stop"},
		{model.HookEventNotification, "Notification"},
		{model.HookEventPreCompact, "PreCompact"},
		{model.HookEventPostCompact, "PostCompact"},
	}
	assets := make([]model.NormalizedAsset, 0, len(events))
	for index, event := range events {
		assets = append(assets, execHook(string(event.portable), event.portable, nil, 1_000, false, index, "true", nil))
	}
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: assets}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	manifest := decodeJSONObject(t, plannedFiles(plan)["hooks/hooks.json"].Bytes)
	nativeEvents := manifest["hooks"].(map[string]any)
	if len(nativeEvents) != len(events) {
		t.Fatalf("native events = %#v", nativeEvents)
	}
	for _, event := range events {
		if _, ok := nativeEvents[event.native]; !ok {
			t.Errorf("native event %q for %q is missing", event.native, event.portable)
		}
	}
}

func TestRenderClaudeMatcherAndZeroArgumentExec(t *testing.T) {
	tools := []model.HookToolCategory{
		model.HookToolCategoryCommand,
		model.HookToolCategoryRead,
		model.HookToolCategoryWrite,
		model.HookToolCategoryEdit,
		model.HookToolCategorySearch,
		model.HookToolCategoryWeb,
		model.HookToolCategoryTask,
		model.HookToolCategoryMCP,
	}
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{
		execHook("match", model.HookEventPreTool, tools, 1_250, false, 0, "validator", nil),
	}}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	manifest := decodeJSONObject(t, plannedFiles(plan)["hooks/hooks.json"].Bytes)
	group := manifest["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if got, want := group["matcher"], "Bash|Read|Write|Edit|NotebookEdit|Glob|Grep|WebFetch|WebSearch|Task|Agent|^mcp__.*$"; got != want {
		t.Fatalf("matcher = %#v, want %#v", got, want)
	}
	handler := group["hooks"].([]any)[0].(map[string]any)
	if got, ok := handler["args"].([]any); !ok || len(got) != 0 {
		t.Fatalf("zero-argument exec args = %#v, want []", handler["args"])
	}
	if handler["timeout"] != 1.25 {
		t.Fatalf("timeout = %#v, want 1.25", handler["timeout"])
	}
}

func TestRenderClaudeReportsHookCollisionsWithoutPartialOutput(t *testing.T) {
	first := execHook("duplicate", model.HookEventStop, nil, 1_000, false, 0, "true", nil)
	second := execHook("duplicate", model.HookEventStop, nil, 1_000, false, 1, "true", nil)
	second.Hook.Location.Path = "source/hooks/copy/hook.json"
	pkg := model.NormalizedPackage{Identity: "demo", Target: Target, Profile: model.TargetProfilePackage, Assets: []model.NormalizedAsset{first, second}}

	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) == 0 || diagnostics[0].Code != "duplicate-hook-id" || !strings.Contains(diagnostics[0].Message, "source/hooks/copy/hook.json") {
		t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
	}
	if len(plan.Files) != 0 || len(plan.NativeChecks) != 0 {
		t.Fatalf("collision produced a partial plan: %#v", plan)
	}
}

func TestRenderClaudeDeclaresStrictValidatorPerPluginRoot(t *testing.T) {
	zeta := hookFreePackage("zeta")
	alpha := hookFreePackage("alpha")
	plan, diagnostics := Render(separate([]model.NormalizedPackage{zeta, alpha}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	want := []model.NativeCheck{
		{Program: "claude", Arguments: []string{"plugin", "validate", "--strict", "./alpha"}, Location: model.SourceLocation{Path: "internal/target/claude/codec.go"}},
		{Program: "claude", Arguments: []string{"plugin", "validate", "--strict", "./zeta"}, Location: model.SourceLocation{Path: "internal/target/claude/codec.go"}},
	}
	if !reflect.DeepEqual(plan.NativeChecks, want) {
		t.Fatalf("NativeChecks = %#v, want %#v", plan.NativeChecks, want)
	}
}

func TestNativeChecksAnchorOptionLikePluginRoots(t *testing.T) {
	checks := nativeChecks([]model.PackageID{"--help", "-version"}, false)
	want := [][]string{
		{"plugin", "validate", "--strict", "./--help"},
		{"plugin", "validate", "--strict", "./-version"},
	}
	for index := range checks {
		if !reflect.DeepEqual(checks[index].Arguments, want[index]) {
			t.Fatalf("NativeChecks[%d].Arguments = %#v, want %#v", index, checks[index].Arguments, want[index])
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

func commandAsset(name string) model.NormalizedAsset {
	identity := model.AssetID("command/" + name)
	location := model.SourceLocation{Path: model.RelativePath("source/commands/" + name + ".md")}
	return model.NormalizedAsset{
		Identity: identity, Kind: model.AssetKindCommand,
		Content:        model.AssetContent{Frontmatter: map[string]any{"description": "Resume from a saved handoff."}, Body: "Resume the session.\n", Files: map[model.RelativePath]model.FileContent{}},
		Command:        &model.CommandDescriptor{Identity: identity, Location: location, Name: name, Description: "Resume from a saved handoff."},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.command", Location: location}},
	}
}

func goldenHookPackage() model.NormalizedPackage {
	guardPath := model.RelativePath("scripts/guard.sh")
	asyncPath := model.RelativePath("scripts/check.js")
	guard := execHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 5_000, false, 10, "bash", []model.HookArgument{
		{Literal: stringPointer("-eu")},
		{PackageFile: &guardPath},
	})
	guard.Content.Files[guardPath] = model.FileContent{Bytes: []byte("#!/usr/bin/env bash\ncat >/dev/null\n"), Executable: true, Origin: []model.SourceLocation{{Path: "source/hooks/guard/scripts/guard.sh"}}}
	async := execHook("async", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryWrite, model.HookToolCategoryEdit}, 7_250, true, 1, "node", []model.HookArgument{
		{PackageFile: &asyncPath},
		{Literal: stringPointer("--fix")},
	})
	async.Content.Files[asyncPath] = model.FileContent{Bytes: []byte("process.stdin.resume();\n"), Origin: []model.SourceLocation{{Path: "source/hooks/async/scripts/check.js"}}}
	shell := shellHook("adopted", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryRead}, 30_000, 2, `cd "${CLAUDE_PLUGIN_ROOT}" && printf ready | cat`)
	skill := model.NormalizedAsset{
		Identity:       "skill/guide",
		Kind:           model.AssetKindSkill,
		Content:        model.AssetContent{Frontmatter: map[string]any{"description": "Guide", "name": "guide"}, Body: "Use this guide.\n", Files: map[model.RelativePath]model.FileContent{}},
		CapabilityUses: capabilityUses("source/skills/guide/SKILL.md", "asset.skill"),
	}
	return model.NormalizedPackage{
		Identity: "demo",
		Target:   Target,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{"version": "1.2.3", "description": "Demo hooks", "author": "Agent Bundler"},
		Assets:   []model.NormalizedAsset{guard, skill, async, shell},
	}
}

func execHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout int, asynchronous bool, order int, program string, arguments []model.HookArgument) model.NormalizedAsset {
	identity := model.AssetID("hook/" + name)
	location := model.SourceLocation{Path: model.RelativePath("source/hooks/" + name + "/hook.json")}
	var matcher *model.HookMatcher
	if tools != nil {
		matcher = &model.HookMatcher{Tools: append([]model.HookToolCategory(nil), tools...)}
	}
	uses := capabilityUses(location.Path,
		"asset.hook",
		"hook.command.exec",
		model.CapabilityKey("hook.event."+string(event)),
	)
	if matcher != nil {
		uses = append(uses, model.CapabilityUse{Key: "hook.matcher.tool-category", Location: location})
	}
	if asynchronous {
		uses = append(uses, model.CapabilityUse{Key: "hook.async", Location: location})
	}
	return model.NormalizedAsset{
		Identity: identity,
		Kind:     model.AssetKindHook,
		Content:  model.AssetContent{Files: map[model.RelativePath]model.FileContent{}},
		Hook: &model.HookDescriptor{
			Identity:            identity,
			Location:            location,
			Event:               event,
			Matcher:             matcher,
			Handler:             model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: arguments},
			TimeoutMilliseconds: timeout,
			Asynchronous:        asynchronous,
			FailurePolicy:       model.HookFailurePolicyOpen,
			Order:               order,
		},
		CapabilityUses: uses,
	}
}

func shellHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout, order int, command string) model.NormalizedAsset {
	asset := execHook(name, event, tools, timeout, false, order, "unused", nil)
	asset.Hook.Handler = model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}
	asset.CapabilityUses = replaceCapability(asset.CapabilityUses, "hook.command.exec", "hook.command.shell")
	return asset
}

func hookFreePackage(identity model.PackageID) model.NormalizedPackage {
	return model.NormalizedPackage{
		Identity: identity,
		Target:   Target,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{"version": "1.0.0", "description": "Plugin " + string(identity), "author": "Agent Bundler"},
		Assets:   []model.NormalizedAsset{},
	}
}

func capabilityUses(path model.RelativePath, keys ...model.CapabilityKey) []model.CapabilityUse {
	uses := make([]model.CapabilityUse, len(keys))
	for index, key := range keys {
		uses[index] = model.CapabilityUse{Key: key, Location: model.SourceLocation{Path: path}}
	}
	return uses
}

func replaceCapability(uses []model.CapabilityUse, oldKey, newKey model.CapabilityKey) []model.CapabilityUse {
	result := append([]model.CapabilityUse(nil), uses...)
	for index := range result {
		if result[index].Key == oldKey {
			result[index].Key = newKey
		}
	}
	return result
}

func assertGoldenTree(t *testing.T, plan model.TargetPlan, root string) {
	t.Helper()
	expected := make(map[model.RelativePath]model.PlannedFile)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
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
	})
	if err != nil {
		t.Fatalf("walk golden tree: %v", err)
	}
	actual := plannedFiles(plan)
	if len(actual) != len(expected) {
		t.Fatalf("planned file count = %d, want %d; paths = %#v", len(actual), len(expected), sortedPlannedPaths(plan))
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok {
			t.Errorf("planned file %q is missing", path)
			continue
		}
		if !reflect.DeepEqual(got.Bytes, want.Bytes) || got.Executable != want.Executable {
			t.Errorf("planned file %q = (bytes %q, executable %t), want (bytes %q, executable %t)", path, got.Bytes, got.Executable, want.Bytes, want.Executable)
		}
	}
}

func plannedFiles(plan model.TargetPlan) map[model.RelativePath]model.PlannedFile {
	files := make(map[model.RelativePath]model.PlannedFile, len(plan.Files))
	for _, file := range plan.Files {
		files[file.Path] = file
	}
	return files
}

func sortedPlannedPaths(plan model.TargetPlan) []model.RelativePath {
	paths := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		paths[index] = file.Path
	}
	sort.Slice(paths, func(left, right int) bool { return paths[left] < paths[right] })
	return paths
}

func decodeJSONObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode JSON %q: %v", data, err)
	}
	return value
}

func stringPointer(value string) *string { return &value }
