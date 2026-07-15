package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNewRelativePathRejectsEscapes(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "/tmp/file", "../file", "dir/../file", "dir//file", "dir/", `dir\\file`, "C:/file", "file\x00name"} {
		t.Run(strings.ReplaceAll(value, "\x00", "NUL"), func(t *testing.T) {
			t.Parallel()
			if _, err := NewRelativePath(value); err == nil {
				t.Fatalf("NewRelativePath(%q) succeeded", value)
			}
		})
	}

	got, err := NewRelativePath("assets/skill.md")
	if err != nil {
		t.Fatalf("NewRelativePath() error = %v", err)
	}
	if got != "assets/skill.md" {
		t.Fatalf("NewRelativePath() = %q", got)
	}
}

func TestDecodeSourceManifestJSONRejectsStrictInvalidInput(t *testing.T) {
	t.Parallel()

	valid := `{"version":1,"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base"]}}`
	cases := []struct {
		name string
		data string
	}{
		{name: "malformed JSON", data: `{"kind":`},
		{name: "duplicate key", data: strings.Replace(valid, `"kind":"bundle"`, `"kind":"bundle","kind":"bundle"`, 1)},
		{name: "unknown field", data: strings.Replace(valid, `"output":"generated"`, `"output":"generated","extra":true`, 1)},
		{name: "duplicate target", data: strings.Replace(valid, `["claude"]`, `["claude","claude"]`, 1)},
		{name: "empty targets", data: strings.Replace(valid, `["claude"]`, `[]`, 1)},
		{name: "invalid capability state", data: strings.Replace(valid, `"bundle"`, `"bundle","composition":[{"target":"claude","capabilities":[{"key":"tool-use","state":"partial"}]}]`, 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, diagnostics := DecodeSourceManifestJSON([]byte(test.data))
			if !hasError(diagnostics) {
				t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v, want error", diagnostics)
			}
		})
	}

	manifest, diagnostics := DecodeSourceManifestJSON([]byte(valid))
	if len(diagnostics) != 0 {
		t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v", diagnostics)
	}
	if manifest.Kind != SourceKindBundle || len(manifest.Bundle.Packages) != 1 {
		t.Fatalf("DecodeSourceManifestJSON() = %#v", manifest)
	}
}

func TestValidateTargetCompositionAllowsUnsupportedCapability(t *testing.T) {
	t.Parallel()

	diagnostics := ValidateTargetComposition(TargetComposition{
		Target: TargetClaude,
		Capabilities: []CapabilityRule{{
			Key:   "tool-use",
			State: CapabilityStateUnsupported,
		}},
	})
	if hasError(diagnostics) {
		t.Fatalf("ValidateTargetComposition() diagnostics = %#v, want no error", diagnostics)
	}
}

func TestDecodeSourceManifestJSONAcceptsVersionOne(t *testing.T) {
	t.Parallel()

	_, diagnostics := DecodeSourceManifestJSON([]byte(`{"version":1,"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base"]}}`))
	if hasError(diagnostics) {
		t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v", diagnostics)
	}
}

func TestValidateSourceInventoryRejectsInvalidCapabilityUse(t *testing.T) {
	t.Parallel()

	inventory := SourceInventory{Packages: []SourcePackage{{
		Identity: "base",
		Assets: []SourceAsset{{
			Identity: "skill/example",
			Kind:     AssetKindSkill,
			CapabilityUses: []CapabilityUse{{
				Key:      "",
				Location: SourceLocation{Path: "../outside"},
			}},
		}},
	}}}
	if diagnostics := ValidateSourceInventory(inventory); !hasError(diagnostics) {
		t.Fatalf("ValidateSourceInventory() diagnostics = %#v, want error", diagnostics)
	}
}

func TestValidateSourceInventoryRejectsInvalidOverlayBodyPatch(t *testing.T) {
	t.Parallel()

	inventory := SourceInventory{
		Packages: []SourcePackage{{
			Identity: "base",
			Assets: []SourceAsset{{
				Identity: "skill/example",
				Kind:     AssetKindSkill,
				Base:     AssetContent{},
				Overlays: []TargetOverlay{{
					Target:       TargetClaude,
					BodyPatch:    &BodyPatch{Mode: BodyModeReplace},
					Files:        []FilePatch{{Path: "replacement"}},
					DeletedFiles: []RelativePath{"replacement"},
				}},
			}},
		}},
	}

	if diagnostics := ValidateSourceInventory(inventory); !hasError(diagnostics) {
		t.Fatalf("ValidateSourceInventory() diagnostics = %#v, want error", diagnostics)
	}
}

