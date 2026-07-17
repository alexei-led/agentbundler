package source

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestImportRoutesExplicitSourceKinds(t *testing.T) {
	tests := []struct {
		name      string
		manifest  model.SourceManifest
		files     map[string]string
		packageID model.PackageID
		assetID   model.AssetID
	}{
		{
			name: "bundle",
			manifest: model.SourceManifest{
				Kind:    model.SourceKindBundle,
				Root:    "bundle",
				Targets: []model.TargetID{model.TargetPi},
				Output:  "generated",
				Bundle:  &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/base.json"}},
			},
			files: map[string]string{
				"bundle/packages/base.json":          `{"id":"bundle-package","metadata":{},"assets":["src/skills/example"]}`,
				"bundle/src/skills/example/SKILL.md": "Bundle skill.",
			},
			packageID: "bundle-package",
			assetID:   "skill/example",
		},
		{
			name: "claude plugin",
			manifest: model.SourceManifest{
				Kind:    model.SourceKindClaudePlugin,
				Root:    "source",
				Targets: []model.TargetID{model.TargetClaude},
				Output:  "generated",
				ClaudePlugin: &model.ClaudePluginSourceConfig{
					PluginRoot: "plugin",
				},
			},
			files: map[string]string{
				"source/plugin/.claude-plugin/plugin.json": `{"name":"plugin-package"}`,
				"source/plugin/skills/example/SKILL.md":    "Plugin skill.",
			},
			packageID: "plugin-package",
			assetID:   "skill/example",
		},
		{
			name: "skills repository",
			manifest: model.SourceManifest{
				Kind:    model.SourceKindSkillsRepository,
				Root:    "source",
				Targets: []model.TargetID{model.TargetPi},
				Output:  "generated",
				SkillsRepository: &model.SkillsRepositorySourceConfig{
					Package:  "repository-package",
					Roots:    []model.RelativePath{"skills"},
					Metadata: model.PackageMetadata{},
				},
			},
			files: map[string]string{
				"source/skills/example/SKILL.md": "Repository skill.",
			},
			packageID: "repository-package",
			assetID:   "skill/example",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			for path, content := range test.files {
				writeFixture(t, workspace, path, content)
			}

			inventory, diagnostics := Import(test.manifest, workspace)
			if len(diagnostics) != 0 {
				t.Fatalf("Import() diagnostics = %#v", diagnostics)
			}
			if len(inventory.Packages) != 1 || inventory.Packages[0].Identity != test.packageID {
				t.Fatalf("Import() packages = %#v", inventory.Packages)
			}
			assets := inventory.Packages[0].Assets
			if len(assets) != 1 || assets[0].Identity != test.assetID {
				t.Fatalf("Import() assets = %#v", assets)
			}
		})
	}
}

func TestImportPreservesNativeGaps(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/plugins/pi/resource"]}`)
	writeFixture(t, workspace, "bundle/src/plugins/pi/resource", "native")
	manifest := model.SourceManifest{
		Kind:    model.SourceKindBundle,
		Root:    "bundle",
		Targets: []model.TargetID{model.TargetPi},
		Output:  "generated",
		Bundle:  &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/base.json"}},
	}

	inventory, diagnostics := Import(manifest, workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("Import() diagnostics = %#v", diagnostics)
	}
	if got, want := inventory.NativeGaps, []model.NativeGap{{
		Package:   "base",
		Component: "resource",
		Asset:     assetID("native-resource/resource"),
		Location:  model.SourceLocation{Path: "src/plugins/pi/resource"},
		Target:    targetID(model.TargetPi),
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Import() native gaps = %#v, want %#v", got, want)
	}
}

func TestImportRejectsInvalidManifestAndWorkspaceBeforeRouting(t *testing.T) {
	workspace := t.TempDir()
	manifest := model.SourceManifest{
		Kind:    "unknown",
		Root:    "source",
		Targets: []model.TargetID{model.TargetPi},
		Output:  "generated",
	}

	inventory, diagnostics := Import(manifest, workspace)
	if !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("Import() inventory = %#v, want empty", inventory)
	}
	if len(diagnostics) != 1 || diagnostics[0].Message != "manifest kind is invalid" {
		t.Fatalf("Import() diagnostics = %#v", diagnostics)
	}

	manifest = model.SourceManifest{
		Kind:    model.SourceKindSkillsRepository,
		Root:    "../source",
		Targets: []model.TargetID{model.TargetPi},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package:  "repository",
			Roots:    []model.RelativePath{"skills"},
			Metadata: model.PackageMetadata{},
		},
	}
	inventory, diagnostics = Import(manifest, workspace)
	if !reflect.DeepEqual(inventory, model.SourceInventory{}) || len(diagnostics) == 0 {
		t.Fatalf("Import() escaped root inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}

	manifest.Root = "source"
	inventory, diagnostics = Import(manifest, "relative-workspace")
	if !reflect.DeepEqual(inventory, model.SourceInventory{}) || len(diagnostics) != 1 || diagnostics[0].Code != diagnosticCodeInvalidSourceImport {
		t.Fatalf("Import() relative workspace inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}
}

func TestImportSortsDiagnostics(t *testing.T) {
	workspace := t.TempDir()
	manifest := model.SourceManifest{
		Kind:    model.SourceKindBundle,
		Root:    "bundle",
		Targets: []model.TargetID{model.TargetPi},
		Output:  "generated",
		Bundle: &model.BundleSourceConfig{Packages: []model.RelativePath{
			"packages/z.json",
			"packages/a.json",
		}},
	}
	if err := os.Mkdir(filepath.Join(workspace, "bundle"), 0o755); err != nil {
		t.Fatal(err)
	}

	inventory, diagnostics := Import(manifest, workspace)
	if !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("Import() inventory = %#v, want empty", inventory)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("Import() diagnostics = %#v", diagnostics)
	}
	if got, want := []model.RelativePath{diagnosticPath(diagnostics[0]), diagnosticPath(diagnostics[1])}, []model.RelativePath{"packages/a.json", "packages/z.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Import() diagnostic paths = %#v, want %#v", got, want)
	}
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func assetID(value model.AssetID) *model.AssetID {
	return &value
}

func targetID(value model.TargetID) *model.TargetID {
	return &value
}
