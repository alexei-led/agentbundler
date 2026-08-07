package claudeplugin

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	claudetarget "github.com/alexei-led/agentbundler/internal/target/claude"
)

func TestInspectClaudePluginImportsOfficialDefaultHooksPayloadsAndComponents(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","description":"Demo","version":"1.0.0"}`)
	writeFixture(t, workspace, "source/plugin/.claude-plugin/marketplace.json", `{"owner":{"name":"team"},"plugins":[{"source":"./"}]}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"description":"Validate commands","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"bash","args":["${CLAUDE_PLUGIN_ROOT}/scripts/check.sh","--strict"],"timeout":5}]}]}}`)
	writeFixture(t, workspace, "source/plugin/scripts/check.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(workspace, "source/plugin/scripts/check.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "---\n{\"description\":\"alpha\"}\n---\nUse alpha.\n")
	writeFixture(t, workspace, "source/plugin/skills/alpha/scripts/run.sh", "#!/bin/sh\n")
	writeFixture(t, workspace, "source/plugin/agents/review.md", "---\n{\"model\":\"sonnet\"}\n---\nReview.\n")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/agent/review/asset.json", `{"capabilities":["tool-use"]}`)
	writeFixture(t, workspace, "source/plugin/commands/malformed.md", "---\n[invalid\n---\nBody.\n")
	writeFixture(t, workspace, "source/plugin/commands/native.md", "native\n")

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if len(inventory.Packages) != 1 || inventory.Packages[0].Identity != "demo" {
		t.Fatalf("packages = %#v", inventory.Packages)
	}
	pkg := inventory.Packages[0]
	if pkg.Metadata["description"] != "Demo" || pkg.Metadata["version"] != "1.0.0" {
		t.Fatalf("metadata = %#v", pkg.Metadata)
	}
	if owner, ok := pkg.Metadata["owner"].(map[string]any); !ok || owner["name"] != "team" {
		t.Fatalf("marketplace metadata = %#v", pkg.Metadata)
	}
	if len(pkg.Assets) != 3 || pkg.Assets[0].Identity != "agent/review" || pkg.Assets[1].Identity != "hook/PreToolUse-1" || pkg.Assets[2].Identity != "skill/alpha" {
		t.Fatalf("assets = %#v", pkg.Assets)
	}
	if got := string(pkg.Assets[2].Base.Files["scripts/run.sh"].Bytes); got != "#!/bin/sh\n" {
		t.Fatalf("skill file = %q", got)
	}
	hookAsset := pkg.Assets[1]
	hook := hookAsset.Hook
	if hook == nil || hook.Location.Path != "source/plugin/hooks/hooks.json" || hook.Event != model.HookEventPreTool || hook.Handler.Mode != model.HookHandlerModeExec || hook.TimeoutMilliseconds != 5_000 || hook.FailurePolicy != model.HookFailurePolicyOpen || hook.Order != 0 {
		t.Fatalf("typed hook = %#v", hook)
	}
	if hook.Handler.Program == nil || *hook.Handler.Program != "bash" || len(hook.Handler.Arguments) != 2 || hook.Handler.Arguments[0].PackageFile == nil || *hook.Handler.Arguments[0].PackageFile != "scripts/check.sh" || hook.Handler.Arguments[1].Literal == nil || *hook.Handler.Arguments[1].Literal != "--strict" {
		t.Fatalf("typed hook command = %#v", hook.Handler)
	}
	if hook.Matcher == nil || !reflect.DeepEqual(hook.Matcher.Tools, []model.HookToolCategory{model.HookToolCategoryCommand}) {
		t.Fatalf("typed hook matcher = %#v", hook.Matcher)
	}
	payload := hookAsset.Base.Files["scripts/check.sh"]
	if string(payload.Bytes) != "#!/bin/sh\n" || !payload.Executable || !reflect.DeepEqual(payload.Origin, []model.SourceLocation{{Path: "source/plugin/scripts/check.sh"}}) {
		t.Fatalf("hook payload = %#v", payload)
	}
	if got, want := capabilityKeys(hookAsset.CapabilityUses), []model.CapabilityKey{"asset.hook", "hook.command.exec", "hook.event.pre-tool", "hook.matcher.tool-category"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hook capabilities = %#v, want %#v", got, want)
	}
	if got := pkg.Assets[0].CapabilityUses; len(got) != 1 || got[0].Key != "tool-use" {
		t.Fatalf("agent capabilities = %#v", got)
	}
	wantNativeGaps := []string{"source/plugin/commands/malformed.md", "source/plugin/commands/native.md"}
	if len(inventory.NativeGaps) != len(wantNativeGaps) {
		t.Fatalf("native gaps = %#v", inventory.NativeGaps)
	}
	for index, component := range wantNativeGaps {
		gap := inventory.NativeGaps[index]
		if gap.Component != component || gap.Target == nil || *gap.Target != model.TargetClaude {
			t.Fatalf("native gap %d = %#v, want Claude gap %q", index, gap, component)
		}
	}
	if len(inventory.Inputs) != 10 || !containsInput(inventory, "source/plugin/hooks/hooks.json") || !containsInput(inventory, "source/plugin/scripts/check.sh") {
		t.Fatalf("inputs = %#v", inventory.Inputs)
	}
}