func TestValidateNormalizedPackageRejectsIncompleteAcknowledgment(t *testing.T) {
	t.Parallel()

	pkg := NormalizedPackage{
		Identity: "base",
		Target:   TargetClaude,
		Assets: []NormalizedAsset{{
			Identity: "skill/example",
			Kind:     AssetKindSkill,
		}},
		Acknowledgments: []Acknowledgment{{}},
	}
	if diagnostics := ValidateNormalizedPackage(pkg); !hasError(diagnostics) {
		t.Fatalf("ValidateNormalizedPackage() diagnostics = %#v, want error", diagnostics)
	}
}

func TestValidateBuildPlanRejectsNativeCheckOutsideTargetRoot(t *testing.T) {
	t.Parallel()

	workingDirectory := RelativePath("../outside")
	plan := BuildPlan{Targets: []TargetPlan{{
		Target: TargetClaude,
		NativeChecks: []NativeCheck{{
			Program:          "claude",
			WorkingDirectory: &workingDirectory,
			Location:         SourceLocation{Path: "adapter/check.go"},
		}},
	}}}
	if diagnostics := ValidateBuildPlan(plan); !hasError(diagnostics) {
		t.Fatalf("ValidateBuildPlan() diagnostics = %#v, want error", diagnostics)
	}
}

func TestModelJSONSerializationIsDeterministic(t *testing.T) {
	t.Parallel()

	first := NormalizedPackage{
		Identity: "base",
		Target:   TargetClaude,
		Metadata: PackageMetadata{"second": 2, "first": 1},
		Assets: []NormalizedAsset{{
			Identity: "skill/example",
			Kind:     AssetKindSkill,
			Content:  AssetContent{Frontmatter: map[string]any{"z": true, "a": false}},
		}},
	}
	second := NormalizedPackage{
		Identity: "base",
		Target:   TargetClaude,
		Metadata: PackageMetadata{"first": 1, "second": 2},
		Assets: []NormalizedAsset{{
			Identity: "skill/example",
			Kind:     AssetKindSkill,
			Content:  AssetContent{Frontmatter: map[string]any{"a": false, "z": true}},
		}},
	}
	if diagnostics := ValidateNormalizedPackage(first); len(diagnostics) != 0 {
		t.Fatalf("ValidateNormalizedPackage(first) diagnostics = %#v", diagnostics)
	}
	if diagnostics := ValidateNormalizedPackage(second); len(diagnostics) != 0 {
		t.Fatalf("ValidateNormalizedPackage(second) diagnostics = %#v", diagnostics)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("json.Marshal(first) error = %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("json.Marshal(second) error = %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("serialization differs:\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
}

func TestDiagnosticJSONLocationContract(t *testing.T) {
	t.Parallel()

	withLocation, err := json.Marshal(Diagnostic{
		Code:     "invalid-source",
		Severity: SeverityError,
		Location: &SourceLocation{Path: "source/SKILL.md"},
		Message:  "invalid source",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var diagnostic map[string]any
	if err := json.Unmarshal(withLocation, &diagnostic); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := diagnostic["location"], map[string]any{"path": "source/SKILL.md", "line": nil, "column": nil}; !reflect.DeepEqual(got, want) {
		t.Fatalf("location = %#v, want %#v", got, want)
	}

	withoutLocation, err := json.Marshal(Diagnostic{Code: "invalid-source", Severity: SeverityError, Message: "invalid source"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var withoutLocationDiagnostic map[string]any
	if err := json.Unmarshal(withoutLocation, &withoutLocationDiagnostic); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, exists := withoutLocationDiagnostic["location"]; exists {
		t.Fatalf("location = %#v, want omitted", withoutLocationDiagnostic["location"])
	}
}

func hasError(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			return true
		}
	}
	return false
}
