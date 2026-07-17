package model

import (
	"encoding/json"
	"reflect"
	"strconv"
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

func TestValidateNativeResourceOptions(t *testing.T) {
	t.Parallel()

	valid := SourceAsset{
		Identity: "native-resource/extensions", Kind: AssetKindNativeResource,
		Base:   AssetContent{Frontmatter: map[string]any{}, Files: map[RelativePath]FileContent{"extensions/custom.ts": {Bytes: []byte("export default {}")}}},
		Native: &NativeResourceOptions{PiExtensions: []RelativePath{"extensions/custom.ts"}},
	}
	if diagnostics := validateSourceAsset(valid); len(diagnostics) != 0 {
		t.Fatalf("valid native resource diagnostics = %#v", diagnostics)
	}
	valid.Kind = AssetKindResource
	valid.Identity = "resource/extensions"
	if diagnostics := validateSourceAsset(valid); !hasError(diagnostics) {
		t.Fatalf("non-native resource diagnostics = %#v, want error", diagnostics)
	}
	valid.Kind = AssetKindNativeResource
	valid.Identity = "native-resource/extensions"
	valid.Native = &NativeResourceOptions{PiExtensions: []RelativePath{"tools/custom.ts"}}
	if diagnostics := validateSourceAsset(valid); !hasError(diagnostics) {
		t.Fatalf("non-extension path diagnostics = %#v, want error", diagnostics)
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

func TestDecodeSourceManifestJSONAcceptsOptionalDistributionAndAggregateFields(t *testing.T) {
	t.Parallel()

	data := `{"version":1,"kind":"bundle","root":"source","targets":["pi"],"output":"generated","distribution":{"name":"Team tools","owner":"platform"},"composition":[{"target":"pi","profile":"package","packageMode":"aggregate","aggregate":{"identity":"team-tools","metadata":{"version":"1.0.0","description":"Team tools"}}}],"bundle":{"packages":["packages/base"]}}`
	manifest, diagnostics := DecodeSourceManifestJSON([]byte(data))
	if len(diagnostics) != 0 {
		t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v", diagnostics)
	}
	if manifest.Version != 1 || manifest.Distribution["name"] != "Team tools" {
		t.Fatalf("manifest = %#v", manifest)
	}
	composition := manifest.Composition[0]
	if composition.PackageMode != TargetPackageModeAggregate || composition.Aggregate == nil || composition.Aggregate.Identity != "team-tools" {
		t.Fatalf("composition = %#v", composition)
	}
}

func TestDecodeSourceManifestJSONStrictlyRejectsRenderConfigurationFields(t *testing.T) {
	t.Parallel()

	valid := `{"version":1,"kind":"bundle","root":"source","targets":["pi"],"output":"generated","distribution":{"name":"Team tools"},"composition":[{"target":"pi","profile":"package","packageMode":"aggregate","aggregate":{"identity":"team-tools","metadata":{}}}],"bundle":{"packages":["packages/base"]}}`
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "duplicate distribution", data: strings.Replace(valid, `"distribution":{"name":"Team tools"}`, `"distribution":{},"distribution":{"name":"Team tools"}`, 1)},
		{name: "duplicate package mode", data: strings.Replace(valid, `"packageMode":"aggregate"`, `"packageMode":"aggregate","packageMode":"aggregate"`, 1)},
		{name: "duplicate aggregate identity", data: strings.Replace(valid, `"identity":"team-tools"`, `"identity":"team-tools","identity":"other"`, 1)},
		{name: "unknown composition field", data: strings.Replace(valid, `"packageMode":"aggregate"`, `"packageMode":"aggregate","packagesTogether":true`, 1)},
		{name: "unknown aggregate field", data: strings.Replace(valid, `"metadata":{}`, `"metadata":{},"displayName":"Team"`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, diagnostics := DecodeSourceManifestJSON([]byte(test.data))
			if !hasError(diagnostics) {
				t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v, want error", diagnostics)
			}
		})
	}
}

func TestDecodeSourceManifestJSONKeepsVersionOneRenderDefaultsBackwardCompatible(t *testing.T) {
	t.Parallel()

	manifest, diagnostics := DecodeSourceManifestJSON([]byte(`{"version":1,"kind":"bundle","root":"source","targets":["claude"],"output":"generated","bundle":{"packages":["packages/base"]}}`))
	if len(diagnostics) != 0 {
		t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v", diagnostics)
	}
	if manifest.Distribution != nil || len(manifest.Composition) != 0 {
		t.Fatalf("manifest defaults = %#v", manifest)
	}
}

func TestValidateTargetCompositionRequiresExplicitPiAggregateConfiguration(t *testing.T) {
	t.Parallel()

	valid := TargetComposition{
		Target:      TargetPi,
		Profile:     TargetProfilePackage,
		PackageMode: TargetPackageModeAggregate,
		Aggregate:   &AggregatePackage{Identity: "team-tools", Metadata: PackageMetadata{}},
	}
	if diagnostics := ValidateTargetComposition(valid); len(diagnostics) != 0 {
		t.Fatalf("ValidateTargetComposition() diagnostics = %#v", diagnostics)
	}

	for _, test := range []struct {
		name   string
		mutate func(*TargetComposition)
	}{
		{name: "missing aggregate", mutate: func(input *TargetComposition) { input.Aggregate = nil }},
		{name: "missing metadata", mutate: func(input *TargetComposition) { input.Aggregate.Metadata = nil }},
		{name: "invalid identity", mutate: func(input *TargetComposition) { input.Aggregate.Identity = "../team" }},
		{name: "project profile", mutate: func(input *TargetComposition) { input.Profile = TargetProfileProject }},
		{name: "non-Pi target", mutate: func(input *TargetComposition) { input.Target = TargetClaude }},
		{name: "aggregate with separate mode", mutate: func(input *TargetComposition) { input.PackageMode = TargetPackageModeSeparate }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			aggregate := *valid.Aggregate
			input.Aggregate = &aggregate
			test.mutate(&input)
			if diagnostics := ValidateTargetComposition(input); !hasError(diagnostics) {
				t.Fatalf("ValidateTargetComposition() diagnostics = %#v, want error", diagnostics)
			}
		})
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

func TestValidateSourceInventoryRejectsInvalidFilePatches(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		files []FilePatch
	}{
		{
			name: "duplicate path",
			files: []FilePatch{
				{Path: "scripts/run.sh", Content: FileContent{Bytes: []byte("one")}},
				{Path: "scripts/run.sh", Content: FileContent{Bytes: []byte("two")}},
			},
		},
		{
			name: "invalid origin",
			files: []FilePatch{{
				Path:    "scripts/run.sh",
				Content: FileContent{Bytes: []byte("one"), Origin: []SourceLocation{{Path: "../run.sh"}}},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inventory := SourceInventory{Packages: []SourcePackage{{
				Identity: "base",
				Assets: []SourceAsset{{
					Identity: "skill/example",
					Kind:     AssetKindSkill,
					Overlays: []TargetOverlay{{Target: TargetClaude, Files: test.files}},
				}},
			}}}
			if diagnostics := ValidateSourceInventory(inventory); !hasError(diagnostics) {
				t.Fatalf("ValidateSourceInventory() diagnostics = %#v, want error", diagnostics)
			}
		})
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

func TestValidateTargetRenderInputOrdersPackagesAndRejectsDuplicateIdentities(t *testing.T) {
	t.Parallel()

	input := TargetRenderInput{
		Packages: []NormalizedPackage{
			{Identity: "zeta", Target: TargetClaude},
			{Identity: "alpha", Target: TargetClaude},
		},
		Distribution: DistributionMetadata{"owner": "platform"},
		PackageMode:  TargetPackageModeSeparate,
	}
	if diagnostics := ValidateTargetRenderInput(input); !hasError(diagnostics) {
		t.Fatalf("ValidateTargetRenderInput(unordered) diagnostics = %#v, want error", diagnostics)
	}
	SortTargetRenderInput(&input)
	if got := []PackageID{input.Packages[0].Identity, input.Packages[1].Identity}; !reflect.DeepEqual(got, []PackageID{"alpha", "zeta"}) {
		t.Fatalf("ordered identities = %#v", got)
	}
	if diagnostics := ValidateTargetRenderInput(input); len(diagnostics) != 0 {
		t.Fatalf("ValidateTargetRenderInput() diagnostics = %#v", diagnostics)
	}

	input.Packages[1].Identity = "alpha"
	if diagnostics := ValidateTargetRenderInput(input); !hasError(diagnostics) {
		t.Fatalf("ValidateTargetRenderInput(duplicate) diagnostics = %#v, want error", diagnostics)
	}
}

func TestValidateTargetRenderInputValidatesAggregateDependencyConflicts(t *testing.T) {
	t.Parallel()

	input := TargetRenderInput{
		Packages: []NormalizedPackage{
			{Identity: "alpha", Target: TargetPi, Profile: TargetProfilePackage, Metadata: PackageMetadata{"dependencies": map[string]any{"shared": "1.0.0"}}},
			{Identity: "zeta", Target: TargetPi, Profile: TargetProfilePackage, Metadata: PackageMetadata{"dependencies": map[string]any{"shared": "1.0.0"}}},
		},
		PackageMode: TargetPackageModeAggregate,
		Aggregate: &AggregatePackage{
			Identity: "team-tools",
			Metadata: PackageMetadata{"version": "1.0.0", "dependencies": map[string]any{"runtime": "2.0.0"}},
		},
	}
	if diagnostics := ValidateTargetRenderInput(input); len(diagnostics) != 0 {
		t.Fatalf("ValidateTargetRenderInput() diagnostics = %#v", diagnostics)
	}

	missingAggregate := input
	missingAggregate.Aggregate = nil
	if diagnostics := ValidateTargetRenderInput(missingAggregate); !hasError(diagnostics) || !diagnosticsContain(diagnostics, "aggregate target render input requires aggregate configuration") {
		t.Fatalf("ValidateTargetRenderInput(missing aggregate) diagnostics = %#v", diagnostics)
	}

	input.Packages[1].Metadata = PackageMetadata{"dependencies": map[string]any{"shared": "2.0.0"}}
	diagnostics := ValidateTargetRenderInput(input)
	if !hasError(diagnostics) || !diagnosticsContain(diagnostics, `aggregate dependency "shared" conflicts between package "alpha" ("1.0.0") and package "zeta" ("2.0.0")`) {
		t.Fatalf("ValidateTargetRenderInput() diagnostics = %#v, want dependency conflict", diagnostics)
	}
}

func TestTargetRenderInputSerializationIsDeterministicAfterOrdering(t *testing.T) {
	t.Parallel()

	first := TargetRenderInput{
		Packages: []NormalizedPackage{
			{Identity: "zeta", Target: TargetClaude, Metadata: PackageMetadata{"b": 2, "a": 1}},
			{Identity: "alpha", Target: TargetClaude},
		},
		Distribution: DistributionMetadata{"publisher": "team", "name": "tools"},
		PackageMode:  TargetPackageModeSeparate,
	}
	second := TargetRenderInput{
		Packages: []NormalizedPackage{
			{Identity: "alpha", Target: TargetClaude},
			{Identity: "zeta", Target: TargetClaude, Metadata: PackageMetadata{"a": 1, "b": 2}},
		},
		Distribution: DistributionMetadata{"name": "tools", "publisher": "team"},
		PackageMode:  TargetPackageModeSeparate,
	}
	SortTargetRenderInput(&first)
	SortTargetRenderInput(&second)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("render input differs:\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
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

func TestValidateNormalizedPackageAcceptsTypedHooks(t *testing.T) {
	t.Parallel()

	literal := "-eu"
	packageFile := RelativePath("scripts/run.sh")
	exec := NormalizedAsset{
		Identity: "hook/check",
		Kind:     AssetKindHook,
		Content: AssetContent{Files: map[RelativePath]FileContent{
			packageFile: {Bytes: []byte("#!/bin/sh\n"), Executable: true, Origin: []SourceLocation{{Path: "src/hooks/check/scripts/run.sh"}}},
		}},
		Hook: &HookDescriptor{
			Identity: "hook/check",
			Location: SourceLocation{Path: "src/hooks/check/hook.json"},
			Event:    HookEventPreTool,
			Matcher:  &HookMatcher{Tools: []HookToolCategory{HookToolCategoryCommand}},
			Handler: HookCommand{
				Mode:      HookHandlerModeExec,
				Program:   stringPointer("bash"),
				Arguments: []HookArgument{{Literal: &literal}, {PackageFile: &packageFile}},
			},
			TimeoutMilliseconds: 10_000,
			FailurePolicy:       HookFailurePolicyClosed,
			Order:               100,
		},
	}
	shell := NormalizedAsset{
		Identity: "hook/notify",
		Kind:     AssetKindHook,
		Hook: &HookDescriptor{
			Identity:            "hook/notify",
			Location:            SourceLocation{Path: "src/hooks/notify.json"},
			Event:               HookEventNotification,
			Handler:             HookCommand{Mode: HookHandlerModeShell, ShellCommand: stringPointer("printf done")},
			TimeoutMilliseconds: MaxHookTimeoutMilliseconds,
			Asynchronous:        true,
			FailurePolicy:       HookFailurePolicyOpen,
		},
	}

	pkg := NormalizedPackage{Identity: "base", Target: TargetClaude, Assets: []NormalizedAsset{exec, shell}}
	if diagnostics := ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		t.Fatalf("ValidateNormalizedPackage() diagnostics = %#v", diagnostics)
	}
	source := SourceInventory{Packages: []SourcePackage{{Identity: "base", Assets: []SourceAsset{
		{Identity: exec.Identity, Kind: exec.Kind, Base: exec.Content, Hook: exec.Hook},
		{Identity: shell.Identity, Kind: shell.Kind, Base: shell.Content, Hook: shell.Hook},
	}}}}
	if diagnostics := ValidateSourceInventory(source); len(diagnostics) != 0 {
		t.Fatalf("ValidateSourceInventory() diagnostics = %#v", diagnostics)
	}
	if content := exec.Content.Files[packageFile]; !content.Executable || string(content.Bytes) != "#!/bin/sh\n" {
		t.Fatalf("file content = %#v", content)
	}
}

func TestValidateNormalizedPackageAcceptsRepeatedExecArguments(t *testing.T) {
	t.Parallel()

	format := "%s %s"
	value := "value"
	asset := validHookAsset()
	asset.Hook.Handler.Program = stringPointer("printf")
	asset.Hook.Handler.Arguments = []HookArgument{
		{Literal: &format},
		{Literal: &value},
		{Literal: &value},
	}
	pkg := NormalizedPackage{Identity: "base", Target: TargetClaude, Assets: []NormalizedAsset{asset}}
	if diagnostics := ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		t.Fatalf("ValidateNormalizedPackage() diagnostics = %#v", diagnostics)
	}
}

func TestValidateNormalizedPackageRejectsMalformedHookDescriptors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*NormalizedAsset)
	}{
		{name: "hook missing descriptor", mutate: func(asset *NormalizedAsset) { asset.Hook = nil }},
		{name: "descriptor on non-hook", mutate: func(asset *NormalizedAsset) {
			asset.Kind = AssetKindSkill
			asset.Identity = "skill/check"
			asset.Hook.Identity = "skill/check"
		}},
		{name: "identity mismatch", mutate: func(asset *NormalizedAsset) { asset.Hook.Identity = "hook/other" }},
		{name: "invalid event", mutate: func(asset *NormalizedAsset) { asset.Hook.Event = "unknown" }},
		{name: "matcher on non-tool event", mutate: func(asset *NormalizedAsset) { asset.Hook.Event = HookEventStop }},
		{name: "empty matcher", mutate: func(asset *NormalizedAsset) { asset.Hook.Matcher.Tools = nil }},
		{name: "invalid tool", mutate: func(asset *NormalizedAsset) { asset.Hook.Matcher.Tools[0] = "database" }},
		{name: "duplicate tool", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Matcher.Tools = append(asset.Hook.Matcher.Tools, HookToolCategoryCommand)
		}},
		{name: "invalid mode", mutate: func(asset *NormalizedAsset) { asset.Hook.Handler.Mode = "http" }},
		{name: "exec missing program", mutate: func(asset *NormalizedAsset) { asset.Hook.Handler.Program = nil }},
		{name: "exec with shell command", mutate: func(asset *NormalizedAsset) { asset.Hook.Handler.ShellCommand = stringPointer("echo bad") }},
		{name: "argument has both kinds", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Handler.Arguments[0].Literal = stringPointer("run")
		}},
		{name: "argument has neither kind", mutate: func(asset *NormalizedAsset) { asset.Hook.Handler.Arguments[0] = HookArgument{} }},
		{name: "package file escapes", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Handler.Arguments[0] = HookArgument{PackageFile: relativePathPointer("../run.sh")}
		}},
		{name: "package file missing", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Handler.Arguments[0] = HookArgument{PackageFile: relativePathPointer("scripts/missing.sh")}
		}},
		{name: "invalid file origin", mutate: func(asset *NormalizedAsset) {
			content := asset.Content.Files["scripts/run.sh"]
			content.Origin = []SourceLocation{{Path: "../run.sh"}}
			asset.Content.Files["scripts/run.sh"] = content
		}},
		{name: "zero timeout", mutate: func(asset *NormalizedAsset) { asset.Hook.TimeoutMilliseconds = 0 }},
		{name: "negative timeout", mutate: func(asset *NormalizedAsset) { asset.Hook.TimeoutMilliseconds = -1 }},
		{name: "timeout over maximum", mutate: func(asset *NormalizedAsset) { asset.Hook.TimeoutMilliseconds = MaxHookTimeoutMilliseconds + 1 }},
		{name: "negative order", mutate: func(asset *NormalizedAsset) { asset.Hook.Order = -1 }},
		{name: "invalid failure policy", mutate: func(asset *NormalizedAsset) { asset.Hook.FailurePolicy = "retry" }},
		{name: "invalid environment name", mutate: func(asset *NormalizedAsset) { asset.Hook.Environment = []string{"BAD-NAME"} }},
		{name: "duplicate environment name", mutate: func(asset *NormalizedAsset) { asset.Hook.Environment = []string{"HOME", "HOME"} }},
		{name: "async blocking event", mutate: func(asset *NormalizedAsset) { asset.Hook.Asynchronous = true }},
		{name: "async closed failure", mutate: func(asset *NormalizedAsset) { asset.Hook.Event = HookEventPostTool; asset.Hook.Asynchronous = true }},
		{name: "async block capability", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Event = HookEventPostTool
			asset.Hook.Asynchronous = true
			asset.Hook.FailurePolicy = HookFailurePolicyOpen
			asset.CapabilityUses = []CapabilityUse{{Key: "hook.decision.block", Location: SourceLocation{Path: "src/hooks/check/hook.json"}}}
		}},
		{name: "shell missing command", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Handler = HookCommand{Mode: HookHandlerModeShell}
		}},
		{name: "shell with program", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Handler = HookCommand{Mode: HookHandlerModeShell, Program: stringPointer("sh"), ShellCommand: stringPointer("echo bad")}
		}},
		{name: "shell with arguments", mutate: func(asset *NormalizedAsset) {
			asset.Hook.Handler = HookCommand{Mode: HookHandlerModeShell, ShellCommand: stringPointer("echo bad"), Arguments: []HookArgument{{Literal: stringPointer("bad")}}}
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			asset := validHookAsset()
			test.mutate(&asset)
			pkg := NormalizedPackage{Identity: "base", Target: TargetClaude, Assets: []NormalizedAsset{asset}}
			diagnostics := ValidateNormalizedPackage(pkg)
			if !hasError(diagnostics) {
				t.Fatalf("ValidateNormalizedPackage() diagnostics = %#v, want error", diagnostics)
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Code != diagnosticCodeInvalidModel {
					t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code, diagnosticCodeInvalidModel)
				}
			}
		})
	}
}

