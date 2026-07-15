//go:build vendor_smoke

package copilot

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

func TestCopilotLocalMarketplaceInstallAndListIsIsolated(t *testing.T) {
	copilot, err := exec.LookPath("copilot")
	if err != nil {
		t.Skip("Copilot CLI is not installed")
	}
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	realHome := currentUser.HomeDir
	normalRoots := []string{
		filepath.Join(realHome, ".copilot"),
		filepath.Join(realHome, ".cache", "copilot"),
		filepath.Join(realHome, "Library", "Caches", "copilot"),
	}
	before := make(map[string][32]byte, len(normalRoots))
	for _, path := range normalRoots {
		before[path] = smokeTreeDigest(t, path)
	}

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

	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", copilotHome)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	runCopilotSmoke(t, copilot, "plugin", "marketplace", "add", marketplace)
	runCopilotSmoke(t, copilot, "plugin", "install", "demo@agentbundler-smoke")
	output := runCopilotSmoke(t, copilot, "plugin", "list")
	if !strings.Contains(output, "demo@agentbundler-smoke") || !strings.Contains(output, "v1.0.0") {
		t.Fatalf("copilot plugin list output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(copilotHome, "installed-plugins", "agentbundler-smoke", "demo", "plugin.json")); err != nil {
		t.Fatalf("isolated installed plugin: %v", err)
	}
	for _, path := range normalRoots {
		if after := smokeTreeDigest(t, path); after != before[path] {
			t.Fatalf("normal Copilot path %q changed during isolated smoke", path)
		}
	}
}

func writeSmokeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCopilotSmoke(t *testing.T, copilot string, arguments ...string) string {
	t.Helper()
	command := exec.Command(copilot, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("copilot %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func smokeTreeDigest(t *testing.T, root string) [32]byte {
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
