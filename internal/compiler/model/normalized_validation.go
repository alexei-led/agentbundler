package model

import (
	"encoding/hex"
	"fmt"
)

// ValidateNormalizedPackage validates a composed package before an adapter consumes it.
func ValidateNormalizedPackage(pkg NormalizedPackage) []Diagnostic {
	var diagnostics []Diagnostic
	if err := validateIdentifier(string(pkg.Identity), "package ID"); err != nil {
		diagnostics = appendInvalid(diagnostics, "normalized package: "+err.Error())
	}
	if !validTargetID(pkg.Target) {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized package target %q is invalid", pkg.Target))
	}
	if pkg.Profile != "" && pkg.Profile != TargetProfileProject && pkg.Profile != TargetProfilePackage {
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized package profile %q is invalid", pkg.Profile))
	}
	if err := validateJSONValue(pkg.Metadata); err != nil {
		diagnostics = appendInvalid(diagnostics, "normalized package metadata: "+err.Error())
	}
	diagnostics = append(diagnostics, validateCapabilityUses(fmt.Sprintf("normalized package %q", pkg.Identity), pkg.CapabilityUses)...)
	assets := make(map[AssetID]struct{}, len(pkg.Assets))
	for _, asset := range pkg.Assets {
		diagnostics = append(diagnostics, validateNormalizedAsset(asset)...)
		if _, ok := assets[asset.Identity]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("normalized asset %q is duplicated", asset.Identity))
		}
		assets[asset.Identity] = struct{}{}
	}
	for _, acknowledgment := range pkg.Acknowledgments {
		diagnostics = append(diagnostics, validateAcknowledgment(acknowledgment)...)
		if acknowledgment.Target != pkg.Target {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("acknowledgment for asset %q has target %q, not package target %q", acknowledgment.Asset, acknowledgment.Target, pkg.Target))
		}
		if _, ok := assets[acknowledgment.Asset]; !ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("acknowledgment references unknown asset %q", acknowledgment.Asset))
		}
	}
	if pkg.AgentPlugin != nil {
		diagnostics = append(diagnostics, ValidateAgentPluginData(*pkg.AgentPlugin)...)
	}
	return diagnostics
}

// ValidateAgentPluginData validates package-owned agent plugin data without
// accessing the filesystem.
func ValidateAgentPluginData(data AgentPluginData) []Diagnostic {
	var diagnostics []Diagnostic
	if data.Profile == "" {
		diagnostics = appendInvalid(diagnostics, "agentPlugin profile must not be empty")
	}
	if data.Manifest.Name == "" {
		diagnostics = appendInvalid(diagnostics, "agentPlugin manifest name must not be empty")
	}

	serverNames := make(map[string]struct{}, len(data.MCPServers))
	for _, srv := range data.MCPServers {
		if srv.Name == "" {
			diagnostics = appendInvalid(diagnostics, "agentPlugin MCP server name must not be empty")
			continue
		}
		if _, ok := serverNames[srv.Name]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin MCP server %q is duplicated", srv.Name))
		}
		serverNames[srv.Name] = struct{}{}
		diagnostics = append(diagnostics, validateModelMCPServer(srv)...)
	}

	extNS := make(map[string]struct{}, len(data.Extensions))
	for _, ext := range data.Extensions {
		if ext.Namespace == "" {
			diagnostics = appendInvalid(diagnostics, "agentPlugin extension namespace must not be empty")
			continue
		}
		if _, ok := extNS[ext.Namespace]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin extension namespace %q is duplicated", ext.Namespace))
		}
		extNS[ext.Namespace] = struct{}{}
		diagnostics = append(diagnostics, validatePackageFiles(ext.Namespace+" extension", ext.PackageFiles)...)
	}

	diagnostics = append(diagnostics, validatePackageFiles("agentPlugin", data.PackageFiles)...)
	return diagnostics
}

func validateModelMCPServer(srv MCPServer) []Diagnostic {
	var diagnostics []Diagnostic
	switch srv.Transport {
	case MCPTransportStdio:
		if srv.Stdio == nil {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin MCP server %q has no stdio transport data", srv.Name))
		} else if srv.Stdio.Command == "" {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin MCP server %q stdio command must not be empty", srv.Name))
		}
	case MCPTransportStreamableHTTP, MCPTransportSSE:
		if srv.Remote == nil {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin MCP server %q has no remote transport data", srv.Name))
		} else if srv.Remote.URL == "" {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin MCP server %q remote URL must not be empty", srv.Name))
		}
	default:
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("agentPlugin MCP server %q has invalid transport %q", srv.Name, srv.Transport))
	}
	return diagnostics
}

func validatePackageFiles(scope string, files []PackageFile) []Diagnostic {
	var diagnostics []Diagnostic
	paths := make(map[RelativePath]struct{}, len(files))
	for _, pf := range files {
		if err := validateRelativePath(string(pf.Path)); err != nil {
			diagnostics = appendInvalid(diagnostics, scope+" package file path: "+err.Error())
			continue
		}
		if _, ok := paths[pf.Path]; ok {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("%s package file path %q is duplicated", scope, pf.Path))
		}
		paths[pf.Path] = struct{}{}
		if len(pf.SHA256) != 64 || !isValidHex(pf.SHA256) {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("%s package file %q has invalid SHA-256", scope, pf.Path))
		}
		for _, origin := range pf.Origin {
			diagnostics = append(diagnostics, validateSourceLocation(origin)...)
		}
	}
	return diagnostics
}

func isValidHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
