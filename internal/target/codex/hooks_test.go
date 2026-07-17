package codex

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestCodexCapabilitiesAndFormatRevision(t *testing.T) {
	if FormatRevision != 5 {
		t.Fatalf("FormatRevision = %d, want 5", FormatRevision)
	}
	want := map[model.CapabilityKey]model.CapabilityState{
		"asset.agent": model.CapabilityStateNative, "asset.hook": model.CapabilityStateNative,
		"asset.resource": model.CapabilityStateNative, "asset.native-resource": model.CapabilityStateUnsupported,
		"asset.skill": model.CapabilityStateNative, "hook.async": model.CapabilityStateUnsupported,
		"hook.command.exec": model.CapabilityStateNative, "hook.command.shell": model.CapabilityStateNative,
		"hook.decision.block": model.CapabilityStateNative, "hook.decision.rewrite-input": model.CapabilityStateUnsupported,
		"hook.event.notification": model.CapabilityStateUnsupported, "hook.event.post-compact": model.CapabilityStateNative,
		"hook.event.post-tool": model.CapabilityStateNative, "hook.event.post-tool-failure": model.CapabilityStateUnsupported,
		"hook.event.pre-compact": model.CapabilityStateNative, "hook.event.pre-tool": model.CapabilityStateNative,
		"hook.event.prompt-submit": model.CapabilityStateNative, "hook.event.session-end": model.CapabilityStateUnsupported,
		"hook.event.session-start": model.CapabilityStateNative, "hook.event.stop": model.CapabilityStateNative,
		"hook.failure.closed": model.CapabilityStateAdvisory, "hook.matcher.tool-category": model.CapabilityStateNative,
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

func TestRenderCodexHookGoldenAndTrustBoundary(t *testing.T) {
	path := model.RelativePath("scripts/check.sh")
	hook := codexExecHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 2_000, false, 0, "bash", []model.HookArgument{
		{Literal: codexStringPointer("-eu")}, {PackageFile: &path}, {Literal: codexStringPointer("a'b; touch /tmp/no")},
	})
	hook.Content.Files[path] = model.FileContent{Bytes: []byte("#!/bin/sh\n"), Executable: true}
	pkg := codexHookPackage("demo", hook)
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	files := codexPlannedFiles(plan)
	assertCodexGoldenTree(t, plan, "testdata/plugin-golden")
	if got, want := codexSortedPaths(plan), []model.RelativePath{".codex-plugin/plugin.json", "README.md", "assets/hooks/guard/scripts/check.sh", "hooks/hooks.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if _, exists := files["hooks.json"]; exists {
		t.Fatal("rendered stale root hooks.json path")
	}
	if got, want := string(files["hooks/hooks.json"].Bytes), "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"^Bash$\",\"hooks\":[{\"type\":\"command\",\"command\":\"'bash' '-eu' \\\"${PLUGIN_ROOT}\\\"'/assets/hooks/guard/scripts/check.sh' 'a'\\\"'\\\"'b; touch /tmp/no'\",\"timeout\":2}]}]}}\n"; got != want {
		t.Fatalf("hooks manifest = %q, want %q", got, want)
	}
	if !files["assets/hooks/guard/scripts/check.sh"].Executable {
		t.Fatal("hook payload lost executable intent")
	}
}

func TestRenderCodexShellQuotesHostilePackageFilePath(t *testing.T) {
	path := model.RelativePath("scripts/check-\"-$AMBIENT-$(touch INJECTED)-`touch BACKTICK`-'.sh")
	hook := codexExecHook("guard", model.HookEventStop, nil, 2_000, false, 0, "printf", []model.HookArgument{
		{Literal: codexStringPointer("%s")}, {PackageFile: &path},
	})
	hook.Content.Files[path] = model.FileContent{Bytes: []byte("payload")}
	plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}

	var manifest nativeHookManifest
	if err := json.Unmarshal(codexPlannedFiles(plan)["hooks/hooks.json"].Bytes, &manifest); err != nil {
		t.Fatal(err)
	}
	command := manifest.Hooks["Stop"][0].Hooks[0].Command
	wantCommand := "'printf' '%s' \"${PLUGIN_ROOT}\"'/assets/hooks/guard/scripts/check-\"-$AMBIENT-$(touch INJECTED)-`touch BACKTICK`-'\"'\"'.sh'"
	if command != wantCommand {
		t.Fatalf("command = %q, want %q", command, wantCommand)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is unavailable: %v", err)
	}
	workingDirectory := t.TempDir()
	pluginRoot := filepath.Join(workingDirectory, "plugin")
	process := exec.Command("sh", "-c", command)
	process.Dir = workingDirectory
	process.Env = []string{"AMBIENT=expanded", "PATH=" + os.Getenv("PATH"), "PLUGIN_ROOT=" + pluginRoot}
	output, err := process.Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), pluginRoot+"/assets/hooks/guard/"+string(path); got != want {
		t.Fatalf("rendered argument = %q, want %q", got, want)
	}
	for _, marker := range []string{"INJECTED", "BACKTICK"} {
		if _, err := os.Stat(filepath.Join(workingDirectory, marker)); !os.IsNotExist(err) {
			t.Fatalf("injection marker %q exists or could not be checked: %v", marker, err)
		}
	}
}

