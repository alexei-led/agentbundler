package composition

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestComposeAppliesOverlayAndSkillPreamble(t *testing.T) {
	preamble := "Target policy"
	packages, diagnostics := Compose(model.SourceInventory{Packages: []model.SourcePackage{{
		Identity: "bundle",
		Assets: []model.SourceAsset{{
			Identity: "skill/demo",
			Kind:     model.AssetKindSkill,
			Base: model.AssetContent{
				Frontmatter: map[string]any{
					"drop":   true,
					"nested": map[string]any{"keep": "yes", "drop": "no"},
				},
				Body:  "base body",
				Files: map[model.RelativePath]model.FileContent{"keep.txt": {Bytes: []byte("base")}, "remove.txt": {Bytes: []byte("remove")}},
			},
			Overlays: []model.TargetOverlay{{
				Target: model.TargetPi,
				FrontmatterPatch: &map[string]any{
					"drop":   nil,
					"nested": map[string]any{"drop": nil, "add": "new"},
				},
				Files:        []model.FilePatch{{Path: "keep.txt", Content: model.FileContent{Bytes: []byte("overlay"), Executable: true}}, {Path: "new.txt", Content: model.FileContent{Bytes: []byte("new")}}},
				DeletedFiles: []model.RelativePath{"remove.txt"},
			}},
		}},
	}}}, model.TargetComposition{Target: model.TargetPi, SkillPreamble: &preamble})
	if len(diagnostics) != 0 {
		t.Fatalf("Compose diagnostics = %#v", diagnostics)
	}

	asset := packages[0].Assets[0]
	wantFrontmatter := map[string]any{"nested": map[string]any{"keep": "yes", "add": "new"}}
	if !reflect.DeepEqual(asset.Content.Frontmatter, wantFrontmatter) {
		t.Errorf("frontmatter = %#v, want %#v", asset.Content.Frontmatter, wantFrontmatter)
	}
	if asset.Content.Body != "Target policy\n\nbase body" {
		t.Errorf("body = %q", asset.Content.Body)
	}
	wantFiles := map[model.RelativePath]model.FileContent{"keep.txt": {Bytes: []byte("overlay"), Executable: true}, "new.txt": {Bytes: []byte("new")}}
	if !reflect.DeepEqual(asset.Content.Files, wantFiles) {
		t.Errorf("files = %#v, want %#v", asset.Content.Files, wantFiles)
	}
}

func TestComposeSelectsTargetFilteredAssets(t *testing.T) {
	inventory := model.SourceInventory{Packages: []model.SourcePackage{{
		Identity: "bundle",
		Assets: []model.SourceAsset{
			{Identity: "agent/claude", Kind: model.AssetKindAgent, Targets: []model.TargetID{model.TargetClaude}},
			{
				Identity: "hook/claude",
				Kind:     model.AssetKindHook,
				Targets:  []model.TargetID{model.TargetClaude},
				Hook: &model.HookDescriptor{
					Identity:            "hook/claude",
					Location:            model.SourceLocation{Path: "hooks.json"},
					Event:               model.HookEventStop,
					Handler:             model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: stringPointer("done")},
					TimeoutMilliseconds: 1_000,
					FailurePolicy:       model.HookFailurePolicyOpen,
				},
			},
			{Identity: "skill/shared", Kind: model.AssetKindSkill},
		},
	}}}
	packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetPi})
	if len(diagnostics) != 0 {
		t.Fatalf("Compose diagnostics = %#v", diagnostics)
	}
	if got, want := len(packages[0].Assets), 1; got != want || packages[0].Assets[0].Identity != "skill/shared" {
		t.Fatalf("selected assets = %#v, want shared skill only", packages[0].Assets)
	}
}

