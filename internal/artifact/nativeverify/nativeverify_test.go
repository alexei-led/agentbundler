package nativeverify

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRunNativeChecksPassesArgumentsLiterally(t *testing.T) {
	t.Setenv("NATIVEVERIFY_HELPER", "1")
	outputRoot := t.TempDir()
	observed := filepath.Join(t.TempDir(), "observed")
	marker := filepath.Join(t.TempDir(), "shell-ran")
	literal := "literal;touch " + marker + ";#"

	result := RunNativeChecks([]model.NativeCheck{testCheck(helperArguments("literal", observed, literal))}, outputRoot)

	if !result.Success {
		t.Fatalf("RunNativeChecks() success = false, diagnostics = %#v", result.Diagnostics)
	}
	contents, err := os.ReadFile(observed)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != literal {
		t.Errorf("received argument = %q, want %q", contents, literal)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("shell marker exists or could not be checked: %v", err)
	}
}

func TestRunNativeChecksReportsUnavailableTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	check := testCheck(nil)
	check.Program = "missing-nativeverify-tool"

	result := RunNativeChecks([]model.NativeCheck{check}, t.TempDir())

	assertDiagnostic(t, result.Diagnostics, 0, toolUnavailableCode, model.SeverityError, check.Location)
	if result.Success {
		t.Error("RunNativeChecks() success = true, want false")
	}
}

func TestRunNativeChecksWorkingDirectory(t *testing.T) {
	t.Run("defaults to output root", func(t *testing.T) {
		t.Setenv("NATIVEVERIFY_HELPER", "1")
		outputRoot := t.TempDir()
		observed := filepath.Join(t.TempDir(), "cwd")
		check := testCheck(helperArguments("cwd", observed))

		result := RunNativeChecks([]model.NativeCheck{check}, outputRoot)

		if !result.Success {
			t.Fatalf("RunNativeChecks() success = false, diagnostics = %#v", result.Diagnostics)
		}
		got, err := os.ReadFile(observed)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		want, err := filepath.EvalSymlinks(outputRoot)
		if err != nil {
			t.Fatalf("EvalSymlinks() error = %v", err)
		}
		if string(got) != want {
			t.Errorf("working directory = %q, want %q", got, want)
		}
	})

	t.Run("rejects escape without starting process", func(t *testing.T) {
		t.Setenv("NATIVEVERIFY_HELPER", "1")
		parent := t.TempDir()
		outputRoot := filepath.Join(parent, "output")
		outside := filepath.Join(parent, "outside")
		if err := os.MkdirAll(outputRoot, 0o755); err != nil {
			t.Fatalf("MkdirAll(output root) error = %v", err)
		}
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatalf("MkdirAll(outside) error = %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(outputRoot, "escape")); err != nil {
			t.Skipf("Symlink() error = %v", err)
		}
		marker := filepath.Join(t.TempDir(), "started")
		workingDirectory := model.RelativePath("escape")
		check := testCheck(helperArguments("marker", marker))
		check.WorkingDirectory = &workingDirectory

		result := RunNativeChecks([]model.NativeCheck{check}, outputRoot)

		assertDiagnostic(t, result.Diagnostics, 0, workingDirectoryEscapeCode, model.SeverityError, check.Location)
		if result.Success {
			t.Error("RunNativeChecks() success = true, want false")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Errorf("process marker exists or could not be checked: %v", err)
		}
	})
}

func TestRunNativeChecksBoundsOutputAndWarns(t *testing.T) {
	t.Setenv("NATIVEVERIFY_HELPER", "1")
	check := testCheck(helperArguments("large-failure", strconv.Itoa(MaxOutputBytesPerStream+1)))

	result := RunNativeChecks([]model.NativeCheck{check}, t.TempDir())

	if result.Success {
		t.Error("RunNativeChecks() success = true, want false")
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostic count = %d, want 2", len(result.Diagnostics))
	}
	assertDiagnostic(t, result.Diagnostics, 0, failedCode, model.SeverityError, check.Location)
	assertDiagnostic(t, result.Diagnostics, 1, outputTruncatedCode, model.SeverityWarning, check.Location)
	if got := strings.Count(result.Diagnostics[0].Message, strings.Repeat("s", MaxOutputBytesPerStream)); got != 1 {
		t.Errorf("stdout evidence count = %d, want 1", got)
	}
	if got := strings.Count(result.Diagnostics[0].Message, strings.Repeat("e", MaxOutputBytesPerStream)); got != 1 {
		t.Errorf("stderr evidence count = %d, want 1", got)
	}
	if !strings.Contains(result.Diagnostics[1].Message, "stdout and stderr") {
		t.Errorf("truncation message = %q, want both streams", result.Diagnostics[1].Message)
	}
	if got := strings.Count(result.Diagnostics[0].Message, "[truncated after 32768 bytes]"); got != 2 {
		t.Errorf("truncation evidence count = %d, want 2", got)
	}
}

func TestRunNativeChecksTruncationWarningDoesNotFailSuccessfulCheck(t *testing.T) {
	t.Setenv("NATIVEVERIFY_HELPER", "1")
	check := testCheck(helperArguments("large-success", strconv.Itoa(MaxOutputBytesPerStream+1)))

	result := RunNativeChecks([]model.NativeCheck{check}, t.TempDir())

	if !result.Success {
		t.Errorf("RunNativeChecks() success = false, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostic count = %d, want 1", len(result.Diagnostics))
	}
	assertDiagnostic(t, result.Diagnostics, 0, outputTruncatedCode, model.SeverityWarning, check.Location)
}

