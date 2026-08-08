// Package agentplugins renders normalized agent-plugin packages in the standard
// Agent Plugins 1.0.0 wire format. It is the authoritative adapter for all
// portable agent-plugin capabilities.
//
// The target emits one explicit archive unit per plugin (separate-only mode).
// Aggregate mode is rejected. Each unit contains plugin.json, optional mcp.json,
// skill files, extension files, and regular package files.
package agentplugins

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentpluginsformat "github.com/alexei-led/agentbundler/internal/agentplugins"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	// Target is the target ID for the standard Agent Plugins format.
	Target = model.TargetAgentPlugins
	// FormatRevision is the current output format revision.
	FormatRevision = 1
)

// capabilityRules declares all portable agent-plugin capabilities as native.
// Every registered capability key in the portable catalog is supported by this
// target since it emits the canonical wire format directly.
var capabilityRules = []model.CapabilityRule{
	{Key: model.CapabilityKeyAgentPluginSkills, State: model.CapabilityStateNative},
	{Key: model.CapabilityKeyAgentPluginMCPStdio, State: model.CapabilityStateNative},
	{Key: model.CapabilityKeyAgentPluginMCPStreamableHTTP, State: model.CapabilityStateNative},
	{Key: model.CapabilityKeyAgentPluginMCPSSE, State: model.CapabilityStateNative},
	{Key: model.CapabilityKeyAgentPluginExtensions, State: model.CapabilityStateNative},
	{Key: model.CapabilityKeyAgentPluginUnknownJSON, State: model.CapabilityStateNative},
	{Key: model.CapabilityKeyAgentPluginPackageFiles, State: model.CapabilityStateNative},
}

// Adapter renders normalized packages for the standard Agent Plugins target.
type Adapter struct{}

// New returns a new Adapter.
func New() Adapter { return Adapter{} }

// Target returns the target ID.
func (Adapter) Target() model.TargetID { return Target }

// FormatRevision returns the current format revision.
func (Adapter) FormatRevision() int { return FormatRevision }

// Capabilities returns the capability rules for this target.
func (Adapter) Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}

// Render renders an explicit target request.
func (adapter Adapter) Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	return Render(input)
}

