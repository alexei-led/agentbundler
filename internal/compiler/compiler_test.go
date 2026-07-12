package compiler

import (
	"path/filepath"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestCompileRejectsNativeVerifyForBuild(t *testing.T) {
	result := Compile(CompileRequest{Mode: BuildModeBuild, NativeVerify: true})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid-native-verify" {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileRejectsUndeclaredTargetBeforeFilesystemWork(t *testing.T) {
	root := t.TempDir()
	manifest := model.SourceManifest{
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