func TestRunNativeChecksEscapesInvalidUTF8(t *testing.T) {
	t.Setenv("NATIVEVERIFY_HELPER", "1")
	check := testCheck(helperArguments("invalid-utf8"))

	result := RunNativeChecks([]model.NativeCheck{check}, t.TempDir())

	if result.Success {
		t.Error("RunNativeChecks() success = true, want false")
	}
	assertDiagnostic(t, result.Diagnostics, 0, failedCode, model.SeverityError, check.Location)
	if !strings.Contains(result.Diagnostics[0].Message, "stdout\n\\xFF") || !strings.Contains(result.Diagnostics[0].Message, "stderr\n\\xFE") {
		t.Errorf("failure diagnostic does not escape invalid UTF-8: %q", result.Diagnostics[0].Message)
	}
}

func TestRunNativeChecksReportsNonzeroAndSignalFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
	}{
		{name: "nonzero exit", mode: "failure"},
		{name: "signal termination", mode: "signal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.mode == "signal" && runtime.GOOS == "windows" {
				t.Skip("os.Kill does not provide Unix signal semantics on Windows")
			}
			t.Setenv("NATIVEVERIFY_HELPER", "1")
			check := testCheck(helperArguments(test.mode))

			result := RunNativeChecks([]model.NativeCheck{check}, t.TempDir())

			if result.Success {
				t.Error("RunNativeChecks() success = true, want false")
			}
			assertDiagnostic(t, result.Diagnostics, 0, failedCode, model.SeverityError, check.Location)
			if !strings.Contains(result.Diagnostics[0].Message, "stdout-evidence") || !strings.Contains(result.Diagnostics[0].Message, "stderr-evidence") {
				t.Errorf("failure diagnostic does not contain both streams: %q", result.Diagnostics[0].Message)
			}
		})
	}
}

func TestRunNativeChecksRejectsInvalidDeclarations(t *testing.T) {
	for _, program := range []string{""} {
		t.Run(program, func(t *testing.T) {
			check := testCheck(nil)
			check.Program = program

			result := RunNativeChecks([]model.NativeCheck{check}, t.TempDir())

			if result.Success {
				t.Error("RunNativeChecks() success = true, want false")
			}
			assertDiagnostic(t, result.Diagnostics, 0, invalidCheckCode, model.SeverityError, check.Location)
		})
	}
}

func TestNativeVerifyHelperProcess(t *testing.T) {
	if os.Getenv("NATIVEVERIFY_HELPER") != "1" {
		return
	}
	arguments := helperProcessArguments(t)
	switch arguments[0] {
	case "literal":
		if err := os.WriteFile(arguments[1], []byte(arguments[2]), 0o600); err != nil {
			os.Exit(2)
		}
	case "cwd":
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(arguments[1], []byte(cwd), 0o600); err != nil {
			os.Exit(2)
		}
	case "marker":
		if err := os.WriteFile(arguments[1], []byte("started"), 0o600); err != nil {
			os.Exit(2)
		}
	case "large-failure":
		size, err := strconv.Atoi(arguments[1])
		if err != nil {
			os.Exit(2)
		}
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("s", size))
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("e", size))
		os.Exit(7)
	case "large-success":
		size, err := strconv.Atoi(arguments[1])
		if err != nil {
			os.Exit(2)
		}
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("s", size))
	case "failure":
		_, _ = fmt.Fprint(os.Stdout, "stdout-evidence")
		_, _ = fmt.Fprint(os.Stderr, "stderr-evidence")
		os.Exit(7)
	case "invalid-utf8":
		_, _ = os.Stdout.Write([]byte("stdout\n\xff"))
		_, _ = os.Stderr.Write([]byte("stderr\n\xfe"))
		os.Exit(7)
	case "signal":
		_, _ = fmt.Fprint(os.Stdout, "stdout-evidence")
		_, _ = fmt.Fprint(os.Stderr, "stderr-evidence")
		process, err := os.FindProcess(os.Getpid())
		if err != nil || process.Signal(os.Kill) != nil {
			os.Exit(2)
		}
		select {}
	default:
		os.Exit(2)
	}
}

func testCheck(arguments []string) model.NativeCheck {
	return model.NativeCheck{
		Program:   os.Args[0],
		Arguments: arguments,
		Location: model.SourceLocation{
			Path: model.RelativePath("native-check.json"),
		},
	}
}

func helperArguments(mode string, arguments ...string) []string {
	return append([]string{"-test.run=^TestNativeVerifyHelperProcess$", "--", mode}, arguments...)
}

func helperProcessArguments(t *testing.T) []string {
	t.Helper()
	for index, argument := range os.Args {
		if argument == "--" {
			if index+1 < len(os.Args) {
				return os.Args[index+1:]
			}
			break
		}
	}
	t.Fatal("helper process arguments are missing")
	return nil
}

func assertDiagnostic(t *testing.T, diagnostics []model.Diagnostic, index int, code string, severity model.Severity, location model.SourceLocation) {
	t.Helper()
	if len(diagnostics) <= index {
		t.Fatalf("diagnostics = %#v, missing index %d", diagnostics, index)
	}
	diagnostic := diagnostics[index]
	if diagnostic.Code != code {
		t.Errorf("diagnostic code = %q, want %q", diagnostic.Code, code)
	}
	if diagnostic.Severity != severity {
		t.Errorf("diagnostic severity = %q, want %q", diagnostic.Severity, severity)
	}
	if diagnostic.Location == nil || *diagnostic.Location != location {
		t.Errorf("diagnostic location = %#v, want %#v", diagnostic.Location, location)
	}
}