func TestDecodeHookDescriptorJSONIsStrict(t *testing.T) {
	t.Parallel()

	valid := `{"event":"stop","handler":{"mode":"shell","arguments":[],"shellCommand":"done"},"timeoutMilliseconds":1000,"asynchronous":false,"failurePolicy":"open","environment":["HOME","CLAUDE_HOOK_CONFIG"],"order":0}`
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "missing event", data: strings.Replace(valid, `"event":"stop",`, "", 1)},
		{name: "missing handler", data: strings.Replace(valid, `"handler":{"mode":"shell","arguments":[],"shellCommand":"done"},`, "", 1)},
		{name: "missing timeout", data: strings.Replace(valid, `"timeoutMilliseconds":1000,`, "", 1)},
		{name: "missing asynchronous", data: strings.Replace(valid, `"asynchronous":false,`, "", 1)},
		{name: "missing failure policy", data: strings.Replace(valid, `"failurePolicy":"open",`, "", 1)},
		{name: "missing order", data: strings.Replace(valid, `,"order":0`, "", 1)},
		{name: "unknown field", data: strings.Replace(valid, `"order":0`, `"order":0,"vendor":true`, 1)},
		{name: "nested unknown field", data: strings.Replace(valid, `"shellCommand":"done"`, `"shellCommand":"done","vendor":true`, 1)},
		{name: "duplicate field", data: strings.Replace(valid, `"event":"stop"`, `"event":"stop","event":"stop"`, 1)},
		{name: "author identity", data: strings.Replace(valid, `"event":"stop"`, `"identity":"hook/check","event":"stop"`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeHookDescriptorJSON([]byte(test.data), "hook/check", SourceLocation{Path: "src/hooks/check.json"})
			if err == nil {
				t.Fatal("DecodeHookDescriptorJSON() succeeded")
			}
		})
	}

	descriptor, err := DecodeHookDescriptorJSON([]byte(valid), "hook/check", SourceLocation{Path: "src/hooks/check.json"})
	if err != nil {
		t.Fatalf("DecodeHookDescriptorJSON() error = %v", err)
	}
	if descriptor.Identity != "hook/check" || descriptor.Location.Path != "src/hooks/check.json" || descriptor.Handler.Mode != HookHandlerModeShell {
		t.Fatalf("DecodeHookDescriptorJSON() = %#v", descriptor)
	}
	if !reflect.DeepEqual(descriptor.Environment, []string{"HOME", "CLAUDE_HOOK_CONFIG"}) {
		t.Fatalf("DecodeHookDescriptorJSON() environment = %#v", descriptor.Environment)
	}
}

