# Agent Plugins Wire Contract

**Path**: `internal/agentplugins/` — the module's code is everything in this folder and its transparent subfolders
**Parent**: repository root
**Submodules**: none (leaf)

## Purpose

This module is the sole owner of the Agent Plugins 1.0.0 wire contract. It decodes, validates, and encodes `plugin.json` and `mcp.json` files against the pinned compatibility profile `agent-plugins/1.0.0-bd383552`. It is deliberately pure: no filesystem, process, network, or compiler imports. Schema selection is offline and pinned to an immutable compatibility profile.

## Pinned Profile

| Field | Value |
| --- | --- |
| Profile ID | `agent-plugins/1.0.0-bd383552` |
| Upstream commit | `bd383552095128f6effe895b9257cfd580a6d179` |
| Spec SHA-256 | `97a658b7dca3ce1b4c2266b95da300fa51d9dc4ade59d73168e5f9104272da18` |
| Plugin schema URL | `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json` |
| Plugin schema SHA-256 | `0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883` |
| MCP schema URL | `https://agent-plugins.org/schemas/1.0.0/mcp.schema.json` |
| MCP schema SHA-256 | `6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb` |

**Schema note:** The embedded schemas are the exact bytes from the upstream repository at the pinned commit. To advance the pinned profile, replace both `schemas/1.0.0/plugin.schema.json` and `schemas/1.0.0/mcp.schema.json` with the exact bytes from the new upstream repository commit, update the SHA-256 constants in `profile.go`, and follow the D10 compatibility review procedure.

## Functional Responsibilities

- Decode `plugin.json` into a `PluginManifest` with known fields typed and unknown permitted top-level fields stored as raw JSON values.
- Decode `mcp.json` into a `MCPConfig` with typed transport entries (stdio, streamable-http, sse) and unknown fields stored.
- Validate plugin name rules, URL fields, extension namespace syntax, MCP command format, reserved environment keys, and schema selector.
- Encode `PluginManifest` and `MCPConfig` to deterministic JSON with sorted map keys.
- Reject duplicate JSON object keys at any depth.
- Store permitted unknown members as semantic JSON values for lossless round-trip reproduction.
- Check schema selectors offline against pinned profile URLs; reject any other selector.

## Subdomain Classification

**Core, high volatility.** The upstream standard is young and subject to revision. Changes are isolated here and require a compatibility review before propagating to the source adapter or target adapter.

## Encapsulated Knowledge

- The exact wire field rules for plugin.json (name pattern, URL format, extension namespace syntax).
- The set of MCP transport types (stdio, streamable-http, sse) and their required/optional fields.
- The reserved environment variable keys (PLUGIN_ROOT, PLUGIN_DATA).
- The distinction between "known portable field" and "permitted unknown field."
- Schema selector pinning and offline schema access.

## Public Contract

