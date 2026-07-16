// Package nativeverify runs declared native verification commands.
package nativeverify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	MaxOutputBytesPerStream = 32 * 1024
	validatorTimeout        = 30 * time.Second
	validatorWaitDelay      = time.Second

	invalidCheckCode                = "NATIVE_VERIFY_INVALID_CHECK"
	isolationUnavailableCode        = "NATIVE_VERIFY_ISOLATION_UNAVAILABLE"
	outputRootUnavailableCode       = "NATIVE_VERIFY_OUTPUT_ROOT_UNAVAILABLE"
	workingDirectoryEscapeCode      = "NATIVE_VERIFY_WORKING_DIRECTORY_ESCAPE"
	workingDirectoryUnavailableCode = "NATIVE_VERIFY_WORKING_DIRECTORY_UNAVAILABLE"
	toolUnavailableCode             = "NATIVE_VERIFY_TOOL_UNAVAILABLE"
	startFailedCode                 = "NATIVE_VERIFY_START_FAILED"
	timeoutCode                     = "NATIVE_VERIFY_TIMEOUT"
	failedCode                      = "NATIVE_VERIFY_FAILED"
	outputTruncatedCode             = "NATIVE_VERIFY_OUTPUT_TRUNCATED"
)

type Result struct {
	Success     bool
	Diagnostics []model.Diagnostic
}

// RunNativeChecks runs each declared check sequentially beneath outputRoot.
func RunNativeChecks(checks []model.NativeCheck, outputRoot string) Result {
	return runNativeChecks(checks, outputRoot, validatorTimeout)
}

func runNativeChecks(checks []model.NativeCheck, outputRoot string, timeout time.Duration) Result {
	if len(checks) == 0 {
		return Result{Success: true}
	}

	if _, err := resolveDirectory(outputRoot); err != nil {
		return Result{
			Diagnostics: []model.Diagnostic{diagnostic(
				outputRootUnavailableCode,
				model.SeverityError,
				checks[0],
				fmt.Sprintf("generated output root %q is unavailable: %v", outputRoot, err),
			)},
		}
	}

	environment, cleanup, err := isolatedEnvironment()
	if err != nil {
		return Result{
			Diagnostics: []model.Diagnostic{diagnostic(
				isolationUnavailableCode,
				model.SeverityError,
				checks[0],
				fmt.Sprintf("native verification isolation is unavailable: %v", err),
			)},
		}
	}
	defer cleanup()

	result := Result{Success: true}
	for _, check := range checks {
		if err := validateCheck(check); err != nil {
			result.add(diagnostic(invalidCheckCode, model.SeverityError, check, err.Error()))
			continue
		}

		resolvedRoot, err := resolveDirectory(outputRoot)
		if err != nil {
			result.add(diagnostic(
				outputRootUnavailableCode,
				model.SeverityError,
				check,
				fmt.Sprintf("generated output root %q is unavailable: %v", outputRoot, err),
			))
			continue
		}

		workingDirectory := outputRoot
		if check.WorkingDirectory != nil {
			workingDirectory = filepath.Join(outputRoot, string(*check.WorkingDirectory))
		}
		resolvedWorkingDirectory, err := resolveDirectory(workingDirectory)
		if err != nil {
			result.add(diagnostic(
				workingDirectoryUnavailableCode,
				model.SeverityError,
				check,
				fmt.Sprintf("working directory %q is unavailable: %v", workingDirectory, err),
			))
			continue
		}
		if !isWithin(resolvedRoot, resolvedWorkingDirectory) {
			result.add(diagnostic(
				workingDirectoryEscapeCode,
				model.SeverityError,
				check,
				fmt.Sprintf("working directory %q resolves outside generated output root", workingDirectory),
			))
			continue
		}

		program, err := exec.LookPath(check.Program)
		if err != nil {
			result.add(diagnostic(
				toolUnavailableCode,
				model.SeverityError,
				check,
				fmt.Sprintf("native verification tool %q is unavailable: %v", check.Program, err),
			))
			continue
		}
		if !filepath.IsAbs(program) {
			program, err = filepath.Abs(program)
			if err != nil {
				result.add(diagnostic(
					toolUnavailableCode,
					model.SeverityError,
					check,
					fmt.Sprintf("native verification tool %q is unavailable: %v", check.Program, err),
				))
				continue
			}
		}

		stdout := &boundedOutput{}
		stderr := &boundedOutput{}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		command := exec.CommandContext(ctx, program, check.Arguments...)
		command.WaitDelay = validatorWaitDelay
		command.Dir = resolvedWorkingDirectory
		command.Env = environment
		command.Stdin = strings.NewReader("")
		command.Stdout = stdout
		command.Stderr = stderr
		if err := command.Start(); err != nil {
			timedOut := ctx.Err() == context.DeadlineExceeded
			cancel()
			if timedOut {
				result.add(timeoutDiagnostic(check, timeout, stdout, stderr))
			} else {
				result.add(diagnostic(
					startFailedCode,
					model.SeverityError,
					check,
					fmt.Sprintf("native verification command %q could not start: %v", check.Program, err),
				))
			}
			continue
		}
		waitErr := command.Wait()
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			result.add(timeoutDiagnostic(check, timeout, stdout, stderr))
		} else if waitErr != nil {
			result.add(diagnostic(
				failedCode,
				model.SeverityError,
				check,
				fmt.Sprintf("native verification command %q failed: %v\nstdout:\n%s\nstderr:\n%s", check.Program, waitErr, stdout.evidence(), stderr.evidence()),
			))
		}
		if stdout.truncated() || stderr.truncated() {
			result.Diagnostics = append(result.Diagnostics, diagnostic(
				outputTruncatedCode,
				model.SeverityWarning,
				check,
				truncationMessage(stdout.truncated(), stderr.truncated()),
			))
		}
	}
	return result
}

