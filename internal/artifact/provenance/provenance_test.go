package provenance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestAppendCanonicalSchemaAndOrdering(t *testing.T) {
	plan := model.BuildPlan{Targets: []model.TargetPlan{
		{
			Target: model.TargetPi,
			Files: []model.PlannedFile{
				{Path: "zeta", Bytes: []byte("pi-zeta")},
				{Path: "alpha", Bytes: []byte("pi-alpha"), Executable: true},
			},
		},
		{
			Target: model.TargetClaude,
			Files:  []model.PlannedFile{{Path: "skill.md", Bytes: []byte("claude-skill")}},
		},
	}}
	input := Input{
		CompilerVersion: "v1.2.3",
		Configuration: json.RawMessage(`{
  "z" : [ 1, 2 ], "a" : true
}`),
		Inputs: []InputFile{
			{Path: "source/z", SHA256: shaB},
			{Path: "source/a", SHA256: shaA},
		},
		Acknowledgments: []Acknowledgment{
			{Asset: "skill/z", Target: model.TargetPi, Key: "z-key", Reason: "z reason"},
			{Asset: "skill/a", Target: model.TargetClaude, Key: "a-key", Reason: "a reason"},
		},
		AdapterRevisions: []AdapterRevision{
			{Target: model.TargetPi, Revision: 2},
			{Target: model.TargetClaude, Revision: 1},
		},
	}

	first, err := Append(plan, input)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	shuffledPlan := plan
	shuffledPlan.Targets = []model.TargetPlan{
		{
			Target: model.TargetClaude,
			Files:  []model.PlannedFile{{Path: "skill.md", Bytes: []byte("claude-skill")}},
		},
		{
			Target: model.TargetPi,
			Files: []model.PlannedFile{
				{Path: "alpha", Bytes: []byte("pi-alpha"), Executable: true},
				{Path: "zeta", Bytes: []byte("pi-zeta")},
			},
		},
	}
	shuffledInput := input
	shuffledInput.Configuration = json.RawMessage(`{"z":[1,2],"a":true}`)
	shuffledInput.Inputs = []InputFile{input.Inputs[1], input.Inputs[0]}
	shuffledInput.Acknowledgments = []Acknowledgment{input.Acknowledgments[1], input.Acknowledgments[0]}
	shuffledInput.AdapterRevisions = []AdapterRevision{input.AdapterRevisions[1], input.AdapterRevisions[0]}
	second, err := Append(shuffledPlan, shuffledInput)
	if err != nil {
		t.Fatalf("Append(shuffled) error = %v", err)
	}

	firstBytes := provenanceFile(t, first)
	const want = `{"schemaVersion":1,"compiler":{"version":"v1.2.3"},"configuration":{"sha256":"993f387914853add224d10bf9d0695a565c916e59d3551708cd7f43e6d000d6b"},"inputs":[{"path":"source/a","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"path":"source/z","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"acknowledgments":[{"asset":"skill/a","target":"claude","key":"a-key","reason":"a reason"},{"asset":"skill/z","target":"pi","key":"z-key","reason":"z reason"}],"outputs":[{"target":"claude","adapterRevision":1,"files":[{"path":"skill.md","sha256":"e90e2a4d7e7577fa26cde21348607ad863ba055348cbc9c516ac04e5c685deda","executable":false}]},{"target":"pi","adapterRevision":2,"files":[{"path":"alpha","sha256":"71e5fecc1171077e5a548c59095a52d2f01ba745411b1f757a75ab39e49ce186","executable":true},{"path":"zeta","sha256":"9d728b2e4eac338fd4eb185a4b6ceac7781d907003e45c7adcd31cb56907e698","executable":false}]}]}`
	if got := string(firstBytes); got != want {
		t.Fatalf("provenance = %s\nwant = %s", got, want)
	}
	if !bytes.Equal(firstBytes, provenanceFile(t, second)) {
		t.Fatalf("provenance bytes differ:\nfirst:  %s\nsecond: %s", firstBytes, provenanceFile(t, second))
	}

	var document struct {
		Configuration struct {
			SHA256 string `json:"sha256"`
		} `json:"configuration"`
		Inputs []struct {
			Path string `json:"path"`
		} `json:"inputs"`
		Acknowledgments []struct {
			Asset string `json:"asset"`
		} `json:"acknowledgments"`
		Outputs []struct {
			Target string `json:"target"`
			Files  []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(firstBytes, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if document.Configuration.SHA256 != "993f387914853add224d10bf9d0695a565c916e59d3551708cd7f43e6d000d6b" {
		t.Fatalf("configuration hash = %q, want fixed compact JSON hash", document.Configuration.SHA256)
	}
	if got := []string{document.Inputs[0].Path, document.Inputs[1].Path}; !reflect.DeepEqual(got, []string{"source/a", "source/z"}) {
		t.Fatalf("input paths = %#v", got)
	}
	if got := []string{document.Acknowledgments[0].Asset, document.Acknowledgments[1].Asset}; !reflect.DeepEqual(got, []string{"skill/a", "skill/z"}) {
		t.Fatalf("acknowledgments = %#v", got)
	}
	if got := []string{document.Outputs[0].Target, document.Outputs[1].Target}; !reflect.DeepEqual(got, []string{"claude", "pi"}) {
		t.Fatalf("outputs = %#v", got)
	}
	if got := []string{document.Outputs[1].Files[0].Path, document.Outputs[1].Files[1].Path}; !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("pi output files = %#v", got)
	}
}

func TestAppendConfigurationMemberOrderChangesHash(t *testing.T) {
	plan := singleTargetPlan()
	first, err := Append(plan, validInput(`{"first":1,"second":2}`))
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := Append(plan, validInput(`{"second":2,"first":1}`))
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if bytes.Equal(provenanceFile(t, first), provenanceFile(t, second)) {
		t.Fatal("provenance bytes match after configuration object member reordering")
	}
}

func TestAppendDoesNotMutateAndRejectsCollisions(t *testing.T) {
	line := 4
	workingDirectory := model.RelativePath("checks")
	plan := model.BuildPlan{
		Targets: []model.TargetPlan{{
			Target:   model.TargetClaude,
			Packages: []model.PackageID{"package"},
			Files: []model.PlannedFile{{
				Path:   "skill.md",
				Bytes:  []byte("original"),
				Origin: []model.SourceLocation{{Path: "source/skill.md", Line: &line}},
			}},
			NativeChecks: []model.NativeCheck{{
				Program:          "check",
				Arguments:        []string{"--strict"},
				WorkingDirectory: &workingDirectory,
				Location:         model.SourceLocation{Path: "checks/check"},
			}},
		}},
		CompilerFiles: []model.PlannedFile{{Path: "compiler.txt", Bytes: []byte("compiler")}},
	}
	input := validInput(`{"enabled":true}`)
	beforePlan, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal(plan) error = %v", err)
	}
	beforeInput, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal(input) error = %v", err)
	}

	result, err := Append(plan, input)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(result.CompilerFiles) != 2 {
		t.Fatalf("compiler files = %d, want 2", len(result.CompilerFiles))
	}
	result.Targets[0].Files[0].Bytes[0] = 'X'
	*result.Targets[0].Files[0].Origin[0].Line = 9
	result.Targets[0].NativeChecks[0].Arguments[0] = "changed"
	*result.Targets[0].NativeChecks[0].WorkingDirectory = "changed"
	result.CompilerFiles[0].Bytes[0] = 'X'

	afterPlan, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal(plan) error = %v", err)
	}
	afterInput, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal(input) error = %v", err)
	}
	if !bytes.Equal(beforePlan, afterPlan) {
		t.Fatalf("Append() mutated plan:\nbefore: %s\nafter:  %s", beforePlan, afterPlan)
	}
	if !bytes.Equal(beforeInput, afterInput) {
		t.Fatalf("Append() mutated input:\nbefore: %s\nafter:  %s", beforeInput, afterInput)
	}

	for _, test := range []struct {
		name string
		plan model.BuildPlan
	}{
		{
			name: "target file",
			plan: model.BuildPlan{Targets: []model.TargetPlan{{
				Target: model.TargetClaude,
				Files:  []model.PlannedFile{{Path: provenancePath}},
			}}},
		},
		{
			name: "compiler file",
			plan: model.BuildPlan{
				Targets:       []model.TargetPlan{{Target: model.TargetClaude}},
				CompilerFiles: []model.PlannedFile{{Path: provenancePath}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Append(test.plan, validInput(`{"enabled":true}`)); err == nil {
				t.Fatal("Append() error = nil, want collision error")
			}
		})
	}
}

func TestAppendRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Input)
	}{
		{name: "empty compiler version", mutate: func(input *Input) { input.CompilerVersion = "" }},
		{name: "empty configuration object", mutate: func(input *Input) { input.Configuration = json.RawMessage(`{}`) }},
		{name: "configuration array", mutate: func(input *Input) { input.Configuration = json.RawMessage(`[]`) }},
		{name: "configuration trailing value", mutate: func(input *Input) { input.Configuration = json.RawMessage(`{"enabled":true} null`) }},
		{name: "configuration duplicate key", mutate: func(input *Input) { input.Configuration = json.RawMessage(`{"enabled":true,"enabled":false}`) }},
		{name: "configuration nested duplicate key", mutate: func(input *Input) { input.Configuration = json.RawMessage(`{"nested":{"key":true,"key":false}}`) }},
		{name: "invalid input path", mutate: func(input *Input) { input.Inputs[0].Path = "../escape" }},
		{name: "invalid input digest", mutate: func(input *Input) { input.Inputs[0].SHA256 = strings.ToUpper(shaA) }},
		{name: "duplicate input path", mutate: func(input *Input) { input.Inputs = append(input.Inputs, input.Inputs[0]) }},
		{name: "empty acknowledgment reason", mutate: func(input *Input) { input.Acknowledgments[0].Reason = "" }},
		{name: "invalid acknowledgment target", mutate: func(input *Input) { input.Acknowledgments[0].Target = "other" }},
		{name: "missing adapter revision", mutate: func(input *Input) { input.AdapterRevisions = nil }},
		{name: "duplicate adapter revision", mutate: func(input *Input) { input.AdapterRevisions = append(input.AdapterRevisions, input.AdapterRevisions[0]) }},
		{name: "extra adapter revision", mutate: func(input *Input) {
			input.AdapterRevisions = append(input.AdapterRevisions, AdapterRevision{Target: model.TargetPi, Revision: 1})
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := validInput(`{"enabled":true}`)
			test.mutate(&input)
			if _, err := Append(singleTargetPlan(), input); err == nil {
				t.Fatal("Append() error = nil, want validation error")
			}
		})
	}
}

