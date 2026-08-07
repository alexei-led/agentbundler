# Shared Package Output

**Path**: `internal/target/packageoutput/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module owns target-neutral package rooting, common asset copying, hook payload copying, catalog placement, and collision detection. It reduces mechanical duplication without owning any vendor schema or semantic translation.

## Functional Responsibilities

- Render ordered separate package roots under stable package identities.
- Provide narrow codec configuration and callbacks for target-owned package, command, agent, hook, and catalog output.
- Copy skills, commands, resources, agent inputs, hook payloads, origins, and executable intent deterministically.
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
HookPayloadFile = immutable { path, packagePath, bytes, executable, origin }
HookInput = immutable { descriptor, payloadRoot, payloadFiles }
HookRenderInput = immutable { packageID, hooks ordered by order/identity/source }
HookManifest = target-owned { path, bytes }
CatalogManifest = target-owned { path, bytes }
PackageCodec = target-owned paths and pure callbacks for package manifest, optional command/agent output, hook manifest, and catalog serialization
render-with-codec(TargetRenderInput, PackageCodec) -> TargetPlan + [Diagnostic]
```

A configured command root emits deterministic `<root>/<command-name>.md` files through the common collision gate. Command support files are rejected until a target codec owns a verified payload layout. The hook callback receives detached descriptor and payload views. Shared code places payload bytes under the codec-selected contained payload root, copies bytes/mode/origin into the plan, and detects collisions before accepting the callback's result. When distribution metadata is present, the shared marketplace builder returns ordered common entries and the codec returns one target-owned catalog path and byte sequence. Catalog paths pass through the same containment and collision gate as all generated files. The codec owns native schemas, manifest paths and bytes, event names, matcher representation, decisions, timeout units, shell/exec representation, and root variables. Agent serialization and its output root are configured together only for package contracts that define an agent component.

## Integrations

- **Counterpart**: vendor target leaves
  - **Direction**: leaves call this renderer and supply target-owned codecs.
  - **Shared knowledge**: narrow immutable package/asset/hook views and planned files.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model-owned values and returns model-owned plan values.
- **Counterpart**: `internal/target/marketplace`
  - **Direction**: consumes validated, ordered target-neutral catalog entries before delegating native serialization to a vendor codec.

## Internal Design

`separate` is rendered explicitly and never inferred from package count. Aggregate requests remain rejected until a target implements its explicit aggregate callback. The common file-add path copies `FileContent.executable` and origins into `PlannedFile`; generated manifests, READMEs, and agents remain non-executable.

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

- Mixed packages preserve command Markdown, payload bytes, origins, executable intent, and stable roots.
- Duplicate packages, assets, hooks, payloads, catalogs, and output paths fail with all origins.
- Reordered input produces the same sorted plan.
- Shared code is checked for vendor-specific strings.
