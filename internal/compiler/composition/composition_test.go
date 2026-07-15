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

func TestComposeNativeGapPolicies(t *testing.T) {
	replacement := model.AssetID("skill/replacement")
	cases := []struct {
		name        string
		action      model.NativeGapAction
		replacement *model.AssetID
		wantError   bool
	}{
		{name: "replace", action: model.NativeGapActionReplace, replacement: &replacement},
		{name: "replacement unavailable for target", action: model.NativeGapActionReplace, replacement: &replacement, wantError: true},
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

func assetPointer(value model.AssetID) *model.AssetID {
	return &value
}