func TestComposeAppliesExplicitBodyModes(t *testing.T) {
	replacement := "replaced"
	base := "# First\nkeep\n## Child\nold\n# Last\nlast\n```markdown\n# Not a heading\n```\n"
	cases := []struct {
		name  string
		patch model.BodyPatch
		want  string
	}{
		{
			name:  "replace",
			patch: model.BodyPatch{Mode: model.BodyModeReplace, Text: &replacement},
			want:  replacement,
		},
		{
			name:  "sections",
			patch: model.BodyPatch{Mode: model.BodyModeSections, Sections: []model.SectionPatch{{HeadingPath: []string{"First", "Child"}, Body: "new\n"}}},
			want:  "# First\nkeep\n## Child\nnew\n# Last\nlast\n```markdown\n# Not a heading\n```\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packages, diagnostics := Compose(oneAssetInventory(base, tc.patch), model.TargetComposition{Target: model.TargetPi})
			if len(diagnostics) != 0 {
				t.Fatalf("Compose diagnostics = %#v", diagnostics)
			}
			if got := packages[0].Assets[0].Content.Body; got != tc.want {
				t.Errorf("body = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestComposeRejectsInvalidSectionAnchor(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "missing", body: "# Present\nbody\n"},
		{name: "duplicate", body: "# Install\none\n# Install\ntwo\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patch := model.BodyPatch{Mode: model.BodyModeSections, Sections: []model.SectionPatch{{HeadingPath: []string{"Install"}, Body: "new"}}}
			if tc.name == "missing" {
				patch.Sections[0].HeadingPath = []string{"Missing"}
			}
			packages, diagnostics := Compose(oneAssetInventory(tc.body, patch), model.TargetComposition{Target: model.TargetPi})
			if packages != nil {
				t.Errorf("packages = %#v, want nil", packages)
			}
			if len(diagnostics) != 1 || diagnostics[0].Code != diagnosticCodeInvalidComposition {
				t.Errorf("diagnostics = %#v", diagnostics)
			}
		})
	}
}

func TestComposeCanonicalizesPackageAndAssetOrder(t *testing.T) {
	inventory := model.SourceInventory{Packages: []model.SourcePackage{
		{Identity: "z", Assets: []model.SourceAsset{{Identity: "skill/z", Kind: model.AssetKindSkill}, {Identity: "skill/a", Kind: model.AssetKindSkill}}},
		{Identity: "a", Assets: []model.SourceAsset{{Identity: "skill/b", Kind: model.AssetKindSkill}}},
	}}
	packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetPi})
	if len(diagnostics) != 0 {
		t.Fatalf("Compose diagnostics = %#v", diagnostics)
	}
	if got := []model.PackageID{packages[0].Identity, packages[1].Identity}; !reflect.DeepEqual(got, []model.PackageID{"a", "z"}) {
		t.Errorf("package order = %#v", got)
	}
	if got := []model.AssetID{packages[1].Assets[0].Identity, packages[1].Assets[1].Identity}; !reflect.DeepEqual(got, []model.AssetID{"skill/a", "skill/z"}) {
		t.Errorf("asset order = %#v", got)
	}
}

func TestComposeRequiresExactAdvisoryAcknowledgment(t *testing.T) {
	inventory := model.SourceInventory{Packages: []model.SourcePackage{{
		Identity: "bundle",
		Assets: []model.SourceAsset{
			{
				Identity:       "skill/demo",
				Kind:           model.AssetKindSkill,
				Base:           model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}},
				CapabilityUses: []model.CapabilityUse{{Key: "network", Location: model.SourceLocation{Path: "demo.md"}}},
				Overlays:       []model.TargetOverlay{{Target: model.TargetPi, Acknowledgments: []model.Acknowledgment{{Asset: "skill/demo", Target: model.TargetPi, Key: "network", Reason: "Target requires approval."}}}},
			},
			{
				Identity:       "skill/other",
				Kind:           model.AssetKindSkill,
				Base:           model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}},
				CapabilityUses: []model.CapabilityUse{{Key: "networking", Location: model.SourceLocation{Path: "other.md"}}},
			},
		},
	}}}
	target := model.TargetComposition{Target: model.TargetPi, Capabilities: []model.CapabilityRule{
		{Key: "network", State: model.CapabilityStateAdvisory},
		{Key: "networking", State: model.CapabilityStateNative},
	}}
	packages, diagnostics := Compose(inventory, target)
	if len(diagnostics) != 0 {
		t.Fatalf("Compose diagnostics = %#v", diagnostics)
	}
	if got := packages[0].Acknowledgments; len(got) != 1 || got[0].Key != "network" {
		t.Errorf("acknowledgments = %#v", got)
	}

	inventory.Packages[0].Assets[0].Overlays[0].Acknowledgments = nil
	packages, diagnostics = Compose(inventory, target)
	if packages != nil || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticCodeInvalidComposition {
		t.Errorf("Compose without acknowledgment = (%#v, %#v)", packages, diagnostics)
	}
}

