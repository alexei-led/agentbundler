//go:build vendor_smoke

package compiler

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestInstalledClaudeStrictlyValidatesGeneratedHookFixtureOffline(t *testing.T) {
	claude := vendorsmoke.RequireExecutable(t, "claude")
	realHome := vendorsmoke.UserHome(t)
	claudeRoot := filepath.Join(realHome, ".claude")
	vendorsmoke.ProtectPath(t, claudeRoot, "context-mode/sessions", "projects")
	vendorsmoke.ProtectPaths(t, filepath.Join(realHome, ".config", "claude"))

	workspace, _ := compileClaudeHookFixture(t)
	isolatedRoot := t.TempDir()
	pluginRoot := filepath.Join(workspace, "generated", "claude")
	vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "claude plugin validate --strict", Path: claude,
		Args: []string{"plugin", "validate", "--strict", pluginRoot},
		Env: vendorsmoke.Environment(map[string]string{
			"HOME": isolatedRoot, "CLAUDE_CONFIG_DIR": filepath.Join(isolatedRoot, ".claude"),
			"XDG_CONFIG_HOME": filepath.Join(isolatedRoot, ".config"),
			"HTTP_PROXY":      "http://127.0.0.1:9", "HTTPS_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
		}),
		Timeout: 30 * time.Second,
	})
}
