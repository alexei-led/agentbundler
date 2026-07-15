package bundle

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
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
	writeFixture(t, workspace, "bundle/src/hooks/check.json", `{"event":"pre-tool","matcher":{"tools":["command"]},"handler":{"mode":"exec","program":"go","arguments":[{"literal":"test"},{"literal":"./..."}]},"timeoutMilliseconds":10000,"asynchronous":false,"failurePolicy":"closed","order":100}`)
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
	if got := string(skill.Base.Files["references/guide.txt"].Bytes); got != "guide" {
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
	hook := alpha.Assets[1].Hook
	if hook == nil || hook.Identity != "hook/check" || hook.Location.Path != "src/hooks/check.json" || hook.Event != model.HookEventPreTool || hook.Handler.Mode != model.HookHandlerModeExec {
		t.Fatalf("typed hook = %#v", hook)
	}
	if got := alpha.Assets[2].Base.Files["resource.bin"].Bytes; !reflect.DeepEqual(got, []byte{0, 1, 2}) {
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

func TestInspectBundleImportsCanonicalHookDirectoryAndLegacyShell(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/hooks/check","src/hooks/notify.json"]}`)
	writeFixture(t, workspace, "bundle/src/hooks/check/hook.json", `{"event":"pre-tool","matcher":{"tools":["command"]},"handler":{"mode":"exec","program":"bash","arguments":[{"literal":"-eu"},{"packageFile":"scripts/check.sh"}]},"timeoutMilliseconds":10000,"asynchronous":false,"failurePolicy":"closed","order":20}`)
	writeFixtureMode(t, workspace, "bundle/src/hooks/check/scripts/check.sh", "#!/bin/sh\nexit 0\n", 0o755)
	writeFixtureMode(t, workspace, "bundle/src/hooks/check/data/rules.json", "{}\n", 0o644)
	writeFixture(t, workspace, "bundle/src/hooks/check/.agentbundler/asset.json", `{"capabilities":["hook.decision.block"]}`)
	writeFixture(t, workspace, "bundle/src/hooks/notify.json", `{"event":"notification","handler":{"mode":"shell","shellCommand":"printf done"},"timeoutMilliseconds":1000,"asynchronous":true,"failurePolicy":"open","order":30}`)

	inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
	if len(diagnostics) != 0 {
		t.Fatalf("InspectBundle() diagnostics = %#v", diagnostics)
	}
	if got, want := assetIDs(inventory.Packages[0]), []model.AssetID{"hook/check", "hook/notify"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("asset IDs = %#v, want %#v", got, want)
	}

	canonical := inventory.Packages[0].Assets[0]
	if canonical.Hook == nil || canonical.Hook.Location.Path != "src/hooks/check/hook.json" || canonical.Hook.Handler.Mode != model.HookHandlerModeExec {
		t.Fatalf("canonical hook = %#v", canonical.Hook)
	}
	if _, exists := canonical.Base.Files["hook.json"]; exists {
		t.Fatal("hook.json was imported as payload")
	}
	if _, exists := canonical.Base.Files[".agentbundler/asset.json"]; exists {
		t.Fatal("asset sidecar was imported as payload")
	}
	if got, want := string(canonical.Base.Files["scripts/check.sh"].Bytes), "#!/bin/sh\nexit 0\n"; got != want {
		t.Fatalf("script bytes = %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" && !canonical.Base.Files["scripts/check.sh"].Executable {
		t.Fatal("executable payload lost executable intent")
	}
	if canonical.Base.Files["data/rules.json"].Executable {
		t.Fatal("non-executable payload gained executable intent")
	}
	if got, want := canonical.Base.Files["scripts/check.sh"].Origin, []model.SourceLocation{{Path: "src/hooks/check/scripts/check.sh"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("script origin = %#v, want %#v", got, want)
	}
	if got, want := capabilityKeys(canonical.CapabilityUses), []model.CapabilityKey{
		"asset.hook",
		"hook.command.exec",
		"hook.decision.block",
		"hook.event.pre-tool",
		"hook.failure.closed",
		"hook.matcher.tool-category",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical capabilities = %#v, want %#v", got, want)
	}
	for _, use := range canonical.CapabilityUses {
		if use.Key != "hook.decision.block" && use.Location.Path != "src/hooks/check/hook.json" {
			t.Fatalf("semantic capability %q location = %#v", use.Key, use.Location)
		}
	}

	legacy := inventory.Packages[0].Assets[1]
	if legacy.Hook == nil || legacy.Hook.Location.Path != "src/hooks/notify.json" || legacy.Hook.Handler.Mode != model.HookHandlerModeShell {
		t.Fatalf("legacy hook = %#v", legacy.Hook)
	}
	if len(legacy.Base.Files) != 0 {
		t.Fatalf("legacy payload files = %#v", legacy.Base.Files)
	}
	if got, want := capabilityKeys(legacy.CapabilityUses), []model.CapabilityKey{
		"asset.hook",
		"hook.async",
		"hook.command.shell",
		"hook.event.notification",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy capabilities = %#v, want %#v", got, want)
	}

	for _, path := range []model.RelativePath{
		"packages/base.json",
		"src/hooks/check/.agentbundler/asset.json",
		"src/hooks/check/data/rules.json",
		"src/hooks/check/hook.json",
		"src/hooks/check/scripts/check.sh",
		"src/hooks/notify.json",
	} {
		if !containsInput(inventory, path) {
			t.Fatalf("input %q was not hashed: %#v", path, inventory.Inputs)
		}
	}
	if !inputsAreSortedAndHashed(inventory) {
		t.Fatalf("inputs are not sorted and hashed: %#v", inventory.Inputs)
	}
}

func TestInspectBundleRejectsInvalidCanonicalHooks(t *testing.T) {
	validPrefix := `{"event":"pre-tool","handler":{"mode":"exec","program":"bash","arguments":[{"packageFile":"scripts/check.sh"}]},"timeoutMilliseconds":1000,"asynchronous":false,"failurePolicy":"closed","order":1`
	tests := []struct {
		name       string
		descriptor string
		want       string
	}{
		{name: "unknown field", descriptor: validPrefix + `,"unexpected":true}`, want: "unknown field"},
		{name: "duplicate key", descriptor: validPrefix + `,"order":2}`, want: `duplicate JSON key "order"`},
		{name: "missing payload", descriptor: validPrefix + `}`, want: `packageFile "scripts/check.sh" does not exist`},
		{name: "traversal payload", descriptor: `{"event":"pre-tool","handler":{"mode":"exec","program":"bash","arguments":[{"packageFile":"../outside.sh"}]},"timeoutMilliseconds":1000,"asynchronous":false,"failurePolicy":"closed","order":1}`, want: "escaping segment"},
		{name: "missing descriptor", want: "required file does not exist"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/hooks/check"]}`)
			if test.descriptor != "" {
				writeFixture(t, workspace, "bundle/src/hooks/check/hook.json", test.descriptor)
			} else if err := os.MkdirAll(filepath.Join(workspace, "bundle/src/hooks/check"), 0o755); err != nil {
				t.Fatalf("MkdirAll(): %v", err)
			}

			inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
			if !hasError(diagnostics) || !reflect.DeepEqual(inventory, model.SourceInventory{}) {
				t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
			}
			if !diagnosticsContain(diagnostics, test.want, "src/hooks/check/hook.json") {
				t.Fatalf("diagnostics = %#v, want message containing %q at hook.json", diagnostics, test.want)
			}
		})
	}
}

func TestInspectBundleRejectsCanonicalHookPayloadSymlink(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/hooks/check"]}`)
	writeFixture(t, workspace, "bundle/src/hooks/check/hook.json", `{"event":"session-start","handler":{"mode":"exec","program":"true","arguments":[]},"timeoutMilliseconds":1000,"asynchronous":false,"failurePolicy":"open","order":1}`)
	writeFixture(t, workspace, "outside", "secret")
	if err := os.Symlink(filepath.Join(workspace, "outside"), filepath.Join(workspace, "bundle/src/hooks/check/linked.sh")); err != nil {
		t.Skipf("os.Symlink() unavailable: %v", err)
	}

	inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
	if !hasError(diagnostics) || !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}
	if !diagnosticsContain(diagnostics, `hook payload "linked.sh" is a symlink`, "src/hooks/check") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestInspectBundleCanonicalHookTraversalIsDeterministic(t *testing.T) {
	inspect := func(t *testing.T, paths []string) model.SourceInventory {
		t.Helper()
		workspace := t.TempDir()
		writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/hooks/check"]}`)
		writeFixture(t, workspace, "bundle/src/hooks/check/hook.json", `{"event":"session-start","handler":{"mode":"exec","program":"true","arguments":[]},"timeoutMilliseconds":1000,"asynchronous":false,"failurePolicy":"open","order":1}`)
		for _, path := range paths {
			writeFixture(t, workspace, "bundle/src/hooks/check/"+path, path)
		}
		inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
		if len(diagnostics) != 0 {
			t.Fatalf("InspectBundle() diagnostics = %#v", diagnostics)
		}
		return inventory
	}

	first := inspect(t, []string{"z.txt", "nested/m.txt", "a.txt"})
	second := inspect(t, []string{"a.txt", "nested/m.txt", "z.txt"})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("inventories differ by creation order:\nfirst = %#v\nsecond = %#v", first, second)
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
	if got := string(inventory.Packages[0].Assets[1].Base.Files["design.md"].Bytes); got != "# Design\n" {
		t.Fatalf("resource file = %q", got)
	}
	if got := inventory.Packages[0].Assets[0].Base.Frontmatter["name"]; got != "reviewer" {
		t.Fatalf("agent frontmatter = %#v", got)
	}
}

func TestInspectBundleRejectsCommandOnlyHookDescriptor(t *testing.T) {
	workspace := t.TempDir()
	writeFixture(t, workspace, "bundle/packages/base.json", `{"id":"base","metadata":{},"assets":["src/hooks/check.json"]}`)
	writeFixture(t, workspace, "bundle/src/hooks/check.json", `{"command":"go test ./..."}`)

	inventory, diagnostics := InspectBundle(bundleManifest("packages/base.json"), workspace)
	if !hasError(diagnostics) || !reflect.DeepEqual(inventory, model.SourceInventory{}) {
		t.Fatalf("inventory = %#v, diagnostics = %#v", inventory, diagnostics)
	}
	if got := diagnostics[0].Message; !strings.Contains(got, "hook descriptor") {
		t.Fatalf("diagnostic = %q", got)
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
	writeFixtureMode(t, root, path, content, 0o644)
}

func writeFixtureMode(t *testing.T, root, path, content string, mode os.FileMode) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%q): %v", fullPath, err)
	}
	if err := os.Chmod(fullPath, mode); err != nil {
		t.Fatalf("Chmod(%q): %v", fullPath, err)
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
		values[file.Path] = string(file.Content.Bytes)
	}
	return values
}

func capabilityKeys(uses []model.CapabilityUse) []model.CapabilityKey {
	keys := make([]model.CapabilityKey, len(uses))
	for index, use := range uses {
		keys[index] = use.Key
	}
	return keys
}

func diagnosticsContain(diagnostics []model.Diagnostic, message string, path model.RelativePath) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, message) && diagnostic.Location != nil && diagnostic.Location.Path == path {
			return true
		}
	}
	return false
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