func TestRenderCodexShellHook(t *testing.T) {
	hook := codexExecHook("shell", model.HookEventStop, nil, 5_000, false, 0, "unused", nil)
	command := `printf '%s\n' done | tee /tmp/hook.log`
	hook.Hook.Handler = model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}
	hook.CapabilityUses = codexReplaceCapability(hook.CapabilityUses, "hook.command.exec", "hook.command.shell")
	plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if !strings.Contains(string(codexPlannedFiles(plan)["hooks/hooks.json"].Bytes), `"command":"printf '%s\\n' done | tee /tmp/hook.log"`) {
		t.Fatalf("shell command not preserved: %s", codexPlannedFiles(plan)["hooks/hooks.json"].Bytes)
	}
}

func TestRenderCodexHookFreePackageRegression(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Profile = model.TargetProfilePackage
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := codexSortedPaths(plan), []model.RelativePath{".codex-plugin/plugin.json", "README.md", "skills/guide/SKILL.md", "skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if _, exists := codexPlannedFiles(plan)["hooks/hooks.json"]; exists {
		t.Fatal("hook-free package emitted hooks/hooks.json")
	}
}

func TestRenderCodexRejectsUnsupportedEventAndPackageAgent(t *testing.T) {
	t.Run("event", func(t *testing.T) {
		hook := codexExecHook("notify", model.HookEventNotification, nil, 1_000, false, 0, "true", nil)
		plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
		if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || !strings.Contains(diagnostics[0].Message, "hook.event.notification") || len(plan.Files) != 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
	t.Run("package agent", func(t *testing.T) {
		pkg := codexAgentPackage(model.TargetProfilePackage)
		plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
		if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || !strings.Contains(diagnostics[0].Message, "asset.agent") || len(plan.Files) != 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
	for _, test := range []struct {
		name     string
		mutate   func(*model.NormalizedAsset)
		wantText string
	}{
		{name: "async", mutate: func(asset *model.NormalizedAsset) {
			asset.Hook.Event = model.HookEventPostCompact
			asset.Hook.Matcher = nil
			asset.CapabilityUses = codexReplaceCapability(asset.CapabilityUses, "hook.event.pre-tool", "hook.event.post-compact")
			asset.CapabilityUses = codexDropCapability(asset.CapabilityUses, "hook.matcher.tool-category")
			asset.Hook.Asynchronous = true
			asset.CapabilityUses = append(asset.CapabilityUses, model.CapabilityUse{Key: "hook.async", Location: asset.Hook.Location})
		}, wantText: "hook.async"},
		{name: "subsecond timeout", mutate: func(asset *model.NormalizedAsset) { asset.Hook.TimeoutMilliseconds = 1_250 }, wantText: "whole seconds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hook := codexExecHook("guard", model.HookEventPreTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 1_000, false, 0, "true", nil)
			test.mutate(&hook)
			plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
			if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" || !strings.Contains(diagnostics[0].Message, test.wantText) || len(plan.Files) != 0 {
				t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
			}
		})
	}
}

func TestRenderCodexRejectsLossyMatcherFailureAndOrdering(t *testing.T) {
	t.Run("unsupported matcher", func(t *testing.T) {
		hook := codexExecHook("read", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryRead}, 1_000, false, 0, "true", nil)
		plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "no lossless Codex matcher") || len(plan.Files) != 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
	t.Run("unmatched tool surface", func(t *testing.T) {
		hook := codexExecHook("all-tools", model.HookEventPostTool, nil, 1_000, false, 0, "true", nil)
		plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "require a lossless tool matcher") || len(plan.Files) != 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
	t.Run("closed failure", func(t *testing.T) {
		hook := codexExecHook("closed", model.HookEventStop, nil, 1_000, false, 0, "true", nil)
		hook.Hook.FailurePolicy = model.HookFailurePolicyClosed
		hook.CapabilityUses = append(hook.CapabilityUses, model.CapabilityUse{Key: "hook.failure.closed", Location: hook.Hook.Location})
		plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", hook)}))
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "hook.failure.closed") || len(plan.Files) != 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
	t.Run("concurrent order", func(t *testing.T) {
		first := codexExecHook("first", model.HookEventStop, nil, 1_000, false, 0, "true", nil)
		second := codexExecHook("second", model.HookEventStop, nil, 1_000, false, 1, "true", nil)
		plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", first, second)}))
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "concurrently") || len(plan.Files) != 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
	t.Run("disjoint matchers", func(t *testing.T) {
		command := codexExecHook("command", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryCommand}, 1_000, false, 0, "true", nil)
		mcp := codexExecHook("mcp", model.HookEventPostTool, []model.HookToolCategory{model.HookToolCategoryMCP}, 1_000, false, 1, "true", nil)
		plan, diagnostics := Render(separate([]model.NormalizedPackage{codexHookPackage("demo", command, mcp)}))
		if len(diagnostics) != 0 || len(plan.Files) == 0 {
			t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
		}
	})
}

func TestRenderCodexPreservesProjectAgentPath(t *testing.T) {
	pkg := codexAgentPackage(model.TargetProfileProject)
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := codexSortedPaths(plan), []model.RelativePath{".codex-plugin/plugin.json", ".codex/agents/reviewer.toml"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if got := string(codexPlannedFiles(plan)[".codex/agents/reviewer.toml"].Bytes); !strings.Contains(got, `description = "Review code"`) || !strings.Contains(got, "developer_instructions") {
		t.Fatalf("agent = %q", got)
	}
}

func TestRenderCodexReportsHookCollisionWithoutPartialOutput(t *testing.T) {
	first := codexExecHook("duplicate", model.HookEventStop, nil, 1_000, false, 0, "true", nil)
	second := codexExecHook("duplicate", model.HookEventStop, nil, 1_000, false, 1, "true", nil)
	second.Hook.Location.Path = "source/hooks/copy/hook.json"
	pkg := codexHookPackage("demo", first, second)
	plan, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) == 0 || diagnostics[0].Code != "duplicate-hook-id" || len(plan.Files) != 0 {
		t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
	}
}

func TestRenderCodexDeterministicSeparatePackages(t *testing.T) {
	alpha := codexHookPackage("alpha", codexExecHook("zeta", model.HookEventStop, nil, 1_000, false, 2, "true", nil), codexExecHook("alpha", model.HookEventSessionStart, nil, 1_000, false, 1, "true", nil))
	zeta := codexHookPackage("zeta", codexExecHook("check", model.HookEventPostCompact, nil, 1_000, false, 0, "true", nil))
	first, diagnostics := Render(separate([]model.NormalizedPackage{zeta, alpha}))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	alpha.Assets[0], alpha.Assets[1] = alpha.Assets[1], alpha.Assets[0]
	second, diagnostics := Render(separate([]model.NormalizedPackage{alpha, zeta}))
	if len(diagnostics) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("renders differ: (%#v, %#v, %#v)", first, second, diagnostics)
	}
	if got, want := codexSortedPaths(first), []model.RelativePath{"alpha/.codex-plugin/plugin.json", "alpha/README.md", "alpha/hooks/hooks.json", "zeta/.codex-plugin/plugin.json", "zeta/README.md", "zeta/hooks/hooks.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func codexHookPackage(identity model.PackageID, assets ...model.NormalizedAsset) model.NormalizedPackage {
	return model.NormalizedPackage{Identity: identity, Target: Target, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{"version": "1.0.0", "description": "Hooks"}, Assets: assets}
}

func codexAgentPackage(profile model.TargetProfile) model.NormalizedPackage {
	location := model.SourceLocation{Path: "source/agents/reviewer.md"}
	return model.NormalizedPackage{Identity: "demo", Target: Target, Profile: profile, Assets: []model.NormalizedAsset{{
		Identity: "agent/reviewer", Kind: model.AssetKindAgent,
		Content:        model.AssetContent{Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"}, Body: "Review carefully.", Files: map[model.RelativePath]model.FileContent{}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.agent", Location: location}},
	}}}
}

func codexExecHook(name string, event model.HookEvent, tools []model.HookToolCategory, timeout int, asynchronous bool, order int, program string, arguments []model.HookArgument) model.NormalizedAsset {
	identity := model.AssetID("hook/" + name)
	location := model.SourceLocation{Path: model.RelativePath("source/hooks/" + name + "/hook.json")}
	var matcher *model.HookMatcher
	if tools != nil {
		matcher = &model.HookMatcher{Tools: append([]model.HookToolCategory(nil), tools...)}
	}
	uses := codexCapabilityUses(location.Path, "asset.hook", "hook.command.exec", model.CapabilityKey("hook.event."+string(event)))
	if matcher != nil {
		uses = append(uses, model.CapabilityUse{Key: "hook.matcher.tool-category", Location: location})
	}
	if asynchronous {
		uses = append(uses, model.CapabilityUse{Key: "hook.async", Location: location})
	}
	return model.NormalizedAsset{Identity: identity, Kind: model.AssetKindHook, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}}, Hook: &model.HookDescriptor{
		Identity: identity, Location: location, Event: event, Matcher: matcher,
		Handler:             model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: arguments},
		TimeoutMilliseconds: timeout, Asynchronous: asynchronous, FailurePolicy: model.HookFailurePolicyOpen, Order: order,
	}, CapabilityUses: uses}
}