func TestDecodeOverlayFileContentJSONIsStrictAndExecutableAware(t *testing.T) {
	t.Parallel()

	line := 3
	location := SourceLocation{Path: "targets/pi.json", Line: &line}
	for _, test := range []struct {
		name       string
		data       string
		want       []byte
		executable bool
	}{
		{name: "string shorthand", data: `"text"`, want: []byte("text")},
		{name: "text object", data: `{"text":"script","executable":true}`, want: []byte("script"), executable: true},
		{name: "base64 object", data: `{"base64":"AAE=","executable":false}`, want: []byte{0, 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := DecodeOverlayFileContentJSON([]byte(test.data), location)
			if err != nil {
				t.Fatalf("DecodeOverlayFileContentJSON() error = %v", err)
			}
			if !reflect.DeepEqual(content.Bytes, test.want) || content.Executable != test.executable || len(content.Origin) != 1 || content.Origin[0].Path != location.Path || content.Origin[0].Line == nil || content.Origin[0].Line == location.Line || *content.Origin[0].Line != line {
				t.Fatalf("DecodeOverlayFileContentJSON() = %#v", content)
			}
		})
	}

	for _, data := range []string{
		`null`,
		`true`,
		`{}`,
		`{"text":"one","base64":"dHdv"}`,
		`{"text":null}`,
		`{"base64":"!"}`,
		`{"text":"one","executable":null}`,
		`{"text":"one","extra":true}`,
		`{"text":"one","text":"two"}`,
	} {
		t.Run("invalid "+data, func(t *testing.T) {
			if _, err := DecodeOverlayFileContentJSON([]byte(data), location); err == nil {
				t.Fatalf("DecodeOverlayFileContentJSON(%s) succeeded", data)
			}
		})
	}
}