func TestComposePreservesHookDescriptorFileAndSourceEvidenceWithoutAliasing(t *testing.T) {
	line := 7
	literal := "-eu"
	payloadPath := model.RelativePath("scripts/check.sh")
	argumentPath := payloadPath
	descriptor := &model.HookDescriptor{
		Identity: "hook/check",
		Location: model.SourceLocation{Path: "src/hooks/check/hook.json", Line: &line},
		Event:    model.HookEventPreTool,
		Matcher:  &model.HookMatcher{Tools: []model.HookToolCategory{model.HookToolCategoryCommand}},
		Handler: model.HookCommand{
			Mode:      model.HookHandlerModeExec,
			Program:   stringPointer("bash"),
			Arguments: []model.HookArgument{{Literal: &literal}, {PackageFile: &argumentPath}},
		},
		TimeoutMilliseconds: 5_000,
		FailurePolicy:       model.HookFailurePolicyClosed,
		Order:               42,
	}
	inventory := model.SourceInventory{Packages: []model.SourcePackage{{
		Identity: "bundle",
		Metadata: model.PackageMetadata{"nested": map[string]any{"items": []any{"source"}}},
		Assets: []model.SourceAsset{{
			Identity: "hook/check",
			Kind:     model.AssetKindHook,
			Targets:  []model.TargetID{model.TargetPi},
			Base: model.AssetContent{
				Frontmatter: map[string]any{"nested": map[string]any{"items": []any{"source"}}},
				Files: map[model.RelativePath]model.FileContent{
					payloadPath: {Bytes: []byte("source"), Executable: true, Origin: []model.SourceLocation{{Path: "src/hooks/check/scripts/check.sh", Line: &line}}},
				},
			},
			Hook: descriptor,
			CapabilityUses: []model.CapabilityUse{
				{Key: "asset.hook", Location: model.SourceLocation{Path: "src/hooks/check/hook.json", Line: &line}},
				{Key: "hook.failure.closed", Location: model.SourceLocation{Path: "src/hooks/check/hook.json", Line: &line}},
			},
			Overlays: []model.TargetOverlay{{
				Target: model.TargetPi,
				Files: []model.FilePatch{{Path: payloadPath, Content: model.FileContent{
					Bytes:  []byte("overlay"),
					Origin: []model.SourceLocation{{Path: "src/hooks/check/.agentbundler/targets/pi.json"}},
				}}},
				Acknowledgments: []model.Acknowledgment{{Asset: "hook/check", Target: model.TargetPi, Key: "hook.failure.closed", Reason: "Pi preserves the decision with review."}},
			}},
		}},
	}}}
	target := model.TargetComposition{Target: model.TargetPi, Capabilities: []model.CapabilityRule{
		{Key: "asset.hook", State: model.CapabilityStateNative},
		{Key: "hook.failure.closed", State: model.CapabilityStateAdvisory},
	}}

	packages, diagnostics := Compose(inventory, target)
	if len(diagnostics) != 0 {
		t.Fatalf("Compose diagnostics = %#v", diagnostics)
	}
	asset := packages[0].Assets[0]
	if asset.Hook == nil || asset.Hook == descriptor || asset.Hook.Order != 42 || asset.Hook.Location.Path != "src/hooks/check/hook.json" || asset.Hook.Handler.Arguments[1].PackageFile == &argumentPath {
		t.Fatalf("hook descriptor = %#v", asset.Hook)
	}
	if got := asset.Content.Files[payloadPath]; string(got.Bytes) != "overlay" || got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "src/hooks/check/.agentbundler/targets/pi.json"}}) {
		t.Fatalf("overlay payload = %#v", got)
	}
	if got := asset.CapabilityUses; len(got) != 2 || got[1].Location.Path != "src/hooks/check/hook.json" {
		t.Fatalf("capability uses = %#v", got)
	}
	if got := packages[0].Acknowledgments; len(got) != 1 || got[0].Key != "hook.failure.closed" {
		t.Fatalf("acknowledgments = %#v", got)
	}

	inventory.Packages[0].Metadata["nested"].(map[string]any)["items"].([]any)[0] = "mutated"
	inventory.Packages[0].Assets[0].Base.Frontmatter["nested"].(map[string]any)["items"].([]any)[0] = "mutated"
	inventory.Packages[0].Assets[0].Overlays[0].Files[0].Content.Bytes[0] = 'X'
	inventory.Packages[0].Assets[0].Overlays[0].Files[0].Content.Origin[0].Path = "mutated"
	inventory.Packages[0].Assets[0].CapabilityUses[0].Location.Path = "mutated"
	descriptor.Matcher.Tools[0] = model.HookToolCategoryWrite
	*descriptor.Handler.Arguments[0].Literal = "mutated"
	*descriptor.Handler.Arguments[1].PackageFile = "mutated"
	*descriptor.Location.Line = 99

	asset = packages[0].Assets[0]
	if got := packages[0].Metadata["nested"].(map[string]any)["items"].([]any)[0]; got != "source" {
		t.Fatalf("metadata aliased source: %#v", packages[0].Metadata)
	}
	if got := asset.Content.Frontmatter["nested"].(map[string]any)["items"].([]any)[0]; got != "source" {
		t.Fatalf("frontmatter aliased source: %#v", asset.Content.Frontmatter)
	}
	if got := asset.Content.Files[payloadPath]; string(got.Bytes) != "overlay" || got.Origin[0].Path != "src/hooks/check/.agentbundler/targets/pi.json" {
		t.Fatalf("payload aliased source: %#v", got)
	}
	if asset.CapabilityUses[0].Location.Path != "src/hooks/check/hook.json" || asset.Hook.Matcher.Tools[0] != model.HookToolCategoryCommand || *asset.Hook.Handler.Arguments[0].Literal != "-eu" || *asset.Hook.Handler.Arguments[1].PackageFile != payloadPath || *asset.Hook.Location.Line != 7 {
		t.Fatalf("normalized hook values aliased source: %#v, %#v", asset.Hook, asset.CapabilityUses)
	}
}

