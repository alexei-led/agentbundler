package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/pi"
)

func TestCompileRejectsNativeVerifyForBuild(t *testing.T) {
	result := Compile(CompileRequest{Mode: BuildModeBuild, NativeVerify: true})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-native-verify" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileBuildsMinimalSkillsRepositoryForEveryTarget(t *testing.T) {
	for _, target := range []model.TargetID{model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor} {
		t.Run(string(target), func(t *testing.T) {
			workspace := t.TempDir()
			writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
			result := Compile(CompileRequest{
				WorkspaceRoot: filepath.Clean(workspace),
				Manifest:      skillsManifest(target),
				Mode:          BuildModeBuild,
			})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
			}
			if len(result.Plan.Targets) != 1 || result.Plan.Targets[0].Target != target {
				t.Fatalf("Compile() targets = %#v", result.Plan.Targets)
			}
		})
	}
}

func TestCompileRecordsResolvedAdapterRevision(t *testing.T) {
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      skillsManifest(model.TargetPi),
		Mode:          BuildModeBuild,
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "generated/.agentbundler/build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var provenance struct {
		Outputs []struct {
			Target          model.TargetID `json:"target"`
			AdapterRevision int            `json:"adapterRevision"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(data, &provenance); err != nil {
		t.Fatal(err)
	}
	if len(provenance.Outputs) != 1 || provenance.Outputs[0].Target != model.TargetPi || provenance.Outputs[0].AdapterRevision != pi.FormatRevision {
		t.Fatalf("provenance outputs = %#v", provenance.Outputs)
	}
}

func TestCompileRejectsSymlinkedOutputAncestor(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	manifest := skillsManifest(model.TargetClaude)
	manifest.Output = "linked/generated"
	result := Compile(CompileRequest{WorkspaceRoot: filepath.Clean(workspace), Manifest: manifest, Mode: BuildModeBuild})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-output-root" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsCapabilityUnsupportedBySelectedTarget(t *testing.T) {
	workspace := t.TempDir()
	writeCompilerFixture(t, workspace, "source/skills/demo/SKILL.md", "# Demo\n")
	writeCompilerFixture(t, workspace, "source/.agentbundler/assets/skill/demo/asset.json", `{"capabilities":["asset.hook"]}`)

	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(workspace),
		Manifest:      skillsManifest(model.TargetPi),
		Mode:          BuildModeBuild,
	})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-composition" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsUndeclaredTargetBeforeFilesystemWork(t *testing.T) {
	root := t.TempDir()
	manifest := model.SourceManifest{
		Version: 1,
		Kind:    model.SourceKindBundle,
		Root:    "source",
		Targets: []model.TargetID{model.TargetClaude},
		Output:  "generated",
		Bundle:  &model.BundleSourceConfig{Packages: []model.RelativePath{"packages/base.json"}},
	}
	result := Compile(CompileRequest{
		WorkspaceRoot: filepath.Clean(root),
		Manifest:      manifest,
		Targets:       []model.TargetID{model.TargetCodex},
		Mode:          BuildModeCheck,
	})
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "invalid-target-selector" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func skillsManifest(target model.TargetID) model.SourceManifest {
	return model.SourceManifest{
		Version: 1,
		Kind:    model.SourceKindSkillsRepository,
		Root:    "source",
		Targets: []model.TargetID{target},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package:  "demo",
			Roots:    []model.RelativePath{"skills"},
			Metadata: model.PackageMetadata{},
		},
	}
}

func writeCompilerFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
