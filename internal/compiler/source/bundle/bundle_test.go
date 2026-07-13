package bundle

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestInspectBundleImportsExplicitAssetsAndOverlayFilesTree(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/z.json", `{"id":"zeta","metadata":{"order":2},"assets":["src/skills/example"]}`)
	writeFixture(t, workspace, "bundle/packages/a.json", `{"id":"alpha","metadata":{"order":1},"assets":["src/skills/example","src/agents/reviewer.md","src/hooks/check.json","src/plugins/pi/resource.bin"]}`)
	writeFixture(t, workspace, "bundle/src/skills/example/SKILL.md", "---\n{\"name\":\"Example\"}\n---\nUse the skill.\n")
	writeFixture(t, workspace, "bundle/src/skills/example/references/guide.txt", "guide")
	writeFixture(t, workspace, "bundle/src/skills/example/.agentbundler/asset.json", `{"capabilities":["tool-use"]}`)
	writeFixture(t, workspace, "bundle/src/skills/example/.agentbundler/targets/pi.json", `{"frontmatterPatch":{"model":"pi"},"bodyPatch":{"mode":"replace","text":"target body"},"files":{"README.md":"from JSON","binary":{"base64":"AQI="}},"deletedFiles":["obsolete.txt"],"acknowledgments":[{"asset":"skill/example","target":"pi","key":"tool-use","reason":"native support"}]}`)
	writeFixture(t, workspace, "bundle/src/skills/example/.agentbundler/targets/pi/files/README.md", "from tree")
	writeFixture(t, workspace, "bundle/src/agents/reviewer.md", "Review changes.\n")
	writeFixture(t, workspace, "bundle/src/hooks/check.json", `{"command":"go test ./..."}`)
	writeFixture(t, workspace, "bundle/src/plugins/pi/resource.bin", string([]byte{0, 1, 2}))
	writeFixture(t, workspace, "bundle/src/skills/unlisted/SKILL.md", "not imported")

	inventory, diagnostics := InspectBundle(bundleManifest("packages/z.json", "packages/a.json"), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("InspectBundle() diagnostics = %#v", diagnostics)
	}
	if got, want := packageIDs(inventory), []model.PackageID{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("package IDs = %#v, want %#v", got, want)
	}
	alpha := inventory.Packages[0]
	if got, want := assetIDs(alpha), []model.AssetID{"agent/reviewer", "hook/check", "native-resource/resource.bin", "skill/example"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("asset IDs = %#v, want %#v", got, want)
	}
	skill := alpha.Assets[3]
	if skill.Base.Body != "Use the skill.\n" || skill.Base.Frontmatter["name"] != "Example" {
		t.Fatalf("skill base = %#v", skill.Base)
	}
	if got := string(skill.Base.Files["references/guide.txt"]); got != "guide" {
		t.Fatalf("skill support file = %q, want guide", got)
	}
	if got, want := skill.CapabilityUses, []model.CapabilityUse{{Key: "tool-use", Location: model.SourceLocation{Path: "src/skills/example/.agentbundler/asset.json"}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %#v, want %#v", got, want)
	}
	if len(skill.Overlays) != 1 {
		t.Fatalf("overlays = %#v", skill.Overlays)
	}
	overlay := skill.Overlays[0]
	if overlay.Target != model.TargetPi || overlay.BodyPatch == nil || overlay.BodyPatch.Text == nil || *overlay.BodyPatch.Text != "target body" {
		t.Fatalf("overlay = %#v", overlay)
	}
	if got, want := filePatchBytes(overlay.Files), map[model.RelativePath]string{"README.md": "from tree", "binary": string([]byte{1, 2})}; !reflect.DeepEqual(got, want) {
		t.Fatalf("overlay files = %#v, want %#v", got, want)
	}
	if got := alpha.Assets[2].Base.Files["resource.bin"]; !reflect.DeepEqual(got, []byte{0, 1, 2}) {
		t.Fatalf("native resource = %#v", got)
	}
	if got, want := inventory.NativeGaps, []model.NativeGap{{
		Component: "resource.bin",
		Asset:     assetID("native-resource/resource.bin"),
		Location:  model.SourceLocation{Path: "src/plugins/pi/resource.bin"},
		Target:    targetID(model.TargetPi),
	}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("native gaps = %#v, want %#v", got, want)
	}
	if containsInput(inventory, "src/skills/unlisted/SKILL.md") {
		t.Fatal("unlisted skill was imported")
	}
	if !inputsAreSortedAndHashed(inventory) {
		t.Fatalf("inputs are not sorted and hashed: %#v", inventory.Inputs)
	}
}

func TestInspectBundleImportsTargetFilteredResources(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":[{"path":"src/agents/reviewer.md","targets":["claude","codex"]},{"path":"src/resources/templates","targets":["claude","codex","pi"]}]}`)
	writeFixture(t, workspace, "bundle/src/agents/reviewer.md", "---\nname: reviewer\ndescription: Review code\n---\nReview.\n")
	writeFixture(t, workspace, "bundle/src/resources/templates/design.md", "# Design\n")

	inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("InspectBundle() diagnostics = %#v", diagnostics)
	}
	if got, want := assetIDs(inventory.Packages[0]), []model.AssetID{"agent/reviewer", "resource/templates"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("asset IDs = %#v, want %#v", got, want)
	}
	if got := string(inventory.Packages[0].Assets[1].Base.Files["design.md"]); got != "# Design\n" {
		t.Fatalf("resource file = %q", got)
	}
	if got := inventory.Packages[0].Assets[0].Base.Frontmatter["name"]; got != "reviewer" {
		t.Fatalf("agent frontmatter = %#v", got)
	}
}

