# Agent Plugin Source Adapter

**Path**: `internal/compiler/source/agentplugin/`
**Parent**: `internal/compiler/source`

## Purpose

Imports one or more Agent Plugin 1.0.0 packages from the filesystem and
returns a target-neutral `SourceInventory`. Each declared plugin root becomes
one `SourcePackage` carrying both standard skill assets and the full
`AgentPluginData` value (manifest, MCP servers, extensions, unknown JSON, and
package files). Rendering and output are handled by the `agent-plugins` target
adapter registered in Task 4.

## Functional Responsibilities

- Validate that each plugin root contains a required `plugin.json` and decode
  it using `internal/agentplugins.DecodePluginManifest`.
- Optionally decode `mcp.json` with `internal/agentplugins.DecodeMCPConfig`.
- Run a bounded traversal of each plugin root using a plugin-specific
  `os.Root`: entry count (10,000), per-file size (64 MiB), total bytes
  (256 MiB), depth (64), and path length (1,024 UTF-8 bytes).
- Materialize contained symlinks: the symlink's path is used for origin and
  provenance; the target bytes are read through `os.Root` which rejects
  external references. Directory symlink cycles are detected with `os.SameFile`.
- Reject external symlinks, special files (devices, FIFOs, sockets), and cycles.
- Discover immediate-child Agent Skills (`<name>/SKILL.md`) using the
  `frontmatter` contract and record support files in `Base.Files`; malformed
  skill identities or frontmatter reject the plugin with a skill-scoped diagnostic.
- Derive package capability uses for each MCP transport, extensions, permitted
  unknown JSON, and ordinary package files.
- Partition traversal files into: manifest, MCP, skill assets + support files,
  extension package files (`extensions/<namespace>/`), and `PackageFiles`.
- Reject duplicate plugin paths and duplicate or case-fold-equivalent plugin names; diagnostics include both declaring plugin roots.
- Record all traversed files as `InputFile` entries with workspace-relative
  paths for provenance.

## Materialization Contract

Contained symlinks are resolved and their bytes copied into the inventory.
Link identity (the symlink path) is preserved as the origin `SourceLocation`.
This is noted in provenance output and is not a silent byte-for-byte
round-trip from the target filesystem.

## Importer Limits

| Limit             | Value      |
|-------------------|------------|
| Max entries       | 10,000     |
| Max file size     | 64 MiB     |
| Max total bytes   | 256 MiB    |
| Max depth         | 64         |
| Max path length   | 1,024 UTF-8 bytes |

Regular files are statted through an open descriptor before allocation, then
read through a `limit + 1` bound. Per-file and remaining total limits are
checked again against the retained bytes.

## Public Contract

```text
InspectAgentPluginRoot(SourceManifest, workspaceRoot, *os.Root) -> SourceInventory + [Diagnostic]
```

This function is called by the source router in `internal/compiler/source`. It
receives the workspace-bounded `os.Root` opened by the router.

## Permitted Imports

- `internal/agentplugins` — pinned wire decode/validate
- `internal/compiler/model` — target-neutral model types
- `internal/compiler/source/frontmatter` — skill frontmatter parsing
- Standard library only; no filesystem, process, or network packages beyond `os`
  (used for `os.Root` boundary traversal).

## No-Execution Guarantee

The importer reads file bytes only. It does not execute MCP server commands,
resolve MCP URLs, install dependencies, or infer trust from any source file.
Stdio command values and remote URLs are stored as opaque strings; the adapter
ceiling for MCP capabilities defaults to unsupported in all first-release targets.

## Change Vectors

- Advance the importer to a new pinned profile (requires design review and
  digest update in `internal/agentplugins`).
- Increase traversal limits (requires capacity review).
- Add new file partition categories (requires model type additions).
