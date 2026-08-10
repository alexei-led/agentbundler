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

func TestCloneCommandDescriptorDetachesLocation(t *testing.T) {
	t.Parallel()

	if clone := CloneCommandDescriptor(nil); clone != nil {
		t.Fatalf("CloneCommandDescriptor(nil) = %#v", clone)
	}
	line, column := 3, 7
	descriptor := &CommandDescriptor{
		Identity: "command/resume-from", Location: SourceLocation{Path: "source/commands/resume-from.md", Line: &line, Column: &column},
		Name: "resume-from", Description: "Resume from a saved handoff.",
	}
	clone := CloneCommandDescriptor(descriptor)
	if !reflect.DeepEqual(clone, descriptor) {
		t.Fatalf("clone = %#v, want %#v", clone, descriptor)
	}
	*clone.Location.Line = 30
	*clone.Location.Column = 70
	clone.Description = "Changed."
	if line != 3 || column != 7 || descriptor.Description != "Resume from a saved handoff." {
		t.Fatal("command descriptor clone aliases its source")
	}
}

func TestValidateCommandAsset(t *testing.T) {
	t.Parallel()

	location := SourceLocation{Path: "src/commands/resume-from.md"}
	valid := NormalizedAsset{
		Identity: "command/resume-from", Kind: AssetKindCommand,
		Content:        AssetContent{Frontmatter: map[string]any{"description": "Resume from a saved handoff."}, Body: "Resume the session.\n", Files: map[RelativePath]FileContent{}},
		Command:        &CommandDescriptor{Identity: "command/resume-from", Location: location, Name: "resume-from", Description: "Resume from a saved handoff."},
		CapabilityUses: []CapabilityUse{{Key: "asset.command", Location: location}},
	}
	if diagnostics := validateNormalizedAsset(valid); len(diagnostics) != 0 {
		t.Fatalf("valid command diagnostics = %#v", diagnostics)
	}
	if id, err := NewAssetID("command/resume-from"); err != nil || id != valid.Identity {
		t.Fatalf("NewAssetID(command) = (%q, %v)", id, err)
	}

	cases := []struct {
		name   string
		mutate func(*NormalizedAsset)
		want   string
	}{
		{name: "missing descriptor", mutate: func(asset *NormalizedAsset) { asset.Command = nil }, want: "requires a command descriptor"},
		{name: "descriptor on another kind", mutate: func(asset *NormalizedAsset) { asset.Kind = AssetKindSkill; asset.Identity = "skill/resume-from" }, want: "non-command asset"},
		{name: "identity mismatch", mutate: func(asset *NormalizedAsset) { asset.Command.Identity = "command/other" }, want: "does not match"},
		{name: "invalid name", mutate: func(asset *NormalizedAsset) { asset.Command.Name = "Resume_From" }, want: "kebab-case"},
		{name: "empty description", mutate: func(asset *NormalizedAsset) { asset.Content.Frontmatter["description"] = "" }, want: "non-empty string description"},
		{name: "description mismatch", mutate: func(asset *NormalizedAsset) { asset.Content.Frontmatter["description"] = "Target wording." }, want: "does not match frontmatter"},
		{name: "missing capability", mutate: func(asset *NormalizedAsset) { asset.CapabilityUses = nil }, want: "requires capability"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			asset := valid
			asset.Content.Frontmatter = map[string]any{"description": valid.Content.Frontmatter["description"]}
			descriptor := *valid.Command
			asset.Command = &descriptor
			test.mutate(&asset)
			diagnostics := validateNormalizedAsset(asset)
			if !hasError(diagnostics) || !diagnosticsContainText(diagnostics, test.want) {
				t.Fatalf("diagnostics = %#v, want %q", diagnostics, test.want)
			}
		})
	}
}

func TestValidateCommandNameBoundaries(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		valid bool
	}{
		{name: "resume-from", valid: true},
		{name: "resume-2", valid: true},
		{name: "2-resume", valid: true},
		{name: "-resume", valid: false},
		{name: "resume-", valid: false},
		{name: "resume--from", valid: false},
		{name: "Resume", valid: false},
		{name: "resume_from", valid: false},
		{name: "résumé", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			identity := AssetID("command/" + test.name)
			location := SourceLocation{Path: "source/commands/command.md"}
			asset := NormalizedAsset{
				Identity: identity, Kind: AssetKindCommand,
				Content:        AssetContent{Frontmatter: map[string]any{"description": "Resume."}, Files: map[RelativePath]FileContent{}},
				Command:        &CommandDescriptor{Identity: identity, Location: location, Name: test.name, Description: "Resume."},
				CapabilityUses: []CapabilityUse{{Key: "asset.command", Location: location}},
			}
			hasDiagnostics := len(validateNormalizedAsset(asset)) != 0
			if hasDiagnostics == test.valid {
				t.Fatalf("valid = %v, diagnostics = %#v", test.valid, validateNormalizedAsset(asset))
			}
		})
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

