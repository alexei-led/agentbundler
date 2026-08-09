package agentplugin

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/agentplugins"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const diagnosticCode = "invalid-agent-plugin"

// InspectAgentPluginRoot imports all plugin packages declared by manifest
// within the workspace bounded by workspace. Each plugin path in
// manifest.AgentPlugin.Plugins is imported as a separate SourcePackage.
// Duplicate plugin paths (case-folded) and duplicate plugin names are rejected.
func InspectAgentPluginRoot(manifest model.SourceManifest, workspaceRoot string, workspace *os.Root) (model.SourceInventory, []model.Diagnostic) {
	if manifest.Kind != model.SourceKindAgentPlugin || manifest.AgentPlugin == nil {
		return model.SourceInventory{}, []model.Diagnostic{diag("", "manifest kind must be agent-plugin")}
	}
	config := manifest.AgentPlugin

	// Check for duplicate plugin paths using case-folded comparison.
	pluginPaths := make([]model.RelativePath, len(config.Plugins))
	copy(pluginPaths, config.Plugins)
	sort.Slice(pluginPaths, func(i, j int) bool {
		return strings.ToLower(string(pluginPaths[i])) < strings.ToLower(string(pluginPaths[j]))
	})
	seenFolded := make(map[string]model.RelativePath, len(pluginPaths))
	for _, pluginPath := range pluginPaths {
		folded := strings.ToLower(string(pluginPath))
		if existing, ok := seenFolded[folded]; ok {
			return model.SourceInventory{}, []model.Diagnostic{diag("", fmt.Sprintf(
				"duplicate plugin path %q (case-fold collision with %q)", pluginPath, existing))}
		}
		seenFolded[folded] = pluginPath
	}

	var (
		packages    []model.SourcePackage
		allInputs   []model.InputFile
		seenNames   = make(map[string]bool)
		diagnostics []model.Diagnostic
	)

	// Process in sorted order for determinism (case-insensitive sort preserves
	// config order for non-colliding paths; manifest order within that).
	for _, pluginPath := range config.Plugins {
		pkg, inputs, pluginDiags := importPlugin(manifest, workspaceRoot, workspace, pluginPath)
		diagnostics = append(diagnostics, pluginDiags...)
		if hasErrors(pluginDiags) {
			continue
		}
		// Reject duplicate package IDs (plugin names).
		name := string(pkg.Identity)
		if seenNames[name] {
			diagnostics = append(diagnostics, diag(string(pluginPath),
				fmt.Sprintf("plugin name %q is already used by another declared plugin", name)))
			continue
		}
		seenNames[name] = true
		packages = append(packages, pkg)
		allInputs = append(allInputs, inputs...)
	}

	if hasErrors(diagnostics) {
		return model.SourceInventory{}, diagnostics
	}

	// Sort packages by identity for determinism.
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Identity < packages[j].Identity
	})

	// Deduplicate and sort inputs.
	allInputs = deduplicateInputs(allInputs)

	inventory := model.SourceInventory{
		Packages: packages,
		Inputs:   allInputs,
	}
	if valDiags := model.ValidateSourceInventory(inventory); len(valDiags) != 0 {
		return model.SourceInventory{}, append(diagnostics, valDiags...)
	}
	return inventory, diagnostics
}

