package packageoutput

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderClaudePackageWithSkillsAgentsAndResources(t *testing.T) {
	plan, diagnostics := Render(model.TargetClaude, []model.NormalizedPackage{packageFixture(model.TargetClaude)})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	paths := make([]model.RelativePath, len(plan.Files))
	for index, file := range plan.Files {
		paths[index] = file.Path
	}
	want := []model.RelativePath{
		".claude-plugin/plugin.json",
		"agents/reviewer.md",
		"resources/templates/design.md",
		"skills/demo/SKILL.md",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	var manifest map[string]any
	if err := json.Unmarshal(plan.Files[0].Bytes, &manifest); err != nil {
		t.Fatalf("manifest JSON: %v", err)
	}
	if manifest["name"] != "demo" || manifest["version"] != "1.0.0" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestRenderCodexPackageAgentIsStandaloneTOML(t *testing.T) {
	plan, diagnostics := Render(model.TargetCodex, []model.NormalizedPackage{packageFixture(model.TargetCodex)})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	for _, file := range plan.Files {
		if file.Path == "agents/reviewer.toml" {
			text := string(file.Bytes)
			if !containsAll(text, `name = "reviewer"`, `description = "Review code"`, `developer_instructions = """`) {
				t.Fatalf("agent TOML = %q", text)
			}
			return
		}
	}
	t.Fatal("standalone agent is missing")
}

func TestRenderPiPackageRejectsAgent(t *testing.T) {
	_, diagnostics := Render(model.TargetPi, []model.NormalizedPackage{packageFixture(model.TargetPi)})
	if len(diagnostics) == 0 || diagnostics[0].Code != "invalid-package-output" {
		t.Fatalf("diagnostics = %#v, want invalid-package-output", diagnostics)
	}
}

func packageFixture(target model.TargetID) model.NormalizedPackage {
	return model.NormalizedPackage{
		Identity: "demo",
		Target:   target,
		Profile:  model.TargetProfilePackage,
		Metadata: model.PackageMetadata{
			"version":     "1.0.0",
			"description": "Demo package",
		},
		Assets: []model.NormalizedAsset{
			{Identity: "skill/demo", Kind: model.AssetKindSkill, Content: model.AssetContent{Frontmatter: map[string]any{"name": "demo", "description": "Demo"}, Body: "Use demo.\n", Files: map[model.RelativePath][]byte{}}},
			{Identity: "agent/reviewer", Kind: model.AssetKindAgent, Content: model.AssetContent{Frontmatter: map[string]any{"name": "reviewer", "description": "Review code"}, Body: "Review.\n", Files: map[model.RelativePath][]byte{}}},
			{Identity: "resource/templates", Kind: model.AssetKindResource, Content: model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath][]byte{"design.md": []byte("# Design\n")}}},
		},
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !contains(value, part) {
			return false
		}
	}
	return true
}

func contains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
