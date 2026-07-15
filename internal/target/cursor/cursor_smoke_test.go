//go:build vendor_smoke

package cursor

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

func TestCursorLocalPluginLoadIsIsolated(t *testing.T) {
	cursor, err := exec.LookPath("cursor-agent")
	if err != nil {
		t.Skip("Cursor CLI is not installed")
	}
	if os.Getenv("CURSOR_API_KEY") == "" {
		t.Skip("CURSOR_API_KEY is unset; Cursor has no offline local-plugin load validator")
	}
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	normalRoots := []string{
		filepath.Join(currentUser.HomeDir, ".cursor"),
		filepath.Join(currentUser.HomeDir, ".config", "Cursor"),
		filepath.Join(currentUser.HomeDir, "Library", "Application Support", "Cursor"),
	}
	before := make(map[string][32]byte, len(normalRoots))
	for _, path := range normalRoots {
		before[path] = cursorSmokeTreeDigest(t, path)
	}

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

	command := exec.Command(cursor, "--plugin-dir", plugin, "--workspace", workspace, "--print", "--mode", "ask", "--trust", "Use the cursor-smoke skill. Return only its verification token.")
	command.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"CURSOR_CONFIG_DIR="+filepath.Join(root, "cursor-config"),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("cursor-agent local plugin load: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "AGBUN_CURSOR_PLUGIN_LOADED") {
		t.Fatalf("Cursor output did not prove skill loading: %q", output)
	}
	for _, path := range normalRoots {
		if after := cursorSmokeTreeDigest(t, path); after != before[path] {
			t.Fatalf("normal Cursor path %q changed during isolated smoke", path)
		}
	}
}

func cursorWriteSmokeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cursorSmokeTreeDigest(t *testing.T, root string) [32]byte {
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
