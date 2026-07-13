package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRunBuildThenCheckDetectsRealDrift(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "agentbundle.json", `{"version":1,"kind":"skills-repository","root":"source","targets":["claude"],"output":"generated","skillsRepository":{"package":"demo","roots":["skills"],"metadata":{}}}`)
	writeCLIFile(t, root, "source/skills/demo/SKILL.md", "# Demo\n")

	var stdout, stderr bytes.Buffer
	if status := run([]string{"build"}, root, &stdout, &stderr, compiler.Compile); status != 0 || stdout.String() != "build: ok\n" || stderr.Len() != 0 {
		t.Fatalf("build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if status := run([]string{"check"}, root, &stdout, &stderr, compiler.Compile); status != 0 || stdout.String() != "check: current\n" || stderr.Len() != 0 {
		t.Fatalf("current check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}

	writeCLIFile(t, root, "generated/claude/.claude/skills/demo/SKILL.md", "changed")
	stdout.Reset()
	stderr.Reset()
	if status := run([]string{"check"}, root, &stdout, &stderr, compiler.Compile); status != 2 || !strings.Contains(stderr.String(), "DRIFT_CHANGED") {
		t.Fatalf("drift check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
}

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

func TestRunReturnsFailureWhenJSONOutputCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentbundle.json"), []byte(`{"version":1,"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base.json"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	status := run([]string{"check", "--json"}, root, failingWriter{}, &stderr, func(compiler.CompileRequest) compiler.CompilationResult {
		return compiler.CompilationResult{}
	})
	if status != 1 || !strings.Contains(stderr.String(), "OUTPUT_WRITE_FAILED") {
		t.Fatalf("status=%d stderr=%q", status, stderr.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("writer failed") }

func writeCLIFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
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
