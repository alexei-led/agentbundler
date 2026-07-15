package codex

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderUsesCodexPluginLayout(t *testing.T) {
	plan, diagnostics := Render(separate(skillPackage()))
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	paths := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path, plan.Files[2].Path}
	if want := []model.RelativePath{".codex-plugin/plugin.json", "skills/guide/SKILL.md", "skills/guide/docs/readme.md"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	if got, want := string(plan.Files[0].Bytes), "{\"description\":\"Demo\",\"name\":\"demo\",\"skills\":\"./skills\",\"version\":\"1.0.0\"}\n"; got != want {
		t.Fatalf("manifest = %q, want %q", got, want)
	}
}

func TestRenderRejectsInvalidPluginName(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Identity = "Demo Plugin"
	_, diagnostics := Render(separate([]model.NormalizedPackage{pkg}))
	if len(diagnostics) != 1 || diagnostics[0].Code != "invalid-plugin-name" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func separate(packages []model.NormalizedPackage) model.TargetRenderInput {
	return model.TargetRenderInput{Packages: packages, PackageMode: model.TargetPackageModeSeparate}
}

func skillPackage() []model.NormalizedPackage {
	return []model.NormalizedPackage{{Identity: "demo", Target: Target, Metadata: model.PackageMetadata{"version": "1.0.0", "description": "Demo"}, Assets: []model.NormalizedAsset{{
		Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Body: "# Guide\n", Files: map[model.RelativePath]model.FileContent{"docs/readme.md": {Bytes: []byte("help")}}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/SKILL.md"}}},
	}}}}
}