func TestInspectClaudePluginImportsGeneratedClaudePackage(t *testing.T) {
	pkg := model.NormalizedPackage{
		Identity: "generated",
		Target:   model.TargetClaude,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{
			"version": "1.2.3", "description": "Generated package", "author": "Agent Bundler",
			"homepage": "https://example.com/generated", "repository": "https://github.com/example/generated",
			"license": "MIT", "keywords": []any{"agents", "hooks"},
		},
		Assets: []model.NormalizedAsset{{
			Identity: "skill/guide", Kind: model.AssetKindSkill,
			Content:        model.AssetContent{Body: "Guide.\n", Files: map[model.RelativePath]model.FileContent{}},
			CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/skills/guide/SKILL.md"}}},
		}},
	}
	plan, diagnostics := claudetarget.Render(model.TargetRenderInput{Packages: []model.NormalizedPackage{pkg}, PackageMode: model.TargetPackageModeSeparate})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	workspace := t.TempDir()
	for _, file := range plan.Files {
		writeFixture(t, workspace, "source/plugin/"+string(file.Path), string(file.Bytes))
	}

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("InspectClaudePlugin() diagnostics = %#v", diagnostics)
	}
	if len(inventory.Packages) != 1 || inventory.Packages[0].Identity != pkg.Identity {
		t.Fatalf("packages = %#v", inventory.Packages)
	}
	if got := inventory.Packages[0].Metadata; got["version"] != "1.2.3" || got["description"] != "Generated package" {
		t.Fatalf("metadata = %#v", got)
	}
}

func TestParsePluginManifestAcceptsAllDocumentedFields(t *testing.T) {
	data := []byte(`{
		"$schema":"https://json.schemastore.org/claude-code-plugin-manifest.json",
		"name":"demo","version":"1.0.0","description":"Demo","author":{"name":"Team"},
		"homepage":"https://example.com","repository":"https://github.com/example/demo","license":"MIT","keywords":[],
		"dependencies":{},"hooks":{},"commands":[],"agents":[],"skills":[],"outputStyles":[],"themes":[],"channels":{},
		"mcpServers":{},"lspServers":{},"monitors":{},"settings":{},"userConfig":{}
	}`)
	manifest, err := parsePluginManifest(data)
	if err != nil {
		t.Fatalf("parsePluginManifest() error = %v", err)
	}
	if manifest.Name != "demo" || manifest.Version == nil || *manifest.Version != "1.0.0" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestInspectClaudePluginImportsExecutableAwareOverlayFilesWithTreePrecedence(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "Alpha.\n")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/pi.json", `{"files":{"scripts/run.sh":{"text":"JSON","executable":true},"data.bin":{"base64":"AQI=","executable":true}}}`)
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/pi/files/scripts/run.sh", "tree")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/pi/files/scripts/exec.sh", "exec")
	if err := os.Chmod(filepath.Join(workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/pi/files/scripts/exec.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	overlay := inventory.Packages[0].Assets[0].Overlays[0]
	if got := claudeFilePatch(overlay.Files, "scripts/run.sh").Content; string(got.Bytes) != "tree" || got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/plugin/.agentbundler/assets/skill/alpha/targets/pi/files/scripts/run.sh"}}) {
		t.Fatalf("tree replacement = %#v", got)
	}
	if got := claudeFilePatch(overlay.Files, "scripts/exec.sh").Content; !got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/plugin/.agentbundler/assets/skill/alpha/targets/pi/files/scripts/exec.sh"}}) {
		t.Fatalf("executable tree replacement = %#v", got)
	}
	if got := claudeFilePatch(overlay.Files, "data.bin").Content; !reflect.DeepEqual(got.Bytes, []byte{1, 2}) || !got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/plugin/.agentbundler/assets/skill/alpha/targets/pi.json"}}) {
		t.Fatalf("JSON replacement = %#v", got)
	}
}

