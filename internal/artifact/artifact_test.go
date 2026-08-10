package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const testSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// validGuard returns a WorkspaceLayoutGuard suitable for tests that exercise
// plan validation, write, compare, or archive behavior without testing the
// guard itself.
func validGuard(t *testing.T) WorkspaceLayoutGuard {
	t.Helper()
	ws := t.TempDir()
	source := filepath.Join(ws, "src")
	output := filepath.Join(ws, "out")
	guard, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err != nil {
		t.Fatalf("NewWorkspaceLayoutGuard() = %v", err)
	}
	return guard
}

func guardForOutput(t *testing.T, output string) WorkspaceLayoutGuard {
	t.Helper()
	ws := t.TempDir()
	source := filepath.Join(ws, "src")
	guard, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err != nil {
		t.Fatalf("NewWorkspaceLayoutGuard() = %v", err)
	}
	return guard
}

func TestWriteAndCompareSharePlanValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		plan model.BuildPlan
	}{
		{
			name: "absolute path",
			plan: planWithFile("/absolute"),
		},
		{
			name: "escaping path",
			plan: planWithFile("../escape"),
		},
		{
			name: "case-fold collision",
			plan: model.BuildPlan{Targets: []model.TargetPlan{{
				Target: model.TargetClaude,
				Files: []model.PlannedFile{
					{Path: "Readme.md"},
					{Path: "README.md"},
				},
			}}},
		},
		{
			name: "reserved platform name",
			plan: planWithFile("CON.txt"),
		},
		{
			name: "case-insensitive target ownership conflict",
			plan: model.BuildPlan{Targets: []model.TargetPlan{{
				Target: model.TargetClaude,
				Files:  []model.PlannedFile{{Path: "FOO/bar"}, {Path: "foo"}},
			}}},
		},
		{
			name: "target root ownership conflict",
			plan: model.BuildPlan{
				Targets: []model.TargetPlan{{
					Target: model.TargetClaude,
					Files:  []model.PlannedFile{{Path: "skill.md"}},
				}},
				CompilerFiles: []model.PlannedFile{{Path: "claude"}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard := validGuard(t)
			outputRoot := guard.outputPath
			writeDiagnostics := Write(guard, test.plan, outputRoot)
			compareDiagnostics, drift := Compare(guard, test.plan, outputRoot)
			if drift {
				t.Fatal("Compare() reported drift for an invalid plan")
			}
			if len(writeDiagnostics) == 0 {
				t.Fatal("Write() accepted invalid plan")
			}
			if !reflect.DeepEqual(writeDiagnostics, compareDiagnostics) {
				t.Fatalf("Write() diagnostics = %#v, Compare() diagnostics = %#v", writeDiagnostics, compareDiagnostics)
			}
			for _, diagnostic := range writeDiagnostics {
				if diagnostic.Code != diagnosticInvalidPlan {
					t.Errorf("diagnostic code = %q, want %q", diagnostic.Code, diagnosticInvalidPlan)
				}
			}
		})
	}
}

func TestDestinationConflictsFindsNonAdjacentAncestor(t *testing.T) {
	diagnostics := destinationConflicts([]string{"a", "a-b", "a/c"})
	want := `planned destination "a" prevents owning target path "a/c"`
	for _, diagnostic := range diagnostics {
		if diagnostic.Message == want {
			return
		}
	}
	t.Fatalf("destinationConflicts() diagnostics = %#v; want %q", diagnostics, want)
}

func TestDestinationConflictsRejectsCrossNamespaceDuplicate(t *testing.T) {
	plan := model.BuildPlan{
		Targets: []model.TargetPlan{{
			Target: model.TargetClaude,
			Files:  []model.PlannedFile{{Path: "x.md"}},
		}},
		CompilerFiles: []model.PlannedFile{{Path: "claude/x.md"}},
	}
	diagnostics := destinationConflicts(planDestinations(plan))
	want := `planned destination "claude/x.md" is duplicated`
	if len(diagnostics) != 1 || diagnostics[0].Message != want {
		t.Fatalf("destinationConflicts() diagnostics = %#v; want %q", diagnostics, want)
	}
}

func TestWriteAndCompareRejectUnboundOutputRoot(t *testing.T) {
	guard := validGuard(t)
	outputRoot := filepath.Join(t.TempDir(), "output")
	writeDiagnostics := Write(guard, planWithFile("skill.md"), outputRoot)
	if len(writeDiagnostics) != 1 || writeDiagnostics[0].Code != diagnosticLayoutGuard {
		t.Fatalf("Write() diagnostics = %#v", writeDiagnostics)
	}
	compareDiagnostics, drift := Compare(guard, planWithFile("skill.md"), outputRoot)
	if drift || len(compareDiagnostics) != 1 || compareDiagnostics[0].Code != diagnosticLayoutGuard {
		t.Fatalf("Compare() = (%#v, %t)", compareDiagnostics, drift)
	}
	if _, err := os.Stat(outputRoot); !os.IsNotExist(err) {
		t.Fatalf("unbound output was mutated: %v", err)
	}
}

