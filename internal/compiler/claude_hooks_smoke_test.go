//go:build vendor_smoke

package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstalledClaudeStrictlyValidatesGeneratedHookFixtureOffline(t *testing.T) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("installed Claude smoke unavailable: claude executable is not on PATH")
	}
	workspace, _ := compileClaudeHookFixture(t)
	isolatedHome := t.TempDir()
	pluginRoot := filepath.Join(workspace, "generated", "claude")
	command := exec.Command(claude, "plugin", "validate", "--strict", pluginRoot)
	command.Env = []string{
		"HOME=" + isolatedHome,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(isolatedHome, ".claude"),
		"XDG_CONFIG_HOME=" + filepath.Join(isolatedHome, ".config"),
		"PATH=" + os.Getenv("PATH"),
		"HTTP_PROXY=http://127.0.0.1:9",
		"HTTPS_PROXY=http://127.0.0.1:9",
		"NO_PROXY=",
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("claude plugin validate --strict: %v\n%s", err, output)
	}
}
