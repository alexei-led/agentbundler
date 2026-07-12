package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRunMapsManifestAndSelectors(t *testing.T) {
	root := t.TempDir()
	manifest := `{"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base.json"]}}`
	if err := os.WriteFile(filepath.Join(root, "agentbundle.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	var request compiler.CompileRequest
	var stdout, stderr bytes.Buffer
	status := run([]string{"check", "--target", "claude", "--json"}, root, &stdout, &stderr, func(got compiler.CompileRequest) compiler.CompilationResult {
		request = got
		return compiler.CompilationResult{}
	})
	if status != 0 {
		t.Fatalf("run() status = %d, stderr = %q", status, stderr.String())
	}
	if request.Manifest.Kind != model.SourceKindBundle || len(request.Targets) != 1 || request.Targets[0] != model.TargetClaude {
		t.Fatalf("request = %#v", request)
	}
	if request.Mode != compiler.BuildModeCheck || request.WorkspaceRoot != root {
		t.Fatalf("request = %#v", request)
	}
	if !strings.Contains(stdout.String(), `"command":"check"`) || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunRejectsNativeBuildBeforeCompile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentbundle.json"), []byte(`{"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base.json"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	var stdout, stderr bytes.Buffer
	status := run([]string{"build", "--native"}, root, &stdout, &stderr, func(compiler.CompileRequest) compiler.CompilationResult {
		called = true
		return compiler.CompilationResult{}
	})
	if status == 0 || called || !strings.Contains(stderr.String(), "--native") {
		t.Fatalf("status=%d called=%v stderr=%q", status, called, stderr.String())
	}
}
