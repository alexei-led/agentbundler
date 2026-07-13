# Native Skill Renderer

**Path**: `internal/target/skills/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module owns the shared lossless skill rendering algorithm. Without it, four vendor adapters would duplicate path validation, deterministic file ordering, and frontmatter serialization.

## Functional Responsibilities

- Render one normalized package of skill assets below a caller-provided native skill root.
- Preserve frontmatter, body, and support files.
- Reject unsupported kinds, capability uses, invalid skill identities, collisions, and multi-package aggregation.

## Subdomain Classification

**Supporting.** The algorithm changes only when the portable skill contract changes. Volatility is moderate.

## Encapsulated Knowledge

- Native-skill path construction and collision detection.
- Deterministic JSON-flow YAML frontmatter encoding.

## Public Contract

`render(TargetID, skillRoot, [NormalizedPackage]) -> TargetPlan + [Diagnostic]`. It accepts exactly one package and only `skill/<name>` assets with `asset.skill` capability uses. It emits `<skillRoot>/<name>/SKILL.md` and support files.

## Integrations

- **Counterpart**: `internal/target` adapter leaves
  - **Direction**: leaves select the vendor skill root and invoke this renderer.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/target` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: the render contract above.

## Change Vectors

- Adopt a portable YAML frontmatter model.
- Add a common skill capability after native evidence exists.

## Constraints and Invariants

- No filesystem, process, network, clock, or environment access.
- No silent semantic loss or partial output on diagnostics.

## Test Specification

### Unit Tests

- **Test name**: deterministic skill tree.
  - **Scenario**: render frontmatter, body, and support files.
  - **Expected behavior**: native paths and bytes are stable.

### Integration Contract Tests

- **Test name**: vendor root selection.
  - **Scenario**: each leaf supplies its root.
  - **Expected behavior**: planned files are below that root.

### Boundary Tests

- **Test name**: unsupported subset rejection.
  - **Scenario**: render agent, unknown capability, or multiple packages.
  - **Expected behavior**: diagnostics and no plan files.

### Behavior Tests

- **Test name**: support files preserved.
  - **Scenario**: render a skill with binary support files.
  - **Expected behavior**: bytes are unchanged.
