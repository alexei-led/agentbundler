package pi

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

	if first.Target != model.TargetPi {
		t.Fatalf("Render() target = %q, want %q", first.Target, model.TargetPi)
	}
	if want := []model.PackageID{"alpha", "zeta"}; !reflect.DeepEqual(first.Packages, want) {
		t.Fatalf("Render() packages = %#v, want %#v", first.Packages, want)
	}
	if len(first.NativeChecks) != 0 {
		t.Fatalf("Render() native checks = %#v, want none", first.NativeChecks)
	}

	wantPaths := []model.RelativePath{
		"package-index.json",
		"packages/alpha/assets/native-resource/extension/asset.json",
		"packages/alpha/assets/native-resource/extension/content.md",
		"packages/alpha/assets/native-resource/extension/files/extensions/index.ts",
		"packages/alpha/assets/skill/guide-%C3%A9/asset.json",
		"packages/alpha/assets/skill/guide-%C3%A9/content.md",
		"packages/alpha/assets/skill/guide-%C3%A9/files/docs/data.bin",
		"packages/alpha/package.json",
		"packages/zeta/assets/skill/start/asset.json",
		"packages/zeta/assets/skill/start/content.md",
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

	if got, want := string(plannedFile(t, first, "package-index.json").Bytes), "{\"format\":\"agentbundler-target-bundle\",\"formatRevision\":1,\"packages\":[\"alpha\",\"zeta\"],\"target\":\"pi\"}\n"; got != want {
		t.Fatalf("package-index.json = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/package.json").Bytes), "{\"identity\":\"alpha\",\"metadata\":{\"labels\":[\"x\",\"y\"],\"name\":\"Alpha\"},\"target\":\"pi\"}\n"; got != want {
		t.Fatalf("package.json = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/assets/skill/guide-%C3%A9/asset.json").Bytes), "{\"capabilityUses\":[{\"key\":\"asset.skill\",\"location\":{\"column\":4,\"line\":2,\"path\":\"source/guide.md\"}},{\"key\":\"asset.skill\",\"location\":{\"path\":\"source/other.md\"}}],\"frontmatter\":{\"description\":\"guide\",\"name\":\"Guide\"},\"identity\":\"skill/guide-é\",\"kind\":\"skill\"}\n"; got != want {
		t.Fatalf("asset.json = %q, want %q", got, want)
	}
	if got, want := string(plannedFile(t, first, "packages/alpha/assets/native-resource/extension/files/extensions/index.ts").Bytes), "export default {};\n"; got != want {
		t.Fatalf("extension = %q, want %q", got, want)
	}
}

func TestRenderRejectsUnsupportedPortableAssets(t *testing.T) {
	t.Parallel()

	for _, kind := range []model.AssetKind{model.AssetKindAgent, model.AssetKindHook} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			plan, diagnostics := Render([]model.NormalizedPackage{{
				Identity: "example",
				Target:   model.TargetPi,
				Assets: []model.NormalizedAsset{{
					Identity: model.AssetID(string(kind) + "/example"),
					Kind:     kind,
					Content: model.AssetContent{
						Frontmatter: map[string]any{},
						Files:       map[model.RelativePath][]byte{},
					},
				}},
			}})
			if len(plan.Files) != 0 {
				t.Fatalf("Render() files = %#v, want none", plan.Files)
			}
			if !hasDiagnostic(diagnostics, "unsupported-capability") {
				t.Fatalf("Render() diagnostics = %#v, want unsupported capability", diagnostics)
			}
		})
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	want := []model.CapabilityRule{
		{Key: "asset.agent", State: model.CapabilityStateUnsupported},
		{Key: "asset.hook", State: model.CapabilityStateUnsupported},
		{Key: "asset.native-resource", State: model.CapabilityStateNative},
		{Key: "asset.skill", State: model.CapabilityStateNative},
	}
	if got := Capabilities(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Capabilities() = %#v, want %#v", got, want)
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
		Target: model.TargetPi,
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
				Identity: "native-resource/extension",
				Kind:     model.AssetKindNativeResource,
				Content: model.AssetContent{
					Frontmatter: map[string]any{},
					Body:        "Pi extension.\n",
					Files:       map[model.RelativePath][]byte{"extensions/index.ts": []byte("export default {};\n")},
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
		Target: model.TargetPi,
		Assets: []model.NormalizedAsset{{
			Identity: "skill/start",
			Kind:     model.AssetKindSkill,
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
