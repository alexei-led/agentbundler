package skills

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderProducesDeterministicNativeSkillTree(t *testing.T) {
	pkg := model.NormalizedPackage{
		Identity: "demo",
		Target:   model.TargetPi,
		Assets: []model.NormalizedAsset{{
			Identity: "skill/guide",
			Kind:     model.AssetKindSkill,
			Content: model.AssetContent{
				Frontmatter: map[string]any{"description": "Guide", "name": "guide"},
				Body:        "# Guide\n",
				Files:       map[model.RelativePath][]byte{"docs/readme.md": []byte("help")},
			},
			CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/SKILL.md"}}},
		}},
	}
	plan, diagnostics := Render(model.TargetPi, ".pi/skills", []model.NormalizedPackage{pkg})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path}, []model.RelativePath{".pi/skills/guide/SKILL.md", ".pi/skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if got, want := string(plan.Files[0].Bytes), "---\n{\"description\":\"Guide\",\"name\":\"guide\"}\n---\n# Guide\n"; got != want {
		t.Fatalf("SKILL.md = %q, want %q", got, want)
	}
}

func TestRenderProducesSiblingResourceTree(t *testing.T) {
	pkg := model.NormalizedPackage{
		Identity: "demo",
		Target:   model.TargetCopilot,
		Assets: []model.NormalizedAsset{
			{
				Identity: "skill/guide",
				Kind:     model.AssetKindSkill,
				Content:  model.AssetContent{Body: "# Guide\n", Files: map[model.RelativePath][]byte{}},
			},
			{
				Identity: "resource/templates",
				Kind:     model.AssetKindResource,
				Content:  model.AssetContent{Files: map[model.RelativePath][]byte{"report.md": []byte("# Report\n")}},
			},
		},
	}
	plan, diagnostics := RenderProject(model.TargetCopilot, ".github/skills", ".github/resources", []model.NormalizedPackage{pkg})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	got := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path}
	want := []model.RelativePath{".github/resources/templates/report.md", ".github/skills/guide/SKILL.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestRenderRejectsUnsupportedSubsetAndAggregation(t *testing.T) {
	base := model.NormalizedPackage{Identity: "demo", Target: model.TargetPi}
	agent := base
	agent.Assets = []model.NormalizedAsset{{Identity: "agent/reviewer", Kind: model.AssetKindAgent}}
	resource := base
	resource.Assets = []model.NormalizedAsset{{Identity: "resource/templates", Kind: model.AssetKindResource}}
	capability := base
	capability.Assets = []model.NormalizedAsset{{
		Identity: "skill/guide",
		Kind:     model.AssetKindSkill,
		CapabilityUses: []model.CapabilityUse{{
			Key: "tool.use", Location: model.SourceLocation{Path: "source"},
		}},
	}}
	for _, tc := range []struct {
		name     string
		packages []model.NormalizedPackage
	}{
		{name: "multiple packages", packages: []model.NormalizedPackage{base, {Identity: "other", Target: model.TargetPi}}},
		{name: "agent", packages: []model.NormalizedPackage{agent}},
		{name: "resource without project resource root", packages: []model.NormalizedPackage{resource}},
		{name: "capability", packages: []model.NormalizedPackage{capability}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, diagnostics := Render(model.TargetPi, ".pi/skills", tc.packages)
			if len(plan.Files) != 0 || len(diagnostics) != 1 || diagnostics[0].Severity != model.SeverityError {
				t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
			}
		})
	}
}