func TestComposeRejectsInvalidOverlayAndCapabilityCombinations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*model.SourceAsset)
		want   string
	}{
		{name: "replaced and deleted file", mutate: func(asset *model.SourceAsset) {
			asset.Overlays[0].DeletedFiles = []model.RelativePath{"scripts/check.sh"}
		}, want: "both replaced and deleted"},
		{name: "delete referenced hook payload", mutate: func(asset *model.SourceAsset) {
			asset.Overlays[0].Files = nil
			asset.Overlays[0].DeletedFiles = []model.RelativePath{"scripts/check.sh"}
		}, want: "deletes hook package file"},
		{name: "duplicate capability use", mutate: func(asset *model.SourceAsset) {
			asset.CapabilityUses = append(asset.CapabilityUses, asset.CapabilityUses[0])
		}, want: "capability use"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := validHookInventoryForComposition()
			tc.mutate(&inventory.Packages[0].Assets[0])
			packages, diagnostics := Compose(inventory, validHookTargetForComposition())
			if packages != nil || len(diagnostics) == 0 || diagnostics[0].Code != diagnosticCodeInvalidComposition || !diagnosticsContainText(diagnostics, tc.want) {
				t.Fatalf("Compose = (%#v, %#v), want %q", packages, diagnostics, tc.want)
			}
		})
	}
}

func TestComposeRejectsUnsupportedHookSecurityCapability(t *testing.T) {
	packages, diagnostics := Compose(validHookInventoryForComposition(), model.TargetComposition{
		Target:       model.TargetPi,
		Capabilities: []model.CapabilityRule{{Key: "hook.failure.closed", State: model.CapabilityStateUnsupported}},
	})
	if packages != nil || !diagnosticsContainText(diagnostics, `unsupported capability "hook.failure.closed"`) {
		t.Fatalf("Compose = (%#v, %#v)", packages, diagnostics)
	}
}

