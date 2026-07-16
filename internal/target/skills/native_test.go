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
				Files:       map[model.RelativePath]model.FileContent{"docs/readme.md": {Bytes: []byte("help")}},
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
				Content:  model.AssetContent{Body: "# Guide\n", Files: map[model.RelativePath]model.FileContent{}},
			},
			{
				Identity: "resource/templates",
				Kind:     model.AssetKindResource,
				Content:  model.AssetContent{Files: map[model.RelativePath]model.FileContent{"report.md": {Bytes: []byte("# Report\n")}}},
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

func TestRenderProjectPreservesSupportFileMetadata(t *testing.T) {
	line := 7
	scriptOrigin := []model.SourceLocation{{Path: "source/skills/guide/scripts/run.sh", Line: &line}}
	resourceOrigin := []model.SourceLocation{{Path: "source/resources/templates/report.md"}}
	scriptBytes := []byte("#!/bin/sh\n")
	pkg := model.NormalizedPackage{
		Identity: "demo",
		Target:   model.TargetCopilot,
		Assets: []model.NormalizedAsset{
			{
				Identity: "skill/guide",
				Kind:     model.AssetKindSkill,
				Content: model.AssetContent{Body: "# Guide\n", Files: map[model.RelativePath]model.FileContent{
					"scripts/run.sh": {Bytes: scriptBytes, Executable: true, Origin: scriptOrigin},
				}},
			},
			{
				Identity: "resource/templates",
				Kind:     model.AssetKindResource,
				Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
					"report.md": {Bytes: []byte("# Report\n"), Origin: resourceOrigin},
				}},
			},
		},
	}

	plan, diagnostics := RenderProject(model.TargetCopilot, ".github/skills", ".github/resources", []model.NormalizedPackage{pkg})
	if len(diagnostics) != 0 {
		t.Fatalf("RenderProject() diagnostics = %#v", diagnostics)
	}
	files := make(map[model.RelativePath]model.PlannedFile, len(plan.Files))
	for _, file := range plan.Files {
		files[file.Path] = file
	}
	script := files[".github/skills/guide/scripts/run.sh"]
	if !script.Executable || !reflect.DeepEqual(script.Origin, scriptOrigin) {
		t.Fatalf("script metadata = %#v, want executable with origin %#v", script, scriptOrigin)
	}
	resource := files[".github/resources/templates/report.md"]
	if resource.Executable || !reflect.DeepEqual(resource.Origin, resourceOrigin) {
		t.Fatalf("resource metadata = %#v, want non-executable with origin %#v", resource, resourceOrigin)
	}

	script.Bytes[0] = 'X'
	*script.Origin[0].Line = 99
	if string(scriptBytes) != "#!/bin/sh\n" || line != 7 {
		t.Fatal("planned support file bytes or origin alias normalized input")
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
