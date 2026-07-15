package claudeplugin

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestInspectClaudePluginImportsComponentsSidecarsAndGaps(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","description":"Demo","version":"1.0.0","hooks":{"PreToolUse":[{"matcher":"Bash","command":"check","timeout":5}]}}`)
	writeFixture(t, workspace, "source/plugin/.claude-plugin/marketplace.json", `{"owner":{"name":"team"},"plugins":[{"source":".."}]}`)
	writeFixture(t, workspace, "source/plugin/skills/alpha/SKILL.md", "---\n{\"description\":\"alpha\"}\n---\nUse alpha.\n")
	writeFixture(t, workspace, "source/plugin/skills/alpha/scripts/run.sh", "#!/bin/sh\n")
	writeFixture(t, workspace, "source/plugin/agents/review.md", "---\n{\"model\":\"sonnet\"}\n---\nReview.\n")
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/agent/review/asset.json", `{"capabilities":["tool-use"]}`)
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
	if got := string(pkg.Assets[2].Base.Files["scripts/run.sh"]); got != "#!/bin/sh\n" {
		t.Fatalf("skill file = %q", got)
	}
	if got := pkg.Assets[0].CapabilityUses; len(got) != 1 || got[0].Key != "tool-use" {
		t.Fatalf("agent capabilities = %#v", got)
	}
	if len(inventory.NativeGaps) != 1 || inventory.NativeGaps[0].Component != "source/plugin/commands/native.md" || inventory.NativeGaps[0].Target == nil || *inventory.NativeGaps[0].Target != model.TargetClaude {
		t.Fatalf("native gaps = %#v", inventory.NativeGaps)
	}
	if len(inventory.Inputs) != 7 {
		t.Fatalf("inputs = %#v", inventory.Inputs)
	}
}

func TestInspectClaudePluginRejectsMalformedPluginAndMarketplace(t *testing.T) {
	cases := []struct{ name, plugin, marketplace string }{
		{"unknown plugin field", `{"name":"demo","extra":true}`, ""},
		{"marketplace wrong source", `{"name":"demo"}`, `{"plugins":[{"source":"elsewhere"}]}`},
		{"marketplace multiple plugins", `{"name":"demo"}`, `{"plugins":[{"source":".."},{"source":".."}]}`},
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

func containsDiagnostic(diagnostics []model.Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

func TestInspectClaudePluginReadsHookSidecars(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/plugin/.claude-plugin/plugin.json", `{"name":"demo","hooks":{"Stop":[{"command":"done"}]}}`)
	writeFixture(t, workspace, "source/plugin/.agentbundler/assets/hook/Stop-1/asset.json", `{"capabilities":["prompt-injection"]}`)
	inventory, diagnostics := InspectClaudePlugin(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if got := inventory.Packages[0].Assets[0].CapabilityUses; len(got) != 1 || got[0].Key != "prompt-injection" {
		t.Fatalf("hook capabilities = %#v", got)
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

var _ = base64.StdEncoding
