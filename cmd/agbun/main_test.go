package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	for _, topic := range []string{"build", "check", "package", "targets", "version"} {
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

func TestTargetsHelpListsAntigravityInLexicalOrder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run([]string{"help", "targets"}, t.TempDir(), &stdout, &stderr, compiler.Compile); status != 0 {
		t.Fatalf("targets help status=%d stderr=%q", status, stderr.String())
	}
	want := []string{"antigravity", "claude", "codex", "copilot", "cursor", "grok", "pi"}
	previous := -1
	for _, target := range want {
		index := strings.Index(stdout.String(), "\n  "+target+" ")
		if index <= previous {
			t.Fatalf("targets help is missing lexical order %q: %q", want, stdout.String())
		}
		previous = index
	}
	for _, help := range []string{buildHelp(), checkHelp(), packageHelp()} {
		if !strings.Contains(help, "antigravity") {
			t.Errorf("command help does not list Antigravity: %q", help)
		}
	}
}

func TestRunCommandHelpDoesNotNeedManifest(t *testing.T) {
	for _, command := range []string{"build", "check", "package"} {
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
			if command == "package" && !strings.Contains(stdout.String(), "--out DIR") {
				t.Errorf("package help missing --out: %q", stdout.String())
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

func TestRunPackageArchivesCurrentTargetRootWithoutRebuilding(t *testing.T) {
	for _, test := range []struct {
		name     string
		manifest string
		archive  string
	}{
		{
			name:     "Claude",
			manifest: `{"version":1,"kind":"skills-repository","root":"source","targets":["claude"],"output":"generated","distribution":{"name":"demo"},"skillsRepository":{"package":"demo","roots":["skills"],"metadata":{}}}`,
			archive:  "demo-claude.tar.gz",
		},
		{
			name:     "Antigravity",
			manifest: `{"version":1,"kind":"skills-repository","root":"source","targets":["antigravity"],"output":"generated","distribution":{"name":"demo"},"composition":[{"target":"antigravity","profile":"package","packageMode":"separate"}],"skillsRepository":{"package":"demo","roots":["skills"],"metadata":{}}}`,
			archive:  "demo-antigravity.tar.gz",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeCLIFile(t, root, "agentbundle.json", test.manifest)
			writeCLIFile(t, root, "source/skills/demo/SKILL.md", "# Demo\n")
			var stdout, stderr bytes.Buffer
			if status := run([]string{"build"}, root, &stdout, &stderr, compiler.Compile); status != 0 {
				t.Fatalf("build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
			stdout.Reset()
			stderr.Reset()
			if status := run([]string{"package", "--out", "release"}, root, &stdout, &stderr, compiler.Compile); status != 0 {
				t.Fatalf("package status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), test.archive) || !strings.Contains(stdout.String(), "package: ok") || stderr.Len() != 0 {
				t.Fatalf("package stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if _, err := os.Stat(filepath.Join(root, "release", test.archive)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRunCompatibilityPackageChecksRootAndArchivesOnlyTargetOutput(t *testing.T) {
	root := t.TempDir()
	manifest := `{"version":1,"kind":"skills-repository","root":"source","targets":["claude"],"output":"dist","distribution":{"name":"demo","owner":"Platform","description":"Demo","version":"1.0.0"},"composition":[{"target":"claude","profile":"package","packageMode":"separate"}],"compatibility":{"rootManifests":["claude"]},"skillsRepository":{"package":"demo","roots":["skills"],"metadata":{"description":"Demo","version":"1.0.0","author":"Platform","homepage":"https://example.com","repository":"https://example.com/repo","license":"MIT","keywords":["demo"]}}}`
	writeCLIFile(t, root, "agentbundle.json", manifest)
	writeCLIFile(t, root, "source/skills/demo/SKILL.md", "# Demo\n")
	var stdout, stderr bytes.Buffer
	if status := run([]string{"build"}, root, &stdout, &stderr, compiler.Compile); status != 0 {
		t.Fatalf("build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if status := run([]string{"package", "--out", "release"}, root, &stdout, &stderr, compiler.Compile); status != 0 {
		t.Fatalf("package status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	archivePath := filepath.Join(root, "release", "demo-claude.tar.gz")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatal(err)
	}
	entries := archiveEntriesForTest(t, archivePath)
	for _, entry := range entries {
		if strings.Contains(entry, "compatibility") || strings.HasPrefix(entry, ".agentbundler") {
			t.Fatalf("compatibility root leaked into archive: %#v", entries)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".claude-plugin/marketplace.json"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if status := run([]string{"check", "--json"}, root, &stdout, &stderr, compiler.Compile); status != 2 || !strings.Contains(stdout.String(), `"code":"COMPATIBILITY_DRIFT_CHANGED"`) {
		t.Fatalf("compatibility JSON check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
}

func TestRunBuildThenCheckDetectsRealDrift(t *testing.T) {
	for _, test := range []struct {
		name      string
		manifest  string
		driftPath string
	}{
		{
			name:      "Claude",
			manifest:  `{"version":1,"kind":"skills-repository","root":"source","targets":["claude"],"output":"generated","skillsRepository":{"package":"demo","roots":["skills"],"metadata":{}}}`,
			driftPath: "generated/claude/.claude/skills/demo/SKILL.md",
		},
		{
			name:      "Antigravity",
			manifest:  `{"version":1,"kind":"skills-repository","root":"source","targets":["antigravity"],"output":"generated","composition":[{"target":"antigravity","profile":"package","packageMode":"separate"}],"skillsRepository":{"package":"demo","roots":["skills"],"metadata":{}}}`,
			driftPath: "generated/antigravity/skills/demo/SKILL.md",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeCLIFile(t, root, "agentbundle.json", test.manifest)
			writeCLIFile(t, root, "source/skills/demo/SKILL.md", "# Demo\n")

			var stdout, stderr bytes.Buffer
			if status := run([]string{"build"}, root, &stdout, &stderr, compiler.Compile); status != 0 || stdout.String() != "build: ok\n" || stderr.Len() != 0 {
				t.Fatalf("build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
			stdout.Reset()
			if status := run([]string{"check"}, root, &stdout, &stderr, compiler.Compile); status != 0 || stdout.String() != "check: current\n" || stderr.Len() != 0 {
				t.Fatalf("current check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}

			writeCLIFile(t, root, test.driftPath, "changed")
			stdout.Reset()
			stderr.Reset()
			if status := run([]string{"check"}, root, &stdout, &stderr, compiler.Compile); status != 2 || !strings.Contains(stderr.String(), "DRIFT_CHANGED") {
				t.Fatalf("drift check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunMapsManifestAndSelectors(t *testing.T) {
	root := t.TempDir()
	manifest := `{"version":1,"kind":"bundle","root":"source","targets":["claude"],"output":"generated","distribution":{"name":"Team tools"},"composition":[{"target":"claude","profile":"package","packageMode":"separate"}],"bundle":{"packages":["packages/base.json"]}}`
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
	if request.Manifest.Distribution["name"] != "Team tools" || len(request.Manifest.Composition) != 1 || request.Manifest.Composition[0].PackageMode != model.TargetPackageModeSeparate {
		t.Fatalf("render configuration = %#v", request.Manifest)
	}
	if request.Mode != compiler.BuildModeCheck || request.WorkspaceRoot != root {
		t.Fatalf("request = %#v", request)
	}
	if !strings.Contains(stdout.String(), `"command":"check"`) || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMapsAntigravityManifestAndSelectors(t *testing.T) {
	root := t.TempDir()
	manifest := `{"version":1,"kind":"bundle","root":"source","targets":["antigravity"],"output":"generated","distribution":{"name":"Team tools"},"composition":[{"target":"antigravity","profile":"package","packageMode":"separate"}],"bundle":{"packages":["packages/base.json"]}}`
	if err := os.WriteFile(filepath.Join(root, "agentbundle.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	var request compiler.CompileRequest
	var stdout, stderr bytes.Buffer
	status := run([]string{"check", "--target", "antigravity", "--package", "core-tools", "--json"}, root, &stdout, &stderr, func(got compiler.CompileRequest) compiler.CompilationResult {
		request = got
		return compiler.CompilationResult{}
	})
	if status != 0 {
		t.Fatalf("run() status = %d, stderr = %q", status, stderr.String())
	}
	if request.Manifest.Kind != model.SourceKindBundle || len(request.Targets) != 1 || request.Targets[0] != model.TargetAntigravity || !reflect.DeepEqual(request.Packages, []model.PackageID{"core-tools"}) {
		t.Fatalf("request = %#v", request)
	}
	if request.Manifest.Distribution["name"] != "Team tools" || len(request.Manifest.Composition) != 1 || request.Manifest.Composition[0].PackageMode != model.TargetPackageModeSeparate {
		t.Fatalf("render configuration = %#v", request.Manifest)
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

func archiveEntriesForTest(t *testing.T, archivePath string) []string {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gzipReader.Close() }()
	reader := tar.NewReader(gzipReader)
	var entries []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, header.Name)
	}
	return entries
}

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

func TestReleaseBuildEmbedsTestedRuntimeAndPrintsInjectedVersion(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agbun")
	const version = "v9.8.7"
	build := exec.Command(
		"go", "build", "-trimpath", "-o", binary,
		"-ldflags", "-X github.com/alexei-led/agentbundler/internal/buildinfo.releaseVersion="+version,
		".",
	)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("release build failed: %v\n%s", err, output)
	}
	for _, argument := range []string{"version", "--version"} {
		output, err := exec.Command(binary, argument).Output()
		if err != nil {
			t.Fatalf("release binary %s failed: %v", argument, err)
		}
		if got, want := string(output), "agbun "+version+"\n"; got != want {
			t.Fatalf("release binary %s = %q, want %q", argument, got, want)
		}
	}

	workspace := t.TempDir()
	fixture := filepath.Join("..", "..", "internal", "compiler", "testdata", "hooks-pi")
	if err := os.CopyFS(workspace, os.DirFS(fixture)); err != nil {
		t.Fatalf("copy Pi release fixture: %v", err)
	}
	command := exec.Command(binary, "build", "--root", workspace)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("release binary Pi build failed: %v\n%s", err, output)
	}
	for _, name := range []string{"index.ts", "matcher.ts", "process.ts", "runtime.ts", "schema.ts"} {
		source, err := os.ReadFile(filepath.Join("..", "..", "internal", "target", "pi", "runtime", "src", name))
		if err != nil {
			t.Fatal(err)
		}
		generated, err := os.ReadFile(filepath.Join(workspace, "generated", "pi", "extensions", "_agentbundler-hooks", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(generated, source) {
			t.Errorf("release binary embedded runtime %q differs from tested source bytes", name)
		}
	}
}

func TestRunCCThingzAcceptanceFixtureBuildCheckAndSelectors(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "..", "testdata", "cc-thingz-hooks"))); err != nil {
		t.Fatalf("copy cc-thingz fixture: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"build"}, root, &stdout, &stderr, compiler.Compile); status != 0 || stdout.String() != "build: ok\n" || stderr.Len() != 0 {
		t.Fatalf("acceptance build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if status := run([]string{"check"}, root, &stdout, &stderr, compiler.Compile); status != 0 || stdout.String() != "check: current\n" || stderr.Len() != 0 {
		t.Fatalf("acceptance check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	for _, relative := range []string{
		"generated/antigravity/core-tools/plugin.json",
		"generated/antigravity/core-tools/rules/conductor_antigravity.md",
		"generated/claude/.claude-plugin/marketplace.json",
		"generated/codex/.agents/plugins/marketplace.json",
		"generated/copilot/.github/plugin/marketplace.json",
		"generated/cursor/.cursor-plugin/marketplace.json",
		"generated/grok/.claude-plugin/marketplace.json",
		"generated/pi/extensions/agentbundler-hooks.ts",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Errorf("acceptance output %q: %v", relative, err)
		}
	}

	for _, selected := range []struct {
		target       string
		manifestPath string
	}{
		{target: "antigravity", manifestPath: "generated/antigravity/plugin.json"},
		{target: "codex", manifestPath: "generated/codex/.codex-plugin/plugin.json"},
	} {
		t.Run("selected "+selected.target, func(t *testing.T) {
			selectedRoot := t.TempDir()
			if err := os.CopyFS(selectedRoot, os.DirFS(filepath.Join("..", "..", "testdata", "cc-thingz-hooks"))); err != nil {
				t.Fatalf("copy selected cc-thingz fixture: %v", err)
			}
			stdout.Reset()
			stderr.Reset()
			if status := run([]string{"build", "--target", selected.target, "--package", "core-tools"}, selectedRoot, &stdout, &stderr, compiler.Compile); status != 0 {
				t.Fatalf("selected acceptance build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
			if _, err := os.Stat(filepath.Join(selectedRoot, filepath.FromSlash(selected.manifestPath))); err != nil {
				t.Fatalf("selected flat %s package: %v", selected.target, err)
			}
			if _, err := os.Stat(filepath.Join(selectedRoot, "generated", "claude")); !os.IsNotExist(err) {
				t.Fatalf("selected build unexpectedly wrote Claude output: %v", err)
			}
		})
	}
}

func TestRunAntigravityNativeCheckReportsUnavailableToolInJSON(t *testing.T) {
	root := t.TempDir()
	writeCLIFile(t, root, "agentbundle.json", `{"version":1,"kind":"skills-repository","root":"source","targets":["antigravity"],"output":"generated","composition":[{"target":"antigravity","profile":"package","packageMode":"separate"}],"skillsRepository":{"package":"demo","roots":["skills"],"metadata":{}}}`)
	writeCLIFile(t, root, "source/skills/demo/SKILL.md", "# Demo\n")

	var stdout, stderr bytes.Buffer
	if status := run([]string{"build"}, root, &stdout, &stderr, compiler.Compile); status != 0 {
		t.Fatalf("Antigravity build status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	t.Setenv("PATH", t.TempDir())
	stdout.Reset()
	stderr.Reset()
	status := run([]string{"check", "--target", "antigravity", "--native", "--json"}, root, &stdout, &stderr, compiler.Compile)
	if status != 3 || stderr.Len() != 0 {
		t.Fatalf("Antigravity native check status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	var result jsonResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.NativeVerificationFailed || result.Drift || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "NATIVE_VERIFY_TOOL_UNAVAILABLE" || !strings.Contains(result.Diagnostics[0].Message, `tool "agy" is unavailable`) {
		t.Fatalf("Antigravity native JSON result = %#v", result)
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
