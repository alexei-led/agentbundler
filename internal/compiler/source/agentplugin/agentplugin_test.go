package agentplugin

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/agentplugins"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// minimalManifest builds a minimal valid SourceManifest for the given plugins.
func minimalManifest(root model.RelativePath, plugins ...model.RelativePath) model.SourceManifest {
	return model.SourceManifest{
		Kind:        model.SourceKindAgentPlugin,
		Root:        root,
		Targets:     []model.TargetID{model.TargetClaude},
		Output:      "generated",
		AgentPlugin: &model.AgentPluginSourceConfig{Plugins: plugins},
	}
}

func validPluginJSON(name string) string {
	return `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"` + name + `"}`
}

func validMCPJSON() string {
	return `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"srv":{"type":"stdio","command":"node"}}}`
}

// write creates a file at root/relative with the given content.
func write(t *testing.T, root, relative, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", p, err)
	}
}

func openWorkspace(t *testing.T, dir string) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// --- Tests ---

func TestAgentPluginMinimalImport(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/my-plugin/plugin.json", validPluginJSON("my-plugin"))

	manifest := minimalManifest("source", "my-plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(inventory.Packages) != 1 {
		t.Fatalf("packages = %d, want 1", len(inventory.Packages))
	}
	pkg := inventory.Packages[0]
	if pkg.Identity != "my-plugin" {
		t.Errorf("identity = %q, want %q", pkg.Identity, "my-plugin")
	}
	if pkg.AgentPlugin == nil {
		t.Fatal("AgentPlugin is nil")
	}
	if pkg.AgentPlugin.Profile != agentplugins.ProfileID {
		t.Errorf("profile = %q, want %q", pkg.AgentPlugin.Profile, agentplugins.ProfileID)
	}
	if pkg.AgentPlugin.Manifest.Name != "my-plugin" {
		t.Errorf("manifest.name = %q, want %q", pkg.AgentPlugin.Manifest.Name, "my-plugin")
	}
}

func TestAgentPluginWithSkill(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/my-skill/SKILL.md", "---\ntitle: My Skill\n---\nBody text.")

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	pkg := inventory.Packages[0]
	if len(pkg.Assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(pkg.Assets))
	}
	skill := pkg.Assets[0]
	if skill.Identity != "skill/my-skill" {
		t.Errorf("skill identity = %q", skill.Identity)
	}
	if skill.Kind != model.AssetKindSkill {
		t.Errorf("skill kind = %q", skill.Kind)
	}
	if skill.Base.Body != "Body text." {
		t.Errorf("skill body = %q", skill.Base.Body)
	}
	// Skill should declare agent-plugin.skills capability use.
	if len(skill.CapabilityUses) == 0 || skill.CapabilityUses[0].Key != model.CapabilityKeyAgentPluginSkills {
		t.Errorf("skill capability uses = %v", skill.CapabilityUses)
	}
}

func TestAgentPluginWithMCP(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/mcp.json", validMCPJSON())

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	pkg := inventory.Packages[0]
	if pkg.AgentPlugin == nil {
		t.Fatal("AgentPlugin is nil")
	}
	if len(pkg.AgentPlugin.MCPServers) != 1 {
		t.Fatalf("MCP servers = %d, want 1", len(pkg.AgentPlugin.MCPServers))
	}
	srv := pkg.AgentPlugin.MCPServers[0]
	if srv.Name != "srv" || srv.Transport != model.MCPTransportStdio {
		t.Errorf("MCP server = %+v", srv)
	}
	if srv.Stdio == nil || srv.Stdio.Command != "node" {
		t.Errorf("MCP stdio = %+v", srv.Stdio)
	}
}

