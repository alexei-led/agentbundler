package compatibility

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestPrepareRejectsIncompleteCompatibilityTargetSelection(t *testing.T) {
	t.Parallel()

	_, diagnostics := Prepare(Request{
		WorkspaceRoot: t.TempDir(), Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude, model.TargetCodex}},
		Plan:   model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude}}},
	})
	if !hasDiagnostic(diagnostics, "compatibility-incomplete-selection") {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
}

func TestPrepareMarketplaceGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target   model.TargetID
		marker   model.RelativePath
		manifest model.RelativePath
		catalog  string
	}{
		{model.TargetClaude, ".claude-plugin/marketplace.json", "alpha/.claude-plugin/plugin.json", `{"name":"tools","plugins":[{"name":"alpha","source":"./alpha"},{"name":"remote","source":"https://example.com/remote.git"}]}`},
		{model.TargetCodex, ".agents/plugins/marketplace.json", "alpha/.codex-plugin/plugin.json", `{"name":"tools","plugins":[{"name":"alpha","source":{"source":"local","path":"./alpha"}},{"name":"remote","source":{"source":"github","repo":"example/remote"}}]}`},
		{model.TargetCopilot, ".github/plugin/marketplace.json", "alpha/plugin.json", `{"name":"tools","plugins":[{"name":"alpha","source":"./alpha"},{"name":"remote","source":{"url":"https://example.com/remote.git"}}]}`},
		{model.TargetCursor, ".cursor-plugin/marketplace.json", "alpha/.cursor-plugin/plugin.json", `{"name":"tools","plugins":[{"name":"alpha","source":"./alpha"},{"name":"remote","source":"github:example/remote"}]}`},
		{model.TargetGrok, ".claude-plugin/marketplace.json", "alpha/.claude-plugin/plugin.json", `{"name":"tools","plugins":[{"name":"alpha","source":"./alpha"},{"name":"remote","source":"https://example.com/remote.git"}]}`},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.target), func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			plan, diagnostics := Prepare(Request{
				WorkspaceRoot: workspace,
				Output:        "dist",
				Config:        &model.CompatibilityConfig{RootManifests: []model.TargetID{test.target}},
				Plan: model.BuildPlan{Targets: []model.TargetPlan{{
					Target: test.target,
					Files: []model.PlannedFile{
						{Path: test.marker, Bytes: []byte(test.catalog + "\n")},
						{Path: test.manifest, Bytes: []byte("{}\n")},
					},
				}}},
			})
			if len(diagnostics) != 0 {
				t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
			}
			file, ok := planFile(plan, test.marker)
			if !ok {
				t.Fatalf("root marker %q missing from %#v", test.marker, plan.Files)
			}
			want, err := os.ReadFile(filepath.Join("testdata", string(test.target)+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(file.Bytes) != string(want) {
				t.Fatalf("root marker differs:\ngot:  %s\nwant: %s", file.Bytes, want)
			}
		})
	}
}

func TestPrepareRejectsUnsafeDanglingAndDuplicateMarketplaceSources(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		sources string
		code    string
	}{
		{"absolute", `[{"name":"alpha","source":"/tmp/alpha"}]`, "compatibility-source-unsafe"},
		{"traversal", `[{"name":"alpha","source":"../alpha"}]`, "compatibility-source-unsafe"},
		{"contained traversal", `[{"name":"alpha","source":"./alpha/../alpha"}]`, "compatibility-source-unsafe"},
		{"dangling", `[{"name":"alpha","source":"./missing"}]`, "compatibility-source-dangling"},
		{"duplicate IDs", `[{"name":"alpha","source":"./alpha"},{"name":"ALPHA","source":"https://example.com/alpha"}]`, "compatibility-duplicate-id"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			catalog := `{"name":"tools","plugins":` + test.sources + `}`
			_, diagnostics := Prepare(Request{
				WorkspaceRoot: t.TempDir(), Output: "dist",
				Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude}},
				Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude, Files: []model.PlannedFile{
					{Path: ".claude-plugin/marketplace.json", Bytes: []byte(catalog)},
					{Path: "alpha/.claude-plugin/plugin.json", Bytes: []byte("{}")},
				}}}},
			})
			if !hasDiagnostic(diagnostics, test.code) {
				t.Fatalf("Prepare() diagnostics = %#v, want %s", diagnostics, test.code)
			}
		})
	}
}