// importPlugin imports one plugin from the workspace and returns its package,
// tracked input files, and any diagnostics.
func importPlugin(
	manifest model.SourceManifest,
	workspaceRoot string,
	workspace *os.Root,
	pluginPath model.RelativePath,
) (model.SourcePackage, []model.InputFile, []model.Diagnostic) {
	_ = workspaceRoot // retained for future absolute-path diagnostics

	// Workspace-relative path to the plugin root.
	wsRelPluginRoot := path.Join(string(manifest.Root), string(pluginPath))

	// Open a root-constrained view of the plugin directory.
	pluginRoot, err := workspace.OpenRoot(wsRelPluginRoot)
	if err != nil {
		return model.SourcePackage{}, nil, []model.Diagnostic{
			diag(string(pluginPath), "open plugin root: "+err.Error()),
		}
	}
	defer func() { _ = pluginRoot.Close() }()

	// Traverse all files under the plugin root.
	files, traversalDiags := traversePluginRoot(pluginRoot)
	if hasErrors(traversalDiags) {
		return model.SourcePackage{}, nil, scopeDiags(traversalDiags, string(pluginPath))
	}

	// Extract plugin.json bytes (required).
	pluginJSONBytes, ok := findFile(files, "plugin.json")
	if !ok {
		return model.SourcePackage{}, nil, []model.Diagnostic{
			diag(string(pluginPath), "plugin.json not found in plugin root"),
		}
	}

	// Decode the plugin manifest.
	pluginManifest, wireDiags := agentplugins.DecodePluginManifest(pluginJSONBytes)
	if len(wireDiags) > 0 {
		return model.SourcePackage{}, nil, pluginManifestDiags(string(pluginPath), wireDiags)
	}

	// Extract optional mcp.json bytes.
	var mcpConfig agentplugins.MCPConfig
	if mcpBytes, hasMCP := findFile(files, "mcp.json"); hasMCP {
		decoded, mcpDiags := agentplugins.DecodeMCPConfig(mcpBytes)
		if len(mcpDiags) > 0 {
			return model.SourcePackage{}, nil, mcpFileDiags(string(pluginPath), mcpDiags)
		}
		mcpConfig = decoded
	}

	// Build the workspace-relative prefix for origin paths.
	wsPrefix := wsRelPluginRoot

	// Discover immediate-child skills.
	skillAssets, skillPaths, skillDiags := discoverSkills(files, wsPrefix)
	if hasErrors(skillDiags) {
		return model.SourcePackage{}, nil, scopeDiags(skillDiags, string(pluginPath))
	}

	// Build extension entries.
	extensions, extensionPaths := buildExtensions(pluginManifest.Extensions, files, wsPrefix)

	// Partition remaining files into AgentPluginData.PackageFiles.
	reservedPaths := make(map[string]bool, 2+len(skillPaths)+len(extensionPaths))
	reservedPaths["plugin.json"] = true
	reservedPaths["mcp.json"] = true
	for p := range skillPaths {
		reservedPaths[p] = true
	}
	for p := range extensionPaths {
		reservedPaths[p] = true
	}

	var pkgFiles []model.PackageFile
	for _, f := range files {
		if reservedPaths[f.relPath] {
			continue
		}
		rp, err := model.NewRelativePath(f.relPath)
		if err != nil {
			// path passed traversal but cannot be a model.RelativePath; skip.
			continue
		}
		origin := workspaceOrigin(wsPrefix, f.relPath)
		pkgFiles = append(pkgFiles, model.PackageFile{
			Path:       rp,
			Bytes:      f.bytes,
			Executable: f.executable,
			SHA256:     f.sha256,
			Origin:     []model.SourceLocation{{Path: origin}},
		})
	}

	// Build AgentPluginData.
	agentPluginData := &model.AgentPluginData{
		Profile:      agentplugins.ProfileID,
		Manifest:     mapPluginManifest(pluginManifest),
		MCPServers:   mapMCPConfig(mcpConfig),
		Extensions:   extensions,
		PackageFiles: pkgFiles,
	}
	if len(pluginManifest.Unknown) > 0 {
		agentPluginData.UnknownManifest = cloneAnyMap(pluginManifest.Unknown)
	}
	if len(mcpConfig.Unknown) > 0 {
		agentPluginData.UnknownMCP = cloneAnyMap(mcpConfig.Unknown)
	}

	// Package identity is the plugin name.
	packageID := model.PackageID(pluginManifest.Name)

	pkg := model.SourcePackage{
		Identity:       packageID,
		Assets:         skillAssets,
		CapabilityUses: agentPluginCapabilityUses(agentPluginData, wsPrefix),
		AgentPlugin:    agentPluginData,
	}

	// Collect all traversed files as provenance inputs.
	inputs := make([]model.InputFile, 0, len(files))
	for _, f := range files {
		origin := workspaceOrigin(wsPrefix, f.relPath)
		inputs = append(inputs, model.InputFile{
			Path:   origin,
			SHA256: f.sha256,
		})
	}

	return pkg, inputs, traversalDiags
}

