//go:build vendor_smoke

package grok

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestGrokLocalPluginInstallAndInspectIsIsolated(t *testing.T) {
	grok := vendorsmoke.RequireExecutable(t, "grok")
	realHome := vendorsmoke.UserHome(t)
	normalRoot := filepath.Join(realHome, ".grok")
	vendorsmoke.ProtectPaths(t, normalRoot)

	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	plugin, err := filepath.Abs("testdata/plugin-golden")
	if err != nil {
		t.Fatal(err)
	}
	leaderSocket := filepath.Join(root, "leader.sock")
	environment := vendorsmoke.Environment(map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_CACHE_HOME": filepath.Join(root, "cache"),
		"HTTPS_PROXY": "http://127.0.0.1:9", "HTTP_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
	})
	runGrokSmoke(t, grok, environment, "--leader-socket", leaderSocket, "plugin", "install", "--trust", plugin)
	details := runGrokSmoke(t, grok, environment, "--leader-socket", leaderSocket, "plugin", "details", "demo")
	for _, evidence := range []string{"demo v1.2.3", "1 skill dir(s)", "1 agent dir(s)", "hooks"} {
		if !strings.Contains(details, evidence) {
			t.Fatalf("grok plugin details lacks %q: %s", evidence, details)
		}
	}
	manifests, err := filepath.Glob(filepath.Join(home, ".grok", "installed-plugins", "*", ".claude-plugin", "plugin.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatalf("isolated installed manifests = %#v, err = %v", manifests, err)
	}
}

func runGrokSmoke(t *testing.T, grok string, environment []string, arguments ...string) string {
	t.Helper()
	return vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "grok " + strings.Join(arguments, " "), Path: grok, Args: arguments,
		Env: environment, Timeout: 30 * time.Second,
	})
}