func TestPrepareRejectsForgedOwnedFilesWithoutDeletingThem(t *testing.T) {
	t.Parallel()

	for _, relative := range []string{
		"README.md",
		".git/config",
		"dist/claude/alpha/.claude-plugin/plugin.json",
		"alpha/package.json",
	} {
		relative := relative
		t.Run(relative, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			writeTestFile(t, workspace, relative, "author-owned\n")
			state := fmt.Sprintf(`{"version":1,"files":[%q]}`, relative)
			writeTestFile(t, workspace, ".agentbundler/compatibility.json", state)

			_, diagnostics := Prepare(Request{WorkspaceRoot: workspace, Output: "dist"})
			if !hasDiagnostic(diagnostics, "compatibility-ownership-invalid") {
				t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
			}
			data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relative)))
			if err != nil || string(data) != "author-owned\n" {
				t.Fatalf("forged owned path changed: data=%q err=%v", data, err)
			}
		})
	}
}

func TestPrepareRejectsForgedCodexAgentNotInCanonicalPlan(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeTestFile(t, workspace, ".codex/agents/evil.toml", "author-owned\n")
	writeTestFile(t, workspace, ".agentbundler/compatibility.json", `{"version":1,"files":[".codex/agents/evil.toml"]}`)
	_, diagnostics := Prepare(Request{
		WorkspaceRoot: workspace, Output: "dist",
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetCodex, Files: []model.PlannedFile{
			{Path: ".codex/agents/reviewer.toml", Bytes: []byte("canonical\n")},
		}}}},
	})
	if !hasDiagnostic(diagnostics, "compatibility-ownership-invalid") {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
	data, err := os.ReadFile(filepath.Join(workspace, ".codex/agents/evil.toml"))
	if err != nil || string(data) != "author-owned\n" {
		t.Fatalf("forged Codex agent changed: data=%q err=%v", data, err)
	}
}

func TestPrepareRejectsSymlinkedOwnershipStateAndOwnedAncestor(t *testing.T) {
	t.Parallel()

	t.Run("ownership state", func(t *testing.T) {
		workspace := t.TempDir()
		external := t.TempDir()
		writeTestFile(t, external, "compatibility.json", `{"version":1,"files":["README.md"]}`)
		if err := os.Symlink(external, filepath.Join(workspace, ".agentbundler")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, diagnostics := Prepare(Request{WorkspaceRoot: workspace, Output: "dist"})
		if !hasDiagnostic(diagnostics, "compatibility-ownership-invalid") {
			t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
		}
	})

	t.Run("owned path ancestor", func(t *testing.T) {
		workspace := t.TempDir()
		external := t.TempDir()
		writeTestFile(t, external, "agents/reviewer.toml", "author-owned\n")
		if err := os.Symlink(external, filepath.Join(workspace, ".codex")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		writeTestFile(t, workspace, ".agentbundler/compatibility.json", `{"version":1,"files":[".codex/agents/reviewer.toml"]}`)
		_, diagnostics := Prepare(Request{
			WorkspaceRoot: workspace, Output: "dist",
			Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetCodex, Files: []model.PlannedFile{
				{Path: ".codex/agents/reviewer.toml", Bytes: []byte("canonical\n")},
			}}}},
		})
		if !hasDiagnostic(diagnostics, "compatibility-path-unsafe") {
			t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
		}
		data, err := os.ReadFile(filepath.Join(external, "agents/reviewer.toml"))
		if err != nil || string(data) != "author-owned\n" {
			t.Fatalf("symlink target changed: data=%q err=%v", data, err)
		}
	})
}

