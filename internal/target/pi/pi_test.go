package pi

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderUsesPiNativeSkillRoot(t *testing.T) {
	plan, diagnostics := Render(skillPackage())
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path}, []model.RelativePath{".pi/skills/guide/SKILL.md", ".pi/skills/guide/docs/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
}

func TestProjectRenderRejectsAgent(t *testing.T) {
	pkg := skillPackage()[0]
	pkg.Assets[0].Kind = model.AssetKindAgent
	pkg.Assets[0].Identity = "agent/reviewer"
	_, diagnostics := Render([]model.NormalizedPackage{pkg})
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
}

func TestPackageRenderIncludesPiSubagent(t *testing.T) {
	pkg := model.NormalizedPackage{
		Identity: "demo",
		Target:   Target,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{"version": "1.0.0", "dependencies": map[string]any{"pi-subagents": "^1.0.0"}},
		Assets: []model.NormalizedAsset{{
			Identity: "agent/reviewer",
			Kind:     model.AssetKindAgent,
			Content: model.AssetContent{
				Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"},
				Body:        "Review.\n",
				Files:       map[model.RelativePath]model.FileContent{},
			},
		}},
	}
	plan, diagnostics := Render([]model.NormalizedPackage{pkg})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	for _, file := range plan.Files {
		if file.Path == "agents/reviewer.md" {
			return
		}
	}
	t.Fatalf("plan files = %#v, want Pi subagent", plan.Files)
}

func TestCapabilitiesExposePiSubagentEquivalent(t *testing.T) {
	for _, rule := range Capabilities() {
		if rule.Key == "asset.agent" {
			if rule.State != model.CapabilityStateEquivalent {
				t.Fatalf("agent capability = %q, want equivalent", rule.State)
			}
			return
		}
	}
	t.Fatal("agent capability is missing")
}

func skillPackage() []model.NormalizedPackage {
	return []model.NormalizedPackage{{Identity: "demo", Target: Target, Assets: []model.NormalizedAsset{{
		Identity: "skill/guide", Kind: model.AssetKindSkill,
		Content:        model.AssetContent{Frontmatter: map[string]any{"name": "guide"}, Body: "# Guide\n", Files: map[model.RelativePath]model.FileContent{"docs/readme.md": {Bytes: []byte("help")}}},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source/SKILL.md"}}},
	}}}}
}
