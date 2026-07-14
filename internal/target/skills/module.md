# Native Skill Renderer

**Path**: `internal/target/skills/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module owns the shared lossless skill rendering algorithm. Without it, four vendor adapters would duplicate path validation, deterministic file ordering, and frontmatter serialization.

## Functional Responsibilities

- Render normalized skill and portable resource assets below caller-provided native project roots.
- Preserve frontmatter, body, support files, and package resources.
- Reject unsupported kinds, capability uses, invalid identities, collisions, and
  multi-package project output.

## Subdomain Classification

**Supporting.** The algorithm changes only when the portable skill contract changes. Volatility is moderate.

## Encapsulated Knowledge

- Native skill and sibling resource path construction and collision detection.
- Deterministic JSON-flow YAML frontmatter encoding.

## Public Contract

`render(TargetID, skillRoot, [NormalizedPackage]) -> TargetPlan + [Diagnostic]` accepts
one skills-only project package. `renderProject(TargetID, skillRoot, resourceRoot,
[NormalizedPackage])` additionally accepts `resource/<name>` assets. Both emit
`<skillRoot>/<name>/SKILL.md` and skill support files; project rendering emits
resources at `<resourceRoot>/<name>/`. Installable package aggregation belongs to
`internal/target/packageoutput`.

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
- **Test name**: package resources are siblings of skills.
  - **Scenario**: render a portable resource beside a project skill root.
  - **Expected behavior**: resource paths preserve skill-relative references.
