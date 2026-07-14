package cursor

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderUsesCursorPluginLayout(t *testing.T) {
	plan, diagnostics := New().Render(skillPackage())
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	paths := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path, plan.Files[2].Path}
	if want := []model.RelativePath{".cursor-plugin/plugin.json", "skills/guide/SKILL.md", "skills/guide/docs/readme.md"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	if got, want := string(plan.Files[0].Bytes), "{\"description\":\"Demo\",\"name\":\"demo\",\"skills\":\"./skills/\",\"version\":\"1.0.0\"}\n"; got != want {
		t.Fatalf("manifest = %q, want %q", got, want)
	}
}

func TestRenderPackageProfileIncludesAgent(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Profile = model.TargetProfilePackage
	pkg.Assets = append(pkg.Assets, model.NormalizedAsset{
		Identity: "agent/reviewer",
		Kind:     model.AssetKindAgent,
		Content: model.AssetContent{
			Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"},
			Body:        "Review.\n",
			Files:       map[model.RelativePath][]byte{},
		},
	})
	plan, diagnostics := New().Render([]model.NormalizedPackage{pkg})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	paths := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		paths[index] = file.Path
	}
	if !reflect.DeepEqual(paths, []model.RelativePath{".cursor-plugin/plugin.json", "README.md", "agents/reviewer.md", "skills/guide/SKILL.md", "skills/guide/docs/readme.md"}) {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestRenderProjectProfileRejectsAgent(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Assets[0].Identity = "agent/reviewer"
	pkg.Assets[0].Kind = model.AssetKindAgent
	_, diagnostics := New().Render([]model.NormalizedPackage{pkg})
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func skillPackage() []model.NormalizedPackage {
	return []model.NormalizedPackage{{Identity: "demo", Target: model.TargetCursor, Metadata: model.PackageMetadata{"version": "1.0.0", "description": "Demo"}, Assets: []model.NormalizedAsset{{
		Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Body: "# Guide\n", Files: map[model.RelativePath][]byte{"docs/readme.md": []byte("help")}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/SKILL.md"}}},
	}}}}
}