func TestInspectClaudePluginImportsAntigravityTargetSidecar(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "Alpha.\n")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/antigravity.json", `{"frontmatterPatch":{"name":"alpha","description":"Alpha skill"},"files":{"guide.md":"Guide\n"}}`)
	manifest := testManifest()
	manifest.Targets = []model.TargetID{model.TargetAntigravity}

	inventory, diagnostics := InspectClaudePlugin(manifest, workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("InspectClaudePlugin() diagnostics = %#v", diagnostics)
	}
	if len(inventory.Packages) != 1 || len(inventory.Packages[0].Assets) != 1 || len(inventory.Packages[0].Assets[0].Overlays) != 1 {
		t.Fatalf("inventory = %#v", inventory)
	}
	overlay := inventory.Packages[0].Assets[0].Overlays[0]
	if overlay.Target != model.TargetAntigravity || overlay.FrontmatterPatch == nil || (*overlay.FrontmatterPatch)["name"] != "alpha" || (*overlay.FrontmatterPatch)["description"] != "Alpha skill" {
		t.Fatalf("overlay = %#v", overlay)
	}
	if got := claudeFilePatch(overlay.Files, "guide.md").Content; string(got.Bytes) != "Guide\n" || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/plugin/.agentbundler/assets/skill/alpha/targets/antigravity.json"}}) {
		t.Fatalf("overlay file = %#v", got)
	}
}

func TestInspectClaudePluginRejectsUnknownTargetSidecar(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "Alpha.\n")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/unknown.json", `{}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if !hasErrors(diagnostics) || len(inventory.Packages) != 0 || !containsDiagnostic(diagnostics, "invalid target") {
		t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}
}

func TestInspectClaudePluginRejectsInvalidExecutableAwareOverlayFileValues(t *testing.T) {
	for _, value := range []string{
		`{"text":"one","base64":"dHdv"}`,
		`{"text":"one","unknown":true}`,
		`{"text":"one","executable":null}`,
		`{"text":null}`,
		`{"executable":true}`,
	} {
		t.Run(value, func(t *testing.T) {
			workspace := t.TempDir()
			writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
			writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "Alpha.\n")
			writeFixture(t, workspace, "source/plugin/.agentbundler/assets/skill/alpha/targets/pi.json", `{"files":{"scripts/run.sh":`+value+`}}`)

			inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
			if !hasErrors(diagnostics) || len(inventory.Packages) != 0 || !containsDiagnostic(diagnostics, "target sidecar") {
				t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
			}
		})
	}
}