func TestWritePreservesCurrentOutputIdentity(t *testing.T) {
	guard := validGuard(t)
	outputRoot := guard.outputPath
	plan := planWithFile("skill.md")
	if diagnostics := Write(guard, plan, outputRoot); len(diagnostics) != 0 {
		t.Fatalf("first Write() diagnostics = %#v", diagnostics)
	}
	path := filepath.Join(outputRoot, "claude", "skill.md")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before second Write() = %v", err)
	}

	if diagnostics := Write(guard, plan, outputRoot); len(diagnostics) != 0 {
		t.Fatalf("second Write() diagnostics = %#v", diagnostics)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after second Write() = %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("second Write() replaced a current output file")
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("second Write() changed file timestamp: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestCompareMapsExactDrift(t *testing.T) {
	outputRoot := t.TempDir()
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetClaude,
		Files: []model.PlannedFile{
			{Path: "changed.txt", Bytes: []byte("expected")},
			{Path: "missing.txt", Bytes: []byte("missing")},
		},
	}}}
	writeArtifactFile(t, outputRoot, "claude/changed.txt", "actual")
	writeArtifactFile(t, outputRoot, "extra.txt", "extra")

	diagnostics, drift := Compare(guardForOutput(t, outputRoot), plan, outputRoot)
	if !drift {
		t.Fatal("Compare() drift = false, want true")
	}
	want := []model.Diagnostic{
		{Code: diagnosticDriftChanged, Severity: model.SeverityError, Message: "generated output changed: claude/changed.txt"},
		{Code: diagnosticDriftMissing, Severity: model.SeverityError, Message: "generated output missing: claude/missing.txt"},
		{Code: diagnosticDriftExtra, Severity: model.SeverityError, Message: "generated output extra: extra.txt"},
	}
	if !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("Compare() diagnostics = %#v, want %#v", diagnostics, want)
	}
}

func TestCompareReportsObservationFailureSeparatelyFromDrift(t *testing.T) {
	guard := validGuard(t)
	diagnostics, drift := Compare(guard, planWithFile("skill.md"), guard.outputPath)
	if drift || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticDriftObservation {
		t.Fatalf("Compare() = (%#v, %t)", diagnostics, drift)
	}
}

func TestWriteAndCompareRejectExecutableIntentOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific executable-intent contract")
	}
	plan := model.BuildPlan{CompilerFiles: []model.PlannedFile{{Path: "tool", Bytes: []byte("tool"), Executable: true}}}
	guard := validGuard(t)
	outputRoot := guard.outputPath
	writeDiagnostics := Write(guard, plan, outputRoot)
	compareDiagnostics, drift := Compare(guard, plan, outputRoot)
	if drift {
		t.Fatal("Compare() reported drift instead of rejecting the plan")
	}
	if !reflect.DeepEqual(writeDiagnostics, compareDiagnostics) || len(writeDiagnostics) != 1 {
		t.Fatalf("Write() diagnostics = %#v, Compare() diagnostics = %#v", writeDiagnostics, compareDiagnostics)
	}
	if diagnostic := writeDiagnostics[0]; diagnostic.Code != diagnosticExecutable || diagnostic.Message != "executable file intent is unsupported on Windows" {
		t.Fatalf("executable-intent diagnostic = %#v", diagnostic)
	}
}

