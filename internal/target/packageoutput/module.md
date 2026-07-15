# Shared Package Output

**Path**: `internal/target/packageoutput/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module owns target-neutral package rooting, common asset copying, hook payload copying, and collision detection. It reduces mechanical duplication without owning any vendor schema or semantic translation.

## Functional Responsibilities

- Render ordered separate package roots under stable package identities.
- Provide a narrow codec callback for target-owned manifests, agents, hooks, and aggregate behavior.
- Copy skills, resources, agent inputs, hook payloads, origins, and executable intent deterministically.
- Detect duplicate package, asset, hook, and output paths with all source origins.

## Subdomain Classification

**Core.** Shared package mechanics change with common asset and render-request shapes, while vendor serialization does not belong here.

## Encapsulated Knowledge

- Stable root selection for one versus several separate packages.
- Deterministic traversal and common collision reporting.
- Immutable views passed to target-owned codecs.

## Public Contract

<!-- contract: TargetRenderInput, NormalizedPackage, NormalizedAsset, FileContent, HookDescriptor, PlannedFile, TargetPlan, Diagnostic — restated from internal/compiler/model/module.md -->

```text
PackageCodec = target-owned pure callbacks for manifest, agent, hook, catalog, and optional aggregate serialization
render-with-codec(TargetID, TargetRenderInput, PackageCodec) -> TargetPlan + [Diagnostic]
```

The hook callback receives sorted immutable hooks and their payload file views. Shared code may place payload bytes under a codec-selected contained package root, but the codec owns native event names, matcher representation, decisions, timeout units, shell/exec representation, root variables, hook manifest bytes, and native check declarations.

## Integrations

- **Counterpart**: vendor target leaves
  - **Direction**: leaves call this renderer and supply target-owned codecs.
  - **Shared knowledge**: narrow immutable package/asset/hook views and planned files.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model-owned values and returns model-owned plan values.

## Internal Design

`separate` is rendered explicitly and never inferred from package count. Aggregate requests are rejected unless the codec explicitly implements them; Pi owns the only version-1 aggregate implementation. The common `add` path always copies `FileContent.executable` and origins into `PlannedFile` rather than defaulting mode.

## Change Vectors

- Add a common portable asset-copy operation.
- Add a narrow target callback needed by more than one vendor.
- Improve common collision diagnostics.

## Constraints and Invariants

- This module contains no Claude, Codex, Pi, Copilot, Cursor, Grok, vendor event, vendor root-variable, catalog-path, or validator string.
- It does not serialize native manifests or infer semantic equivalence.
- Package paths and package-file references remain contained; duplicate hook IDs and output paths fail.
- Executable intent is set only from explicit `FileContent` intent.
- No filesystem, process, network, clock, Git, locale, environment, publication, or installation behavior.

## Test Specification

- Mixed packages preserve payload bytes, origins, executable intent, and stable roots.
- Duplicate packages, assets, hooks, payloads, and output paths fail with all origins.
- Reordered input produces the same sorted plan.
- Shared code is checked for vendor-specific strings.
