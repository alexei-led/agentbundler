package skillrepo

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestInspectSkillRepoImportsSkillsAndSidecars(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "source/skills/alpha/SKILL.md", "---\n{\"description\":\"alpha\"}\n---\nUse alpha.\n")
	writeFixture(t, workspace, "source/skills/alpha/scripts/run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(workspace, "source/skills/alpha/scripts/run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, workspace, "source/unlisted/ignored/SKILL.md", "Ignored.\n")
	writeFixture(t, workspace, "source/.agentbundler/assets/skill/alpha/asset.json", `{"capabilities":["tool-use"]}`)
	writeFixture(t, workspace, "source/.agentbundler/assets/skill/alpha/targets/pi.json", `{
		"frontmatterPatch":{"model":"pi"},
		"bodyPatch":{"mode":"replace","text":"Pi body."},
		"files":{"README.md":{"text":"JSON copy","executable":true},"bin.dat":{"base64":"`+base64.StdEncoding.EncodeToString([]byte{0, 1, 2})+`","executable":true}},
		"deletedFiles":["scripts/run.sh"],
		"acknowledgments":[{"key":"tool-use","reason":"Pi requires approval."}]
	}`)
	writeFixture(t, workspace, "source/.agentbundler/assets/skill/alpha/targets/pi/files/README.md", "tree copy")

	inventory, diagnostics := InspectSkillRepo(testManifest(), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("InspectSkillRepo() diagnostics = %#v", diagnostics)
	}
	if len(inventory.Packages) != 1 || inventory.Packages[0].Identity != "adopted" {
		t.Fatalf("InspectSkillRepo() packages = %#v", inventory.Packages)
	}
	assets := inventory.Packages[0].Assets
	if len(assets) != 1 || assets[0].Identity != "skill/alpha" {
		t.Fatalf("InspectSkillRepo() assets = %#v", assets)
	}
	asset := assets[0]
	if asset.Base.Body != "Use alpha.\n" || asset.Base.Frontmatter["description"] != "alpha" {
		t.Fatalf("InspectSkillRepo() base = %#v", asset.Base)
	}
	if got := asset.Base.Files["scripts/run.sh"]; string(got.Bytes) != "#!/bin/sh\n" || !got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/skills/alpha/scripts/run.sh"}}) {
		t.Fatalf("InspectSkillRepo() support file = %#v", got)
	}
	if !reflect.DeepEqual(asset.CapabilityUses, []model.CapabilityUse{{
		Key:      "tool-use",
		Location: model.SourceLocation{Path: "source/.agentbundler/assets/skill/alpha/asset.json"},
	}}) {
		t.Fatalf("InspectSkillRepo() capabilities = %#v", asset.CapabilityUses)
	}
	if len(asset.Overlays) != 1 {
		t.Fatalf("InspectSkillRepo() overlays = %#v", asset.Overlays)
	}
	overlay := asset.Overlays[0]
	if overlay.Target != model.TargetPi || overlay.FrontmatterPatch == nil || (*overlay.FrontmatterPatch)["model"] != "pi" {
		t.Fatalf("InspectSkillRepo() overlay frontmatter = %#v", overlay)
	}
	if overlay.BodyPatch == nil || overlay.BodyPatch.Mode != model.BodyModeReplace || overlay.BodyPatch.Text == nil || *overlay.BodyPatch.Text != "Pi body." {
		t.Fatalf("InspectSkillRepo() overlay body patch = %#v", overlay.BodyPatch)
	}
	if got := filePatch(overlay.Files, "README.md").Content; string(got.Bytes) != "tree copy" || got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/.agentbundler/assets/skill/alpha/targets/pi/files/README.md"}}) {
		t.Fatalf("InspectSkillRepo() files-tree precedence = %#v", got)
	}
	if got := filePatch(overlay.Files, "bin.dat").Content; !reflect.DeepEqual(got.Bytes, []byte{0, 1, 2}) || !got.Executable || !reflect.DeepEqual(got.Origin, []model.SourceLocation{{Path: "source/.agentbundler/assets/skill/alpha/targets/pi.json"}}) {
		t.Fatalf("InspectSkillRepo() base64 file = %#v", got)
	}
	if !reflect.DeepEqual(overlay.DeletedFiles, []model.RelativePath{"scripts/run.sh"}) {
		t.Fatalf("InspectSkillRepo() deleted files = %#v", overlay.DeletedFiles)
	}
	if !reflect.DeepEqual(overlay.Acknowledgments, []model.Acknowledgment{{
		Asset: "skill/alpha", Target: model.TargetPi, Key: "tool-use", Reason: "Pi requires approval.",
	}}) {
		t.Fatalf("InspectSkillRepo() acknowledgments = %#v", overlay.Acknowledgments)
	}

	wantInputs := []model.RelativePath{
		"source/.agentbundler/assets/skill/alpha/asset.json",
		"source/.agentbundler/assets/skill/alpha/targets/pi.json",
		"source/.agentbundler/assets/skill/alpha/targets/pi/files/README.md",
		"source/skills/alpha/SKILL.md",
		"source/skills/alpha/scripts/run.sh",
	}
	if len(inventory.Inputs) != len(wantInputs) {
		t.Fatalf("InspectSkillRepo() inputs = %#v", inventory.Inputs)
	}
	for index, input := range inventory.Inputs {
		if input.Path != wantInputs[index] || len(input.SHA256) != 64 {
			t.Fatalf("InspectSkillRepo() input[%d] = %#v, want path %q and SHA-256", index, input, wantInputs[index])
		}
	}
}