func TestPrepareRejectsForgedPiOwnershipWithoutChangingAuthorFields(t *testing.T) {
	t.Parallel()

	for _, forged := range []string{
		`"dependencies":["author-runtime"]`,
		`"extensions":["./author/extension.ts"]`,
		`"skills":["./author/skills"]`,
		`"agents":["./author/agents"]`,
	} {
		forged := forged
		t.Run(forged, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			rootPackage := `{"name":"development","dependencies":{"author-runtime":"1.0.0"},"pi":{"extensions":["./author/extension.ts"],"skills":["./author/skills"],"subagents":{"agents":["./author/agents"]},"custom":true}}`
			writeTestFile(t, workspace, "package.json", rootPackage)
			writeTestFile(t, workspace, "README.md", "author-owned\n")
			state := fmt.Sprintf(`{"version":1,"files":[],"pi":{%s}}`, forged)
			writeTestFile(t, workspace, ".agentbundler/compatibility.json", state)
			generated := `{"dependencies":{"generated-runtime":"1.0.0"},"pi":{"extensions":["./extensions/generated.ts"],"skills":["./skills"],"subagents":{"agents":["./agents"]}}}`
			request := piCompatibilityRequest(workspace, generated)
			request.Config = nil

			_, diagnostics := Prepare(request)
			if !hasDiagnostic(diagnostics, "compatibility-ownership-invalid") {
				t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
			}
			data, err := os.ReadFile(filepath.Join(workspace, "package.json"))
			if err != nil || string(data) != rootPackage {
				t.Fatalf("author package.json changed: data=%q err=%v", data, err)
			}
			readme, err := os.ReadFile(filepath.Join(workspace, "README.md"))
			if err != nil || string(readme) != "author-owned\n" {
				t.Fatalf("author README changed: data=%q err=%v", readme, err)
			}
		})
	}
}

func TestPrepareRejectsTraversingPiOwnershipState(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeTestFile(t, workspace, ".agentbundler/compatibility.json", `{"version":1,"files":[],"pi":{"extensions":["./../outside"]}}`)
	_, diagnostics := Prepare(Request{WorkspaceRoot: workspace, Output: "dist"})
	if !hasDiagnostic(diagnostics, "compatibility-ownership-invalid") {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
}

func TestPrepareRejectsSymlinkedRepositoryMarker(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(workspace, ".claude-plugin")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, diagnostics := Prepare(Request{
		WorkspaceRoot: workspace, Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude, Files: []model.PlannedFile{
			{Path: ".claude-plugin/marketplace.json", Bytes: []byte(`{"plugins":[{"name":"alpha","source":"./alpha"}]}`)},
			{Path: "alpha/.claude-plugin/plugin.json", Bytes: []byte("{}")},
		}}}},
	})
	if !hasDiagnostic(diagnostics, "compatibility-path-unsafe") {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
}

func TestPrepareRejectsMissingGeneratedMarketplace(t *testing.T) {
	t.Parallel()

	_, diagnostics := Prepare(Request{
		WorkspaceRoot: t.TempDir(), Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude}},
		Plan:   model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude}}},
	})
	if !hasDiagnostic(diagnostics, "compatibility-catalog-missing") {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
}