func TestComposeRejectsAmbiguousNativeGapAssetCollision(t *testing.T) {
	inventory := model.SourceInventory{
		Packages: []model.SourcePackage{
			{Identity: "first", Assets: []model.SourceAsset{{Identity: "native-resource/tool", Kind: model.AssetKindNativeResource}}},
			{Identity: "second", Assets: []model.SourceAsset{{Identity: "native-resource/tool", Kind: model.AssetKindNativeResource}}},
		},
		NativeGaps: []model.NativeGap{{Component: "tool", Asset: assetPointer("native-resource/tool"), Location: model.SourceLocation{Path: "native.json"}}},
	}
	packages, diagnostics := Compose(inventory, model.TargetComposition{
		Target:     model.TargetPi,
		NativeGaps: []model.NativeGapPolicy{{Component: "tool", Action: model.NativeGapActionExclude}},
	})
	if packages != nil || !diagnosticsContainText(diagnostics, `ambiguous across packages "first", "second"`) {
		t.Fatalf("Compose = (%#v, %#v)", packages, diagnostics)
	}
}

func TestNativeAssetSupportedByTargetIsExplicitForPiAndAntigravity(t *testing.T) {
	piAsset := model.SourceAsset{Kind: model.AssetKindNativeResource, Native: &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extensions/custom.ts"}}}
	antigravityAsset := model.SourceAsset{Kind: model.AssetKindNativeResource}
	for _, test := range []struct {
		name   string
		asset  model.SourceAsset
		target model.TargetID
		want   bool
	}{
		{name: "Pi declarative extension", asset: piAsset, target: model.TargetPi, want: true},
		{name: "Pi rejects undeclared native resource", asset: antigravityAsset, target: model.TargetPi},
		{name: "Antigravity explicit tree", asset: antigravityAsset, target: model.TargetAntigravity, want: true},
		{name: "Antigravity rejects Pi declaration", asset: piAsset, target: model.TargetAntigravity},
		{name: "other target rejects native resource", asset: antigravityAsset, target: model.TargetClaude},
		{name: "non-native asset rejected", asset: model.SourceAsset{Kind: model.AssetKindResource}, target: model.TargetAntigravity},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeAssetSupportedByTarget(test.asset, test.target); got != test.want {
				t.Fatalf("nativeAssetSupportedByTarget() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestComposeIncludesDeclarativePiNativeResourceWithoutGapPolicy(t *testing.T) {
	asset := model.SourceAsset{
		Identity: "native-resource/extensions", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetPi},
		Base:           model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{"extensions/custom.ts": {Bytes: []byte("export default {}")}}},
		Native:         &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extensions/custom.ts"}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.native-resource", Location: model.SourceLocation{Path: "plugins/pi/extensions/.agentbundler/asset.json"}}},
	}
	inventory := model.SourceInventory{Packages: []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{asset}}}, NativeGaps: []model.NativeGap{{Component: "extensions", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetPi), Location: model.SourceLocation{Path: "plugins/pi/extensions"}}}}
	packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetPi, Capabilities: []model.CapabilityRule{{Key: "asset.native-resource", State: model.CapabilityStateNative}}})
	if len(diagnostics) != 0 {
		t.Fatalf("Compose() diagnostics = %#v", diagnostics)
	}
	if len(packages) != 1 || len(packages[0].Assets) != 1 || packages[0].Assets[0].Native == nil {
		t.Fatalf("Compose() packages = %#v", packages)
	}
}

func TestComposeRejectsWrongTargetSelectedNativeResource(t *testing.T) {
	asset := model.SourceAsset{
		Identity: "native-resource/foo", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetAntigravity},
		Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{"rules/foo.md": {Bytes: []byte("rule\n")}}},
	}
	inventory := model.SourceInventory{
		Packages:   []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{asset}}},
		NativeGaps: []model.NativeGap{{Component: "foo", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetClaude), Location: model.SourceLocation{Path: "src/plugins/claude/foo"}}},
	}

	packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetAntigravity})
	if packages != nil || !diagnosticsContainText(diagnostics, `native resource asset "native-resource/foo" belongs to target "claude" by path but is selected for target "antigravity"`) {
		t.Fatalf("Compose() = (%#v, %#v)", packages, diagnostics)
	}
}

