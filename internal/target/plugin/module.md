# Legacy Native Plugin Envelope

**Path**: `internal/target/plugin/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module retains the narrow hook-free plugin envelope used by existing adapters during migration to `internal/target/packageoutput`. It is not the target-wide hook, catalog, aggregate, or validator boundary.

## Functional Responsibilities

- Validate one package identity against the caller's native plugin-name syntax.
- Prepend a caller-owned manifest to shared skill output.
- Reject unsupported assets, collisions, and multi-package input.

## Subdomain Classification

**Supporting.** This compatibility helper is stable and deliberately narrow.

## Public Contract

<!-- contract: NormalizedPackage, TargetPlan, Diagnostic — restated from internal/compiler/model/module.md -->

```text
render(TargetID, manifestPath, [NormalizedPackage], manifestBytes) -> TargetPlan + [Diagnostic]
```

The helper emits the caller-supplied manifest and hook-free skill files only. New target-wide render requests, typed hooks/payloads, executable propagation, separate catalogs, and Pi aggregation use `internal/target/packageoutput` or vendor-owned logic instead.

## Integrations

- **Counterpart**: vendor target leaves
  - **Direction**: legacy hook-free callers supply all vendor paths and bytes.
- **Counterpart**: `internal/target/skills`
  - **Direction**: reuses shared skill rendering.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model-owned normalized values and returns plans.

## Constraints and Invariants

- No vendor hook schema, event mapping, package mode, catalog path, root variable, or validator declaration is introduced here.
- No executable intent is defaulted or lost for files accepted by the helper.
- Multi-package aggregation remains unsupported here and is never inferred.
- No filesystem, process, network, clock, environment, publication, or installation behavior.

## Test Specification

- Existing hook-free one-package manifest/skill output remains deterministic.
- Invalid names, unsupported assets, collisions, and multiple packages return no partial plan.
- Typed-hook or aggregate requests cannot be silently rendered through this legacy helper.
