//go:build vendor_smoke

package pi_test

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/compiler"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/testutil/vendorsmoke"
)

func TestInstalledPiDiscoversAggregatePackageInProjectSettings(t *testing.T) {
	pi := vendorsmoke.RequireExecutable(t, "pi")
	workspace, packageRoot := compilePiSmokeFixture(t)
	isolatedRoot := t.TempDir()
	environment := vendorsmoke.Environment(map[string]string{
		"HOME":                filepath.Join(isolatedRoot, "home"),
		"PI_CODING_AGENT_DIR": filepath.Join(isolatedRoot, "pi-agent"),
		"XDG_CONFIG_HOME":     filepath.Join(isolatedRoot, "config"), "XDG_CACHE_HOME": filepath.Join(isolatedRoot, "cache"),
		"PI_OFFLINE": "1", "HTTP_PROXY": "http://127.0.0.1:9", "HTTPS_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
	})

	vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "pi install -l", Path: pi, Args: []string{"install", packageRoot, "-l", "--approve"},
		Dir: workspace, Env: environment, Timeout: 30 * time.Second,
	})
	projectSettings := filepath.Join(workspace, ".pi", "settings.json")
	settings := readOptionalFile(t, projectSettings)
	if !settings.exists || !bytes.Contains(settings.bytes, []byte(packageRoot)) {
		t.Fatalf("project settings do not contain aggregate package %q: %s", packageRoot, settings.bytes)
	}

	output := vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "pi list", Path: pi, Args: []string{"list", "--approve"},
		Dir: workspace, Env: environment, Timeout: 30 * time.Second,
	})
	if !strings.Contains(output, packageRoot) && !strings.Contains(output, "pi-hook-suite") {
		t.Fatalf("pi list did not discover aggregate package:\n%s", output)
	}
}

func TestInstalledPiLoaderImportsGeneratedAdapterOnceAndReportsSchemaMismatch(t *testing.T) {
	pi := vendorsmoke.RequireExecutable(t, "pi")
	node := vendorsmoke.RequireExecutable(t, "node")
	loader, ok := installedPiLoader(pi)
	if !ok {
		t.Skip("vendor smoke unavailable: installed pi is not a Node package with the documented extension loader")
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
	isolatedRoot := filepath.Join(cwd, ".vendor-smoke")
	output := vendorsmoke.Run(t, vendorsmoke.Command{
		Name: "Pi extension loader", Path: node,
		Args: []string{"--input-type=module", "--eval", script, fileURL(loader), adapter, cwd},
		Env: vendorsmoke.Environment(map[string]string{
			"HOME": filepath.Join(isolatedRoot, "home"), "PI_CODING_AGENT_DIR": filepath.Join(isolatedRoot, "pi-agent"),
			"XDG_CONFIG_HOME": filepath.Join(isolatedRoot, "config"), "XDG_CACHE_HOME": filepath.Join(isolatedRoot, "cache"),
			"PI_OFFLINE": "1", "HTTP_PROXY": "http://127.0.0.1:9", "HTTPS_PROXY": "http://127.0.0.1:9", "NO_PROXY": "",
		}),
		Timeout: 30 * time.Second,
	})
	var result loaderResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
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
