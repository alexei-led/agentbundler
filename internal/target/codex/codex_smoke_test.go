//go:build vendor_smoke

package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestCodexLocalMarketplaceInstallAndList(t *testing.T) {
	codex := vendorsmoke.RequireExecutable(t, "codex")
	realHome := vendorsmoke.UserHome(t)
	vendorsmoke.ProtectPaths(t, filepath.Join(realHome, ".codex"))

	root := t.TempDir()
	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex")
	marketplace := filepath.Join(root, "marketplace")
	plugin := filepath.Join(marketplace, "plugins", "demo")
	for _, path := range []string{home, codexHome, filepath.Join(marketplace, ".agents", "plugins"), filepath.Join(plugin, ".codex-plugin")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeSmokeFile(t, filepath.Join(marketplace, ".agents", "plugins", "marketplace.json"), `{"name":"local","owner":{"name":"agentbundler-test"},"plugins":[{"name":"demo","source":"./plugins/demo","description":"isolated smoke"}]}`)
	writeSmokeFile(t, filepath.Join(plugin, ".codex-plugin", "plugin.json"), `{"name":"demo","version":"1.0.0","description":"isolated smoke"}`)
	environment := vendorsmoke.Environment(map[string]string{
		"HOME": home, "CODEX_HOME": codexHome,
		"XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_CACHE_HOME": filepath.Join(root, "cache"),
		"HTTP_PROXY": "http://127.0.0.1:9", "HTTPS_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
	})

	runCodexSmoke(t, codex, environment, "plugin", "marketplace", "add", marketplace)
	runCodexSmoke(t, codex, environment, "plugin", "add", "demo@local")
	output := runCodexSmoke(t, codex, environment, "plugin", "list")
	if !strings.Contains(output, "demo@local") || !strings.Contains(output, "installed, enabled") {
		t.Fatalf("codex plugin list output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "plugins", "cache", "local", "demo", "1.0.0", ".codex-plugin", "plugin.json")); err != nil {
		t.Fatalf("isolated installed plugin: %v", err)
	}
}

func writeSmokeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCodexSmoke(t *testing.T, codex string, environment []string, arguments ...string) string {
	t.Helper()
	return vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "codex " + strings.Join(arguments, " "), Path: codex, Args: arguments,
		Env: environment, Timeout: 30 * time.Second,
	})
}
