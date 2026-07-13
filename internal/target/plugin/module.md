# Native Plugin Renderer

**Path**: `internal/target/plugin/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module owns the shared native-plugin envelope used by Codex and Cursor. Without it, the adapters would duplicate one-package validation, plugin-name validation, and manifest insertion.

## Functional Responsibilities

- Validate a single package identity against native plugin-name syntax.
- Render the native skill subset and prepend a caller-supplied plugin manifest.
- Reject multiple packages and invalid plugin names.

## Subdomain Classification

**Supporting.** Plugin envelopes change with Codex and Cursor contracts. Volatility is moderate.

## Encapsulated Knowledge

- Native plugin-name syntax.
- Deterministic manifest serialization and placement.

## Public Contract

`render(TargetID, manifestPath, [NormalizedPackage], manifest) -> TargetPlan + [Diagnostic]`. It emits the manifest at `manifestPath` and skill files at `skills/<name>/`.

## Integrations

- **Counterpart**: `internal/target/codex` and `internal/target/cursor`
  - **Direction**: leaves provide vendor manifest path and manifest fields.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/target` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: the render contract above.
- **Counterpart**: `internal/target/skills`
  - **Direction**: plugin rendering reuses the native skill subset.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/target` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: native skill planned files.

## Change Vectors

- Add declared plugin manifest fields.
- Add modeled agent or hook components after their native contracts exist.

## Constraints and Invariants

- No plugin aggregation.
- No inferred or unvalidated vendor fields.
- No filesystem, process, network, clock, or environment access.

## Test Specification

### Unit Tests

- **Test name**: invalid plugin name fails.
  - **Scenario**: a package identity has spaces or uppercase letters.
  - **Expected behavior**: no plan files and an invalid-plugin-name diagnostic.

### Integration Contract Tests

- **Test name**: manifest precedes native skills.
  - **Scenario**: render a valid plugin package.
  - **Expected behavior**: manifest and skill tree are complete and deterministic.

### Boundary Tests

- **Test name**: aggregation fails.
  - **Scenario**: render two packages.
  - **Expected behavior**: no plan files and a diagnostic.

### Behavior Tests

- **Test name**: manifest bytes are canonical.
  - **Scenario**: render equivalent metadata maps.
  - **Expected behavior**: manifest bytes are equal.
