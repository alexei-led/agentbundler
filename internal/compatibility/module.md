# Repository-Root Compatibility

**Path**: `internal/compatibility/` — opt-in repository-root compatibility files
**Parent**: repository root
**Submodules**: none (leaf)

## Purpose

This module derives and maintains compatibility files that vendor tools discover at repository root. It adapts the canonical target plan without changing target-native package serialization.

## Functional Responsibilities

- Prepare root marketplace manifests, Codex project agents, and Pi package merges from selected target plans.
- Track compiler-owned paths and fields in `.agentbundler/compatibility.json`.
- Compare owned root files without writing.
- Write changed owned files and remove stale owned files while preserving author-owned content.

## Subdomain Classification

**Supporting.** Compatibility is not the compiler's core normalization or vendor rendering, but vendor discovery contracts are volatile.

## Public Contract

```text
prepare(Request) -> Plan + [Diagnostic]
compare(Plan, workspace-root) -> [Diagnostic] + Boolean
write(Plan, workspace-root) -> [Diagnostic]
```

`Request.Plan` is the canonical target plan. Compatibility may own only explicitly derived paths and fields. It never changes target-native package output and never publishes, installs, authenticates, fetches, or invokes vendor tools.

## Integrations

- **Counterpart**: `internal/compiler`
  - **Direction**: receives the canonical plan after provenance and returns a root compatibility plan before build/check completion.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes target plans, manifest configuration, relative paths, and normalized metadata.
  - **Shared knowledge**: model-owned plan and compatibility configuration only.

## Constraints and Invariants

- Root compatibility is opt-in and requires every configured compatibility target to be selected.
- Author-owned fields and files are preserved; compiler-owned state is explicit and validated.
- Root paths are relative, contained, symlink-free, regular-file paths.
- `compare` never writes. `write` skips unchanged files and removes only stale paths recorded as compiler-owned.
- Compatibility files never enter target-root release archives.

## Test Specification

- Prepare rejects incomplete target selections and forged ownership state.
- Compare distinguishes missing, changed, and stale owned files.
- Write preserves author-owned fields and skips unchanged files.
- Pi package, npmrc, marketplace, and Codex agent cleanup stays bounded by ownership state.