func TestInspectClaudePluginRejectsMalformedPluginAndMarketplace(t *testing.T) {
	cases := []struct{ name, plugin, marketplace string }{
		{"unknown plugin field", `{"name":"demo","extra":true}`, ""},
		{"marketplace parent source", `{"name":"demo"}`, `{"plugins":[{"source":".."}]}`},
		{"marketplace wrong source", `{"name":"demo"}`, `{"plugins":[{"source":"elsewhere"}]}`},
		{"marketplace multiple plugins", `{"name":"demo"}`, `{"plugins":[{"source":"./"},{"source":"./"}]}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", test.plugin)
			if test.marketplace != "" {
				writeFixture(t, workspace, "source/plugin/.claude-plugin/marketplace.json", test.marketplace)
			}
			_, diagnostics := InspectClaudePlugin(testManifest(), workspace)
			if !hasErrors(diagnostics) {
				t.Fatalf("diagnostics = %#v, want error", diagnostics)
			}
		})
	}
}

func TestInspectClaudePluginRejectsSymlinkedManifestRootAncestor(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, outside, "plugin/.claude-plugin/plugin.json", `{"name":"outside"}`)
	if err := os.Symlink(outside, filepath.Join(workspace, "source")); err != nil {
		t.Fatal(err)
	}
	_, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if !hasErrors(diagnostics) || !containsDiagnostic(diagnostics, "symlink") {
		t.Fatalf("diagnostics = %#v, want symlink rejection", diagnostics)
	}
}

func TestInspectClaudePluginRejectsSymlinkedTargetSidecars(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "Alpha.\n")
	writeFixture(t, workspace, "unintended-targets/pi/files/leak.txt", "must not import\n")
	sidecarRoot := filepath.Join(workspace, "source/plugin/.agentbundler/assets/skill/alpha")
	if err := os.MkdirAll(sidecarRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspace, "unintended-targets"), filepath.Join(sidecarRoot, "targets")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if !hasErrors(diagnostics) || len(inventory.Packages) != 0 || !containsDiagnostic(diagnostics, "symlink") {
		t.Fatalf("inventory = %#v, diagnostics = %#v, want symlink rejection", inventory, diagnostics)
	}
}

func containsDiagnostic(diagnostics []model.Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

func TestInspectClaudePluginReadsLegacyInlineHooksAndSidecars(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":{"Stop":[{"command":"done"}]}}`)
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/hook/Stop-1/asset.json", `{"capabilities":["prompt-injection"]}`)
	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	asset := inventory.Packages[0].Assets[0]
	if got, want := capabilityKeys(asset.CapabilityUses), []model.CapabilityKey{"asset.hook", "hook.command.shell", "hook.event.stop", "prompt-injection"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("hook capabilities = %#v, want %#v", got, want)
	}
	if asset.Hook == nil || asset.Hook.Location.Path != "source/plugin/.claude-plugin/plugin.json" || asset.Hook.Handler.Mode != model.HookHandlerModeShell || asset.Hook.Handler.ShellCommand == nil || *asset.Hook.Handler.ShellCommand != "done" || asset.Hook.TimeoutMilliseconds != model.MaxHookTimeoutMilliseconds || asset.Hook.Order != 0 {
		t.Fatalf("Stop hook = %#v", asset.Hook)
	}
}

func TestInspectClaudePluginUsesManifestSelectedAndInlineOfficialHooks(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":["./config/hooks.json",{"Stop":[{"hooks":[{"type":"command","command":"inline"}]}]}]}`)
	writeFixture(t, workspace, "source/plugin/config/hooks.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"selected"}]}]}}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"hooks":{"Notification":[{"hooks":[{"type":"command","command":"default-must-not-load"}]}]}}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assets := inventory.Packages[0].Assets
	if len(assets) != 2 || assets[0].Identity != "hook/SessionStart-1" || assets[0].Hook.Location.Path != "source/plugin/config/hooks.json" || assets[1].Identity != "hook/Stop-1" || assets[1].Hook.Location.Path != "source/plugin/.claude-plugin/plugin.json" {
		t.Fatalf("assets = %#v", assets)
	}
	if len(inventory.NativeGaps) != 1 || inventory.NativeGaps[0].Component != "source/plugin/hooks/hooks.json" {
		t.Fatalf("native gaps = %#v", inventory.NativeGaps)
	}
	if !containsInput(inventory, "source/plugin/config/hooks.json") || !containsInput(inventory, "source/plugin/hooks/hooks.json") {
		t.Fatalf("inputs = %#v", inventory.Inputs)
	}
}

func TestInspectClaudePluginNormalizesSimpleScriptAndPreservesComplexShell(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"hooks":{"PostToolUse":[{"matcher":"Write|Edit|NotebookEdit","hooks":[{"type":"command","command":"bash \"${CLAUDE_PLUGIN_ROOT}/scripts/check.sh\""},{"type":"command","command":"printf '%s\\n' done | tee /tmp/hook.log"},{"type":"command","command":"python3","args":["${CLAUDE_PLUGIN_ROOT}/scripts/check.py","--quiet"],"timeout":7,"async":true}]}]}}`)
	writeFixture(t, workspace, "source/plugin/scripts/check.sh", "#!/bin/sh\n")
	writeFixture(t, workspace, "source/plugin/scripts/check.py", "print('ok')\n")

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assets := inventory.Packages[0].Assets
	if len(assets) != 3 {
		t.Fatalf("assets = %#v", assets)
	}

	simple := assets[0]
	if simple.Hook.Handler.Mode != model.HookHandlerModeExec || simple.Hook.Handler.Program == nil || *simple.Hook.Handler.Program != "bash" || len(simple.Hook.Handler.Arguments) != 1 || simple.Hook.Handler.Arguments[0].PackageFile == nil || *simple.Hook.Handler.Arguments[0].PackageFile != "scripts/check.sh" {
		t.Fatalf("simple handler = %#v", simple.Hook.Handler)
	}
	if simple.Base.Files["scripts/check.sh"].Executable {
		t.Fatalf("interpreter-backed payload unexpectedly executable: %#v", simple.Base.Files["scripts/check.sh"])
	}

	complex := assets[1]
	if complex.Hook.Handler.Mode != model.HookHandlerModeShell || complex.Hook.Handler.ShellCommand == nil || *complex.Hook.Handler.ShellCommand != "printf '%s\\n' done | tee /tmp/hook.log" {
		t.Fatalf("complex handler = %#v", complex.Hook.Handler)
	}
	if got, want := capabilityKeys(complex.CapabilityUses), []model.CapabilityKey{"asset.hook", "hook.command.shell", "hook.event.post-tool", "hook.matcher.tool-category"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("complex portability capabilities = %#v, want %#v", got, want)
	}

	async := assets[2]
	if async.Hook.Handler.Mode != model.HookHandlerModeExec || async.Hook.TimeoutMilliseconds != 7_000 || !async.Hook.Asynchronous || async.Hook.Order != 2 {
		t.Fatalf("async handler = %#v", async.Hook)
	}
	if got, want := capabilityKeys(async.CapabilityUses), []model.CapabilityKey{"asset.hook", "hook.async", "hook.command.exec", "hook.event.post-tool", "hook.matcher.tool-category"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("async capabilities = %#v, want %#v", got, want)
	}
	if got, want := async.Hook.Matcher.Tools, []model.HookToolCategory{model.HookToolCategoryWrite, model.HookToolCategoryEdit}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matcher tools = %#v, want %#v", got, want)
	}
}

func TestInspectClaudePluginPreservesOnlyCompleteNativeMatcherCategories(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":{"PreToolUse":[{"matcher":"NotebookEdit|Edit|Grep|Glob|WebSearch|WebFetch|Agent|Task","command":"check"}]}}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assets := inventory.Packages[0].Assets
	if len(assets) != 1 || assets[0].Hook == nil {
		t.Fatalf("assets = %#v", assets)
	}
	if got, want := assets[0].Hook.Matcher.Tools, []model.HookToolCategory{
		model.HookToolCategoryEdit,
		model.HookToolCategorySearch,
		model.HookToolCategoryWeb,
		model.HookToolCategoryTask,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matcher tools = %#v, want %#v", got, want)
	}
}

func TestInspectClaudePluginRejectsPartialNativeMatcherCategories(t *testing.T) {
	for _, matcher := range []string{"Edit", "NotebookEdit", "Glob", "Grep", "WebFetch", "WebSearch", "Task", "Agent"} {
		t.Run(matcher, func(t *testing.T) {
			workspace := t.TempDir()
			writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":{"PreToolUse":[{"matcher":`+fmt.Sprintf("%q", matcher)+`,"command":"check"}]}}`)

			inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
			if !hasErrors(diagnostics) || len(inventory.Packages) != 0 || !containsDiagnostic(diagnostics, "complete native expansion") {
				t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
			}
		})
	}
}

