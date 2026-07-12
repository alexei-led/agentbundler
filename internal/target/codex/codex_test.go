package codex

import (
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderDeterministicBaseline(t *testing.T) {
	t.Parallel()

	first, firstDiagnostics := Render(testPackages(false))
	if len(firstDiagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", firstDiagnostics)
	}
	second, secondDiagnostics := Render(testPackages(true))
	if len(secondDiagnostics) != 0 {
		t.Fatalf("Render() diagnostics = %#v", secondDiagnostics)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Render() plans differ for equivalent input\nfirst: %#v\nsecond: %#v", first, second)
	}

	if first.Target != model.TargetCodex {
		t.Fatalf("Render() target = %q, want %q", first.Target, model.TargetCodex)
	}
	if want := []model.PackageID{"alpha", "zeta"}; !reflect.DeepEqual(first.Packages, want) {
		t.Fatalf("Render() packages = %#v, want %#v", first.Packages, want)
	}
	if len(first.NativeChecks) != 0 {
		t.Fatalf("Render() native checks = %#v, want none", first.NativeChecks)
	}

	wantPaths := []model.RelativePath{
		"package-index.json",
		"packages/alpha/assets/native-resource/reviewer/asset.json",
		"packages/alpha/assets/native-resource/reviewer/content.md",
		"packages/alpha/assets/skill/guide-%C3%A9/asset.json",
		"packages/alpha/assets/skill/guide-%C3%A9/content.md",
		"packages/alpha/assets/skill/guide-%C3%A9/files/docs/data.bin",
		"packages/alpha/package.json",
		"packages/zeta/assets/hook/start/asset.json",
		"packages/zeta/assets/hook/start/content.md",
		"packages/zeta/package.json",
	}
	gotPaths := make([]model.RelativePath, 0, len(first.Files))
	for _, file := range first.Files {
		gotPaths = append(gotPaths, file.Path)
		if file.Executable {
			t.Fatalf("Render() file %q is executable", file.Path)
		}
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("Render() paths = %#v, want %#v", gotPaths, wantPaths)
	}

	if got, want := string(plannedFile(t, first, "package-index.json").Bytes), "{\"format\":\"agentbundler-target-bundle\",\"formatRevision\":1,\"packages\":[\"alpha\",\"zeta\"],\"target\":\"codex\"}\n"; got != want {
		t.Fatalf("package-index.json = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/package.json").Bytes), "{\"identity\":\"alpha\",\"metadata\":{\"labels\":[\"x\",\"y\"],\"name\":\"Alpha\"},\"target\":\"codex\"}\n"; got != want {
		t.Fatalf("package.json = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/assets/skill/guide-%C3%A9/asset.json").Bytes), "{\"capabilityUses\":[{\"key\":\"asset.skill\",\"location\":{\"column\":4,\"line\":2,\"path\":\"source/guide.md\"}},{\"key\":\"asset.skill\",\"location\":{\"path\":\"source/other.md\"}}],\"frontmatter\":{\"description\":\"guide\",\"name\":\"Guide\"},\"identity\":\"skill/guide-é\",\"kind\":\"skill\"}\n"; got != want {
		t.Fatalf("asset.json = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/assets/skill/guide-%C3%A9/content.md").Bytes), "# Guide\n"; got != want {
		t.Fatalf("content.md = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/assets/skill/guide-%C3%A9/files/docs/data.bin").Bytes), "\x00data"; got != want {
		t.Fatalf("support file = %q, want %q", got, want)
	}
}

func TestRenderRejectsMismatchAndUndeclaredCapability(t *testing.T) {
	t.Parallel()

	plan, diagnostics := Render([]model.NormalizedPackage{{
		Identity: "example",
		Target:   model.TargetClaude,
		Assets: []model.NormalizedAsset{{
			Identity: "agent/example",
			Kind:     model.AssetKindAgent,
			CapabilityUses: []model.CapabilityUse{{
				Key:      "tool.use",
				Location: model.SourceLocation{Path: "source/skill.md"},
			}},
		}},
	}})
	if len(plan.Files) != 0 {
		t.Fatalf("Render() files = %#v, want none", plan.Files)
	}
	if !hasDiagnostic(diagnostics, "target-mismatch") {
		t.Fatalf("Render() diagnostics = %#v, want target mismatch", diagnostics)
	}
	if !hasDiagnostic(diagnostics, "unsupported-capability") {
		t.Fatalf("Render() diagnostics = %#v, want unsupported capability", diagnostics)
	}
}

func testPackages(reversed bool) []model.NormalizedPackage {
	line, column := 2, 4
	alpha := model.NormalizedPackage{
		Identity: "alpha",
		Metadata: model.PackageMetadata{
			"name":   "Alpha",
			"labels": []any{"x", "y"},
		},
		Target: model.TargetCodex,
		Assets: []model.NormalizedAsset{
			{
				Identity: "skill/guide-é",
				Kind:     model.AssetKindSkill,
				Content: model.AssetContent{
					Frontmatter: map[string]any{"name": "Guide", "description": "guide"},
					Body:        "# Guide\n",
					Files:       map[model.RelativePath][]byte{"docs/data.bin": {0, 'd', 'a', 't', 'a'}},
				},
				CapabilityUses: []model.CapabilityUse{
					{Key: "asset.skill", Location: model.SourceLocation{Path: "source/other.md"}},
					{Key: "asset.skill", Location: model.SourceLocation{Path: "source/guide.md", Line: &line, Column: &column}},
				},
			},
			{
				Identity: "native-resource/reviewer",
				Kind:     model.AssetKindNativeResource,
				Content: model.AssetContent{
					Frontmatter: map[string]any{},
					Body:        "Review.\n",
					Files:       map[model.RelativePath][]byte{},
				},
			},
		},
	}
	zeta := model.NormalizedPackage{
		Identity: "zeta",
		Metadata: model.PackageMetadata{
			"version": 1,
			"name":    "Zeta",
		},
		Target: model.TargetCodex,
		Assets: []model.NormalizedAsset{{
			Identity: "hook/start",
			Kind:     model.AssetKindHook,
			Content: model.AssetContent{
				Frontmatter: map[string]any{},
				Body:        "Start.\n",
				Files:       map[model.RelativePath][]byte{},
			},
		}},
	}
	if reversed {
		alpha.Assets[0], alpha.Assets[1] = alpha.Assets[1], alpha.Assets[0]
		return []model.NormalizedPackage{zeta, alpha}
	}
	return []model.NormalizedPackage{alpha, zeta}
}

func plannedFile(t *testing.T, plan model.TargetPlan, path model.RelativePath) model.PlannedFile {
	t.Helper()
	for _, file := range plan.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("planned file %q not found", path)
	return model.PlannedFile{}
}

func hasDiagnostic(diagnostics []model.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
