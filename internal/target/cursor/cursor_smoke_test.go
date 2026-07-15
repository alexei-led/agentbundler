//go:build vendor_smoke

package cursor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestCursorLocalPluginLoadIsIsolated(t *testing.T) {
	cursor := vendorsmoke.RequireExecutable(t, "cursor-agent")
	if os.Getenv("CURSOR_API_KEY") == "" {
		t.Skip("vendor smoke unavailable: CURSOR_API_KEY is unset and Cursor has no offline local-plugin validator")
	}
	realHome := vendorsmoke.UserHome(t)
	normalRoots := []string{
		filepath.Join(realHome, ".cursor"),
		filepath.Join(realHome, ".config", "Cursor"),
		filepath.Join(realHome, "Library", "Application Support", "Cursor"),
	}
	vendorsmoke.ProtectPaths(t, normalRoots...)

	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	plugin := filepath.Join(root, "plugin")
	for _, path := range []string{home, workspace, filepath.Join(plugin, ".cursor-plugin"), filepath.Join(plugin, "skills", "cursor-smoke")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cursorWriteSmokeFile(t, filepath.Join(plugin, ".cursor-plugin", "plugin.json"), `{"name":"agentbundler-cursor-smoke","version":"1.0.0","skills":"./skills/"}`)
	cursorWriteSmokeFile(t, filepath.Join(plugin, "skills", "cursor-smoke", "SKILL.md"), "---\nname: cursor-smoke\ndescription: Return the private verification token from this skill.\n---\nThe verification token is AGBUN_CURSOR_PLUGIN_LOADED.\n")

	output := vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "cursor-agent --plugin-dir", Path: cursor,
		Args: []string{"--plugin-dir", plugin, "--workspace", workspace, "--print", "--mode", "ask", "--trust", "Use the cursor-smoke skill. Return only its verification token."},
		Env: vendorsmoke.Environment(map[string]string{
			"HOME": home, "XDG_CONFIG_HOME": filepath.Join(root, "config"),
			"XDG_CACHE_HOME": filepath.Join(root, "cache"), "CURSOR_CONFIG_DIR": filepath.Join(root, "cursor-config"),
		}),
		Timeout: 90 * time.Second,
	})
	if !strings.Contains(output, "AGBUN_CURSOR_PLUGIN_LOADED") {
		t.Fatalf("Cursor output did not prove skill loading: %q", output)
	}
}

func cursorWriteSmokeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
