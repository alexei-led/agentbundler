package cursor

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderDeterministicBaseline(t *testing.T) {
	adapter := New()
	first, diagnostics := adapter.Render(cursorPackages(false))
	if len(diagnostics) != 0 {
		t.Fatalf("first render diagnostics = %v", diagnostics)
	}
	second, diagnostics := adapter.Render(cursorPackages(true))
	if len(diagnostics) != 0 {
		t.Fatalf("second render diagnostics = %v", diagnostics)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("renders differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}

	wantPaths := []model.RelativePath{
		"package-index.json",
		"packages/Team%20%CF%80/assets/agent/reviewer/asset.json",
		"packages/Team%20%CF%80/assets/agent/reviewer/content.md",
		"packages/Team%20%CF%80/assets/skill/research/asset.json",
		"packages/Team%20%CF%80/assets/skill/research/content.md",
		"packages/Team%20%CF%80/assets/skill/research/files/tools/run.sh",
		"packages/Team%20%CF%80/package.json",
		"packages/zeta/assets/skill/small/asset.json",
		"packages/zeta/assets/skill/small/content.md",
		"packages/zeta/package.json",
	}
	if len(first.Files) != len(wantPaths) {
		t.Fatalf("planned file count = %d, want %d", len(first.Files), len(wantPaths))
	}
	for index, want := range wantPaths {
		if first.Files[index].Path != want {
			t.Errorf("file %d path = %q, want %q", index, first.Files[index].Path, want)
		}
		if first.Files[index].Executable {
			t.Errorf("file %q is executable", first.Files[index].Path)
		}
	}
	if got, want := string(first.Files[0].Bytes), "{\"format\":\"agentbundler-target-bundle\",\"formatRevision\":1,\"packages\":[\"Team π\",\"zeta\"],\"target\":\"cursor\"}\n"; got != want {
		t.Errorf("package index = %q, want %q", got, want)
	}
	if got, want := string(first.Files[6].Bytes), "{\"identity\":\"Team π\",\"metadata\":{\"priority\":2,\"topic\":\"cursor\"},\"target\":\"cursor\"}\n"; got != want {
		t.Errorf("package metadata = %q, want %q", got, want)
	}
}

func TestRenderRejectsUnsupportedAssets(t *testing.T) {
	for _, kind := range []model.AssetKind{model.AssetKindHook, model.AssetKindNativeResource} {
		t.Run(string(kind), func(t *testing.T) {
			plan, diagnostics := New().Render([]model.NormalizedPackage{{
				Identity: "example",
				Target:   model.TargetCursor,
				Assets: []model.NormalizedAsset{{
					Identity: model.AssetID(string(kind) + "/example"),
					Kind:     kind,
					Content:  model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath][]byte{}},
				}},
			}})
			if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported-capability" {
				t.Fatalf("diagnostics = %v, want unsupported capability diagnostic", diagnostics)
			}
			if len(plan.Files) != 0 {
				t.Errorf("unsupported %q planned files = %v, want none", kind, plan.Files)
			}
		})
	}
}

func cursorPackages(reverse bool) []model.NormalizedPackage {
	uses := []model.CapabilityUse{
		{Key: "instruction", Location: model.SourceLocation{Path: "skills/research.md", Line: intPointer(4)}},
		{Key: "instruction", Location: model.SourceLocation{Path: "skills/research.md", Line: intPointer(2)}},
	}
	assets := []model.NormalizedAsset{
		{
			Identity: "skill/research",
			Kind:     model.AssetKindSkill,
			Content: model.AssetContent{
				Frontmatter: map[string]any{"description": "research"},
				Body:        "Research carefully.\n",
				Files:       map[model.RelativePath][]byte{"tools/run.sh": []byte("#!/bin/sh\n")},
			},
			CapabilityUses: uses,
		},
		{
			Identity: "agent/reviewer",
			Kind:     model.AssetKindAgent,
			Content: model.AssetContent{
				Frontmatter: map[string]any{"model": "fast"},
				Body:        "Review changes.\n",
				Files:       map[model.RelativePath][]byte{},
			},
		},
	}
	packages := []model.NormalizedPackage{
		{
			Identity: "Team π",
			Metadata: model.PackageMetadata{"topic": "cursor", "priority": 2},
			Target:   model.TargetCursor,
			Assets:   assets,
		},
		{
			Identity: "zeta",
			Metadata: model.PackageMetadata{},
			Target:   model.TargetCursor,
			Assets: []model.NormalizedAsset{{
				Identity: "skill/small",
				Kind:     model.AssetKindSkill,
				Content:  model.AssetContent{Frontmatter: map[string]any{}, Body: "Small.\n", Files: map[model.RelativePath][]byte{}},
			}},
		},
	}
	if reverse {
		packages[0], packages[1] = packages[1], packages[0]
		packages[1].Assets[0], packages[1].Assets[1] = packages[1].Assets[1], packages[1].Assets[0]
		packages[1].Assets[1].CapabilityUses[0], packages[1].Assets[1].CapabilityUses[1] = packages[1].Assets[1].CapabilityUses[1], packages[1].Assets[1].CapabilityUses[0]
	}
	return packages
}

func intPointer(value int) *int {
	return &value
}