func (result *Result) add(diagnostic model.Diagnostic) {
	result.Success = false
	result.Diagnostics = append(result.Diagnostics, diagnostic)
}

func validateCheck(check model.NativeCheck) error {
	if strings.TrimSpace(check.Program) == "" || strings.IndexByte(check.Program, 0) >= 0 {
		return fmt.Errorf("native verification program is invalid")
	}
	for _, argument := range check.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return fmt.Errorf("native verification argument is invalid")
		}
	}
	return nil
}

func isolatedEnvironment() ([]string, func(), error) {
	root, err := os.MkdirTemp("", "agbun-native-verify-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }

	home := filepath.Join(root, "home")
	config := filepath.Join(root, "config")
	cache := filepath.Join(root, "cache")
	temporary := filepath.Join(root, "tmp")
	for _, directory := range []string{home, config, cache, temporary} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			cleanup()
			return nil, func() {}, err
		}
	}

	environment := []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + temporary,
		"XDG_CACHE_HOME=" + cache,
		"XDG_CONFIG_HOME=" + config,
	}
	if runtime.GOOS == "windows" {
		environment = append(environment,
			"APPDATA="+config,
			"LOCALAPPDATA="+cache,
			"TEMP="+temporary,
			"TMP="+temporary,
			"USERPROFILE="+home,
		)
		for _, name := range []string{"COMSPEC", "PATHEXT", "SYSTEMROOT", "WINDIR"} {
			if value, ok := os.LookupEnv(name); ok {
				environment = append(environment, name+"="+value)
			}
		}
	}
	return environment, cleanup, nil
}

func resolveDirectory(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return resolved, nil
}

func isWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func diagnostic(code string, severity model.Severity, check model.NativeCheck, message string) model.Diagnostic {
	location := check.Location
	return model.Diagnostic{
		Code:     code,
		Severity: severity,
		Location: &location,
		Message:  message,
	}
}

func timeoutDiagnostic(check model.NativeCheck, timeout time.Duration, stdout, stderr *boundedOutput) model.Diagnostic {
	return diagnostic(
		timeoutCode,
		model.SeverityError,
		check,
		fmt.Sprintf("native verification command %q timed out after %s\nstdout:\n%s\nstderr:\n%s", check.Program, timeout, stdout.evidence(), stderr.evidence()),
	)
}

func truncationMessage(stdout, stderr bool) string {
	switch {
	case stdout && stderr:
		return "native verification stdout and stderr were truncated after 32768 bytes"
	case stdout:
		return "native verification stdout was truncated after 32768 bytes"
	default:
		return "native verification stderr was truncated after 32768 bytes"
	}
}

type boundedOutput struct {
	mu       sync.Mutex
	data     []byte
	overflow bool
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()

	remaining := MaxOutputBytesPerStream - len(output.data)
	if remaining <= 0 {
		output.overflow = output.overflow || len(data) > 0
		return len(data), nil
	}
	if len(data) > remaining {
		output.data = append(output.data, data[:remaining]...)
		output.overflow = true
		return len(data), nil
	}
	output.data = append(output.data, data...)
	return len(data), nil
}

func (output *boundedOutput) truncated() bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.overflow
}

func (output *boundedOutput) evidence() string {
	output.mu.Lock()
	defer output.mu.Unlock()

	evidence := renderBytes(output.data)
	if output.overflow {
		evidence += "[truncated after 32768 bytes]"
	}
	return evidence
}

func renderBytes(data []byte) string {
	var rendered strings.Builder
	for len(data) > 0 {
		_, size := utf8.DecodeRune(data)
		if size == 1 && data[0] >= utf8.RuneSelf {
			fmt.Fprintf(&rendered, "\\x%02X", data[0])
			data = data[1:]
			continue
		}
		rendered.Write(data[:size])
		data = data[size:]
	}
	return rendered.String()
}
