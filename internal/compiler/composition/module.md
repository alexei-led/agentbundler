# Package Composition

**Path**: `internal/compiler/composition/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler`
**Submodules**: none (leaf)

## Purpose

This module turns one source inventory into ordered normalized packages for one target. It owns bounded overlays, target allow-list selection, semantic capability policy, and native-gap resolution without knowing vendor schemas.

## Functional Responsibilities

- Select packages and assets for one target.
- Apply exactly base plus one target overlay.
- Compose frontmatter, Markdown body, file content, and exact file deletion.
- Preserve typed hook descriptors, file executable intent, origins, and deterministic order.
- Enforce exact semantic capability rules and native-gap policy.

## Subdomain Classification

**Core.** Composition and strict semantic-loss handling are high-volatility product behavior.

## Encapsulated Knowledge

- RFC 7396 frontmatter Merge Patch and explicit body patch modes.
- One-layer overlay and target allow-list semantics.
- Exact capability acknowledgments and gap replacement/exclusion/source-only policy.

## Public Contract

<!-- contract: SourceInventory, TargetComposition, NormalizedPackage, NormalizedAsset, FileContent, HookDescriptor, CapabilityRule, Diagnostic — restated from internal/compiler/model/module.md -->

```text
compose(SourceInventory, TargetComposition) -> [NormalizedPackage] + [Diagnostic]
```

Composition applies target selection, overlay merge, skill preamble, file additions/deletions, semantic capability resolution, and native-gap policy, then sorts packages, assets, files, hooks, and acknowledgments. Target-native capability recognition is explicit per target: Pi requires declared extension entries, while Antigravity requires a native-resource tree without Pi declarations. Recognition is never inferred from a file or directory name. Composition produces no target files and no distribution catalog.

Agent Plugin data (`AgentPluginData`) carried on a `SourcePackage` is deep-copied to the corresponding `NormalizedPackage` without merging, filtering, or reordering. Package-level capability uses are copied separately and checked before rendering, so unsupported MCP transports, extensions, unknown JSON, and package files cannot be dropped by vendor targets. Composition is not the authority on plugin semantics; it preserves the value exactly as imported.

A hook descriptor is immutable through ordinary composition. The selected target overlay may change payload `FileContent` and acknowledgments but cannot inject a vendor schema or silently change event, matcher, handler form, timeout, async, failure policy, or order. Any future descriptor patch must be a separately modeled portable operation.

## Integrations

- **Counterpart**: `internal/compiler/model`
  - **Direction**: reads inventory values and creates model-owned normalized packages.
  - **Shared knowledge**: composition, hook, file, capability, gap, and diagnostic contracts only.
- **Counterpart**: `internal/compiler`
  - **Direction**: orchestration supplies selected target composition and consumes ordered packages.
  - **Shared knowledge**: `compose` operation only.

## Internal Design

Composition clones values before modification. File patches replace both bytes and explicit executable intent as one `FileContent`; unchanged files retain both. Hook payload origins are retained so later collisions and provenance can identify all sources. Target-specific event names and command-root syntax do not exist here.

## Change Vectors

- Add a bounded portable overlay operation.
- Add an exact semantic capability.
- Improve deterministic collision or source-location reporting.

## Constraints and Invariants

- No overlay chain, target inheritance, environment layer, glob deletion, rename, or cross-asset patch.
- Body mode is explicit; target instruction preambles remain repository policy.
- Target allow-lists are exact and fail closed.
- A hook-kind normalized asset retains exactly one valid descriptor and all referenced payload files.
- Executable intent and origins cannot be reset by cloning, overlay, deletion, replacement, or package selection.
- `advisory` succeeds only with an exact target/asset/key acknowledgment and reason. `unsupported` is always an error. No force flag or global acknowledgment exists.
- Semantic hook capabilities are checked individually; `asset.hook` alone never authorizes event, matcher, decision, async, shell, or closed-failure behavior.
- Native resources pass without a gap policy only through an explicit target branch. A path-derived gap that names another target excludes its asset from composition. There is no generic native-resource fallback.
- This module imports no source, target, artifact, filesystem, process, network, or environment behavior.
- `AgentPluginData` passes through composition as a deep copy; composition does not inspect, filter, or merge plugin data. Its separately derived package capability uses must be supported by the target.

## Test Specification

- Merge, body, file-mode, hook preservation, deletion, allow-list, acknowledgment, gap, and collision cases are covered.
- Equivalent insertion orders produce identical normalized packages.
- Unsupported security behavior prevents normalization rather than being weakened.