func TestAntigravityTargetValidatesAcrossModelBoundaries(t *testing.T) {
	t.Parallel()

	manifest := SourceManifest{
		Version: 1,
		Kind:    SourceKindBundle,
		Root:    "source",
		Targets: []TargetID{TargetAntigravity},
		Output:  "generated",
		Composition: []TargetComposition{{
			Target:      TargetAntigravity,
			Profile:     TargetProfilePackage,
			PackageMode: TargetPackageModeSeparate,
		}},
		Bundle: &BundleSourceConfig{Packages: []RelativePath{"packages/base.json"}},
	}
	if diagnostics := ValidateSourceManifest(manifest); len(diagnostics) != 0 {
		t.Fatalf("ValidateSourceManifest() diagnostics = %#v", diagnostics)
	}

	asset := SourceAsset{
		Identity: "native-resource/conductor",
		Kind:     AssetKindNativeResource,
		Targets:  []TargetID{TargetAntigravity},
		Base: AssetContent{Frontmatter: map[string]any{}, Files: map[RelativePath]FileContent{
			"rules/conductor.md": {Bytes: []byte("# Rule\n"), Origin: []SourceLocation{{Path: "src/plugins/antigravity/conductor/rules/conductor.md"}}},
		}},
		CapabilityUses: []CapabilityUse{{Key: "asset.native-resource", Location: SourceLocation{Path: "src/plugins/antigravity/conductor/.agentbundler/asset.json"}}},
		Overlays: []TargetOverlay{{
			Target: TargetAntigravity,
			Acknowledgments: []Acknowledgment{{
				Asset: "native-resource/conductor", Target: TargetAntigravity, Key: "native-review", Reason: "Reviewed for Antigravity.",
			}},
		}},
	}
	assetID := asset.Identity
	targetID := TargetAntigravity
	inventory := SourceInventory{
		Packages: []SourcePackage{{Identity: "base", Metadata: PackageMetadata{}, Assets: []SourceAsset{asset}}},
		NativeGaps: []NativeGap{{
			Package: "base", Component: "conductor", Asset: &assetID, Location: SourceLocation{Path: "src/plugins/antigravity/conductor"}, Target: &targetID,
		}},
	}
	if diagnostics := ValidateSourceInventory(inventory); len(diagnostics) != 0 {
		t.Fatalf("ValidateSourceInventory() diagnostics = %#v", diagnostics)
	}

	normalized := NormalizedPackage{
		Identity: "base",
		Metadata: PackageMetadata{},
		Target:   TargetAntigravity,
		Profile:  TargetProfilePackage,
		Assets: []NormalizedAsset{{
			Identity: asset.Identity, Kind: asset.Kind, Content: asset.Base, CapabilityUses: asset.CapabilityUses,
		}},
		Acknowledgments: []Acknowledgment{{
			Asset: asset.Identity, Target: TargetAntigravity, Key: "native-review", Reason: "Reviewed for Antigravity.",
		}},
	}
	if diagnostics := ValidateNormalizedPackage(normalized); len(diagnostics) != 0 {
		t.Fatalf("ValidateNormalizedPackage() diagnostics = %#v", diagnostics)
	}
	if diagnostics := ValidateTargetRenderInput(TargetRenderInput{
		Packages: []NormalizedPackage{normalized}, Distribution: DistributionMetadata{}, PackageMode: TargetPackageModeSeparate,
	}); len(diagnostics) != 0 {
		t.Fatalf("ValidateTargetRenderInput() diagnostics = %#v", diagnostics)
	}

	plan := BuildPlan{Targets: []TargetPlan{{
		Target: TargetAntigravity, Packages: []PackageID{"base"},
		Files:        []PlannedFile{{Path: "plugin.json", Bytes: []byte(`{"name":"base"}`)}},
		NativeChecks: []NativeCheck{{Program: "agy", Arguments: []string{"plugin", "validate", "."}, Location: SourceLocation{Path: "internal/target/antigravity/antigravity.go"}}},
	}}}
	if diagnostics := ValidateBuildPlan(plan); len(diagnostics) != 0 {
		t.Fatalf("ValidateBuildPlan() diagnostics = %#v", diagnostics)
	}
}

