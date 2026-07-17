//go:build vendor_smoke

package antigravity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestAgyValidatesGoldenPluginOfflineWithoutChangingUserState(t *testing.T) {
	agy := vendorsmoke.RequireExecutable(t, "agy")
	realHome := vendorsmoke.UserHome(t)
	vendorsmoke.ProtectPaths(t, filepath.Join(realHome, ".gemini"))

	pluginRoot, err := filepath.Abs("testdata/plugin-golden")
	if err != nil {
		t.Fatal(err)
	}
	isolatedRoot := t.TempDir()
	for _, path := range []string{"home", "config", "cache", "tmp"} {
		if err := os.MkdirAll(filepath.Join(isolatedRoot, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	output := vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "agy plugin validate", Path: agy,
		Args: []string{"plugin", "validate", pluginRoot},
		Env: vendorsmoke.Environment(map[string]string{
			"HOME": filepath.Join(isolatedRoot, "home"), "XDG_CONFIG_HOME": filepath.Join(isolatedRoot, "config"),
			"XDG_CACHE_HOME": filepath.Join(isolatedRoot, "cache"), "TMPDIR": filepath.Join(isolatedRoot, "tmp"),
			"HTTP_PROXY": "http://127.0.0.1:9", "HTTPS_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
		}),
		Timeout: 30 * time.Second,
	})
	for _, evidence := range []string{"skills", "agents"} {
		if !strings.Contains(output, evidence) {
			t.Fatalf("agy validator output lacks %q: %q", evidence, output)
		}
	}
}
