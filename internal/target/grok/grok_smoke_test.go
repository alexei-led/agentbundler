//go:build vendor_smoke

package grok

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestGrokLocalPluginInstallAndInspectIsIsolated(t *testing.T) {
	grok, err := exec.LookPath("grok")
	if err != nil {
		t.Skip("Grok CLI is not installed")
	}
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	normalRoot := filepath.Join(currentUser.HomeDir, ".grok")
	before := grokSmokeTreeDigest(t, normalRoot)

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
	environment := append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"HTTPS_PROXY=http://127.0.0.1:1",
		"HTTP_PROXY=http://127.0.0.1:1",
	)
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
	if after := grokSmokeTreeDigest(t, normalRoot); after != before {
		t.Fatalf("normal Grok path %q changed during isolated smoke", normalRoot)
	}
}

func runGrokSmoke(t *testing.T, grok string, environment []string, arguments ...string) string {
	t.Helper()
	command := exec.Command(grok, arguments...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("grok %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func grokSmokeTreeDigest(t *testing.T, root string) [32]byte {
	t.Helper()
	entries := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s\x00%d\x00%d\x00", filepath.ToSlash(relative), info.Mode(), info.Size())
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf("%x", sha256.Sum256(data))
		} else if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += target
		}
		entries = append(entries, value)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	sort.Strings(entries)
	return sha256.Sum256([]byte(strings.Join(entries, "\n")))
}