func TestPrepareCopiesCodexProjectAgentsFromCanonicalTargetPlan(t *testing.T) {
	t.Parallel()

	catalog := `{"name":"tools","plugins":[{"name":"alpha","source":{"source":"local","path":"./alpha"}}]}`
	plan, diagnostics := Prepare(Request{
		WorkspaceRoot: t.TempDir(), Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetCodex}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetCodex, Files: []model.PlannedFile{
			{Path: ".agents/plugins/marketplace.json", Bytes: []byte(catalog)},
			{Path: "alpha/.codex-plugin/plugin.json", Bytes: []byte("{}")},
			{Path: ".codex/agents/reviewer.toml", Bytes: []byte("name = \"reviewer\"\n")},
		}}}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
	file, ok := planFile(plan, ".codex/agents/reviewer.toml")
	if !ok || string(file.Bytes) != "name = \"reviewer\"\n" {
		t.Fatalf("Codex compatibility agent = %#v, present=%t", file, ok)
	}
}

func TestPrepareMergesPiPackageWithoutDeletingDevelopmentFields(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, workspace, "package.json", `{
  "name": "development-package",
  "private": true,
  "scripts": {"test": "go test ./..."},
  "dependencies": {"unrelated": "1.0.0"},
  "pi": {"extensions": ["./dev/local.ts"], "custom": true}
}
`)
	generated := `{"name":"tools","dependencies":{"pi-subagents":"0.34.0","chalk":"5.6.2"},"pi":{"extensions":["./extensions/hooks.ts","./node_modules/pi-subagents/src/extension/index.ts"],"skills":["./skills"],"subagents":{"agents":["./agents"]}}}`
	plan, diagnostics := Prepare(Request{
		WorkspaceRoot: workspace, Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetPi}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetPi, Files: []model.PlannedFile{
			{Path: "package.json", Bytes: []byte(generated)},
			{Path: "extensions/hooks.ts", Bytes: []byte("export {}")},
			{Path: "node_modules/pi-subagents/src/extension/index.ts", Bytes: []byte("export {}")},
			{Path: "skills/demo/SKILL.md", Bytes: []byte("# Demo")},
			{Path: "agents/reviewer.md", Bytes: []byte("review")},
		}}}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
	file, ok := planFile(plan, "package.json")
	if !ok {
		t.Fatalf("merged package.json missing from %#v", plan.Files)
	}
	for _, pair := range [][2]string{{`"name"`, `"private"`}, {`"private"`, `"scripts"`}, {`"scripts"`, `"dependencies"`}, {`"dependencies"`, `"pi"`}} {
		if strings.Index(string(file.Bytes), pair[0]) >= strings.Index(string(file.Bytes), pair[1]) {
			t.Fatalf("root package field order changed: %s", file.Bytes)
		}
	}
	var document map[string]any
	if err := json.Unmarshal(file.Bytes, &document); err != nil {
		t.Fatal(err)
	}
	if document["name"] != "development-package" || document["private"] != true || document["scripts"] == nil {
		t.Fatalf("development fields changed: %#v", document)
	}
	dependencies := document["dependencies"].(map[string]any)
	if !reflect.DeepEqual(dependencies, map[string]any{"chalk": "5.6.2", "pi-subagents": "0.34.0", "unrelated": "1.0.0"}) {
		t.Fatalf("dependencies = %#v", dependencies)
	}
	pi := document["pi"].(map[string]any)
	if pi["custom"] != true {
		t.Fatalf("unrelated Pi field removed: %#v", pi)
	}
	assertStrings(t, pi["extensions"], []string{"./dev/local.ts", "./dist/pi/extensions/hooks.ts", "./node_modules/pi-subagents/src/extension/index.ts"})
	assertStrings(t, pi["skills"], []string{"./dist/pi/skills"})
	subagents := pi["subagents"].(map[string]any)
	assertStrings(t, subagents["agents"], []string{"./dist/pi/agents"})
}

