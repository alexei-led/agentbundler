package copilot_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/copilot"
)

func TestRenderDeterministicBaseline(t *testing.T) {
	t.Parallel()

	packages := []model.NormalizedPackage{
		{
			Identity: "zeta",
			Metadata: model.PackageMetadata{"second": 2, "first": 1},
			Target:   model.TargetCopilot,
			Assets: []model.NormalizedAsset{{
				Identity: "agent/review",
				Kind:     model.AssetKindAgent,
				Content:  model.AssetContent{Frontmatter: map[string]any{}, Body: "Review changes.\n", Files: map[model.RelativePath][]byte{}},
			}},
		},
		{
			Identity: "team space",
			Metadata: model.PackageMetadata{"z": true, "a": "first"},
			Target:   model.TargetCopilot,
			Assets: []model.NormalizedAsset{{
				Identity: "skill/über review",
				Kind:     model.AssetKindSkill,
				Content: model.AssetContent{
					Frontmatter: map[string]any{"z": true, "a": "first"},
					Body:        "Review changes.\n",
					Files:       map[model.RelativePath][]byte{"notes/readme.txt": []byte("reference\n")},
				},
				CapabilityUses: []model.CapabilityUse{{
					Key:      "asset.skill",
					Location: model.SourceLocation{Path: "assets/skill.md"},
				}},
			}},
		},
	}

	plan, diagnostics := copilot.Render(packages)
	if len(diagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", diagnostics)
	}
	if plan.Target != model.TargetCopilot {
		t.Fatalf("Render() target = %q, want %q", plan.Target, model.TargetCopilot)
	}
	if !reflect.DeepEqual(plan.Packages, []model.PackageID{"team space", "zeta"}) {
		t.Fatalf("Render() packages = %#v", plan.Packages)
	}
	if len(plan.NativeChecks) != 0 {
		t.Fatalf("Render() native checks = %#v, want none", plan.NativeChecks)
	}

	got := make(map[model.RelativePath]string, len(plan.Files))
	for _, file := range plan.Files {
		if file.Executable {
			t.Fatalf("Render() planned executable file %q", file.Path)
		}
		got[file.Path] = string(file.Bytes)
	}
	want := map[model.RelativePath]string{
		"package-index.json":                                                           "{\"format\":\"agentbundler-target-bundle\",\"formatRevision\":1,\"packages\":[\"team space\",\"zeta\"],\"target\":\"copilot\"}\n",
		"packages/team%20space/package.json":                                           "{\"identity\":\"team space\",\"metadata\":{\"a\":\"first\",\"z\":true},\"target\":\"copilot\"}\n",
		"packages/team%20space/assets/skill/%C3%BCber%20review/asset.json":             "{\"capabilityUses\":[{\"key\":\"asset.skill\",\"location\":{\"path\":\"assets/skill.md\"}}],\"frontmatter\":{\"a\":\"first\",\"z\":true},\"identity\":\"skill/über review\",\"kind\":\"skill\"}\n",
		"packages/team%20space/assets/skill/%C3%BCber%20review/content.md":             "Review changes.\n",
		"packages/team%20space/assets/skill/%C3%BCber%20review/files/notes/readme.txt": "reference\n",
		"packages/zeta/package.json":                                                   "{\"identity\":\"zeta\",\"metadata\":{\"first\":1,\"second\":2},\"target\":\"copilot\"}\n",
		"packages/zeta/assets/agent/review/asset.json":                                 "{\"capabilityUses\":null,\"frontmatter\":{},\"identity\":\"agent/review\",\"kind\":\"agent\"}\n",
		"packages/zeta/assets/agent/review/content.md":                                 "Review changes.\n",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Render() files = %#v, want %#v", got, want)
	}

	reversed := []model.NormalizedPackage{packages[1], packages[0]}
	second, diagnostics := copilot.Render(reversed)
	if len(diagnostics) != 0 {
		t.Fatalf("Render(reversed) diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(plan, second) {
		t.Fatalf("Render() output differs when package order changes:\nfirst:  %#v\nsecond: %#v", plan, second)
	}
}

func TestRenderRejectsInvalidTargetAndCapability(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		pkg      model.NormalizedPackage
		wantCode string
	}{
		{
			name: "target mismatch",
			pkg: model.NormalizedPackage{
				Identity: "base",
				Metadata: model.PackageMetadata{},
				Target:   model.TargetClaude,
			},
			wantCode: "target-mismatch",
		},
		{
			name: "undeclared capability",
			pkg: model.NormalizedPackage{
				Identity: "base",
				Metadata: model.PackageMetadata{},
				Target:   model.TargetCopilot,
				Assets: []model.NormalizedAsset{{
					Identity: "skill/example",
					Kind:     model.AssetKindSkill,
					Content:  model.AssetContent{Frontmatter: map[string]any{}, Files: map[model.RelativePath][]byte{}},
					CapabilityUses: []model.CapabilityUse{{
						Key:      "permission.shell",
						Location: model.SourceLocation{Path: "assets/example.md"},
					}},
				}},
			},
			wantCode: "unsupported-capability",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan, diagnostics := copilot.Render([]model.NormalizedPackage{test.pkg})
			if len(plan.Files) != 0 {
				t.Fatalf("Render() files = %#v, want none", plan.Files)
			}
			if !containsCode(diagnostics, test.wantCode) {
				t.Fatalf("Render() diagnostics = %#v, want %q", diagnostics, test.wantCode)
			}
		})
	}
}

func TestCapabilitiesAreCompleteAndIndependent(t *testing.T) {
	t.Parallel()

	want := []model.CapabilityRule{
		{Key: "asset.agent", State: model.CapabilityStateNative},
		{Key: "asset.hook", State: model.CapabilityStateNative},
		{Key: "asset.native-resource", State: model.CapabilityStateNative},
		{Key: "asset.skill", State: model.CapabilityStateNative},
	}
	got := copilot.Capabilities()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities() = %#v, want %#v", got, want)
	}
	got[0].State = model.CapabilityStateUnsupported
	if !reflect.DeepEqual(copilot.Capabilities(), want) {
		t.Fatal("Capabilities() returned mutable shared state")
	}
}

func containsCode(diagnostics []model.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