func TestInspectClaudePluginImportsPortableMarkdownCommand(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/commands/resume-from.md", "---\n{\"description\":\"Resume from a saved handoff.\"}\n---\nResume the session.\n")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/command/resume-from/targets/pi.json", `{"frontmatterPatch":{"description":"Resume a Pi session."}}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assets := inventory.Packages[0].Assets
	if len(assets) != 1 || assets[0].Identity != "command/resume-from" || assets[0].Command == nil {
		t.Fatalf("assets = %#v", assets)
	}
	command := assets[0]
	if command.Command.Name != "resume-from" || command.Command.Description != "Resume from a saved handoff." || command.Base.Body != "Resume the session.\n" {
		t.Fatalf("command = %#v", command)
	}
	if got := capabilityKeys(command.CapabilityUses); !reflect.DeepEqual(got, []model.CapabilityKey{"asset.command"}) {
		t.Fatalf("capabilities = %#v", got)
	}
	if len(command.Overlays) != 1 || command.Overlays[0].Target != model.TargetPi {
		t.Fatalf("overlays = %#v", command.Overlays)
	}
	if len(inventory.NativeGaps) != 0 {
		t.Fatalf("native gaps = %#v", inventory.NativeGaps)
	}
}

func TestInspectClaudePluginReportsUnsupportedNativeHandlerAsTargetNeutralGap(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"hooks":{"PostToolUse":[{"hooks":[{"type":"http","url":"https://example.test/hook"}]}]}}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if len(inventory.Packages[0].Assets) != 0 || len(inventory.NativeGaps) != 1 || inventory.NativeGaps[0].Component != "source/plugin/hooks/hooks.json#hooks.PostToolUse[0].hooks[0]" || inventory.NativeGaps[0].Target != nil {
		t.Fatalf("inventory = %#v", inventory)
	}
}