// agentPluginCapabilityUses derives package component capability uses.
func agentPluginCapabilityUses(data *model.AgentPluginData, workspacePrefix string) []model.CapabilityUse {
	locations := make(map[model.CapabilityKey]model.SourceLocation)
	pluginJSON := model.SourceLocation{Path: workspaceOrigin(workspacePrefix, "plugin.json")}
	mcpJSON := model.SourceLocation{Path: workspaceOrigin(workspacePrefix, "mcp.json")}

	for _, server := range data.MCPServers {
		var key model.CapabilityKey
		switch server.Transport {
		case model.MCPTransportStdio:
			key = model.CapabilityKeyAgentPluginMCPStdio
		case model.MCPTransportStreamableHTTP:
			key = model.CapabilityKeyAgentPluginMCPStreamableHTTP
		case model.MCPTransportSSE:
			key = model.CapabilityKeyAgentPluginMCPSSE
		}
		if key != "" {
			locations[key] = mcpJSON
		}
		if len(server.Unknown) != 0 {
			locations[model.CapabilityKeyAgentPluginUnknownJSON] = mcpJSON
		}
	}
	if len(data.Extensions) != 0 {
		locations[model.CapabilityKeyAgentPluginExtensions] = pluginJSON
	}
	if len(data.UnknownManifest) != 0 {
		locations[model.CapabilityKeyAgentPluginUnknownJSON] = pluginJSON
	} else if len(data.UnknownMCP) != 0 {
		locations[model.CapabilityKeyAgentPluginUnknownJSON] = mcpJSON
	}
	if len(data.PackageFiles) != 0 {
		location := model.SourceLocation{Path: workspaceOrigin(workspacePrefix, string(data.PackageFiles[0].Path))}
		if len(data.PackageFiles[0].Origin) != 0 {
			location = data.PackageFiles[0].Origin[0]
		}
		locations[model.CapabilityKeyAgentPluginPackageFiles] = location
	}

	uses := make([]model.CapabilityUse, 0, len(locations))
	for key, location := range locations {
		uses = append(uses, model.CapabilityUse{Key: key, Location: location})
	}
	sort.Slice(uses, func(i, j int) bool { return uses[i].Key < uses[j].Key })
	return uses
}

// mapPluginManifest maps a decoded wire manifest to the model type.
func mapPluginManifest(wm agentplugins.PluginManifest) model.AgentPluginManifest {
	m := model.AgentPluginManifest{
		Name:        wm.Name,
		Version:     wm.Version,
		Description: wm.Description,
		Author:      wm.Author,
		Homepage:    wm.Homepage,
		Repository:  wm.Repository,
		License:     wm.License,
	}
	if wm.Keywords != nil {
		m.Keywords = append([]string(nil), wm.Keywords...)
	}
	return m
}

// findFile returns the bytes of the first file with the given plugin-relative
// relPath from the traversal results.
func findFile(files []traversedFile, relPath string) ([]byte, bool) {
	for _, f := range files {
		if f.relPath == relPath {
			return f.bytes, true
		}
	}
	return nil, false
}

// deduplicateInputs returns inputs sorted by path with duplicates removed.
func deduplicateInputs(inputs []model.InputFile) []model.InputFile {
	seen := make(map[model.RelativePath]bool, len(inputs))
	result := inputs[:0]
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })
	for _, inp := range inputs {
		if !seen[inp.Path] {
			seen[inp.Path] = true
			result = append(result, inp)
		}
	}
	return result
}

// scopeDiags prepends a plugin path scope to each diagnostic message.
func scopeDiags(diags []model.Diagnostic, pluginPath string) []model.Diagnostic {
	result := make([]model.Diagnostic, len(diags))
	for i, d := range diags {
		result[i] = d
		result[i].Message = "plugin " + pluginPath + ": " + d.Message
	}
	return result
}

// pluginManifestDiags converts agentplugins diagnostics for plugin.json errors.
func pluginManifestDiags(pluginPath string, wireDiags []agentplugins.Diagnostic) []model.Diagnostic {
	result := make([]model.Diagnostic, 0, len(wireDiags))
	for _, wd := range wireDiags {
		msg := wd.Message
		if wd.Path != "" {
			msg = "plugin.json " + wd.Path + ": " + msg
		} else {
			msg = "plugin.json: " + msg
		}
		result = append(result, diag(string(pluginPath), msg))
	}
	return result
}

// mcpFileDiags converts agentplugins diagnostics for mcp.json errors.
func mcpFileDiags(pluginPath string, wireDiags []agentplugins.Diagnostic) []model.Diagnostic {
	result := make([]model.Diagnostic, 0, len(wireDiags))
	for _, wd := range wireDiags {
		msg := wd.Message
		if wd.Path != "" {
			msg = "mcp.json " + wd.Path + ": " + msg
		} else {
			msg = "mcp.json: " + msg
		}
		result = append(result, diag(string(pluginPath), msg))
	}
	return result
}

// diag creates an error diagnostic with an optional source location.
func diag(relPath, message string) model.Diagnostic {
	d := model.Diagnostic{
		Code:     diagnosticCode,
		Severity: model.SeverityError,
		Message:  message,
	}
	if relPath != "" {
		d.Location = &model.SourceLocation{Path: model.RelativePath(relPath)}
	}
	return d
}

func hasErrors(diags []model.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
