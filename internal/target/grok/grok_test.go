package grok

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderUsesGrokNativeSkillRoot(t *testing.T) {
	plan, diagnostics := Render(skillPackage())
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path, plan.Files[2].Path}, []model.RelativePath{".grok/resources/templates/report.md", ".grok/skills/guide/SKILL.md", ".grok/skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestCapabilitiesSupportPortableResources(t *testing.T) {
	for _, rule := range Capabilities() {
		if rule.Key == "asset.resource" {
			if rule.State != model.CapabilityStateNative {
				t.Fatalf("resource capability = %q, want native", rule.State)
			}
			return
		}
	}
	t.Fatal("resource capability is missing")
}

func TestRenderRejectsNonSkillAssets(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Assets[0].Kind = model.AssetKindAgent
	pkg.Assets[0].Identity = "agent/reviewer"
	_, diagnostics := Render([]model.NormalizedPackage{pkg})
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func skillPackage() []model.NormalizedPackage {
	return []model.NormalizedPackage{{Identity: "demo", Target: Target, Assets: []model.NormalizedAsset{
		{
			Identity: "skill/guide", Kind: model.AssetKindSkill,
			Content:        model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "# Guide\n", Files: map[model.RelativePath][]byte{"docs/readme.md": []byte("help")}},
			CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/SKILL.md"}}},
		},
		{
			Identity: "resource/templates", Kind: model.AssetKindResource,
			Content: model.AssetContent{Files: map[model.RelativePath][]byte{"report.md": []byte("# Report\n")}},
		},
	}}}
}