// Render renders all packages in the request to the standard Agent Plugins wire
// format. Aggregate mode is rejected. Each package with AgentPlugin data emits
// one explicit archive unit.
func Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if input.PackageMode == model.TargetPackageModeAggregate {
		return emptyPlan(), []model.Diagnostic{diag("agent-plugins target does not support aggregate package mode")}
	}

	packageIDs := make([]model.PackageID, 0, len(input.Packages))
	var files []model.PlannedFile
	var units []model.ArchiveUnit

	// Track paths across packages to detect cross-package collisions.
	usedPaths := make(map[model.RelativePath]string)

	// Packages are pre-sorted by the render dispatch in target.go.
	for _, pkg := range input.Packages {
		if pkg.AgentPlugin == nil {
			return emptyPlan(), []model.Diagnostic{diag(fmt.Sprintf("package %q has no agent plugin data", pkg.Identity))}
		}

		root := string(pkg.Identity)
		pkgFiles, pkgDiags := renderPlugin(root, pkg, usedPaths)
		if len(pkgDiags) != 0 {
			return emptyPlan(), pkgDiags
		}
		files = append(files, pkgFiles...)
		packageIDs = append(packageIDs, pkg.Identity)
		units = append(units, model.ArchiveUnit{
			Root:   root,
			Stem:   root,
			Suffix: ".tar.gz",
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	return model.TargetPlan{
		Target:       Target,
		Packages:     packageIDs,
		Files:        files,
		NativeChecks: []model.NativeCheck{},
		ArchiveUnits: units,
	}, nil
}

// renderPlugin renders one plugin package under its root prefix and returns the
// planned files. Each file path is prefixed with root + "/". Collision detection
// uses usedPaths to reject duplicate paths across packages.
func renderPlugin(root string, pkg model.NormalizedPackage, usedPaths map[model.RelativePath]string) ([]model.PlannedFile, []model.Diagnostic) {
	data := pkg.AgentPlugin

	var files []model.PlannedFile

	addFile := func(relPath model.RelativePath, bytes []byte, executable bool, origin []model.SourceLocation) []model.Diagnostic {
		full := model.RelativePath(root + "/" + string(relPath))
		if owner, exists := usedPaths[full]; exists {
			return []model.Diagnostic{diag(fmt.Sprintf("package %q output path %q collides with %s", pkg.Identity, full, owner))}
		}
		usedPaths[full] = fmt.Sprintf("package %q", pkg.Identity)
		files = append(files, model.PlannedFile{
			Path:       full,
			Bytes:      append([]byte(nil), bytes...),
			Executable: executable,
			Origin:     model.CloneSourceLocations(origin),
		})
		return nil
	}

	// plugin.json — always present.
	pluginBytes, err := encodePluginManifest(*data)
	if err != nil {
		return nil, []model.Diagnostic{diag(fmt.Sprintf("package %q: encode plugin.json: %v", pkg.Identity, err))}
	}
	pluginBytes = append(pluginBytes, '\n')
	if diags := addFile("plugin.json", pluginBytes, false, nil); len(diags) != 0 {
		return nil, diags
	}

	// mcp.json — only if there are MCP servers or unknown MCP fields.
	if len(data.MCPServers) > 0 || len(data.UnknownMCP) > 0 {
		mcpBytes, err := encodeMCPConfig(*data)
		if err != nil {
			return nil, []model.Diagnostic{diag(fmt.Sprintf("package %q: encode mcp.json: %v", pkg.Identity, err))}
		}
		mcpBytes = append(mcpBytes, '\n')
		if diags := addFile("mcp.json", mcpBytes, false, nil); len(diags) != 0 {
			return nil, diags
		}
	}

	// Skill assets from pkg.Assets (kind == skill).
	for _, asset := range sortedSkillAssets(pkg.Assets) {
		skillName := strings.TrimPrefix(string(asset.Identity), "skill/")
		if skillName == string(asset.Identity) || skillName == "" {
			continue // not a well-formed skill identity
		}
		skillFiles, err := renderSkill(asset)
		if err != nil {
			return nil, []model.Diagnostic{diag(fmt.Sprintf("package %q skill %q: %v", pkg.Identity, asset.Identity, err))}
		}
		for _, sf := range skillFiles {
			relPath := model.RelativePath(skillName + "/" + string(sf.Path))
			if diags := addFile(relPath, sf.Bytes, sf.Executable, sf.Origin); len(diags) != 0 {
				return nil, diags
			}
		}
	}

	// Extension package files — emit under extensions/<namespace>/<path>.
	for _, ext := range data.Extensions {
		for _, pf := range ext.PackageFiles {
			relPath := model.RelativePath("extensions/" + ext.Namespace + "/" + string(pf.Path))
			if diags := addFile(relPath, pf.Bytes, pf.Executable, pf.Origin); len(diags) != 0 {
				return nil, diags
			}
		}
	}

	// Regular package files.
	for _, pf := range data.PackageFiles {
		if diags := addFile(pf.Path, pf.Bytes, pf.Executable, pf.Origin); len(diags) != 0 {
			return nil, diags
		}
	}

	return files, nil
}

// sortedSkillAssets returns assets of kind skill sorted by identity for determinism.
func sortedSkillAssets(assets []model.NormalizedAsset) []model.NormalizedAsset {
	var result []model.NormalizedAsset
	for _, a := range assets {
		if a.Kind == model.AssetKindSkill {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Identity < result[j].Identity })
	return result
}

// skillFile is one planned file relative to the skill name root.
type skillFile struct {
	Path       model.RelativePath
	Bytes      []byte
	Executable bool
	Origin     []model.SourceLocation
}

// renderSkill renders a skill asset's SKILL.md and support files.
func renderSkill(asset model.NormalizedAsset) ([]skillFile, error) {
	data, err := renderMarkdown(asset.Content.Frontmatter, asset.Content.Body)
	if err != nil {
		return nil, fmt.Errorf("render SKILL.md: %w", err)
	}
	files := []skillFile{{
		Path:  "SKILL.md",
		Bytes: data,
	}}

	// Add support files sorted for determinism.
	supportPaths := make([]model.RelativePath, 0, len(asset.Content.Files))
	for p := range asset.Content.Files {
		supportPaths = append(supportPaths, p)
	}
	sort.Slice(supportPaths, func(i, j int) bool { return supportPaths[i] < supportPaths[j] })
	for _, p := range supportPaths {
		content := asset.Content.Files[p]
		files = append(files, skillFile{
			Path:       p,
			Bytes:      append([]byte(nil), content.Bytes...),
			Executable: content.Executable,
			Origin:     model.CloneSourceLocations(content.Origin),
		})
	}
	return files, nil
}

// renderMarkdown encodes frontmatter (if non-empty) plus body as markdown bytes.
// Frontmatter is encoded as a single JSON line between YAML fences.
func renderMarkdown(frontmatter map[string]any, body string) ([]byte, error) {
	if len(frontmatter) == 0 {
		return []byte(body), nil
	}
	encoded, err := json.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(encoded) + "\n---\n" + body), nil
}

// encodeJSONFrontmatter is not needed; renderMarkdown handles it inline.

func emptyPlan() model.TargetPlan {
	return model.TargetPlan{
		Target:       Target,
		Packages:     []model.PackageID{},
		Files:        []model.PlannedFile{},
		NativeChecks: []model.NativeCheck{},
		ArchiveUnits: []model.ArchiveUnit{},
	}
}

func diag(message string) model.Diagnostic {
	return model.Diagnostic{Code: "invalid-agent-plugins-render", Severity: model.SeverityError, Message: message}
}

// encodePluginManifest converts AgentPluginData back to the wire PluginManifest
// and encodes it as deterministic JSON.
func encodePluginManifest(data model.AgentPluginData) ([]byte, error) {
	m := data.Manifest
	wire := agentpluginsformat.PluginManifest{
		Schema:      agentpluginsformat.PluginSchemaURL,
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Homepage:    m.Homepage,
		Repository:  m.Repository,
		License:     m.License,
		Unknown:     cloneAnyMap(data.UnknownManifest),
	}
	if len(m.Keywords) > 0 {
		wire.Keywords = append([]string(nil), m.Keywords...)
	}
	// Build extensions map from extensions list.
	if len(data.Extensions) > 0 {
		exts := make(map[string]any, len(data.Extensions))
		for _, ext := range data.Extensions {
			exts[ext.Namespace] = ext.Manifest
		}
		wire.Extensions = exts
	}
	return agentpluginsformat.EncodePluginManifest(wire)
}

// encodeMCPConfig converts AgentPluginData MCP fields to the wire MCPConfig
// and encodes it as deterministic JSON.
func encodeMCPConfig(data model.AgentPluginData) ([]byte, error) {
	servers := make([]agentpluginsformat.MCPServer, 0, len(data.MCPServers))
	for _, s := range data.MCPServers {
		ws := agentpluginsformat.MCPServer{
			Name:      s.Name,
			Transport: agentpluginsformat.MCPTransportType(s.Transport),
			Unknown:   cloneAnyMap(s.Unknown),
		}
		if s.Stdio != nil {
			env := make(map[string]string, len(s.Stdio.Env))
			for k, v := range s.Stdio.Env {
				env[k] = v
			}
			ws.Stdio = &agentpluginsformat.StdioTransport{
				Command: s.Stdio.Command,
				Args:    append([]string(nil), s.Stdio.Args...),
				Env:     env,
				Cwd:     s.Stdio.Cwd,
			}
		}
		if s.Remote != nil {
			headers := make(map[string]string, len(s.Remote.Headers))
			for k, v := range s.Remote.Headers {
				headers[k] = v
			}
			ws.Remote = &agentpluginsformat.RemoteTransport{
				URL:     s.Remote.URL,
				Headers: headers,
			}
		}
		servers = append(servers, ws)
	}
	config := agentpluginsformat.MCPConfig{
		Schema:  agentpluginsformat.MCPSchemaURL,
		Servers: servers,
		Unknown: cloneAnyMap(data.UnknownMCP),
	}
	return agentpluginsformat.EncodeMCPConfig(config)
}

// cloneAnyMap returns a shallow copy of m, or nil if m is empty.
func cloneAnyMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
