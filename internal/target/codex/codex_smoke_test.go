//go:build vendor_smoke

package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexLocalMarketplaceInstallAndList(t *testing.T) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex CLI is not installed")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	marketplace := filepath.Join(root, "marketplace")
	plugin := filepath.Join(marketplace, "plugins", "demo")
	if err := os.MkdirAll(filepath.Join(marketplace, ".agents", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugin, ".codex-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSmokeFile(t, filepath.Join(marketplace, ".agents", "plugins", "marketplace.json"), `{"name":"local","owner":{"name":"agentbundler-test"},"plugins":[{"name":"demo","source":"./plugins/demo","description":"isolated smoke"}]}`)
	writeSmokeFile(t, filepath.Join(plugin, ".codex-plugin", "plugin.json"), `{"name":"demo","version":"1.0.0","description":"isolated smoke"}`)

	runCodexSmoke(t, codex, home, "plugin", "marketplace", "add", marketplace)
	runCodexSmoke(t, codex, home, "plugin", "add", "demo@local")
	output := runCodexSmoke(t, codex, home, "plugin", "list")
	if !strings.Contains(output, "demo@local") || !strings.Contains(output, "installed, enabled") {
		t.Fatalf("codex plugin list output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(home, "plugins", "cache", "local", "demo", "1.0.0", ".codex-plugin", "plugin.json")); err != nil {
		t.Fatalf("isolated installed plugin: %v", err)
	}
}

func writeSmokeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCodexSmoke(t *testing.T, codex, home string, arguments ...string) string {
	t.Helper()
	command := exec.Command(codex, arguments...)
	command.Env = append(os.Environ(), "CODEX_HOME="+home, "HOME="+home)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("codex %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
