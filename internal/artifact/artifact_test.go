package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const testSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
			outputRoot := filepath.Join(t.TempDir(), "output")
			writeDiagnostics := Write(test.plan, outputRoot)
			compareDiagnostics, drift := Compare(test.plan, outputRoot)
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

	diagnostics, drift := Compare(plan, outputRoot)
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
	diagnostics, drift := Compare(planWithFile("skill.md"), filepath.Join(t.TempDir(), "missing"))
	if drift || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticDriftObservation {
		t.Fatalf("Compare() = (%#v, %t)", diagnostics, drift)
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
	plan, diagnostics := Provenance(planWithFile("skill.md"), validProvenanceInput())
	if len(diagnostics) != 0 {
		t.Fatalf("Provenance() diagnostics = %#v", diagnostics)
	}
	outputRoot := filepath.Join(t.TempDir(), "output")
	if diagnostics := Write(plan, outputRoot); len(diagnostics) != 0 {
		t.Fatalf("Write() diagnostics = %#v", diagnostics)
	}
	if diagnostics, drift := Compare(plan, outputRoot); len(diagnostics) != 0 || drift {
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
