//go:build vendor_smoke

package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestCopilotLocalMarketplaceInstallAndListIsIsolated(t *testing.T) {
	copilot := vendorsmoke.RequireExecutable(t, "copilot")
	realHome := vendorsmoke.UserHome(t)
	normalRoots := []string{
		filepath.Join(realHome, ".copilot"),
		filepath.Join(realHome, ".cache", "copilot"),
		filepath.Join(realHome, "Library", "Caches", "copilot"),
	}
	vendorsmoke.ProtectPaths(t, normalRoots...)

	root := t.TempDir()
	home := filepath.Join(root, "home")
	copilotHome := filepath.Join(root, "copilot")
	marketplace := filepath.Join(root, "marketplace")
	plugin := filepath.Join(marketplace, "plugins", "demo")
	for _, path := range []string{home, copilotHome, filepath.Join(marketplace, ".github", "plugin"), filepath.Join(plugin, "skills", "demo")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeSmokeFile(t, filepath.Join(marketplace, ".github", "plugin", "marketplace.json"), `{"name":"agentbundler-smoke","owner":{"name":"Agent Bundler test"},"plugins":[{"name":"demo","source":"./plugins/demo","version":"1.0.0"}]}`)
	writeSmokeFile(t, filepath.Join(plugin, "plugin.json"), `{"name":"demo","version":"1.0.0","description":"isolated smoke","skills":["skills/"]}`)
	writeSmokeFile(t, filepath.Join(plugin, "skills", "demo", "SKILL.md"), "# Demo\n")
	environment := vendorsmoke.Environment(map[string]string{
		"HOME": home, "COPILOT_HOME": copilotHome,
		"XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_CACHE_HOME": filepath.Join(root, "cache"),
		"HTTPS_PROXY": "http://127.0.0.1:9", "HTTP_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
	})

	runCopilotSmoke(t, copilot, environment, "plugin", "marketplace", "add", marketplace)
	runCopilotSmoke(t, copilot, environment, "plugin", "install", "demo@agentbundler-smoke")
	output := runCopilotSmoke(t, copilot, environment, "plugin", "list")
	if !strings.Contains(output, "demo@agentbundler-smoke") || !strings.Contains(output, "v1.0.0") {
		t.Fatalf("copilot plugin list output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(copilotHome, "installed-plugins", "agentbundler-smoke", "demo", "plugin.json")); err != nil {
		t.Fatalf("isolated installed plugin: %v", err)
	}
}

func writeSmokeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCopilotSmoke(t *testing.T, copilot string, environment []string, arguments ...string) string {
	t.Helper()
	return vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "copilot " + strings.Join(arguments, " "), Path: copilot, Args: arguments,
		Env: environment, Timeout: 30 * time.Second,
	})
}