func codexCapabilityUses(path model.RelativePath, keys ...model.CapabilityKey) []model.CapabilityUse {
	uses := make([]model.CapabilityUse, len(keys))
	for index, key := range keys {
		uses[index] = model.CapabilityUse{Key: key, Location: model.SourceLocation{Path: path}}
	}
	return uses
}

func codexDropCapability(uses []model.CapabilityUse, key model.CapabilityKey) []model.CapabilityUse {
	result := make([]model.CapabilityUse, 0, len(uses))
	for _, use := range uses {
		if use.Key != key {
			result = append(result, use)
		}
	}
	return result
}

func codexReplaceCapability(uses []model.CapabilityUse, oldKey, newKey model.CapabilityKey) []model.CapabilityUse {
	result := append([]model.CapabilityUse(nil), uses...)
	for index := range result {
		if result[index].Key == oldKey {
			result[index].Key = newKey
		}
	}
	return result
}

func codexStringPointer(value string) *string { return &value }

func codexPlannedFiles(plan model.TargetPlan) map[model.RelativePath]model.PlannedFile {
	files := make(map[model.RelativePath]model.PlannedFile, len(plan.Files))
	for _, file := range plan.Files {
		files[file.Path] = file
	}
	return files
}

func codexSortedPaths(plan model.TargetPlan) []model.RelativePath {
	paths := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		paths[index] = file.Path
	}
	return paths
}

func assertCodexGoldenTree(t *testing.T, plan model.TargetPlan, root string) {
	t.Helper()
	files := codexPlannedFiles(plan)
	seen := make(map[model.RelativePath]struct{}, len(files))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		plannedPath := model.RelativePath(filepath.ToSlash(relative))
		planned, exists := files[plannedPath]
		if !exists {
			t.Errorf("golden file %q is absent from plan", plannedPath)
			return nil
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(planned.Bytes, want) {
			t.Errorf("file %q = %q, want %q", plannedPath, planned.Bytes, want)
		}
		seen[plannedPath] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(files) {
		t.Fatalf("golden has %d files, plan has %d", len(seen), len(files))
	}
}