func TestComposeIncludesAntigravityNativeResourceOnlyForItsTargetWithoutAliasing(t *testing.T) {
	line := 9
	asset := model.SourceAsset{
		Identity: "native-resource/conductor", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetAntigravity},
		Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{
			"rules/conductor.md": {Bytes: []byte("rule\n"), Origin: []model.SourceLocation{{Path: "plugins/antigravity/conductor/rules/conductor.md", Line: &line}}},
			"scripts/check.sh":   {Bytes: []byte("#!/bin/sh\n"), Executable: true, Origin: []model.SourceLocation{{Path: "plugins/antigravity/conductor/scripts/check.sh"}}},
		}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.native-resource", Location: model.SourceLocation{Path: "plugins/antigravity/conductor/.agentbundler/asset.json"}}},
	}
	inventory := model.SourceInventory{
		Packages:   []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{asset}}},
		NativeGaps: []model.NativeGap{{Component: "conductor", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetAntigravity), Location: model.SourceLocation{Path: "plugins/antigravity/conductor"}}},
	}
	target := model.TargetComposition{Target: model.TargetAntigravity, Capabilities: []model.CapabilityRule{{Key: "asset.native-resource", State: model.CapabilityStateNative}}}

	packages, diagnostics := Compose(inventory, target)
	if len(diagnostics) != 0 {
		t.Fatalf("Compose() diagnostics = %#v", diagnostics)
	}
	second, secondDiagnostics := Compose(inventory, target)
	if len(secondDiagnostics) != 0 || !reflect.DeepEqual(packages, second) {
		t.Fatalf("second Compose() = (%#v, %#v), want deterministic clone %#v", second, secondDiagnostics, packages)
	}
	if len(packages) != 1 || len(packages[0].Assets) != 1 {
		t.Fatalf("Compose() packages = %#v", packages)
	}
	normalized := packages[0].Assets[0]
	if normalized.Identity != asset.Identity || normalized.Native != nil {
		t.Fatalf("normalized asset = %#v", normalized)
	}
	if got := normalized.Content.Files["rules/conductor.md"]; string(got.Bytes) != "rule\n" || got.Executable || len(got.Origin) != 1 || got.Origin[0].Path != "plugins/antigravity/conductor/rules/conductor.md" || got.Origin[0].Line == nil || *got.Origin[0].Line != 9 {
		t.Fatalf("rule file = %#v", got)
	}
	if got := normalized.Content.Files["scripts/check.sh"]; string(got.Bytes) != "#!/bin/sh\n" || !got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "plugins/antigravity/conductor/scripts/check.sh"}}) {
		t.Fatalf("script file = %#v", got)
	}

	inventory.Packages[0].Assets[0].Base.Files["rules/conductor.md"].Bytes[0] = 'X'
	*inventory.Packages[0].Assets[0].Base.Files["rules/conductor.md"].Origin[0].Line = 99
	inventory.Packages[0].Assets[0].CapabilityUses[0].Location.Path = "mutated"
	got := packages[0].Assets[0]
	if string(got.Content.Files["rules/conductor.md"].Bytes) != "rule\n" || *got.Content.Files["rules/conductor.md"].Origin[0].Line != 9 || got.CapabilityUses[0].Location.Path != "plugins/antigravity/conductor/.agentbundler/asset.json" {
		t.Fatalf("normalized asset aliased source = %#v", got)
	}

	wrongTargetPackages, wrongTargetDiagnostics := Compose(model.SourceInventory{
		Packages:   []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{asset}}},
		NativeGaps: []model.NativeGap{{Component: "conductor", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetAntigravity), Location: model.SourceLocation{Path: "plugins/antigravity/conductor"}}},
	}, model.TargetComposition{Target: model.TargetClaude})
	if len(wrongTargetDiagnostics) != 0 || len(wrongTargetPackages) != 1 || len(wrongTargetPackages[0].Assets) != 0 {
		t.Fatalf("wrong-target Compose() = (%#v, %#v)", wrongTargetPackages, wrongTargetDiagnostics)
	}
}

func TestComposeIncludesEmptyAntigravityNativeResourceForAdapterValidation(t *testing.T) {
	asset := model.SourceAsset{
		Identity: "native-resource/empty", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetAntigravity},
		Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}},
	}
	inventory := model.SourceInventory{
		Packages:   []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{asset}}},
		NativeGaps: []model.NativeGap{{Component: "empty", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetAntigravity), Location: model.SourceLocation{Path: "plugins/antigravity/empty"}}},
	}

	packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetAntigravity})
	if len(diagnostics) != 0 || len(packages) != 1 || len(packages[0].Assets) != 1 || len(packages[0].Assets[0].Content.Files) != 0 {
		t.Fatalf("Compose() = (%#v, %#v)", packages, diagnostics)
	}
}

