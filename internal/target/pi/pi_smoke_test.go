//go:build vendor_smoke

package pi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestInstalledPiDiscoversAggregatePackageWithoutRealConfigChanges(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("installed Pi smoke unavailable: pi executable is not on PATH")
	}
	workspace, packageRoot := compilePiSmokeFixture(t)
	isolatedConfig := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", isolatedConfig)
	realConfig := realPiConfigPath(t)
	before := readOptionalFile(t, realConfig)

	install := exec.Command(pi, "install", packageRoot, "-l", "--approve")
	install.Dir = workspace
	install.Env = append(os.Environ(), "PI_OFFLINE=1")
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("pi install -l: %v\n%s", err, output)
	}
	projectSettings := filepath.Join(workspace, ".pi", "settings.json")
	settings := readOptionalFile(t, projectSettings)
	if !settings.exists || !bytes.Contains(settings.bytes, []byte(packageRoot)) {
		t.Fatalf("project settings do not contain aggregate package %q: %s", packageRoot, settings.bytes)
	}

	list := exec.Command(pi, "list", "--approve")
	list.Dir = workspace
	list.Env = append(os.Environ(), "PI_OFFLINE=1")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("pi list: %v\n%s", err, output)
	}
	if !bytes.Contains(output, []byte(packageRoot)) && !bytes.Contains(output, []byte("pi-hook-suite")) {
		t.Fatalf("pi list did not discover aggregate package:\n%s", output)
	}
	if after := readOptionalFile(t, realConfig); after.exists != before.exists || !bytes.Equal(after.bytes, before.bytes) {
		t.Fatalf("Pi smoke changed real config %q", realConfig)
	}
}

func TestInstalledPiLoaderImportsGeneratedAdapterOnceAndReportsSchemaMismatch(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("installed Pi loader smoke unavailable: pi executable is not on PATH")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("installed Pi loader smoke unavailable: node executable is not on PATH")
	}
	loader, ok := installedPiLoader(pi)
	if !ok {
		t.Skip("installed Pi loader smoke unavailable: pi is not a Node package installation")
	}
	workspace, packageRoot := compilePiSmokeFixture(t)
	adapter := filepath.Join(packageRoot, "extensions", "agentbundler-hooks.ts")

	loaded := runPiLoader(t, node, loader, adapter, workspace)
	if len(loaded.Errors) != 0 {
		t.Fatalf("Pi loader errors = %#v", loaded.Errors)
	}
	if loaded.Extensions != 1 || loaded.Paths[0] != adapter || loaded.HandlerCounts["tool_call"] != 1 || loaded.HandlerCounts["tool_result"] != 1 {
		t.Fatalf("Pi loader registration = %#v, want one extension with one tool handler registration", loaded)
	}

	configPath := filepath.Join(packageRoot, "hooks", "hooks.v1.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"version":1`), []byte(`"version":2`), 1)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	mismatch := runPiLoader(t, node, loader, adapter, workspace)
	if len(mismatch.Errors) != 1 || !strings.Contains(mismatch.Errors[0], "unsupported hook schema version 2") || !strings.Contains(mismatch.Errors[0], "agentbundler-hooks.ts") {
		t.Fatalf("Pi loader schema mismatch = %#v", mismatch)
	}
}

type loaderResult struct {
	Extensions    int            `json:"extensions"`
	Paths         []string       `json:"paths"`
	HandlerCounts map[string]int `json:"handlerCounts"`
	Errors        []string       `json:"errors"`
}

func runPiLoader(t *testing.T, node, loader, adapter, cwd string) loaderResult {
	t.Helper()
	script := `const {loadExtensions}=await import(process.argv[1]);
const result=await loadExtensions([process.argv[2]],process.argv[3]);
const counts={};
for(const extension of result.extensions) for(const [name, handlers] of extension.handlers) counts[name]=(counts[name]??0)+handlers.length;
console.log(JSON.stringify({extensions:result.extensions.length,paths:result.extensions.map(value=>value.path),handlerCounts:counts,errors:result.errors.map(value=>value.path+": "+String(value.error?.stack??value.error))}));`
	command := exec.Command(node, "--input-type=module", "--eval", script, fileURL(loader), adapter, cwd)
	command.Env = append(os.Environ(), "PI_OFFLINE=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Pi extension loader: %v\n%s", err, output)
	}
	var result loaderResult
	if err := json.Unmarshal(bytes.TrimSpace(output), &result); err != nil {
		t.Fatalf("decode Pi loader output %q: %v", output, err)
	}
	return result
}

func installedPiLoader(pi string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(pi)
	if err != nil {
		return "", false
	}
	root := filepath.Dir(filepath.Dir(resolved))
	loader := filepath.Join(root, "dist", "core", "extensions", "loader.js")
	if info, err := os.Stat(loader); err != nil || info.IsDir() {
		return "", false
	}
	return loader, true
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func compilePiSmokeFixture(t *testing.T) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	fixture := filepath.Join("..", "..", "compiler", "testdata", "hooks-pi")
	if err := os.CopyFS(workspace, os.DirFS(fixture)); err != nil {
		t.Fatalf("copy Pi fixture: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "agentbundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, diagnostics := model.DecodeSourceManifestJSON(data)
	if len(diagnostics) != 0 {
		t.Fatalf("decode Pi fixture: %#v", diagnostics)
	}
	result := compiler.Compile(compiler.CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: compiler.BuildModeBuild})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile Pi fixture: %#v", result.Diagnostics)
	}
	return workspace, filepath.Join(workspace, "generated", "pi")
}

type optionalFile struct {
	exists bool
	bytes  []byte
}

func readOptionalFile(t *testing.T, path string) optionalFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return optionalFile{}
	}
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return optionalFile{exists: true, bytes: data}
}

func realPiConfigPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home == "" {
		t.Fatal(fmt.Errorf("user home directory is empty"))
	}
	return filepath.Join(home, ".pi", "agent", "settings.json")
}
