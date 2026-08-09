// Package agentplugins decodes, validates, and encodes the Agent Plugins 1.0.0
// wire contract. It is a pure package: no filesystem, process, network, or
// compiler imports. Schema selection is offline and pinned to an immutable
// compatibility profile.
//
// Advancing the pinned profile requires a compatibility review, fixture delta,
// adapter revision bump, and provenance update. Do not silently replace the
// embedded schemas from the network or implement unmerged upstream proposals.
package agentplugins

import _ "embed"

// ProfileID is the canonical compatibility profile identifier.
const ProfileID = "agent-plugins/1.0.0-bd383552"

// UpstreamCommit is the exact upstream repository commit this profile is pinned to.
//
// Repository: https://github.com/agentplugins/agent-plugins-spec
// Commit date: 2026-07-24
const UpstreamCommit = "bd383552095128f6effe895b9257cfd580a6d179"

// PluginSchemaURL is the canonical schema selector for plugin.json files.
const PluginSchemaURL = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"

// MCPSchemaURL is the canonical schema selector for mcp.json files.
const MCPSchemaURL = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

// SpecSHA256 is the SHA-256 hex digest of the pinned specification text
// (spec/1.0.0.md at UpstreamCommit).
const SpecSHA256 = "97a658b7dca3ce1b4c2266b95da300fa51d9dc4ade59d73168e5f9104272da18"

// PluginSchemaSHA256 is the SHA-256 hex digest of the plugin schema at
// UpstreamCommit. It must match schemas/1.0.0/plugin.schema.json exactly.
const PluginSchemaSHA256 = "0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883"

// MCPSchemaSHA256 is the SHA-256 hex digest of the MCP schema at
// UpstreamCommit. It must match schemas/1.0.0/mcp.schema.json exactly.
const MCPSchemaSHA256 = "6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb"

//go:embed schemas/1.0.0/plugin.schema.json
var pluginSchemaBytes []byte

//go:embed schemas/1.0.0/mcp.schema.json
var mcpSchemaBytes []byte

// PluginSchemaBytes returns the embedded plugin schema bytes (read-only).
func PluginSchemaBytes() []byte { return pluginSchemaBytes }

// MCPSchemaBytes returns the embedded MCP schema bytes (read-only).
func MCPSchemaBytes() []byte { return mcpSchemaBytes }

// IsPluginSchemaURL reports whether url is the recognized plugin.json schema selector.
func IsPluginSchemaURL(url string) bool { return url == PluginSchemaURL }

// IsMCPSchemaURL reports whether url is the recognized mcp.json schema selector.
func IsMCPSchemaURL(url string) bool { return url == MCPSchemaURL }
