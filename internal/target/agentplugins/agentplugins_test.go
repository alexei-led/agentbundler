package agentplugins

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestRenderRejectsAggregateMode(t *testing.T) {
	t.Parallel()
	input := model.TargetRenderInput{
		Packages: []model.NormalizedPackage{{
			Identity:    "hello",
			Target:      model.TargetAgentPlugins,
			AgentPlugin: minimalPluginData("hello"),
		}},
		PackageMode: model.TargetPackageModeAggregate,
	}
	plan, diags := Render(input)
	if len(diags) == 0 || diags[0].Code != "invalid-agent-plugins-render" || !strings.Contains(diags[0].Message, "aggregate") {
		t.Fatalf("Render() = (%v, %v), want aggregate rejection", plan, diags)
	}
	if len(plan.Files) != 0 || len(plan.ArchiveUnits) != 0 {
		t.Fatalf("Render() produced partial plan: files=%v units=%v", plan.Files, plan.ArchiveUnits)
	}
}

func TestRenderMinimalPlugin(t *testing.T) {
	t.Parallel()
	input := model.TargetRenderInput{
		Packages: []model.NormalizedPackage{{
			Identity:    "hello",
			Target:      model.TargetAgentPlugins,
			AgentPlugin: minimalPluginData("hello"),
		}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	if plan.Target != model.TargetAgentPlugins {
		t.Fatalf("plan.Target = %q, want %q", plan.Target, model.TargetAgentPlugins)
	}

	// Verify plugin.json is present and at the correct path.
	pluginFile := findFile(plan, "hello/plugin.json")
	if pluginFile == nil {
		t.Fatalf("hello/plugin.json absent; paths = %v", plannedPaths(plan))
	}
	var doc map[string]any
	if err := json.Unmarshal(pluginFile.Bytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal plugin.json: %v", err)
	}
	if doc["name"] != "hello" {
		t.Fatalf("plugin.json name = %v, want hello", doc["name"])
	}
	if doc["$schema"] == nil {
		t.Fatalf("plugin.json missing $schema")
	}

	// No mcp.json for a plugin with no servers.
	if f := findFile(plan, "hello/mcp.json"); f != nil {
		t.Fatalf("mcp.json present for plugin with no MCP servers")
	}

	// Verify archive unit is set.
	if len(plan.ArchiveUnits) != 1 {
		t.Fatalf("ArchiveUnits = %v, want 1", plan.ArchiveUnits)
	}
	unit := plan.ArchiveUnits[0]
	if unit.Root != "hello" || unit.Stem != "hello" || unit.Suffix != ".tar.gz" {
		t.Fatalf("ArchiveUnit = %+v", unit)
	}
}

func TestRenderSkillAsset(t *testing.T) {
	t.Parallel()
	body := "Use this skill to test.\n"
	input := model.TargetRenderInput{
		Packages: []model.NormalizedPackage{{
			Identity:    "myplugin",
			Target:      model.TargetAgentPlugins,
			AgentPlugin: minimalPluginData("myplugin"),
			Assets: []model.NormalizedAsset{{
				Identity: "skill/my-skill",
				Kind:     model.AssetKindSkill,
				Content: model.AssetContent{
					Frontmatter: map[string]any{"description": "My skill"},
					Body:        body,
					Files:       map[model.RelativePath]model.FileContent{},
				},
				CapabilityUses: []model.CapabilityUse{
					{Key: model.CapabilityKeyAgentPluginSkills, Location: model.SourceLocation{Path: "skill/SKILL.md"}},
				},
			}},
		}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	skillFile := findFile(plan, "myplugin/my-skill/SKILL.md")
	if skillFile == nil {
		t.Fatalf("my-skill/SKILL.md absent; paths = %v", plannedPaths(plan))
	}
	if !strings.Contains(string(skillFile.Bytes), body) {
		t.Fatalf("SKILL.md body = %q, want to contain %q", skillFile.Bytes, body)
	}
}

func TestRenderMCPServers(t *testing.T) {
	t.Parallel()
	data := minimalPluginData("mcp-plugin")
	data.MCPServers = []model.MCPServer{{
		Name:      "my-server",
		Transport: model.MCPTransportStdio,
		Stdio:     &model.StdioMCPServer{Command: "my-tool", Args: []string{"--run"}},
	}}
	input := model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{{Identity: "mcp-plugin", Target: model.TargetAgentPlugins, AgentPlugin: data}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	mcpFile := findFile(plan, "mcp-plugin/mcp.json")
	if mcpFile == nil {
		t.Fatalf("mcp.json absent; paths = %v", plannedPaths(plan))
	}
	var doc map[string]any
	if err := json.Unmarshal(mcpFile.Bytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal mcp.json: %v", err)
	}
	servers, ok := doc["mcpServers"].(map[string]any)
	if !ok || servers["my-server"] == nil {
		t.Fatalf("mcp.json mcpServers = %v", doc["mcpServers"])
	}
}

func TestRenderExtensionFiles(t *testing.T) {
	t.Parallel()
	data := minimalPluginData("ext-plugin")
	data.Extensions = []model.ClientExtension{{
		Namespace: "com.example.ext",
		Manifest:  map[string]any{"version": "1.0"},
		PackageFiles: []model.PackageFile{{
			Path:   "ext-data.json",
			Bytes:  []byte(`{"key":"value"}`),
			SHA256: strings.Repeat("a", 64),
		}},
	}}
	input := model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{{Identity: "ext-plugin", Target: model.TargetAgentPlugins, AgentPlugin: data}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	extFile := findFile(plan, "ext-plugin/extensions/com.example.ext/ext-data.json")
	if extFile == nil {
		t.Fatalf("extension file absent; paths = %v", plannedPaths(plan))
	}
	// Extension manifest appears in plugin.json extensions field.
	pluginFile := findFile(plan, "ext-plugin/plugin.json")
	if pluginFile == nil {
		t.Fatalf("plugin.json absent")
	}
	var doc map[string]any
	if err := json.Unmarshal(pluginFile.Bytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal plugin.json: %v", err)
	}
	exts, ok := doc["extensions"].(map[string]any)
	if !ok || exts["com.example.ext"] == nil {
		t.Fatalf("plugin.json extensions = %v", doc["extensions"])
	}
}

func TestRenderUnknownJSONPreserved(t *testing.T) {
	t.Parallel()
	data := minimalPluginData("unknown-plugin")
	data.UnknownManifest = map[string]any{"customField": "customValue"}
	input := model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{{Identity: "unknown-plugin", Target: model.TargetAgentPlugins, AgentPlugin: data}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	pluginFile := findFile(plan, "unknown-plugin/plugin.json")
	if pluginFile == nil {
		t.Fatalf("plugin.json absent")
	}
	var doc map[string]any
	if err := json.Unmarshal(pluginFile.Bytes, &doc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if doc["customField"] != "customValue" {
		t.Fatalf("unknown field not preserved: %v", doc)
	}
}

func TestRenderPackageFilesIncluded(t *testing.T) {
	t.Parallel()
	data := minimalPluginData("file-plugin")
	data.PackageFiles = []model.PackageFile{{
		Path:   "README.md",
		Bytes:  []byte("# README\n"),
		SHA256: strings.Repeat("b", 64),
	}}
	input := model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{{Identity: "file-plugin", Target: model.TargetAgentPlugins, AgentPlugin: data}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	if f := findFile(plan, "file-plugin/README.md"); f == nil {
		t.Fatalf("package file absent; paths = %v", plannedPaths(plan))
	}
}

func TestRenderMultiplePluginsAreIndependent(t *testing.T) {
	t.Parallel()
	input := model.TargetRenderInput{
		Packages: []model.NormalizedPackage{
			{Identity: "alpha", Target: model.TargetAgentPlugins, AgentPlugin: minimalPluginData("alpha")},
			{Identity: "beta", Target: model.TargetAgentPlugins, AgentPlugin: minimalPluginData("beta")},
		},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	if len(plan.ArchiveUnits) != 2 {
		t.Fatalf("ArchiveUnits = %v, want 2", plan.ArchiveUnits)
	}
	if findFile(plan, "alpha/plugin.json") == nil || findFile(plan, "beta/plugin.json") == nil {
		t.Fatalf("missing plugin.json; paths = %v", plannedPaths(plan))
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()
	data := minimalPluginData("det-plugin")
	data.MCPServers = []model.MCPServer{
		{Name: "server-b", Transport: model.MCPTransportStdio, Stdio: &model.StdioMCPServer{Command: "b"}},
		{Name: "server-a", Transport: model.MCPTransportStdio, Stdio: &model.StdioMCPServer{Command: "a"}},
	}
	input := model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{{Identity: "det-plugin", Target: model.TargetAgentPlugins, AgentPlugin: data}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	first, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("Render() diagnostics = %v", diags)
	}
	second, diags := Render(input)
	if len(diags) != 0 {
		t.Fatalf("second Render() diagnostics = %v", diags)
	}
	if !reflect.DeepEqual(first.Files, second.Files) {
		t.Fatalf("Render is not deterministic:\nfirst  = %v\nsecond = %v", first.Files, second.Files)
	}
}

func TestCapabilitiesDeclareAllPortableKeys(t *testing.T) {
	t.Parallel()
	adapter := New()
	caps := adapter.Capabilities()
	expected := []model.CapabilityKey{
		model.CapabilityKeyAgentPluginSkills,
		model.CapabilityKeyAgentPluginMCPStdio,
		model.CapabilityKeyAgentPluginMCPStreamableHTTP,
		model.CapabilityKeyAgentPluginMCPSSE,
		model.CapabilityKeyAgentPluginExtensions,
		model.CapabilityKeyAgentPluginUnknownJSON,
		model.CapabilityKeyAgentPluginPackageFiles,
	}
	declared := make(map[model.CapabilityKey]model.CapabilityState, len(caps))
	for _, c := range caps {
		declared[c.Key] = c.State
	}
	for _, key := range expected {
		if declared[key] != model.CapabilityStateNative {
			t.Errorf("Capabilities()[%q] = %q, want native", key, declared[key])
		}
	}
}

func TestRenderRejectsMissingAgentPluginData(t *testing.T) {
	t.Parallel()
	input := model.TargetRenderInput{
		Packages: []model.NormalizedPackage{{
			Identity: "no-data",
			Target:   model.TargetAgentPlugins,
			// AgentPlugin is nil
		}},
		PackageMode: model.TargetPackageModeSeparate,
	}
	plan, diags := Render(input)
	if len(diags) == 0 || diags[0].Code != "invalid-agent-plugins-render" {
		t.Fatalf("Render() = (%v, %v), want rejection for missing AgentPlugin", plan, diags)
	}
}

// minimalPluginData returns an AgentPluginData with the required Profile and Manifest.Name set.
func minimalPluginData(name string) *model.AgentPluginData {
	return &model.AgentPluginData{
		Profile: "agent-plugins/1.0.0-bd383552",
		Manifest: model.AgentPluginManifest{
			Name: name,
		},
	}
}

func findFile(plan model.TargetPlan, path model.RelativePath) *model.PlannedFile {
	for i := range plan.Files {
		if plan.Files[i].Path == path {
			return &plan.Files[i]
		}
	}
	return nil
}

func plannedPaths(plan model.TargetPlan) []model.RelativePath {
	paths := make([]model.RelativePath, 0, len(plan.Files))
	for _, f := range plan.Files {
		paths = append(paths, f.Path)
	}
	return paths
}