func TestInspectClaudePluginPreservesOneShotCommandsAsTargetNeutralGaps(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"once","once":true},{"type":"command","command":"repeat","once":false}]}]}}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assets := inventory.Packages[0].Assets
	if len(assets) != 1 || assets[0].Identity != "hook/SessionStart-2" || assets[0].Hook == nil || assets[0].Hook.Handler.ShellCommand == nil || *assets[0].Hook.Handler.ShellCommand != "repeat" {
		t.Fatalf("assets = %#v", assets)
	}
	if len(inventory.NativeGaps) != 1 || inventory.NativeGaps[0].Component != "source/plugin/hooks/hooks.json#hooks.SessionStart[0].hooks[0]" || inventory.NativeGaps[0].Target != nil {
		t.Fatalf("native gaps = %#v", inventory.NativeGaps)
	}
}

func TestInspectClaudePluginRejectsMalformedAndUnprovableHooks(t *testing.T) {
	cases := []struct {
		name     string
		plugin   string
		hookPath string
		hook     string
		want     string
	}{
		{name: "unknown hook file field", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{},"extra":true}`, want: "unknown field"},
		{name: "duplicate hook file field", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{},"hooks":{}}`, want: "duplicate"},
		{name: "empty handler list", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"Stop":[{"hooks":[]}]}}`, want: "must not be empty"},
		{name: "missing handler type", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"Stop":[{"hooks":[{"command":"done"}]}]}}`, want: "type is required"},
		{name: "unknown handler field", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"done","extra":true}]}]}}`, want: "unknown field"},
		{name: "unknown native handler field", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"PostToolUse":[{"hooks":[{"type":"http","url":"https://example.test/hook","extra":true}]}]}}`, want: "unknown field"},
		{name: "null args", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"done","args":null}]}]}}`, want: "args must be an array"},
		{name: "custom path without prefix", plugin: `{"name":"demo","hooks":"config/hooks.json"}`, want: "must start with ./"},
		{name: "custom path traversal", plugin: `{"name":"demo","hooks":"./../hooks.json"}`, want: "path"},
		{name: "missing custom file", plugin: `{"name":"demo","hooks":"./config/hooks.json"}`, want: "no such file"},
		{name: "missing package file", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"bash","args":["${CLAUDE_PLUGIN_ROOT}/scripts/missing.sh"]}]}]}}`, want: "missing.sh"},
		{name: "complex package shell", plugin: `{"name":"demo"}`, hookPath: "hooks/hooks.json", hook: `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"bash \"${CLAUDE_PLUGIN_ROOT}/scripts/check.sh\" && echo done"}]}]}}`, want: "not statically provable"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", test.plugin)
			if test.hookPath != "" {
				writeFixture(t, workspace, "source/plugin/"+test.hookPath, test.hook)
			}
			inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
			if !hasErrors(diagnostics) || len(inventory.Packages) != 0 || !containsDiagnostic(diagnostics, test.want) {
				t.Fatalf("inventory = %#v, diagnostics = %#v, want %q", inventory, diagnostics, test.want)
			}
		})
	}
}

