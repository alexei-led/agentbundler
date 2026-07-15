# Native Skill Renderer

**Path**: `internal/target/skills/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module owns target-neutral mechanical rendering for the common project-profile skill/resource subset.

## Functional Responsibilities

- Render one normalized project package below caller-provided skill/resource roots.
- Preserve frontmatter, body, support-file bytes, origins, and executable intent.
- Reject unsupported assets, capability uses, invalid identities, collisions, and multiple project packages.

## Subdomain Classification

**Supporting.** The common portable skill algorithm is moderately stable.

## Public Contract

<!-- contract: NormalizedPackage, PlannedFile, TargetPlan, Diagnostic — restated from internal/compiler/model/module.md -->

```text
render(TargetID, skillRoot, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
render-project(TargetID, skillRoot, resourceRoot, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The output includes `<skillRoot>/<name>/SKILL.md`, skill support files, and optionally `<resourceRoot>/<name>/...`. Every support `FileContent.executable` and origin becomes the corresponding `PlannedFile` metadata unchanged.

Installable packages, hooks/payloads, catalogs, and Pi aggregation belong to `internal/target/packageoutput` and vendor leaves.

## Integrations

- **Counterpart**: vendor target leaves
  - **Direction**: callers select project roots and invoke this common subset.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model-owned normalized files and returns plans.

## Constraints and Invariants

- No vendor event, manifest, catalog, command-root, or validator knowledge.
- No silent semantic loss or partial output on diagnostics.
- No filesystem, process, network, clock, environment, publication, or installation behavior.

## Test Specification

- Frontmatter, body, binary support bytes, origins, and executable intent render deterministically.
- Caller roots contain every path.
- Unsupported assets/capabilities, collisions, and multiple packages return no partial plan.
