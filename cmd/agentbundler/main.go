package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

type compileFunc func(compiler.CompileRequest) compiler.CompilationResult

type options struct {
	command    string
	root       string
	targets    []model.TargetID
	packages   []model.PackageID
	jsonOutput bool
	native     bool
}

type jsonResult struct {
	Version                  int              `json:"version"`
	Command                  string           `json:"command"`
	Diagnostics              []jsonDiagnostic `json:"diagnostics"`
	Drift                    bool             `json:"drift"`
	NativeVerificationFailed bool             `json:"nativeVerificationFailed"`
}

type jsonDiagnostic struct {
	Code     string                `json:"code"`
	Severity model.Severity        `json:"severity"`
	Location *model.SourceLocation `json:"location"`
	Message  string                `json:"message"`
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], cwd, os.Stdout, os.Stderr, compiler.Compile))
}

func run(args []string, workingDirectory string, stdout io.Writer, stderr io.Writer, compile compileFunc) int {
	if help, ok, err := helpText(args); ok {
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "USAGE: %s\n", err)
			return 1
		}
		_, _ = fmt.Fprint(stdout, help)
		return 0
	}
	parsed, err := parseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "USAGE: %s\n", err)
		return 1
	}
	manifestPath, manifestDirectory, err := locateManifest(parsed.root, workingDirectory)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "MANIFEST_NOT_FOUND: %s\n", err)
		return 1
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return renderFailure(parsed, stderr, stdout, model.Diagnostic{Code: "MANIFEST_READ_FAILED", Severity: model.SeverityError, Message: err.Error()})
	}
	manifest, diagnostics := model.DecodeSourceManifestJSON(data)
	if len(diagnostics) != 0 {
		return renderFailure(parsed, stderr, stdout, diagnostics...)
	}
	result := compile(compiler.CompileRequest{
		WorkspaceRoot: manifestDirectory,
		Manifest:      manifest,
		Targets:       parsed.targets,
		Packages:      parsed.packages,
		Mode:          compiler.BuildMode(parsed.command),
		NativeVerify:  parsed.native,
	})
	return renderResult(parsed, stdout, stderr, result)
}

func parseArgs(args []string) (options, error) {
	if len(args) == 0 || (args[0] != "build" && args[0] != "check") {
		return options{}, errors.New("expected build or check")
	}
	result := options{command: args[0]}
	seenRoot := false
	seenTargets := map[string]bool{}
	seenPackages := map[string]bool{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--json":
			if result.jsonOutput {
				return options{}, errors.New("--json may not be repeated")
			}
			result.jsonOutput = true
		case "--native":
			if result.command != "check" {
				return options{}, errors.New("--native is valid only for check")
			}
			if result.native {
				return options{}, errors.New("--native may not be repeated")
			}
			result.native = true
		case "--root", "--target", "--package":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options{}, fmt.Errorf("%s requires a value", args[i])
			}
			value := args[i+1]
			i++
			switch args[i-1] {
			case "--root":
				if seenRoot {
					return options{}, errors.New("--root may not be repeated")
				}
				seenRoot = true
				result.root = value
			case "--target":
				if seenTargets[value] {
					return options{}, fmt.Errorf("target %q is repeated", value)
				}
				seenTargets[value] = true
				result.targets = append(result.targets, model.TargetID(value))
			case "--package":
				if seenPackages[value] {
					return options{}, fmt.Errorf("package %q is repeated", value)
				}
				seenPackages[value] = true
				result.packages = append(result.packages, model.PackageID(value))
			}
		default:
			return options{}, fmt.Errorf("unknown flag or positional argument %q", args[i])
		}
	}
	return result, nil
}