func TestProvenanceAdaptsInputAndMapsFailures(t *testing.T) {
	plan := planWithFile("skill.md")
	input := validProvenanceInput()
	beforePlan := plan
	beforeInput := input

	result, diagnostics := Provenance(plan, input)
	if len(diagnostics) != 0 {
		t.Fatalf("Provenance() diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(plan, beforePlan) || !reflect.DeepEqual(input, beforeInput) {
		t.Fatal("Provenance() mutated its input")
	}
	if len(result.CompilerFiles) != 1 || result.CompilerFiles[0].Path != ".agentbundler/build.json" {
		t.Fatalf("Provenance() compiler files = %#v", result.CompilerFiles)
	}

	var document struct {
		Acknowledgments []struct {
			Asset  string `json:"asset"`
			Target string `json:"target"`
			Key    string `json:"key"`
			Reason string `json:"reason"`
		} `json:"acknowledgments"`
	}
	if err := json.Unmarshal(result.CompilerFiles[0].Bytes, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := document.Acknowledgments[0], (struct {
		Asset  string `json:"asset"`
		Target string `json:"target"`
		Key    string `json:"key"`
		Reason string `json:"reason"`
	}{Asset: "skill/example", Target: "claude", Key: "advisory", Reason: "accepted"}); got != want {
		t.Fatalf("provenance acknowledgment = %#v, want %#v", got, want)
	}

	input.Configuration = []byte("[]")
	unchanged, diagnostics := Provenance(plan, input)
	if !reflect.DeepEqual(unchanged, plan) {
		t.Fatalf("Provenance() result = %#v, want unchanged plan %#v", unchanged, plan)
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != diagnosticProvenance || diagnostics[0].Severity != model.SeverityError {
		t.Fatalf("Provenance() diagnostics = %#v", diagnostics)
	}
}

func TestVerifyRejectsProgramPaths(t *testing.T) {
	for _, program := range []string{"/tmp/untrusted-tool", `relative\\tool`} {
		t.Run(program, func(t *testing.T) {
			result := Verify([]model.NativeCheck{{Program: program, Location: model.SourceLocation{Path: "native-check"}}}, t.TempDir())
			if result.Success || len(result.Diagnostics) == 0 || result.Diagnostics[0].Severity != model.SeverityError {
				t.Fatalf("Verify() result = %#v", result)
			}
		})
	}
}

func TestArtifactWorkflowWritesComparesThenVerifies(t *testing.T) {
	guard := validGuard(t)
	plan, diagnostics := Provenance(planWithFile("skill.md"), validProvenanceInput())
	if len(diagnostics) != 0 {
		t.Fatalf("Provenance() diagnostics = %#v", diagnostics)
	}
	outputRoot := guard.outputPath
	if diagnostics := Write(guard, plan, outputRoot); len(diagnostics) != 0 {
		t.Fatalf("Write() diagnostics = %#v", diagnostics)
	}
	if diagnostics, drift := Compare(guard, plan, outputRoot); len(diagnostics) != 0 || drift {
		t.Fatalf("Compare() = (%#v, %t)", diagnostics, drift)
	}

	verification := Verify([]model.NativeCheck{{
		Program:   "go",
		Arguments: []string{"version"},
		Location:  model.SourceLocation{Path: "native-check"},
	}}, outputRoot)
	if !verification.Success || len(verification.Diagnostics) != 0 {
		t.Fatalf("Verify() result = %#v", verification)
	}
}

func planWithFile(path model.RelativePath) model.BuildPlan {
	return model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetClaude,
		Files:  []model.PlannedFile{{Path: path, Bytes: []byte("content")}},
	}}}
}

func validProvenanceInput() ProvenanceInput {
	return ProvenanceInput{
		CompilerVersion: "v1",
		Configuration:   []byte(`{"enabled":true}`),
		Inputs: []ProvenanceInputFile{{
			Path:   "source/input",
			SHA256: testSHA256,
		}},
		Acknowledgments: []ProvenanceAcknowledgment{{
			Asset:  "skill/example",
			Target: model.TargetClaude,
			Key:    "advisory",
			Reason: "accepted",
		}},
		AdapterRevisions: []AdapterRevision{{Target: model.TargetClaude, Revision: 1}},
	}
}

func writeArtifactFile(t *testing.T, root, relativePath, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRejectsInvalidGuard(t *testing.T) {
	var guard WorkspaceLayoutGuard
	outputRoot := filepath.Join(t.TempDir(), "output")
	diagnostics := Write(guard, planWithFile("skill.md"), outputRoot)
	if len(diagnostics) != 1 || diagnostics[0].Code != diagnosticLayoutGuard || diagnostics[0].Severity != model.SeverityError {
		t.Fatalf("Write() with invalid guard = %#v", diagnostics)
	}
	if _, err := os.Stat(outputRoot); !os.IsNotExist(err) {
		t.Fatal("Write() mutated the filesystem despite an invalid guard")
	}
}

func TestCompareRejectsInvalidGuard(t *testing.T) {
	var guard WorkspaceLayoutGuard
	diagnostics, drift := Compare(guard, planWithFile("skill.md"), t.TempDir())
	if drift || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticLayoutGuard || diagnostics[0].Severity != model.SeverityError {
		t.Fatalf("Compare() with invalid guard = (%#v, %v)", diagnostics, drift)
	}
}

func TestArchiveRejectsDestinationInsideSourceBeforeMutation(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "source")
	output := filepath.Join(workspace, "generated")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := NewWorkspaceLayoutGuard(workspace, source, output)
	if err != nil {
		t.Fatal(err)
	}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target:       model.TargetClaude,
		Files:        []model.PlannedFile{{Path: "skill.md", Bytes: []byte("skill")}},
		ArchiveUnits: []model.ArchiveUnit{{Root: ".", Stem: "claude", Suffix: ".tar.gz"}},
	}}}
	paths, diagnostics := Archive(guard, model.DistributionMetadata{"name": "demo"}, plan, source)
	if paths != nil || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticLayoutGuard {
		t.Fatalf("Archive() = (%v, %#v); want layout rejection", paths, diagnostics)
	}
	if _, err := os.Stat(filepath.Join(source, "demo-claude.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("Archive() mutated source: %v", err)
	}
}

func TestArchiveRejectsInvalidGuard(t *testing.T) {
	var guard WorkspaceLayoutGuard
	paths, diagnostics := Archive(guard, model.DistributionMetadata{}, model.BuildPlan{}, t.TempDir())
	if paths != nil || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticLayoutGuard || diagnostics[0].Severity != model.SeverityError {
		t.Fatalf("Archive() with invalid guard = (%v, %#v)", paths, diagnostics)
	}
}