```text
ProfileID     = "agent-plugins/1.0.0-bd383552"
UpstreamCommit = "bd383552095128f6effe895b9257cfd580a6d179"
PluginSchemaURL = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
MCPSchemaURL    = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

Diagnostic = { Code: String, Path: String, Message: String }

PluginManifest = {
  Schema:      String    // always PluginSchemaURL
  Name:        String    // required; 1-64 chars, lowercase letters/digits/hyphens/periods
  Version:     String?
  Description: String?
  Author:      String?
  Homepage:    String?   // http/https URL if set
  Repository:  String?   // http/https URL if set
  License:     String?
  Keywords:    [String]
  Extensions:  Map<String, JSONValue>  // keyed by reverse-domain namespace
  Unknown:     Map<String, JSONValue>  // permitted unknown top-level fields
}

MCPTransportType = stdio | streamable-http | sse

StdioTransport = {
  Command: String    // bare executable or plugin-relative ./path
  Args:    [String]  // may contain ${PLUGIN_ROOT} and ${PLUGIN_DATA}
  Env:     Map<String, String>  // no PLUGIN_ROOT or PLUGIN_DATA keys
  Cwd:     String?   // may use ${PLUGIN_ROOT} or ${PLUGIN_DATA} prefix
}

RemoteTransport = {
  URL:     String    // http/https URL
  Headers: Map<String, String>  // fixed HTTP headers
}

MCPServer = {
  Name:      String
  Transport: MCPTransportType
  Stdio:     StdioTransport?   // non-nil when Transport == stdio
  Remote:    RemoteTransport?  // non-nil when Transport is remote
  Unknown:   Map<String, JSONValue>
}

MCPConfig = {
  Schema:  String   // always MCPSchemaURL
  Servers: [MCPServer]  // in lexical name order
  Unknown: Map<String, JSONValue>
}

DecodePluginManifest(data: []byte) -> (PluginManifest, []Diagnostic)
DecodeMCPConfig(data: []byte) -> (MCPConfig, []Diagnostic)
ValidatePluginManifest(manifest: PluginManifest) -> []Diagnostic
ValidateMCPConfig(config: MCPConfig) -> []Diagnostic
EncodePluginManifest(manifest: PluginManifest) -> ([]byte, error)
EncodeMCPConfig(config: MCPConfig) -> ([]byte, error)
PluginSchemaBytes() -> []byte
MCPSchemaBytes() -> []byte
IsPluginSchemaURL(url: String) -> bool
IsMCPSchemaURL(url: String) -> bool
```

### Encoding contract

`EncodePluginManifest` emits JSON with: `$schema` first, then `name`, then optional portable fields in spec definition order, then extension namespace keys sorted alphabetically, then unknown top-level fields sorted alphabetically. `EncodeMCPConfig` emits `$schema` first, then `mcpServers` with server names sorted alphabetically, then unknown top-level fields sorted alphabetically. Within each server entry: `type` first, then transport-specific fields, then unknown server fields sorted alphabetically.

This is a value-for-value semantic round trip, not a byte-identical round trip. Numbers, booleans, and null values are reproduced faithfully; JSON formatting (indentation, key order) differs from the source.

## Integrations

- **Counterpart**: `internal/compiler/source/agentplugin` (Task 3)
  - **Direction**: source adapter imports this package to decode plugin.json and mcp.json.
  - **Strength**: contract.
  - **Shared knowledge**: wire types and diagnostics only.
- **Counterpart**: `internal/target/agentplugins` (Task 4)
  - **Direction**: target adapter imports this package to encode output plugin.json and mcp.json.
  - **Strength**: contract.
  - **Shared knowledge**: encode functions and wire types only.

## Import Allowlist

Non-test source files may import only:
`bytes`, `encoding/hex`, `encoding/json`, `embed`, `fmt`, `io`, `net/url`, `regexp`, `sort`, `strings`, `unicode`, `unicode/utf8`

This is statically enforced by `imports_test.go` and the archfit forbidden dependency rules.

## Change Vectors

- Upstream spec revision: replace schema bytes, update SHA-256 constants, bump profile ID, run compatibility review.
- New transport type: add MCPTransportType constant and transport struct, update decode/validate/encode.
- New portable manifest field: add to PluginManifest, decode/validate/encode functions, and remove from Unknown in the profile test.

## Constraints and Invariants

- No filesystem, process, or network import ever.
- Schema selector is checked offline against the pinned URL constants.
- Duplicate object keys are rejected before decoding begins.
- Unknown top-level fields are stored, not rejected; they carry no compiler semantics.
- PLUGIN_ROOT and PLUGIN_DATA are reserved and cannot appear as stdio env keys.
- Plugin names: 1-64 lowercase letters/digits/hyphens/periods; begin/end alphanumeric; no `--`, `..`, `.-`, `-.`.
- Extension namespaces: at least two reverse-domain segments; each segment starts with a letter.
- Encode output is always valid JSON and deterministic across identical input values.