func TestInspectBundleRejectsInvalidPackageAndSidecar(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/skills/example","src/skills/example"]}`)
	writeFixture(t, workspace, "bundle/src/skills/example/SKILL.md", "body")

	inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
	if !hasError(diagnostics) || !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("duplicate asset inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}

	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/skills/example"]}`)
	writeFixture(t, workspace, "bundle/src/skills/example/.agentbundler/asset.json", `{"capabilities":[],"unexpected":true}`)
	inventory, diagnostics = InspectBundle(bundleManifest("packages/base.json"), workspace)
	if !hasError(diagnostics) || !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("invalid sidecar inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}
}

func TestInspectBundleRejectsSupportSymlink(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/skills/example"]}`)
	writeFixture(t, workspace, "bundle/src/skills/example/SKILL.md", "body")
	outside := filepath.Join(workspace, "outside")
	writeFixture(t, workspace, "outside", "secret")
	if err := os.Symlink(outside, filepath.Join(workspace, "bundle/src/skills/example/link")); err != nil {
		t.Skipf("os.Symlink() unavailable: %v", err)
	}

	inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
	if !hasError(diagnostics) || !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("symlink inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}
}

func bundleManifest(packages ...string) model.SourceManifest {
	paths := make([]model.RelativePath, len(packages))
	for index, packagePath := range packages {
		paths[index] = model.RelativePath(packagePath)
	}
	return model.SourceManifest{
		Kind:    model.SourceKindBundle,
		Root:    "bundle",
		Targets: []model.TargetID{model.TargetPi},
		Output:  "generated",
		Bundle:  &model.BundleSourceConfig{Packages: paths},
	}
}

func writeFixture(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", fullPath, err)
	}
}

func packageIDs(inventory model.SourceInventory) []model.PackageID {
	ids := make([]model.PackageID, len(inventory.Packages))
	for index, pkg := range inventory.Packages {
		ids[index] = pkg.Identity
	}
	return ids
}

func assetIDs(pkg model.SourcePackage) []model.AssetID {
	ids := make([]model.AssetID, len(pkg.Assets))
	for index, asset := range pkg.Assets {
		ids[index] = asset.Identity
	}
	return ids
}

func filePatchBytes(files []model.FilePatch) map[model.RelativePath]string {
	values := make(map[model.RelativePath]string, len(files))
	for _, file := range files {
		values[file.Path] = string(file.Bytes)
	}
	return values
}

func containsInput(inventory model.SourceInventory, path model.RelativePath) bool {
	for _, input := range inventory.Inputs {
		if input.Path == path {
			return true
		}
	}
	return false
}

func inputsAreSortedAndHashed(inventory model.SourceInventory) bool {
	inputs := append([]model.InputFile(nil), inventory.Inputs...)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })
	if !reflect.DeepEqual(inputs, inventory.Inputs) {
		return false
	}
	for _, input := range inputs {
		if len(input.SHA256) != 64 {
			return false
		}
	}
	return true
}

func assetID(value model.AssetID) *model.AssetID {
	return &value
}

func targetID(value model.TargetID) *model.TargetID {
	return &value
}

func hasError(diagnostics []model.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