func TestUnknownTargetStillFailsAcrossModelBoundaries(t *testing.T) {
	t.Parallel()

	unknown := TargetID("unknown")
	asset := SourceAsset{
		Identity: "skill/demo", Kind: AssetKindSkill, Base: AssetContent{Frontmatter: map[string]any{}},
		Targets: []TargetID{unknown}, Overlays: []TargetOverlay{{Target: unknown}},
	}
	checks := []struct {
		name        string
		diagnostics []Diagnostic
	}{
		{name: "manifest", diagnostics: ValidateSourceManifest(SourceManifest{Version: 1, Kind: SourceKindBundle, Root: "source", Targets: []TargetID{unknown}, Output: "generated", Bundle: &BundleSourceConfig{Packages: []RelativePath{"packages/base.json"}}})},
		{name: "composition", diagnostics: ValidateTargetComposition(TargetComposition{Target: unknown})},
		{name: "inventory", diagnostics: ValidateSourceInventory(SourceInventory{Packages: []SourcePackage{{Identity: "base", Metadata: PackageMetadata{}, Assets: []SourceAsset{asset}}}})},
		{name: "normalized package", diagnostics: ValidateNormalizedPackage(NormalizedPackage{Identity: "base", Metadata: PackageMetadata{}, Target: unknown})},
		{name: "build plan", diagnostics: ValidateBuildPlan(BuildPlan{Targets: []TargetPlan{{Target: unknown}}})},
	}
	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			if !hasError(check.diagnostics) {
				t.Fatalf("diagnostics = %#v, want invalid target error", check.diagnostics)
			}
		})
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

func TestDecodeSourceManifestJSONAcceptsRepositoryRootCompatibility(t *testing.T) {
	t.Parallel()

	data := `{"version":1,"kind":"bundle","root":"source","targets":["claude","pi"],"output":"dist","distribution":{"name":"tools"},"compatibility":{"rootManifests":["claude","pi"]},"composition":[{"target":"claude","profile":"package","packageMode":"separate"},{"target":"pi","profile":"package","packageMode":"aggregate","aggregate":{"identity":"tools","metadata":{}}}],"bundle":{"packages":["packages/base"]}}`
	manifest, diagnostics := DecodeSourceManifestJSON([]byte(data))
	if len(diagnostics) != 0 {
		t.Fatalf("DecodeSourceManifestJSON() diagnostics = %#v", diagnostics)
	}
	if manifest.Compatibility == nil || !reflect.DeepEqual(manifest.Compatibility.RootManifests, []TargetID{TargetClaude, TargetPi}) {
		t.Fatalf("compatibility = %#v", manifest.Compatibility)
	}
}

