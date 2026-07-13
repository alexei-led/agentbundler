package plugin

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderAddsManifestToNativeSkillTree(t *testing.T) {
	packages := []model.NormalizedPackage{{Identity: "demo", Target: model.TargetCursor, Assets: []model.NormalizedAsset{{
		Identity: "skill/guide", Kind: model.AssetKindSkill, Content: model.AssetContent{Body: "guide"},
		CapabilityUses: []model.CapabilityUse{{Key: "asset.skill", Location: model.SourceLocation{Path: "source"}}},
	}}}}
	plan, diagnostics := Render(model.TargetCursor, ".cursor-plugin/plugin.json", packages, map[string]any{"name": "demo", "skills": "./skills/"})
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{plan.Files[0].Path, plan.Files[1].Path}, []model.RelativePath{".cursor-plugin/plugin.json", "skills/guide/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if got, want := string(plan.Files[0].Bytes), "{\"name\":\"demo\",\"skills\":\"./skills/\"}\n"; got != want {
		t.Fatalf("manifest = %q, want %q", got, want)
	}
}

func TestRenderRejectsInvalidPluginName(t *testing.T) {
	plan, diagnostics := Render(model.TargetCursor, ".cursor-plugin/plugin.json", []model.NormalizedPackage{{Identity: "Not Valid", Target: model.TargetCursor}}, map[string]any{"name": "bad"})
	if len(plan.Files) != 0 || len(diagnostics) != 1 || diagnostics[0].Code != "invalid-plugin-name" {
		t.Fatalf("Render() = (%#v, %#v)", plan, diagnostics)
	}
}