func TestComposeAntigravityNativeGapPolicies(t *testing.T) {
	replacement := model.AssetID("skill/replacement")
	for _, test := range []struct {
		name        string
		action      model.NativeGapAction
		replacement *model.AssetID
		wantAsset   model.AssetID
	}{
		{name: "replace", action: model.NativeGapActionReplace, replacement: &replacement, wantAsset: replacement},
		{name: "exclude", action: model.NativeGapActionExclude},
		{name: "source only", action: model.NativeGapActionSourceOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			asset := model.SourceAsset{
				Identity: "native-resource/conductor", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetAntigravity},
				Base:   model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{"extensions/pi.ts": {Bytes: []byte("export {}")}}},
				Native: &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extensions/pi.ts"}},
			}
			inventory := model.SourceInventory{
				Packages: []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{
					asset,
					{Identity: replacement, Kind: model.AssetKindSkill, Targets: []model.TargetID{model.TargetAntigravity}, Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}}},
				}}},
				NativeGaps: []model.NativeGap{{Component: "conductor", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetAntigravity), Location: model.SourceLocation{Path: "plugins/antigravity/conductor"}}},
			}
			packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetAntigravity, NativeGaps: []model.NativeGapPolicy{{Component: "conductor", Action: test.action, Replacement: test.replacement}}})
			if len(diagnostics) != 0 || len(packages) != 1 {
				t.Fatalf("Compose() = (%#v, %#v)", packages, diagnostics)
			}
			assets := packages[0].Assets
			if test.wantAsset == "" {
				if len(assets) != 1 || assets[0].Identity != replacement {
					t.Fatalf("assets = %#v, want only independently selected replacement", assets)
				}
				return
			}
			if len(assets) != 1 || assets[0].Identity != test.wantAsset {
				t.Fatalf("assets = %#v, want %q", assets, test.wantAsset)
			}
		})
	}

	asset := model.SourceAsset{
		Identity: "native-resource/conductor", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetAntigravity},
		Base:   model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{"extensions/pi.ts": {Bytes: []byte("export {}")}}},
		Native: &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extensions/pi.ts"}},
	}
	packages, diagnostics := Compose(model.SourceInventory{
		Packages:   []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{asset}}},
		NativeGaps: []model.NativeGap{{Component: "conductor", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetAntigravity), Location: model.SourceLocation{Path: "plugins/antigravity/conductor"}}},
	}, model.TargetComposition{Target: model.TargetAntigravity})
	if packages != nil || !diagnosticsContainText(diagnostics, `native gap "conductor" has no policy`) {
		t.Fatalf("Compose() = (%#v, %#v), want Pi-declaration gap policy error", packages, diagnostics)
	}
}

func TestComposeRejectsDuplicateAntigravityNativeResourceReferences(t *testing.T) {
	asset := model.SourceAsset{Identity: "native-resource/conductor", Kind: model.AssetKindNativeResource, Targets: []model.TargetID{model.TargetAntigravity}, Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}}}
	inventory := model.SourceInventory{
		Packages:   []model.SourcePackage{{Identity: "first", Assets: []model.SourceAsset{asset}}, {Identity: "second", Assets: []model.SourceAsset{asset}}},
		NativeGaps: []model.NativeGap{{Component: "conductor", Asset: assetPointer(asset.Identity), Target: targetPointer(model.TargetAntigravity), Location: model.SourceLocation{Path: "plugins/antigravity/conductor"}}},
	}

	packages, diagnostics := Compose(inventory, model.TargetComposition{Target: model.TargetAntigravity})
	if packages != nil || !diagnosticsContainText(diagnostics, `ambiguous across packages "first", "second"`) {
		t.Fatalf("Compose() = (%#v, %#v)", packages, diagnostics)
	}
}

