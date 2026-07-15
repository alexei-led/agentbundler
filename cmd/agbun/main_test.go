package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestFormatDiagnosticIncludesActionableHint(t *testing.T) {
	diagnostic := model.Diagnostic{
		Code: "unsupported-agent-field", Severity: model.SeverityError,
		Message: "field is unsupported", Hint: "move it to a target sidecar",
	}
	want := "error[unsupported-agent-field]: field is unsupported\n  hint: move it to a target sidecar"
	if got := formatDiagnostic(diagnostic); got != want {
		t.Fatalf("formatDiagnostic() = %q, want %q", got, want)
	}
}

func TestRunHelpListsCommandsAndDiscoveryTopics(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run([]string{"help"}, t.TempDir(), &stdout, &stderr, compiler.Compile); status != 0 {
		t.Fatalf("help status=%d stderr=%q", status, stderr.String())
	}
	for _, want := range []string{
		"Agent Bundler", "agbun <command>", "Start here:", "build", "check", "help [topic]",
		"version", "--version", "agbun help <topic>",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help output missing %q: %q", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help stderr=%q", stderr.String())
	}
}

func TestRunVersionDoesNotNeedManifest(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if status := run(args, t.TempDir(), &stdout, &stderr, compiler.Compile); status != 0 {
				t.Fatalf("version status=%d stderr=%q", status, stderr.String())
			}
			if !strings.HasPrefix(stdout.String(), "agbun ") || !strings.HasSuffix(stdout.String(), "\n") {
				t.Fatalf("version output=%q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("version stderr=%q", stderr.String())
			}
		})
	}
}

func TestRunHelpTopicsDoNotNeedManifest(t *testing.T) {
	for _, topic := range []string{"build", "check", "targets", "version"} {
		t.Run(topic, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if status := run([]string{"help", topic}, t.TempDir(), &stdout, &stderr, compiler.Compile); status != 0 {
				t.Fatalf("help status=%d stderr=%q", status, stderr.String())
			}
			if stdout.Len() == 0 || stderr.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunCommandHelpDoesNotNeedManifest(t *testing.T) {
	for _, command := range []string{"build", "check"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if status := run([]string{command, "--help"}, t.TempDir(), &stdout, &stderr, compiler.Compile); status != 0 {
				t.Fatalf("help status=%d stderr=%q", status, stderr.String())
			}
			for _, want := range []string{"Usage:", "--root DIR", "--target TARGET", "--package ID", "--json"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("help output missing %q: %q", want, stdout.String())
				}
			}
			if command == "check" && !strings.Contains(stdout.String(), "--native") {
				t.Errorf("check help missing --native: %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("help stderr=%q", stderr.String())
			}
		})
	}
}

func TestRunRejectsUnknownHelpTopicWithDiscoveryHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run([]string{"help", "unknown"}, t.TempDir(), &stdout, &stderr, compiler.Compile)
	if status != 1 || !strings.Contains(stderr.String(), `unknown help topic "unknown"`) || !strings.Contains(stderr.String(), `Run "agbun help"`) {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
}

func TestRunRejectsUnknownCommandWithDiscoveryHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	status := run([]string{"unknown"}, t.TempDir(), &stdout, &stderr, compiler.Compile)
	if status != 1 || !strings.Contains(stderr.String(), "expected a command") || !strings.Contains(stderr.String(), `Run "agbun help"`) {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
}

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

func TestRunJSONIncludesStructuredDiagnosticHint(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "agentbundle.json", `{"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base.json"]}}`)
	var stdout, stderr bytes.Buffer
	status := run([]string{"check", "--json"}, root, &stdout, &stderr, func(compiler.CompileRequest) compiler.CompilationResult {
		return compiler.CompilationResult{Diagnostics: []model.Diagnostic{{
			Code: "unsupported-agent-field", Severity: model.SeverityError,
			Message: "unsupported field", Hint: "move it", Asset: "agent/demo",
			Field: "sandbox_mode", Targets: []model.TargetID{model.TargetClaude},
		}}}
	})
	if status != 1 || stderr.Len() != 0 {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"hint":"move it"`, `"asset":"agent/demo"`, `"field":"sandbox_mode"`, `"targets":["claude"]`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("JSON output missing %q: %q", want, stdout.String())
		}
	}
}

func TestRunJSONLocationContract(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "agentbundle.json", `{"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base.json"]}}`)
	location := model.SourceLocation{Path: "source/SKILL.md"}
	var stdout, stderr bytes.Buffer
	status := run([]string{"check", "--json"}, root, &stdout, &stderr, func(compiler.CompileRequest) compiler.CompilationResult {
		return compiler.CompilationResult{Diagnostics: []model.Diagnostic{
			{Code: "invalid-source", Severity: model.SeverityError, Location: &location, Message: "invalid source"},
			{Code: "missing-source", Severity: model.SeverityError, Message: "missing source"},
		}}
	})
	if status != 1 || stderr.Len() != 0 {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}

	var envelope struct {
		Diagnostics []map[string]any `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(envelope.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", envelope.Diagnostics)
	}
	locationJSON, ok := envelope.Diagnostics[0]["location"].(map[string]any)
	if !ok {
		t.Fatalf("location = %#v", envelope.Diagnostics[0]["location"])
	}
	if locationJSON["path"] != "source/SKILL.md" || locationJSON["line"] != nil || locationJSON["column"] != nil {
		t.Fatalf("location = %#v", locationJSON)
	}
	if _, exists := envelope.Diagnostics[1]["location"]; exists {
		t.Fatalf("location = %#v, want omitted", envelope.Diagnostics[1]["location"])
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

func TestReleaseBuildPrintsInjectedVersion(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agbun")
	const version = "v9.8.7"
	build := exec.Command(
		"go", "build", "-o", binary,
		"-ldflags", "-X github.com/alexei-led/agentbundler/internal/buildinfo.releaseVersion="+version,
		".",
	)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("release build failed: %v\n%s", err, output)
	}
	output, err := exec.Command(binary, "--version").Output()
	if err != nil {
		t.Fatalf("release binary --version failed: %v", err)
	}
	if got, want := string(output), "agbun "+version+"\n"; got != want {
		t.Fatalf("release binary version = %q, want %q", got, want)
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