func TestValidateSourceManifestRejectsRepositoryRootCompatibilityConflicts(t *testing.T) {
	t.Parallel()

	base := SourceManifest{
		Version: 1, Kind: SourceKindBundle, Root: "source", Output: "dist",
		Targets:      []TargetID{TargetClaude, TargetGrok},
		Distribution: DistributionMetadata{"name": "tools"},
		Composition: []TargetComposition{
			{Target: TargetClaude, Profile: TargetProfilePackage, PackageMode: TargetPackageModeSeparate},
			{Target: TargetGrok, Profile: TargetProfilePackage, PackageMode: TargetPackageModeSeparate},
		},
		Bundle: &BundleSourceConfig{Packages: []RelativePath{"packages/base.json"}},
	}
	base.Compatibility = &CompatibilityConfig{RootManifests: []TargetID{TargetClaude, TargetGrok}}
	if diagnostics := ValidateSourceManifest(base); !diagnosticsHaveCode(diagnostics, "compatibility-marker-collision") {
		t.Fatalf("collision diagnostics = %#v", diagnostics)
	}

	base.Compatibility = &CompatibilityConfig{RootManifests: []TargetID{TargetClaude, TargetClaude}}
	if diagnostics := ValidateSourceManifest(base); !diagnosticsHaveCode(diagnostics, "duplicate-compatibility-target") {
		t.Fatalf("duplicate diagnostics = %#v", diagnostics)
	}

	base.Compatibility = &CompatibilityConfig{RootManifests: []TargetID{TargetPi}}
	if diagnostics := ValidateSourceManifest(base); !diagnosticsHaveCode(diagnostics, "invalid-compatibility-target") {
		t.Fatalf("unselected diagnostics = %#v", diagnostics)
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

func TestValidateBuildPlanEnforcesArchiveUnitPartition(t *testing.T) {
	baseFiles := []PlannedFile{{Path: "foo/a"}, {Path: "bar/b"}}
	for _, test := range []struct {
		name  string
		units []ArchiveUnit
		want  string
	}{
		{
			name:  "complete non-overlapping partition",
			units: []ArchiveUnit{{Root: "foo", Stem: "foo", Suffix: ".tar.gz"}, {Root: "bar", Stem: "bar", Suffix: ".tar.gz"}},
		},
		{
			name:  "uncovered file",
			units: []ArchiveUnit{{Root: "foo", Stem: "foo", Suffix: ".tar.gz"}},
			want:  "is not covered by an archive unit",
		},
		{
			name:  "overlapping roots",
			units: []ArchiveUnit{{Root: ".", Stem: "all", Suffix: ".tar.gz"}, {Root: "foo", Stem: "foo", Suffix: ".tar.gz"}},
			want:  "is covered by multiple archive units",
		},
		{
			name:  "duplicate destination",
			units: []ArchiveUnit{{Root: "foo", Stem: "same", Suffix: ".tar.gz"}, {Root: "bar", Stem: "same", Suffix: ".tar.gz"}},
			want:  "archive destination \"same.tar.gz\" is duplicated",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := BuildPlan{Targets: []TargetPlan{{Target: TargetAgentPlugins, Files: baseFiles, ArchiveUnits: test.units}}}
			diagnostics := ValidateBuildPlan(plan)
			if test.want == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("ValidateBuildPlan() diagnostics = %#v", diagnostics)
				}
				return
			}
			if !diagnosticsContainText(diagnostics, test.want) {
				t.Fatalf("ValidateBuildPlan() diagnostics = %#v; want %q", diagnostics, test.want)
			}
		})
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

func diagnosticsHaveCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func diagnosticsContain(diagnostics []Diagnostic, message string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Message == message {
			return true
		}
	}
	return false
}