func TestAppendExcludesCompilerFilesAndItselfFromOutputEvidence(t *testing.T) {
	plan := singleTargetPlan()
	plan.CompilerFiles = []model.PlannedFile{{Path: "compiler.txt", Bytes: []byte("compiler")}}
	result, err := Append(plan, validInput(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	changedCompilerFile := plan
	changedCompilerFile.CompilerFiles = []model.PlannedFile{{Path: "compiler.txt", Bytes: []byte("changed")}}
	changedResult, err := Append(changedCompilerFile, validInput(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("Append(changed compiler file) error = %v", err)
	}
	if !bytes.Equal(provenanceFile(t, result), provenanceFile(t, changedResult)) {
		t.Fatal("compiler-owned files changed output evidence")
	}

	var document struct {
		Outputs []struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(provenanceFile(t, result), &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(document.Outputs) != 1 || len(document.Outputs[0].Files) != 1 || document.Outputs[0].Files[0].Path != "skill.md" {
		t.Fatalf("output evidence = %#v", document.Outputs)
	}
}

func singleTargetPlan() model.BuildPlan {
	return model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetClaude,
		Files:  []model.PlannedFile{{Path: "skill.md", Bytes: []byte("content")}},
	}}}
}

func validInput(configuration string) Input {
	return Input{
		CompilerVersion: "v1",
		Configuration:   json.RawMessage(configuration),
		Inputs:          []InputFile{{Path: "source/input", SHA256: shaA}},
		Acknowledgments: []Acknowledgment{{
			Asset:  "skill/example",
			Target: model.TargetClaude,
			Key:    "advisory",
			Reason: "accepted",
		}},
		AdapterRevisions: []AdapterRevision{{Target: model.TargetClaude, Revision: 1}},
	}
}

func provenanceFile(t *testing.T, plan model.BuildPlan) []byte {
	t.Helper()
	if len(plan.CompilerFiles) == 0 {
		t.Fatal("plan has no compiler files")
	}
	file := plan.CompilerFiles[len(plan.CompilerFiles)-1]
	if file.Path != provenancePath {
		t.Fatalf("provenance path = %q, want %q", file.Path, provenancePath)
	}
	if file.Executable {
		t.Fatal("provenance file is executable")
	}
	if len(file.Origin) != 0 {
		t.Fatalf("provenance origins = %#v, want none", file.Origin)
	}
	return file.Bytes
}