func locateManifest(root, cwd string) (string, string, error) {
	if root != "" {
		candidate := root
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(cwd, candidate)
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return "", "", err
		}
		absolute = filepath.Clean(absolute)
		info, err := os.Stat(absolute)
		if err != nil || !info.IsDir() {
			return "", "", fmt.Errorf("root is not a directory")
		}
		path := filepath.Join(absolute, "agentbundle.json")
		if _, err := os.Stat(path); err != nil {
			return "", "", err
		}
		return path, absolute, nil
	}
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", err
	}
	for {
		path := filepath.Join(current, "agentbundle.json")
		if _, err := os.Stat(path); err == nil {
			return path, current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", "", os.ErrNotExist
}

func renderFailure(parsed options, stderr, stdout io.Writer, diagnostics ...model.Diagnostic) int {
	return renderResult(parsed, stdout, stderr, compiler.CompilationResult{Diagnostics: diagnostics})
}

func renderResult(parsed options, stdout, stderr io.Writer, result compiler.CompilationResult) int {
	if parsed.jsonOutput {
		output := jsonResult{Version: 1, Command: parsed.command, Diagnostics: make([]jsonDiagnostic, len(result.Diagnostics)), Drift: result.Drift, NativeVerificationFailed: result.NativeVerificationFailed}
		for i, diagnostic := range result.Diagnostics {
			output.Diagnostics[i] = jsonDiagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Location: diagnostic.Location, Message: diagnostic.Message}
		}
		if err := json.NewEncoder(stdout).Encode(output); err != nil {
			_, _ = fmt.Fprintf(stderr, "OUTPUT_WRITE_FAILED: %v\n", err)
			return 1
		}
	} else {
		for _, diagnostic := range result.Diagnostics {
			_, _ = fmt.Fprintln(stderr, formatDiagnostic(diagnostic))
		}
		if !hasError(result.Diagnostics) && !result.Drift && !result.NativeVerificationFailed {
			if parsed.command == "build" {
				_, _ = fmt.Fprintln(stdout, "build: ok")
			} else {
				_, _ = fmt.Fprintln(stdout, "check: current")
			}
		}
	}
	if result.NativeVerificationFailed {
		return 3
	}
	if result.Drift {
		return 2
	}
	if hasError(result.Diagnostics) {
		return 1
	}
	return 0
}

func formatDiagnostic(diagnostic model.Diagnostic) string {
	location := ""
	if diagnostic.Location != nil {
		location = string(diagnostic.Location.Path)
		if diagnostic.Location.Line != nil {
			location += fmt.Sprintf(":%d", *diagnostic.Location.Line)
		}
		if diagnostic.Location.Column != nil {
			location += fmt.Sprintf(":%d", *diagnostic.Location.Column)
		}
		location += ": "
	}
	return fmt.Sprintf("%s%s[%s]: %s", location, diagnostic.Severity, diagnostic.Code, diagnostic.Message)
}

func hasError(diagnostics []model.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == model.SeverityError {
			return true
		}
	}
	return false
}

func helpText(args []string) (string, bool, error) {
	if len(args) == 0 {
		return "", false, nil
	}
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h", "help":
			return usage(), true, nil
		}
	}
	if len(args) == 2 && args[0] == "help" {
		switch args[1] {
		case "-h", "--help":
			return usage(), true, nil
		case "build":
			return buildHelp(), true, nil
		case "check":
			return checkHelp(), true, nil
		default:
			return "", true, fmt.Errorf("unknown help topic %q", args[1])
		}
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		switch args[0] {
		case "build":
			return buildHelp(), true, nil
		case "check":
			return checkHelp(), true, nil
		}
	}
	return "", false, nil
}

func usage() string {
	return `agentbundler compiles one source bundle into target-specific coding-agent layouts.

Usage:
  agentbundler <command> [options]

Commands:
  build    Compile and replace the configured output directory.
  check    Compare the configured output directory with the current build plan.
  help     Show help for a command: agentbundler help build|check.

Global help:
  agentbundler --help
  agentbundler help
  agentbundler help <command>

Run "agentbundler help <command>" for command-specific options.
`
}

func buildHelp() string {
	return `Usage:
  agentbundler build [options]

Compile the selected package and targets, then replace the complete output
directory configured by agentbundle.json. Use a dedicated generated directory;
build removes files that are not in the current build plan.

Options:
  --root DIR       Read agentbundle.json from DIR instead of searching the
                   current directory and its parents.
  --target TARGET  Build one declared target. Repeat for multiple targets.
  --package ID     Build one imported package. Repeat for multiple packages.
  --json           Write one machine-readable result object to stdout.
  -h, --help       Show this help.

Targets:
  claude, codex, pi, copilot, grok, cursor

Examples:
  agentbundler build
  agentbundler build --root ./plugin --target pi
  agentbundler build --target codex --package team-skills --json
`
}

func checkHelp() string {
	return `Usage:
  agentbundler check [options]

Compare the selected build plan with the configured output directory. check does
not write files and exits 2 when output is missing, changed, extra, non-regular,
or symlinked.

Options:
  --root DIR       Read agentbundle.json from DIR instead of searching the
                   current directory and its parents.
  --target TARGET  Check one declared target. Repeat for multiple targets.
  --package ID     Check one imported package. Repeat for multiple packages.
  --native         Run declared native checks after output comparison.
  --json           Write one machine-readable result object to stdout.
  -h, --help       Show this help.

Targets:
  claude, codex, pi, copilot, grok, cursor

Exit statuses:
  0  Output is current.
  1  Source, validation, capability, render, or write failure.
  2  Output drift.
  3  Native verification failure.

Examples:
  agentbundler check
  agentbundler check --root ./plugin --target pi
  agentbundler check --target codex --package team-skills --native --json
`
}