func diagnosticsContainText(diagnostics []Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, text) {
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

// --- AgentPlugin model tests ---

func TestValidateSourceManifestAcceptsAgentPlugin(t *testing.T) {
	t.Parallel()
	manifest := SourceManifest{
		Version: 1,
		Kind:    SourceKindAgentPlugin,
		Root:    "plugins",
		Targets: []TargetID{TargetAgentPlugins},
		Output:  "generated",
		AgentPlugin: &AgentPluginSourceConfig{
			Plugins: []RelativePath{"deploy-tools"},
		},
	}
	if diags := ValidateSourceManifest(manifest); len(diags) != 0 {
		t.Fatalf("valid agent-plugin manifest diagnostics = %v", diags)
	}
}

func TestValidateSourceManifestRejectsAgentPluginEmptyPlugins(t *testing.T) {
	t.Parallel()
	manifest := SourceManifest{
		Version:     1,
		Kind:        SourceKindAgentPlugin,
		Root:        "plugins",
		Targets:     []TargetID{TargetAgentPlugins},
		Output:      "generated",
		AgentPlugin: &AgentPluginSourceConfig{Plugins: nil},
	}
	if diags := ValidateSourceManifest(manifest); !hasError(diags) {
		t.Fatal("empty plugins: expected error diagnostics")
	}
}

func TestValidateSourceManifestRejectsAgentPluginDuplicatePaths(t *testing.T) {
	t.Parallel()
	manifest := SourceManifest{
		Version: 1,
		Kind:    SourceKindAgentPlugin,
		Root:    "plugins",
		Targets: []TargetID{TargetAgentPlugins},
		Output:  "generated",
		AgentPlugin: &AgentPluginSourceConfig{
			Plugins: []RelativePath{"tools", "tools"},
		},
	}
	if diags := ValidateSourceManifest(manifest); !hasError(diags) {
		t.Fatal("duplicate plugin paths: expected error diagnostics")
	}
}

func TestValidateSourceManifestRejectsAgentPluginWithOtherConfigs(t *testing.T) {
	t.Parallel()
	manifest := SourceManifest{
		Version: 1,
		Kind:    SourceKindAgentPlugin,
		Root:    "plugins",
		Targets: []TargetID{TargetAgentPlugins},
		Output:  "generated",
		AgentPlugin: &AgentPluginSourceConfig{
			Plugins: []RelativePath{"tools"},
		},
		Bundle: &BundleSourceConfig{Packages: []RelativePath{"pkg"}},
	}
	if diags := ValidateSourceManifest(manifest); !hasError(diags) {
		t.Fatal("agent-plugin with bundle config: expected error diagnostics")
	}
}

func TestValidateSourceManifestRejectsAgentPluginMissingConfig(t *testing.T) {
	t.Parallel()
	manifest := SourceManifest{
		Version: 1,
		Kind:    SourceKindAgentPlugin,
		Root:    "plugins",
		Targets: []TargetID{TargetAgentPlugins},
		Output:  "generated",
		// AgentPlugin is nil
	}
	if diags := ValidateSourceManifest(manifest); !hasError(diags) {
		t.Fatal("agent-plugin without agentPlugin config: expected error diagnostics")
	}
}

func TestAgentPluginsTargetAggregateRejected(t *testing.T) {
	t.Parallel()
	composition := TargetComposition{
		Target:      TargetAgentPlugins,
		PackageMode: TargetPackageModeAggregate,
		Aggregate: &AggregatePackage{
			Identity: "combined",
			Metadata: map[string]any{},
		},
		Profile: TargetProfilePackage,
	}
	if diags := ValidateTargetComposition(composition); !hasError(diags) {
		t.Fatal("agent-plugins aggregate mode: expected error diagnostics")
	}
}

func TestCloneAgentPluginDataNilSafe(t *testing.T) {
	t.Parallel()
	if clone := CloneAgentPluginData(nil); clone != nil {
		t.Fatalf("CloneAgentPluginData(nil) = %v; want nil", clone)
	}
}

func TestCloneAgentPluginDataIsDetached(t *testing.T) {
	t.Parallel()

	originLine := 3
	data := &AgentPluginData{
		Profile: "agent-plugins/1.0.0-bd383552",
		Manifest: AgentPluginManifest{
			Name:     "test-plugin",
			Author:   &AgentPluginAuthor{Name: "Test Author", Email: "author@example.com"},
			Keywords: []string{"a", "b"},
		},
		MCPServers: []MCPServer{
			{
				Name:      "srv",
				Transport: MCPTransportStdio,
				Stdio: &StdioMCPServer{
					Command: "server",
					Args:    []string{"--port", "3000"},
					Env:     map[string]string{"LOG": "debug"},
				},
				Unknown: map[string]any{"nested": []any{map[string]any{"value": "source"}}},
			},
		},
		Extensions: []ClientExtension{{
			Namespace: "com.example.extension",
			Manifest:  map[string]any{"options": []any{map[string]any{"mode": "source"}}},
		}},
		PackageFiles: []PackageFile{
			{
				Path:   "README.md",
				Bytes:  []byte("hello"),
				SHA256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
				Origin: []SourceLocation{{Path: "README.md", Line: &originLine}},
			},
		},
		UnknownManifest: map[string]any{"future": map[string]any{"items": []any{map[string]any{"value": "source"}}}},
		UnknownMCP:      map[string]any{"future": []any{map[string]any{"value": "source"}}},
	}

	clone := CloneAgentPluginData(data)
	if clone == nil {
		t.Fatal("CloneAgentPluginData returned nil")
	}

	// Verify values equal.
	if clone.Profile != data.Profile {
		t.Errorf("Profile = %q; want %q", clone.Profile, data.Profile)
	}
	if clone.Manifest.Name != data.Manifest.Name {
		t.Errorf("Manifest.Name = %q; want %q", clone.Manifest.Name, data.Manifest.Name)
	}

	// Mutate clone and verify original is unaffected.
	clone.Manifest.Author.Name = "mutated"
	if data.Manifest.Author.Name == "mutated" {
		t.Error("Manifest.Author not detached")
	}

	clone.Manifest.Keywords[0] = "mutated"
	if data.Manifest.Keywords[0] == "mutated" {
		t.Error("Manifest.Keywords not detached")
	}

	clone.MCPServers[0].Stdio.Args[0] = "mutated"
	if data.MCPServers[0].Stdio.Args[0] == "mutated" {
		t.Error("MCPServer.Stdio.Args not detached")
	}

	clone.MCPServers[0].Stdio.Env["LOG"] = "mutated"
	if data.MCPServers[0].Stdio.Env["LOG"] == "mutated" {
		t.Error("MCPServer.Stdio.Env not detached")
	}

	clone.PackageFiles[0].Bytes[0] = 'X'
	if data.PackageFiles[0].Bytes[0] == 'X' {
		t.Error("PackageFile.Bytes not detached")
	}

	clone.MCPServers[0].Unknown["nested"].([]any)[0].(map[string]any)["value"] = "mutated"
	if data.MCPServers[0].Unknown["nested"].([]any)[0].(map[string]any)["value"] == "mutated" {
		t.Error("MCPServer.Unknown nested value not detached")
	}

	clone.Extensions[0].Manifest.(map[string]any)["options"].([]any)[0].(map[string]any)["mode"] = "mutated"
	if data.Extensions[0].Manifest.(map[string]any)["options"].([]any)[0].(map[string]any)["mode"] == "mutated" {
		t.Error("ClientExtension.Manifest nested value not detached")
	}

	clone.UnknownManifest["future"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "mutated"
	if data.UnknownManifest["future"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] == "mutated" {
		t.Error("UnknownManifest nested value not detached")
	}

	clone.UnknownMCP["future"].([]any)[0].(map[string]any)["value"] = "mutated"
	if data.UnknownMCP["future"].([]any)[0].(map[string]any)["value"] == "mutated" {
		t.Error("UnknownMCP nested value not detached")
	}
}

func TestValidateAgentPluginDataRejectsEmpty(t *testing.T) {
	t.Parallel()
	if diags := ValidateAgentPluginData(AgentPluginData{}); !hasError(diags) {
		t.Fatal("empty AgentPluginData: expected error diagnostics")
	}
}

func TestValidateAgentPluginDataAcceptsMinimal(t *testing.T) {
	t.Parallel()
	data := AgentPluginData{
		Profile: "agent-plugins/1.0.0-bd383552",
		Manifest: AgentPluginManifest{
			Name:   "test-plugin",
			Author: &AgentPluginAuthor{URL: "not a validated URL", Email: "not a validated email"},
		},
	}
	if diags := ValidateAgentPluginData(data); len(diags) != 0 {
		t.Fatalf("minimal AgentPluginData diagnostics = %v", diags)
	}
}

func TestValidateNormalizedPackageCarriesAgentPluginData(t *testing.T) {
	t.Parallel()
	pkg := NormalizedPackage{
		Identity: "test-plugin",
		Target:   TargetAgentPlugins,
		Metadata: map[string]any{},
		AgentPlugin: &AgentPluginData{
			Profile:  "agent-plugins/1.0.0-bd383552",
			Manifest: AgentPluginManifest{Name: "test-plugin"},
		},
	}
	if diags := ValidateNormalizedPackage(pkg); len(diags) != 0 {
		t.Fatalf("normalized package with AgentPlugin diagnostics = %v", diags)
	}
}

func TestDecodeSourceManifestJSONAcceptsAgentPluginKind(t *testing.T) {
	t.Parallel()
	const data = `{
		"version": 1,
		"kind": "agent-plugin",
		"root": "plugins",
		"targets": ["agent-plugins"],
		"output": "generated",
		"composition": [],
		"agentPlugin": {"plugins": ["deploy-tools", "review-tools"]}
	}`
	manifest, diags := DecodeSourceManifestJSON([]byte(data))
	if len(diags) != 0 {
		t.Fatalf("agent-plugin manifest diagnostics = %v", diags)
	}
	if manifest.Kind != SourceKindAgentPlugin {
		t.Errorf("Kind = %q; want agent-plugin", manifest.Kind)
	}
	if manifest.AgentPlugin == nil {
		t.Fatal("AgentPlugin is nil")
	}
	if len(manifest.AgentPlugin.Plugins) != 2 {
		t.Errorf("Plugins len = %d; want 2", len(manifest.AgentPlugin.Plugins))
	}
}

func TestPortableCapabilityKeysAreDefined(t *testing.T) {
	t.Parallel()
	keys := []CapabilityKey{
		CapabilityKeyAgentPluginSkills,
		CapabilityKeyAgentPluginMCPStdio,
		CapabilityKeyAgentPluginMCPStreamableHTTP,
		CapabilityKeyAgentPluginMCPSSE,
		CapabilityKeyAgentPluginExtensions,
		CapabilityKeyAgentPluginUnknownJSON,
		CapabilityKeyAgentPluginPackageFiles,
	}
	seen := make(map[CapabilityKey]bool, len(keys))
	for _, key := range keys {
		if key == "" {
			t.Error("capability key must not be empty")
		}
		if seen[key] {
			t.Errorf("capability key %q is duplicated", key)
		}
		seen[key] = true
	}
}