func TestPiNPMRCOwnershipPreservesUnrelatedKeysAndCleansStaleSetting(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		before string
		built  string
	}{
		{name: "new file", before: "", built: ownedLegacyPeerDepsSetting},
		{name: "unrelated key", before: "registry=https://registry.example\n", built: "registry=https://registry.example\n" + ownedLegacyPeerDepsSetting},
		{name: "missing final newline", before: "audit=false", built: "audit=false\n" + ownedLegacyPeerDepsSetting},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeTestFile(t, workspace, "package.json", `{}`)
			if test.before != "" {
				writeTestFile(t, workspace, ".npmrc", test.before)
			}
			request := piCompatibilityRequest(workspace, `{"pi":{"skills":["./skills"]}}`)
			plan, diagnostics := Prepare(request)
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			npmrc, ok := planFile(plan, ".npmrc")
			if !ok || string(npmrc.Bytes) != test.built {
				t.Fatalf("generated .npmrc = %q, present=%t, want %q", npmrc.Bytes, ok, test.built)
			}
			if diagnostics := Write(plan, workspace); len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			cleanupRequest := request
			cleanupRequest.Config = nil
			cleanup, diagnostics := Prepare(cleanupRequest)
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			if diagnostics := Write(cleanup, workspace); len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			data, err := os.ReadFile(filepath.Join(workspace, ".npmrc"))
			if test.before == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("generated-only .npmrc remains: data=%q err=%v", data, err)
				}
			} else if err != nil || string(data) != test.before {
				t.Fatalf("cleaned .npmrc = %q err=%v, want %q", data, err, test.before)
			}
		})
	}
}

func TestPiNPMRCRejectsForgedOwnershipWithoutRemovingAuthorSetting(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeTestFile(t, workspace, "package.json", `{}`)
	writeTestFile(t, workspace, ".npmrc", legacyPeerDepsSetting)
	writeTestFile(t, workspace, ".agentbundler/compatibility.json", `{"version":1,"files":[],"pi":{"legacyPeerDeps":true}}`)
	request := piCompatibilityRequest(workspace, `{"pi":{"skills":["./skills"]}}`)
	request.Config = nil
	_, diagnostics := Prepare(request)
	if !hasDiagnostic(diagnostics, "compatibility-ownership-invalid") {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
	data, err := os.ReadFile(filepath.Join(workspace, ".npmrc"))
	if err != nil || string(data) != legacyPeerDepsSetting {
		t.Fatalf("author .npmrc changed: data=%q err=%v", data, err)
	}
}

func TestPiNPMRCPreservesAuthorOwnedTrueAndRejectsConflicts(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeTestFile(t, workspace, "package.json", `{}`)
	writeTestFile(t, workspace, ".npmrc", "legacy-peer-deps=true\nregistry=https://registry.example\n")
	request := piCompatibilityRequest(workspace, `{"pi":{"skills":["./skills"]}}`)
	plan, diagnostics := Prepare(request)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if diagnostics := Write(plan, workspace); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	cleanupRequest := request
	cleanupRequest.Config = nil
	cleanup, diagnostics := Prepare(cleanupRequest)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if diagnostics := Write(cleanup, workspace); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	data, err := os.ReadFile(filepath.Join(workspace, ".npmrc"))
	if err != nil || string(data) != "legacy-peer-deps=true\nregistry=https://registry.example\n" {
		t.Fatalf("author .npmrc changed: data=%q err=%v", data, err)
	}

	for _, content := range []string{
		"legacy-peer-deps=false\n",
		"legacy-peer-deps=true\nlegacy-peer-deps=true\n",
	} {
		workspace := t.TempDir()
		writeTestFile(t, workspace, "package.json", `{}`)
		writeTestFile(t, workspace, ".npmrc", content)
		_, diagnostics := Prepare(piCompatibilityRequest(workspace, `{"pi":{"skills":["./skills"]}}`))
		if !hasDiagnostic(diagnostics, "compatibility-npmrc-conflict") {
			t.Fatalf("Prepare(.npmrc=%q) diagnostics = %#v", content, diagnostics)
		}
	}
}

func piCompatibilityRequest(workspace, generated string) Request {
	return Request{
		WorkspaceRoot: workspace, Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetPi}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetPi, Files: []model.PlannedFile{
			{Path: "package.json", Bytes: []byte(generated)},
		}}}},
	}
}