func TestComposeNativeGapPolicies(t *testing.T) {
	replacement := model.AssetID("skill/replacement")
	self := model.AssetID("native-resource/tool")
	cases := []struct {
		name        string
		action      model.NativeGapAction
		replacement *model.AssetID
		wantError   bool
	}{
		{name: "replace", action: model.NativeGapActionReplace, replacement: &replacement},
		{name: "replacement unavailable for target", action: model.NativeGapActionReplace, replacement: &replacement, wantError: true},
		{name: "self replacement", action: model.NativeGapActionReplace, replacement: &self, wantError: true},
		{name: "exclude", action: model.NativeGapActionExclude},
		{name: "source only", action: model.NativeGapActionSourceOnly},
		{name: "missing", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := model.SourceInventory{
				Packages: []model.SourcePackage{{Identity: "bundle", Assets: []model.SourceAsset{
					{Identity: "native-resource/tool", Kind: model.AssetKindNativeResource, Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}}},
					{Identity: "skill/replacement", Kind: model.AssetKindSkill, Base: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath]model.FileContent{}}},
				}}},
				NativeGaps: []model.NativeGap{{Component: "tool", Asset: assetPointer("native-resource/tool"), Location: model.SourceLocation{Path: "native.json"}}},
			}
			target := model.TargetComposition{Target: model.TargetPi}
			if tc.name == "replacement unavailable for target" {
				inventory.Packages[0].Assets[1].Targets = []model.TargetID{model.TargetClaude}
			}
			if tc.name != "missing" {
				target.NativeGaps = []model.NativeGapPolicy{{Component: "tool", Action: tc.action, Replacement: tc.replacement}}
			}
			packages, diagnostics := Compose(inventory, target)
			if tc.wantError {
				if packages != nil || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticCodeInvalidComposition {
					t.Errorf("Compose = (%#v, %#v)", packages, diagnostics)
				}
				if tc.name == "replacement unavailable for target" && !strings.Contains(diagnostics[0].Message, "unavailable") {
					t.Errorf("diagnostic = %q, want unavailable replacement", diagnostics[0].Message)
				}
				return
			}
			if len(diagnostics) != 0 {
				t.Fatalf("Compose diagnostics = %#v", diagnostics)
			}
			if got := packages[0].Assets; len(got) != 1 || got[0].Identity != "skill/replacement" {
				t.Errorf("assets = %#v", got)
			}
		})
	}
}

func validHookInventoryForComposition() model.SourceInventory {
	payloadPath := model.RelativePath("scripts/check.sh")
	return model.SourceInventory{Packages: []model.SourcePackage{{
		Identity: "bundle",
		Assets: []model.SourceAsset{{
			Identity: "hook/check",
			Kind:     model.AssetKindHook,
			Base: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
				payloadPath: {Bytes: []byte("source")},
			}},
			Hook: &model.HookDescriptor{
				Identity: "hook/check",
				Location: model.SourceLocation{Path: "src/hooks/check/hook.json"},
				Event:    model.HookEventPreTool,
				Handler: model.HookCommand{
					Mode:      model.HookHandlerModeExec,
					Program:   stringPointer("bash"),
					Arguments: []model.HookArgument{{PackageFile: &payloadPath}},
				},
				TimeoutMilliseconds: 1_000,
				FailurePolicy:       model.HookFailurePolicyClosed,
			},
			CapabilityUses: []model.CapabilityUse{{Key: "hook.failure.closed", Location: model.SourceLocation{Path: "src/hooks/check/hook.json"}}},
			Overlays: []model.TargetOverlay{{Target: model.TargetPi, Files: []model.FilePatch{{
				Path:    payloadPath,
				Content: model.FileContent{Bytes: []byte("overlay")},
			}}}},
		}},
	}}}
}

func validHookTargetForComposition() model.TargetComposition {
	return model.TargetComposition{Target: model.TargetPi, Capabilities: []model.CapabilityRule{{
		Key: "hook.failure.closed", State: model.CapabilityStateNative,
	}}}
}

func diagnosticsContainText(diagnostics []model.Diagnostic, text string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}

func oneAssetInventory(body string, patch model.BodyPatch) model.SourceInventory {
	return model.SourceInventory{Packages: []model.SourcePackage{{
		Identity: "bundle",
		Assets: []model.SourceAsset{{
			Identity: "skill/demo",
			Kind:     model.AssetKindSkill,
			Base:     model.AssetContent{Frontmatter: map[string]any{}, Body: body, Files: map[model.RelativePath]model.FileContent{}},
			Overlays: []model.TargetOverlay{{Target: model.TargetPi, BodyPatch: &patch}},
		}},
	}}}
}

func targetPointer(value model.TargetID) *model.TargetID {
	return &value
}

func assetPointer(value model.AssetID) *model.AssetID {
	return &value
}