func TestAgentPluginAllMCPTransports(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/mcp.json", `{
		"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers":{
			"a":{"type":"stdio","command":"node"},
			"b":{"type":"streamable-http","url":"https://example.com/mcp"},
			"c":{"type":"sse","url":"https://example.com/sse"}
		}
	}`)

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	servers := inventory.Packages[0].AgentPlugin.MCPServers
	if len(servers) != 3 {
		t.Fatalf("MCP servers = %d, want 3", len(servers))
	}
	byName := make(map[string]model.MCPServer)
	for _, s := range servers {
		byName[s.Name] = s
	}
	if byName["a"].Transport != model.MCPTransportStdio {
		t.Errorf("server a transport = %q", byName["a"].Transport)
	}
	if byName["b"].Transport != model.MCPTransportStreamableHTTP || byName["b"].Remote == nil {
		t.Errorf("server b = %+v", byName["b"])
	}
	if byName["c"].Transport != model.MCPTransportSSE || byName["c"].Remote == nil {
		t.Errorf("server c = %+v", byName["c"])
	}
}

func TestAgentPluginDerivesPackageCapabilityUses(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", `{
		"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name":"plugin",
		"extensions":{"com.example.client":{"theme":"dark"}},
		"future-field":"preserved"
	}`)
	write(t, tmp, "source/plugin/mcp.json", `{
		"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers":{
			"stdio":{"type":"stdio","command":"node"},
			"http":{"type":"streamable-http","url":"https://example.com/mcp"},
			"sse":{"type":"sse","url":"https://example.com/sse"}
		}
	}`)
	write(t, tmp, "source/plugin/README.md", "package file")

	manifest := minimalManifest("source", "plugin")
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, openWorkspace(t, tmp))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	got := make(map[model.CapabilityKey]bool)
	for _, use := range inventory.Packages[0].CapabilityUses {
		got[use.Key] = true
		if !strings.HasPrefix(string(use.Location.Path), "source/plugin/") {
			t.Errorf("capability %q location = %q", use.Key, use.Location.Path)
		}
	}
	want := []model.CapabilityKey{
		model.CapabilityKeyAgentPluginMCPStdio,
		model.CapabilityKeyAgentPluginMCPStreamableHTTP,
		model.CapabilityKeyAgentPluginMCPSSE,
		model.CapabilityKeyAgentPluginExtensions,
		model.CapabilityKeyAgentPluginUnknownJSON,
		model.CapabilityKeyAgentPluginPackageFiles,
	}
	for _, key := range want {
		if !got[key] {
			t.Errorf("package capability uses = %v; missing %q", got, key)
		}
	}
}

func TestAgentPluginWithExtensions(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", `{
		"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name":"plugin",
		"extensions":{"com.example.client":{"theme":"dark"}}
	}`)
	write(t, tmp, "source/plugin/extensions/com.example.client/icon.png", "png-bytes")

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	ap := inventory.Packages[0].AgentPlugin
	if len(ap.Extensions) != 1 {
		t.Fatalf("extensions = %d, want 1", len(ap.Extensions))
	}
	ext := ap.Extensions[0]
	if ext.Namespace != "com.example.client" {
		t.Errorf("namespace = %q", ext.Namespace)
	}
	if m, ok := ext.Manifest.(map[string]any); !ok || m["theme"] != "dark" {
		t.Errorf("manifest = %v", ext.Manifest)
	}
	if len(ext.PackageFiles) != 1 || ext.PackageFiles[0].Path != "icon.png" {
		t.Errorf("extension files = %v", ext.PackageFiles)
	}
	// Extension files must NOT appear in PackageFiles.
	for _, pf := range ap.PackageFiles {
		if strings.Contains(string(pf.Path), "extensions") {
			t.Errorf("extension file leaked into PackageFiles: %q", pf.Path)
		}
	}
}

func TestAgentPluginUnknownJSONPreserved(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", `{
		"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name":"plugin",
		"future-field":"preserved"
	}`)

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	ap := inventory.Packages[0].AgentPlugin
	if ap.UnknownManifest == nil || ap.UnknownManifest["future-field"] != "preserved" {
		t.Errorf("UnknownManifest = %v", ap.UnknownManifest)
	}
}

func TestAgentPluginMultipleRoots(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/alpha/plugin.json", validPluginJSON("alpha"))
	write(t, tmp, "source/beta/plugin.json", validPluginJSON("beta"))

	manifest := minimalManifest("source", "alpha", "beta")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(inventory.Packages) != 2 {
		t.Fatalf("packages = %d, want 2", len(inventory.Packages))
	}
	names := []string{string(inventory.Packages[0].Identity), string(inventory.Packages[1].Identity)}
	if !reflect.DeepEqual(names, []string{"alpha", "beta"}) {
		t.Errorf("package names = %v", names)
	}
}

func TestAgentPluginDuplicatePathCaseFold(t *testing.T) {
	tmp := t.TempDir()
	manifest := minimalManifest("source", "plugin", "Plugin")
	ws := openWorkspace(t, tmp)
	_, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if !hasDiag(diags, "duplicate plugin path") {
		t.Errorf("expected duplicate plugin path diagnostic, got: %v", diags)
	}
}

func TestAgentPluginDuplicateName(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/a/plugin.json", validPluginJSON("same-name"))
	write(t, tmp, "source/b/plugin.json", validPluginJSON("same-name"))

	manifest := minimalManifest("source", "a", "b")
	ws := openWorkspace(t, tmp)
	_, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if !hasDiag(diags, "same-name") {
		t.Errorf("expected duplicate name diagnostic, got: %v", diags)
	}
}

func TestAgentPluginMissingPluginJSON(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "source", "plugin"), 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	_, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if !hasDiag(diags, "plugin.json not found") {
		t.Errorf("expected missing plugin.json diagnostic, got: %v", diags)
	}
}

func TestAgentPluginPackageFiles(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/README.md", "# Plugin\n")
	write(t, tmp, "source/plugin/data/config.json", `{"key":"val"}`)

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	pkgFiles := inventory.Packages[0].AgentPlugin.PackageFiles
	paths := make(map[string]bool, len(pkgFiles))
	for _, pf := range pkgFiles {
		paths[string(pf.Path)] = true
	}
	if !paths["README.md"] {
		t.Error("README.md missing from PackageFiles")
	}
	if !paths["data/config.json"] {
		t.Error("data/config.json missing from PackageFiles")
	}
	// plugin.json must not appear in PackageFiles.
	if paths["plugin.json"] {
		t.Error("plugin.json should not be in PackageFiles")
	}
}

func TestAgentPluginSkillFilesExcludedFromPackageFiles(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/my-skill/SKILL.md", "# Skill")
	write(t, tmp, "source/plugin/my-skill/context.txt", "some context")

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	ap := inventory.Packages[0].AgentPlugin
	for _, pf := range ap.PackageFiles {
		if strings.HasPrefix(string(pf.Path), "my-skill/") {
			t.Errorf("skill file leaked into PackageFiles: %q", pf.Path)
		}
	}
	// Skill support file should be in skill Base.Files.
	assets := inventory.Packages[0].Assets
	if len(assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(assets))
	}
	if _, ok := assets[0].Base.Files["context.txt"]; !ok {
		t.Errorf("skill support file missing from Base.Files")
	}
}

func TestAgentPluginRejectsMalformedImmediateChildSkill(t *testing.T) {
	for _, test := range []struct {
		name     string
		skillDir string
		content  string
		want     string
	}{
		{name: "invalid identity", skillDir: " bad", content: "# Skill\n", want: "invalid identity"},
		{name: "invalid frontmatter", skillDir: "broken", content: "---\nname: one\nname: two\n---\nBody\n", want: "SKILL.md"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmp := t.TempDir()
			write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
			write(t, tmp, "source/plugin/"+test.skillDir+"/SKILL.md", test.content)

			inventory, diags := InspectAgentPluginRoot(minimalManifest("source", "plugin"), tmp, openWorkspace(t, tmp))
			if !hasDiag(diags, test.want) {
				t.Fatalf("diagnostics = %v; want %q", diags, test.want)
			}
			if len(inventory.Packages) != 0 {
				t.Fatalf("malformed skill produced packages: %#v", inventory.Packages)
			}
			wantPath := model.RelativePath("source/plugin/" + test.skillDir + "/SKILL.md")
			if diags[0].Location == nil || diags[0].Location.Path != wantPath {
				t.Fatalf("diagnostic location = %#v; want %q", diags[0].Location, wantPath)
			}
		})
	}
}

func TestAgentPluginInputsTracked(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/mcp.json", validMCPJSON())

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	paths := make(map[string]bool, len(inventory.Inputs))
	for _, inp := range inventory.Inputs {
		paths[string(inp.Path)] = true
		if len(inp.SHA256) != 64 {
			t.Errorf("input %q has invalid SHA256 length %d", inp.Path, len(inp.SHA256))
		}
	}
	if !paths["source/plugin/plugin.json"] {
		t.Error("plugin.json not in inputs")
	}
	if !paths["source/plugin/mcp.json"] {
		t.Error("mcp.json not in inputs")
	}
}

func TestAgentPluginContainedSymlinkMaterialized(t *testing.T) {
	if !symlinkSupported() {
		t.Skip("symlinks not supported on this platform")
	}
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/real.txt", "real content")
	// Create a contained symlink: link.txt -> real.txt
	if err := os.Symlink("real.txt", filepath.Join(tmp, "source", "plugin", "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics for contained symlink: %v", diags)
	}
	pkgFiles := inventory.Packages[0].AgentPlugin.PackageFiles
	paths := make(map[string][]byte)
	for _, pf := range pkgFiles {
		paths[string(pf.Path)] = pf.Bytes
	}
	// The materialized symlink should appear as the symlink's path.
	if string(paths["link.txt"]) != "real content" {
		t.Errorf("symlink materialization: link.txt bytes = %q", paths["link.txt"])
	}
	// Both the real file and the materialized link should be present.
	if _, ok := paths["real.txt"]; !ok {
		t.Error("real.txt missing from PackageFiles")
	}
}

func TestAgentPluginExternalSymlinkRejected(t *testing.T) {
	if !symlinkSupported() {
		t.Skip("symlinks not supported on this platform")
	}
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	// Create a symlink pointing outside the workspace.
	if err := os.Symlink("/etc/passwd", filepath.Join(tmp, "source", "plugin", "evil.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	_, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if !hasErrors(diags) {
		t.Error("expected error for external symlink, got none")
	}
}

func TestAgentPluginDirectoryCycleRejected(t *testing.T) {
	if !symlinkSupported() {
		t.Skip("symlinks not supported on this platform")
	}
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	// sub -> . (creates a cycle: plugin/sub points back to plugin root)
	if err := os.Symlink(".", filepath.Join(tmp, "source", "plugin", "sub")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	_, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if !hasErrors(diags) {
		t.Error("expected error for directory cycle, got none")
	}
}

func TestAgentPluginSpecialFileRejected(t *testing.T) {
	// We test the error path by ensuring a FIFO (named pipe) is rejected.
	// This is Unix-only.
	if !symlinkSupported() {
		t.Skip("requires Unix")
	}
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	pipePath := filepath.Join(tmp, "source", "plugin", "fifo")
	if err := createFIFO(pipePath); err != nil {
		t.Skip("cannot create FIFO: " + err.Error())
	}

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	_, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if !hasErrors(diags) {
		t.Error("expected error for special file, got none")
	}
}

func TestAgentPluginRejectsOversizedFileBeforeReading(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	oversized := filepath.Join(tmp, "source", "plugin", "oversized.bin")
	file, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxFileSizeB + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, diags := InspectAgentPluginRoot(minimalManifest("source", "plugin"), tmp, openWorkspace(t, tmp))
	if !hasDiag(diags, "file exceeds 64 MiB size limit") {
		t.Fatalf("diagnostics = %v; want file size limit error", diags)
	}
}

func TestAgentPluginPackageFilesWithinLimits(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	write(t, tmp, "source/plugin/a.txt", "a")
	write(t, tmp, "source/plugin/b.txt", "b")

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	files := inventory.Packages[0].AgentPlugin.PackageFiles
	if len(files) != 2 {
		t.Errorf("PackageFiles = %d, want 2 (a.txt, b.txt)", len(files))
	}
}

func TestAgentPluginTraversalDepthLimit(t *testing.T) {
	tmp := t.TempDir()
	pluginRoot := filepath.Join(tmp, "plugin")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	path := pluginRoot
	for i := 0; i < maxDepth+1; i++ {
		path = filepath.Join(path, "nested")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, diags := traversePluginRoot(openWorkspace(t, pluginRoot))
	if !hasDiag(diags, "traversal depth limit exceeded") {
		t.Fatalf("diagnostics = %v; want traversal depth limit error", diags)
	}
}

func TestAgentPluginTraversalPathByteLimit(t *testing.T) {
	tmp := t.TempDir()
	root := openWorkspace(t, tmp)
	current := root
	relPathBytes := 0
	for relPathBytes <= maxPathBytes {
		const segment = "pppppppppppppppppppppppppppppppppppppppp"
		if relPathBytes > 0 {
			relPathBytes++ // slash between relative path components
		}
		relPathBytes += len(segment)
		if err := current.Mkdir(segment, 0o755); err != nil {
			t.Fatal(err)
		}
		child, err := current.OpenRoot(segment)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = child.Close() })
		current = child
	}

	_, diags := traversePluginRoot(root)
	if !hasDiag(diags, "path exceeds byte limit") {
		t.Fatalf("diagnostics = %v; want path byte limit error", diags)
	}
}

func TestAgentPluginTraversalEntryLimit(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "a", "a")
	write(t, tmp, "b", "b")
	limits := defaultTraversalLimits()
	limits.maxEntries = 1

	_, diags := traversePluginRootWithLimits(openWorkspace(t, tmp), limits)
	if !hasDiag(diags, "traversal entry limit exceeded") {
		t.Fatalf("diagnostics = %v; want traversal entry limit error", diags)
	}
}

func TestAgentPluginTraversalTotalBytesLimit(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "file", "ab")
	limits := defaultTraversalLimits()
	limits.maxTotalBytes = 1

	_, diags := traversePluginRootWithLimits(openWorkspace(t, tmp), limits)
	if !hasDiag(diags, "total file bytes exceed 256 MiB limit") {
		t.Fatalf("diagnostics = %v; want total byte limit error", diags)
	}
}

func TestAgentPluginInvalidManifestKind(t *testing.T) {
	manifest := model.SourceManifest{Kind: model.SourceKindBundle}
	ws := openWorkspace(t, t.TempDir())
	_, diags := InspectAgentPluginRoot(manifest, t.TempDir(), ws)
	if !hasErrors(diags) {
		t.Error("expected error for wrong manifest kind")
	}
}

func TestAgentPluginNestedSkillNotDiscovered(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))
	// Nested skill at depth 2 should NOT be discovered (only immediate children).
	write(t, tmp, "source/plugin/subdir/nested-skill/SKILL.md", "Nested skill.")

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(inventory.Packages[0].Assets) != 0 {
		t.Errorf("expected 0 skills for nested SKILL.md, got %d", len(inventory.Packages[0].Assets))
	}
}

func TestAgentPluginProvenance(t *testing.T) {
	tmp := t.TempDir()
	write(t, tmp, "source/plugin/plugin.json", validPluginJSON("plugin"))

	manifest := minimalManifest("source", "plugin")
	ws := openWorkspace(t, tmp)
	inventory, diags := InspectAgentPluginRoot(manifest, tmp, ws)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	// Inputs must have workspace-relative paths.
	for _, inp := range inventory.Inputs {
		if !strings.HasPrefix(string(inp.Path), "source/") {
			t.Errorf("input path %q is not workspace-relative", inp.Path)
		}
	}
	// PackageFile origins must also be workspace-relative.
	for _, pf := range inventory.Packages[0].AgentPlugin.PackageFiles {
		for _, o := range pf.Origin {
			if !strings.HasPrefix(string(o.Path), "source/") {
				t.Errorf("origin path %q is not workspace-relative", o.Path)
			}
		}
	}
}

// --- helper functions ---

func hasDiag(diags []model.Diagnostic, substr string) bool {
	for _, d := range diags {
		if strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}

func symlinkSupported() bool {
	tmp, err := os.MkdirTemp("", "symtest")
	if err != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	err = os.Symlink(tmp, filepath.Join(tmp, "link"))
	return err == nil
}