func TestPiCleanupPreservesPreexistingEqualEntries(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, workspace, "package.json", `{"dependencies":{"runtime":"1.0.0"},"pi":{"skills":["./dist/pi/skills"]}}`)
	request := Request{
		WorkspaceRoot: workspace, Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetPi}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetPi, Files: []model.PlannedFile{
			{Path: "package.json", Bytes: []byte(`{"dependencies":{"runtime":"1.0.0"},"pi":{"skills":["./skills"]}}`)},
		}}}},
	}
	plan, diagnostics := Prepare(request)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if diagnostics := Write(plan, workspace); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	cleanupRequest := request
	cleanupRequest.Config = nil
	cleanup, diagnostics := Prepare(cleanupRequest)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if diagnostics := Write(cleanup, workspace); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["dependencies"].(map[string]any)["runtime"] != "1.0.0" || !arrayContains(document["pi"].(map[string]any)["skills"], "./dist/pi/skills") {
		t.Fatalf("preexisting entries were removed: %#v", document)
	}
}

func TestPrepareRejectsPiDependencyCollisionAndUnsafePath(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		root      string
		generated string
		code      string
	}{
		{name: "dependency collision", root: `{"dependencies":{"runtime":"2.0.0"}}`, generated: `{"dependencies":{"runtime":"1.0.0"},"pi":{"skills":["./skills"]}}`, code: "compatibility-dependency-collision"},
		{name: "unsafe path", root: `{}`, generated: `{"pi":{"skills":["./skills/../outside"]}}`, code: "compatibility-pi-manifest-invalid"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeTestFile(t, workspace, "package.json", test.root)
			_, diagnostics := Prepare(Request{
				WorkspaceRoot: workspace, Output: "dist",
				Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetPi}},
				Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetPi, Files: []model.PlannedFile{
					{Path: "package.json", Bytes: []byte(test.generated)},
				}}}},
			})
			if !hasDiagnostic(diagnostics, test.code) {
				t.Fatalf("Prepare() diagnostics = %#v, want %s", diagnostics, test.code)
			}
		})
	}
}

func TestPrepareMergesSeparatePiPackagePaths(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, workspace, "package.json", `{"name":"development"}`)
	plan, diagnostics := Prepare(Request{
		WorkspaceRoot: workspace, Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetPi}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{
			Target: model.TargetPi, Packages: []model.PackageID{"alpha", "beta"},
			Files: []model.PlannedFile{
				{Path: "alpha/package.json", Bytes: []byte(`{"dependencies":{"shared":"1.0.0"},"pi":{"skills":["./skills"]}}`)},
				{Path: "beta/package.json", Bytes: []byte(`{"dependencies":{"shared":"1.0.0"},"pi":{"extensions":["./extensions/tool.ts"],"subagents":{"agents":["./agents"]}}}`)},
			},
		}}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
	file, ok := planFile(plan, "package.json")
	if !ok {
		t.Fatal("root package.json missing")
	}
	var document map[string]any
	if err := json.Unmarshal(file.Bytes, &document); err != nil {
		t.Fatal(err)
	}
	pi := document["pi"].(map[string]any)
	assertStrings(t, pi["skills"], []string{"./dist/pi/alpha/skills"})
	assertStrings(t, pi["extensions"], []string{"./dist/pi/beta/extensions/tool.ts"})
	assertStrings(t, pi["subagents"].(map[string]any)["agents"], []string{"./dist/pi/beta/agents"})
}

