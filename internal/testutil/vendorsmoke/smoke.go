// Package vendorsmoke provides bounded, isolated helpers for opt-in vendor CLI tests.
package vendorsmoke

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const MaxOutputBytes = 32 * 1024

type Command struct {
	Name    string
	Path    string
	Args    []string
	Dir     string
	Env     []string
	Timeout time.Duration
}

// RequireExecutable resolves a required CLI or skips with one exact diagnostic.
func RequireExecutable(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("vendor smoke unavailable: required executable %q is not on PATH", name)
	}
	return path
}

// Run executes one bounded subprocess with bounded combined output.
func Run(t *testing.T, command Command) string {
	t.Helper()
	output, err := run(command)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func run(command Command) (string, error) {
	if command.Timeout <= 0 {
		return "", fmt.Errorf("vendor smoke command %q requires a positive timeout", command.Name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), command.Timeout)
	defer cancel()
	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.WaitDelay = time.Second
	process.Dir = command.Dir
	process.Env = command.Env
	process.Stdin = strings.NewReader("")
	output := &boundedBuffer{limit: MaxOutputBytes}
	process.Stdout = output
	process.Stderr = output
	err := process.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return output.String(), fmt.Errorf("vendor smoke command %q timed out after %s\n%s", command.Name, command.Timeout, output.String())
	}
	if err != nil {
		return output.String(), fmt.Errorf("vendor smoke command %q failed: %w\n%s", command.Name, err, output.String())
	}
	return output.String(), nil
}

// Environment returns a minimal runtime environment plus explicit values.
func Environment(replacements map[string]string) []string {
	values := make(map[string]string)
	inherited := []string{"LANG", "LC_ALL", "LC_CTYPE", "PATH", "TERM", "TZ"}
	if runtime.GOOS == "windows" {
		inherited = append(inherited, "COMSPEC", "PATHEXT", "SYSTEMROOT", "WINDIR")
	}
	for _, key := range inherited {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	for key, value := range replacements {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

// UserHome returns the real account home, independent of temporary HOME values.
func UserHome(t *testing.T) string {
	t.Helper()
	current, err := user.Current()
	if err != nil {
		t.Fatalf("resolve real user home: %v", err)
	}
	if current.HomeDir == "" {
		t.Fatal("resolve real user home: path is empty")
	}
	return current.HomeDir
}

// ProtectPaths registers an assertion that normal config paths remain unchanged,
// including when the smoke test stops early through Fatal or a subprocess failure.
func ProtectPaths(t *testing.T, paths ...string) {
	t.Helper()
	before := Snapshot(t, paths...)
	t.Cleanup(func() {
		AssertUnchanged(t, before)
	})
}

// ProtectPath registers an assertion that root remains unchanged except for
// caller-owned relative runtime paths known to be volatile.
func ProtectPath(t *testing.T, root string, ignored ...string) {
	t.Helper()
	ignored = normalizeIgnoredPaths(t, ignored)
	before := treeDigestIgnoring(t, root, ignored)
	t.Cleanup(func() {
		if after := treeDigestIgnoring(t, root, ignored); after != before {
			t.Fatalf("vendor smoke changed normal configuration path %q", root)
		}
	})
}

// Snapshot records content and metadata digests for paths that must not change.
func Snapshot(t *testing.T, paths ...string) map[string][32]byte {
	t.Helper()
	result := make(map[string][32]byte, len(paths))
	for _, path := range paths {
		result[path] = treeDigest(t, path)
	}
	return result
}

// AssertUnchanged proves that a smoke test did not mutate normal config roots.
func AssertUnchanged(t *testing.T, before map[string][32]byte) {
	t.Helper()
	for path, digest := range before {
		if after := treeDigest(t, path); after != digest {
			t.Fatalf("vendor smoke changed normal configuration path %q", path)
		}
	}
}

func treeDigest(t *testing.T, root string) [32]byte {
	t.Helper()
	return treeDigestIgnoring(t, root, nil)
}

func treeDigestIgnoring(t *testing.T, root string, ignored []string) [32]byte {
	t.Helper()
	entries := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		relative := "."
		if path != root {
			value, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(value)
		}
		if isIgnoredPath(relative, ignored) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s\x00%d\x00%d\x00", relative, info.Mode(), info.Size())
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
		t.Fatalf("snapshot %q: %v", root, err)
	}
	sort.Strings(entries)
	return sha256.Sum256([]byte(strings.Join(entries, "\n")))
}

func normalizeIgnoredPaths(t *testing.T, paths []string) []string {
	t.Helper()
	result := make([]string, len(paths))
	for index, value := range paths {
		clean := filepath.ToSlash(filepath.Clean(value))
		if clean == "." || filepath.IsAbs(value) || clean == ".." || strings.HasPrefix(clean, "../") {
			t.Fatalf("ignored vendor smoke path must be relative and below its root: %q", value)
		}
		result[index] = clean
	}
	return result
}

func isIgnoredPath(relative string, ignored []string) bool {
	for _, value := range ignored {
		if relative == value || strings.HasPrefix(relative, value+"/") {
			return true
		}
	}
	return false
}

type boundedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	length := len(data)
	remaining := buffer.limit - len(buffer.data)
	if remaining <= 0 {
		buffer.truncated = buffer.truncated || length > 0
		return length, nil
	}
	if len(data) > remaining {
		buffer.data = append(buffer.data, data[:remaining]...)
		buffer.truncated = true
		return length, nil
	}
	buffer.data = append(buffer.data, data...)
	return length, nil
}

func (buffer *boundedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	value := string(buffer.data)
	if buffer.truncated {
		value += "\n[output truncated after 32768 bytes]"
	}
	return value
}