func TestSortHookDescriptorsUsesDeterministicTieBreaks(t *testing.T) {
	t.Parallel()

	lineOne := 1
	lineTwo := 2
	hooks := []HookDescriptor{
		{Identity: "hook/zeta", Order: 10, Location: SourceLocation{Path: "z.json"}},
		{Identity: "hook/alpha", Order: 10, Location: SourceLocation{Path: "z.json"}},
		{Identity: "hook/first", Order: 1, Location: SourceLocation{Path: "z.json"}},
		{Identity: "hook/alpha", Order: 10, Location: SourceLocation{Path: "a.json", Line: &lineTwo}},
		{Identity: "hook/alpha", Order: 10, Location: SourceLocation{Path: "a.json", Line: &lineOne}},
	}
	SortHookDescriptors(hooks)

	got := make([]string, len(hooks))
	for index, hook := range hooks {
		line := 0
		if hook.Location.Line != nil {
			line = *hook.Location.Line
		}
		got[index] = string(hook.Identity) + ":" + string(hook.Location.Path) + ":" + strconv.Itoa(line)
	}
	want := []string{"hook/first:z.json:0", "hook/alpha:a.json:1", "hook/alpha:a.json:2", "hook/alpha:z.json:0", "hook/zeta:z.json:0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted hooks = %#v, want %#v", got, want)
	}
}

func diagnosticsContain(diagnostics []Diagnostic, message string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Message == message {
			return true
		}
	}
	return false
}

func validHookAsset() NormalizedAsset {
	packageFile := RelativePath("scripts/run.sh")
	return NormalizedAsset{
		Identity: "hook/check",
		Kind:     AssetKindHook,
		Content: AssetContent{Files: map[RelativePath]FileContent{
			packageFile: {Bytes: []byte("exit 0\n")},
		}},
		Hook: &HookDescriptor{
			Identity: "hook/check",
			Location: SourceLocation{Path: "src/hooks/check/hook.json"},
			Event:    HookEventPreTool,
			Matcher:  &HookMatcher{Tools: []HookToolCategory{HookToolCategoryCommand}},
			Handler: HookCommand{
				Mode:      HookHandlerModeExec,
				Program:   stringPointer("bash"),
				Arguments: []HookArgument{{PackageFile: &packageFile}},
			},
			TimeoutMilliseconds: 10_000,
			FailurePolicy:       HookFailurePolicyClosed,
			Order:               100,
		},
	}
}

func stringPointer(value string) *string { return &value }

func relativePathPointer(value RelativePath) *RelativePath { return &value }

func hasError(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			return true
		}
	}
	return false
}