func TestInspectSkillRepoRejectsSymlinkedSidecarAncestor(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, workspace, "source/skills/alpha/SKILL.md", "Alpha.")
	writeFixture(t, outside, "assets/skill/alpha/asset.json", `{"capabilities":["asset.skill"]}`)
	if err := os.Symlink(outside, filepath.Join(workspace, "source", ".agentbundler")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, diagnostics := InspectSkillRepo(testManifest(), workspace)
	if !hasErrors(diagnostics) {
		t.Fatalf("InspectSkillRepo() diagnostics = %#v, want error", diagnostics)
	}
}

func TestInspectSkillRepoRejectsInvalidTopologies(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "duplicate identities across roots",
			files: map[string]string{
				"source/first/shared/SKILL.md":  "First.",
				"source/second/shared/SKILL.md": "Second.",
			},
		},
		{
			name: "empty declared root",
			files: map[string]string{
				"source/skills/.keep": "",
			},
		},
		{
			name: "unknown target sidecar field",
			files: map[string]string{
				"source/skills/alpha/SKILL.md":                            "Alpha.",
				"source/.agentbundler/assets/skill/alpha/targets/pi.json": `{"extra":true}`,
			},
		},
		{
			name: "acknowledgment cannot override assigned identity",
			files: map[string]string{
				"source/skills/alpha/SKILL.md": "Alpha.",
				"source/.agentbundler/assets/skill/alpha/targets/pi.json": `{
					"acknowledgments":[{
						"asset":"skill/other",
						"target":"codex",
						"key":"tool-use",
						"reason":"Pi requires approval."
					}]
				}`,
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			for path, content := range test.files {
				writeFixture(t, workspace, path, content)
			}
			manifest := testManifest()
			if test.name == "duplicate identities across roots" {
				manifest.SkillsRepository.Roots = []model.RelativePath{"first", "second"}
			}
			_, diagnostics := InspectSkillRepo(manifest, workspace)
			if !hasErrors(diagnostics) {
				t.Fatalf("InspectSkillRepo() diagnostics = %#v, want error", diagnostics)
			}
		})
	}
}

func testManifest() model.SourceManifest {
	return model.SourceManifest{
		Kind:    model.SourceKindSkillsRepository,
		Root:    "source",
		Targets: []model.TargetID{model.TargetPi},
		Output:  "generated",
		SkillsRepository: &model.SkillsRepositorySourceConfig{
			Package:  "adopted",
			Roots:    []model.RelativePath{"skills"},
			Metadata: model.PackageMetadata{"name": "Adopted"},
		},
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

func filePatch(files []model.FilePatch, path model.RelativePath) model.FilePatch {
	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	return model.FilePatch{}
}
