# Source Import

**Path**: `internal/compiler/source/` — the module's code is everything in this folder and its transparent subfolders, excluding child module folders
**Parent**: `internal/compiler`
**Submodules**: `agentplugin`, `bundle`, `claudeplugin`, `skillrepo`, `frontmatter`

## Purpose

This module selects one explicit source-topology importer and returns a complete target-neutral inventory. It prevents vendor-layout parsing and canonical bundle traversal from entering compiler orchestration.

## Functional Responsibilities

- Validate and route the declared source kind to exactly one importer.
- Normalize typed assets, including hook descriptors and payload file metadata.
- Preserve source locations, executable intent, target allow-lists, capabilities, native gaps, and input hashes.
- Detect known layouts only for starter-manifest guidance when no manifest exists.

## Subdomain Classification

**Core.** Adoption and canonical source import are high-volatility product behavior.

## Encapsulated Knowledge

- Explicit source-kind selection and no auto-adoption.
- Complete inventory propagation without target rendering.
- Sorted, contained, symlink-free source traversal.

## Public Contract

<!-- contract: SourceManifest, SourceInventory, Diagnostic — restated from internal/compiler/model/module.md -->

```text
SourceManifest = strict versioned source declaration with one source kind, target compositions, optional common distribution metadata, and optional explicit package-mode/aggregate configuration
SourceInventory = ordered packages, typed source assets, native gaps, and input hashes
Diagnostic = stable source/capability diagnostic with optional location, asset, field, and targets
import(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

`import` validates an existing cleaned absolute `workspace-root`, resolves the manifest root beneath it, and chooses the importer only from `SourceManifest.kind`. It returns no partial unsafe values. Hooks remain typed assets rather than opaque native resources: descriptor, payload bytes, executable intent, payload origins, semantic capabilities, and optional target allow-list all survive routing unchanged.

## Integrations

- **Counterpart**: `internal/compiler/source/agentplugin`
  - **Direction**: delegates Agent Plugin 1.0.0 import.
  - **Shared knowledge**: `InspectAgentPluginRoot(SourceManifest, workspace-root, *os.Root)`.
- **Counterpart**: `internal/compiler/source/bundle`
  - **Direction**: delegates canonical bundle parsing.
  - **Shared knowledge**: `inspect-bundle(SourceManifest, workspace-root)`.
- **Counterpart**: `internal/compiler/source/claudeplugin`
  - **Direction**: delegates Claude plugin adoption.
  - **Shared knowledge**: `inspect-claudeplugin(SourceManifest, workspace-root)`.
- **Counterpart**: `internal/compiler/source/skillrepo`
  - **Direction**: delegates skills-repository adoption.
  - **Shared knowledge**: `inspect-skillrepo(SourceManifest, workspace-root)`.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: all importers construct model-owned values.
  - **Shared knowledge**: source declarations, `FileContent`, `HookDescriptor`, inventory, and diagnostics only.
- **Counterpart**: `internal/compiler/source/frontmatter`
  - **Direction**: importers delegate Skills frontmatter parsing without exposing YAML implementation details to the parent.
  - **Shared knowledge**: bounded frontmatter bytes and normalized metadata only.

## Internal Design

The parent performs no topology-specific traversal beyond manifest discovery. A child importer returns one complete inventory. The parent neither converts native-only content into portable semantics nor loses typed hook/file data.

All child importers use one strict overlay file value: a string is non-executable UTF-8 shorthand; an object contains exactly one of `text` or `base64` plus optional `executable`. JSON sidecars provide the origin. A filesystem overlay file replaces the complete JSON `FileContent` at the same path and derives executable intent and origin from that file.

## Change Vectors

- Add a source kind after repeated adoption evidence.
- Add a portable asset representation after model approval.
- Improve source-location or input-hash precision.

## Constraints and Invariants

- Source kind, targets, package membership, package mode, and aggregate identity are explicit committed input.
- Importers do not call composition, target, artifact, process, clock, or network behavior.
- Every filesystem walk is sorted and contained and rejects symlinks and path escape.
- Executable intent is observed from source mode and never cleared because an interpreter-backed handler does not require it.
- Generated output is never re-imported as source.
- Version-1 hook-free manifests retain their behavior; optional hook/distribution/package-mode fields do not force a source-version bump.

## Test Specification

- Explicit kinds select one importer; an absent manifest never auto-adopts.
- Typed hook descriptors, file bytes, executable intent, origins, target allow-lists, gaps, and inputs survive parent routing.
- Arbitrary filesystem order produces an identical inventory.