func TestInspectClaudePluginRejectsSymlinkedHookPayload(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"bash","args":["${CLAUDE_PLUGIN_ROOT}/scripts/check.sh"]}]}]}}`)
	writeFixture(t, outside, "check.sh", "#!/bin/sh\n")
	if err := os.MkdirAll(filepath.Join(workspace, "source/plugin/scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "check.sh"), filepath.Join(workspace, "source/plugin/scripts/check.sh")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if !hasErrors(diagnostics) || !containsDiagnostic(diagnostics, "symlink") {
		t.Fatalf("diagnostics = %#v, want symlink rejection", diagnostics)
	}
}

func TestInspectClaudePluginReadsOnlyOwnedPayloadsWithoutWritingSource(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeFixture(t, workspace, "source/plugin/hooks/hooks.json", `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"bash","args":["${CLAUDE_PLUGIN_ROOT}/scripts/owned.sh"]}]}]}}`)
	writeFixture(t, workspace, "source/plugin/scripts/owned.sh", "#!/bin/sh\n")
	writeFixture(t, workspace, "source/plugin/scripts/unowned.sh", "do not copy\n")
	ownedPath := filepath.Join(workspace, "source/plugin/scripts/owned.sh")
	if err := os.Chmod(ownedPath, 0o744); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ownedPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(ownedPath)
	if err != nil {
		t.Fatal(err)
	}

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	after, err := os.ReadFile(ownedPath)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(ownedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || beforeInfo.Mode().Perm() != afterInfo.Mode().Perm() {
		t.Fatalf("source changed: before mode=%#o bytes=%q, after mode=%#o bytes=%q", beforeInfo.Mode().Perm(), before, afterInfo.Mode().Perm(), after)
	}
	asset := inventory.Packages[0].Assets[0]
	if len(asset.Base.Files) != 1 || string(asset.Base.Files["scripts/owned.sh"].Bytes) != string(before) || !asset.Base.Files["scripts/owned.sh"].Executable {
		t.Fatalf("owned payloads = %#v", asset.Base.Files)
	}
	if len(inventory.NativeGaps) != 1 || inventory.NativeGaps[0].Component != "source/plugin/scripts/unowned.sh" {
		t.Fatalf("native gaps = %#v", inventory.NativeGaps)
	}
}

func TestInspectClaudePluginUsesPortableTimeoutDefaultsAndOrdinalOrder(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":{"Stop":[{"command":"first"},{"command":"second"}],"UserPromptSubmit":[{"command":"prompt"}]}}`)

	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assets := inventory.Packages[0].Assets
	byID := make(map[model.AssetID]*model.HookDescriptor, len(assets))
	for _, asset := range assets {
		byID[asset.Identity] = asset.Hook
	}
	if hook := byID["hook/Stop-1"]; hook == nil || hook.TimeoutMilliseconds != 600_000 || hook.Order != 0 {
		t.Fatalf("Stop-1 hook = %#v", hook)
	}
	if hook := byID["hook/Stop-2"]; hook == nil || hook.TimeoutMilliseconds != 600_000 || hook.Order != 1 {
		t.Fatalf("Stop-2 hook = %#v", hook)
	}
	if hook := byID["hook/UserPromptSubmit-1"]; hook == nil || hook.TimeoutMilliseconds != 30_000 || hook.Order != 0 {
		t.Fatalf("UserPromptSubmit hook = %#v", hook)
	}
}

func TestInspectClaudePluginRejectsUnportableHookValues(t *testing.T) {
	cases := []struct {
		name  string
		hooks string
	}{
		{name: "zero timeout", hooks: `{"Stop":[{"command":"done","timeout":0}]}`},
		{name: "timeout above model bound", hooks: `{"Stop":[{"command":"done","timeout":601}]}`},
		{name: "regex matcher", hooks: `{"PreToolUse":[{"matcher":"Bash.*","command":"check"}]}`},
		{name: "matcher on non-tool event", hooks: `{"Stop":[{"matcher":"Bash","command":"done"}]}`},
		{name: "unknown event", hooks: `{"SomethingNew":[{"command":"done"}]}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":`+test.hooks+`}`)
			inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
			if !hasErrors(diagnostics) || len(inventory.Packages) != 0 {
				t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
			}
		})
	}
}

func testManifest() model.SourceManifest {
	return model.SourceManifest{Kind: model.SourceKindClaudePlugin, Root: "source", Targets: []model.TargetID{model.TargetClaude}, Output: "generated", ClaudePlugin: &model.ClaudePluginSourceConfig{PluginRoot: "plugin"}}
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func capabilityKeys(uses []model.CapabilityUse) []model.CapabilityKey {
	keys := make([]model.CapabilityKey, len(uses))
	for index, use := range uses {
		keys[index] = use.Key
	}
	return keys
}

func containsInput(inventory model.SourceInventory, path model.RelativePath) bool {
	for _, input := range inventory.Inputs {
		if input.Path == path {
			return true
		}
	}
	return false
}

func claudeFilePatch(files []model.FilePatch, path model.RelativePath) model.FilePatch {
	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	return model.FilePatch{}
}