func TestWriteCompareDriftAndStaleCleanup(t *testing.T) {
	workspace := t.TempDir()
	request := Request{
		WorkspaceRoot: workspace, Output: "dist",
		Config: &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude, Files: []model.PlannedFile{
			{Path: ".claude-plugin/marketplace.json", Bytes: []byte(`{"plugins":[{"name":"alpha","source":"./alpha"}]}`)},
			{Path: "alpha/.claude-plugin/plugin.json", Bytes: []byte("{}")},
		}}}},
	}
	plan, diagnostics := Prepare(request)
	if len(diagnostics) != 0 {
		t.Fatalf("Prepare() diagnostics = %#v", diagnostics)
	}
	if diagnostics := Write(plan, workspace); len(diagnostics) != 0 {
		t.Fatalf("Write() diagnostics = %#v", diagnostics)
	}
	if diagnostics, drift := Compare(plan, workspace); drift || len(diagnostics) != 0 {
		t.Fatalf("Compare(current) = (%#v, %t)", diagnostics, drift)
	}

	writeTestFile(t, workspace, ".claude-plugin/marketplace.json", "drift\n")
	if diagnostics, drift := Compare(plan, workspace); !drift || !hasDiagnostic(diagnostics, "COMPATIBILITY_DRIFT_CHANGED") {
		t.Fatalf("Compare(drift) = (%#v, %t)", diagnostics, drift)
	}
	if diagnostics := Write(plan, workspace); len(diagnostics) != 0 {
		t.Fatalf("Write(restore) diagnostics = %#v", diagnostics)
	}

	cleanup, diagnostics := Prepare(Request{WorkspaceRoot: workspace, Output: "dist"})
	if len(diagnostics) != 0 {
		t.Fatalf("Prepare(cleanup) diagnostics = %#v", diagnostics)
	}
	if diagnostics, drift := Compare(cleanup, workspace); !drift || !hasDiagnostic(diagnostics, "COMPATIBILITY_DRIFT_EXTRA") {
		t.Fatalf("Compare(stale) = (%#v, %t)", diagnostics, drift)
	}
	if diagnostics := Write(cleanup, workspace); len(diagnostics) != 0 {
		t.Fatalf("Write(cleanup) diagnostics = %#v", diagnostics)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude-plugin", "marketplace.json")); !os.IsNotExist(err) {
		t.Fatalf("stale marker still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".agentbundler", "compatibility.json")); !os.IsNotExist(err) {
		t.Fatalf("stale ownership state still exists: %v", err)
	}
}

func planFile(plan Plan, path model.RelativePath) (File, bool) {
	for _, file := range plan.Files {
		if file.Path == path {
			return file, true
		}
	}
	return File{}, false
}

func hasDiagnostic(diagnostics []model.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func arrayContains(value any, want string) bool {
	values, _ := value.([]any)
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertStrings(t *testing.T, value any, want []string) {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want string array", value)
	}
	got := make([]string, len(items))
	for index, item := range items {
		got[index], ok = item.(string)
		if !ok {
			t.Fatalf("value = %#v, want string array", value)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
}

func TestRootMarketplaceJSONIsDeterministic(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	catalog := `{"z":1,"plugins":[{"source":"./alpha","name":"alpha"}],"a":2}`
	request := Request{
		WorkspaceRoot: workspace,
		Output:        "dist",
		Config:        &model.CompatibilityConfig{RootManifests: []model.TargetID{model.TargetClaude}},
		Plan: model.BuildPlan{Targets: []model.TargetPlan{{
			Target: model.TargetClaude,
			Files: []model.PlannedFile{
				{Path: ".claude-plugin/marketplace.json", Bytes: []byte(catalog)},
				{Path: "alpha/.claude-plugin/plugin.json", Bytes: []byte("{}")},
			},
		}}},
	}
	first, diagnostics := Prepare(request)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	second, diagnostics := Prepare(request)
	if len(diagnostics) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("plans differ: first=%#v second=%#v diagnostics=%#v", first, second, diagnostics)
	}
	file, _ := planFile(first, ".claude-plugin/marketplace.json")
	if !strings.HasSuffix(string(file.Bytes), "\n") {
		t.Fatalf("marketplace has no final newline: %q", file.Bytes)
	}
}
