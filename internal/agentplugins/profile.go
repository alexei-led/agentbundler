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

// PluginSchemaSHA256 is the SHA-256 hex digest of the embedded plugin schema.
// This must match the bytes of the vendored schemas/1.0.0/plugin.schema.json.
// NOTE: The embedded schema is a best-effort reconstruction from the public
// specification. Replace the schema bytes and update this constant when
// verifying against the exact upstream pinned commit.
const PluginSchemaSHA256 = "1798728e6926de4e891a83dca65e2634d718bd0f5d2bf2d50f01b181192f12d5"

// MCPSchemaSHA256 is the SHA-256 hex digest of the embedded MCP schema.
// This must match the bytes of the vendored schemas/1.0.0/mcp.schema.json.
// NOTE: See note on PluginSchemaSHA256.
const MCPSchemaSHA256 = "525518d1f09665e07ab798d8ef594f5c2df5da07fe34ee58bd94579bc47f0c99"

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
