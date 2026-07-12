# Package Composition

**Path**: `internal/compiler/composition/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler`
**Submodules**: none (leaf)

## Purpose

This module turns imported source assets into one normalized package for one target. It owns bounded overlay semantics and explicit loss policy. Without it, importers would encode target behavior and adapters would need to understand source-layer composition.

## Functional Responsibilities

- Select assets and package metadata for one target.
- Apply exactly base plus one target overlay.
- Apply YAML Merge Patch to frontmatter.
- Apply explicit Markdown `replace` or `sections` body composition.
- Apply one configured target skill preamble and exact support-file deletion.
- Enforce capability rules and native-gap `replace`, `exclude`, or `source-only` policy.

## Subdomain Classification

**Core.** Overlay semantics and strict loss handling are the product's most differentiated behavior and are likely to evolve. This module has high volatility.

## Encapsulated Knowledge

- Merge Patch behavior: maps merge, scalars and arrays replace, and null deletes.
- Explicit body-mode grammar and heading-path anchor rules.
- Preamble placement and one-layer inheritance limit.
- Exact canonical capability-key matching, acknowledgment validity, native-gap policy, and stale-policy detection.

## Public Contract

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, InputFile, PackageMetadata, TargetID, AssetKind, CapabilityKey, CapabilityState, Severity, AssetContent, TargetOverlay, NativeGap, Acknowledgment, CapabilityUse, CapabilityRule, SourceAsset, SourcePackage, SourceInventory, NormalizedAsset, NormalizedPackage, Diagnostic — restated from internal/compiler/model/module.md (subset: minimal recursively closed contract) -->
```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
AssetID = stable asset identity in the form kind/name
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Int?, column: Int? }
InputFile = { path: RelativePath, sha256: String }
PackageMetadata = Map<String, JsonValue>
TargetID = claude | codex | pi | copilot | grok | cursor
AssetKind = skill | agent | hook | native-resource
CapabilityKey = canonical non-empty identifier
CapabilityState = native | equivalent | advisory | unsupported
Severity = error | warning | information
AssetContent = { frontmatter: Map<String, JsonValue>, body: String, files: Map<RelativePath, ByteSequence> }
TargetOverlay = { target: TargetID, content: AssetContent?, deletedFiles: [RelativePath], acknowledgments: [Acknowledgment] }
NativeGap = { component: String, location: SourceLocation, target: TargetID? }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
CapabilityUse = { key: CapabilityKey, location: SourceLocation }
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }
Diagnostic = { code: String, severity: Severity, location: SourceLocation, message: String }
```

<!-- contract: CapabilityKey, CapabilityState, CapabilityRule — restated from internal/compiler/model/module.md (subset: minimal recursively closed contract) -->
```text
CapabilityKey = canonical non-empty identifier
CapabilityState = native | equivalent | advisory | unsupported
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
```

```text
compose(SourceInventory, TargetID, [CapabilityRule]) -> [NormalizedPackage] + [Diagnostic]
```

Composition produces no target files. It either preserves a capability natively or equivalently, records an acknowledged advisory result, applies an explicit target replacement or exclusion, or returns an error. A missing or unused policy is an error.

## Integrations

- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module reads source inventory and creates normalized packages.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: the restated inventory, package, diagnostic, and capability-rule contracts above.
- **Counterpart**: `internal/compiler`
  - **Direction**: parent orchestration supplies selected target capability rules and consumes results.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: the normative `compose` operation above.

## Change Vectors

- Add an explicit, documented overlay operation after repeated author need.
- Add an asset kind's composition rules.
- Refine target capability handling or acknowledgment diagnostics.
- Improve Markdown fence and heading parsing for real source cases.

## Constraints and Invariants

- There is no overlay chain, target inheritance, environment layer, package-body inheritance, glob deletion, rename, or cross-asset patch.
- Body-mode auto-detection is forbidden.
- Target preamble text is repository policy, never adapter-owned instruction text.
- Exact deletion paths are contained inside the owning asset.
- Capability rules match exact canonical keys only; there are no wildcards, attributes, predicates, or expressions.
- Unsupported semantics cannot be acknowledged globally or downgraded by a force flag.
- This module performs no target serialization or output I/O.

## Test Specification

### Unit Tests

- **Test name**: Merge Patch deletion.
  - **Scenario**: target frontmatter sets an inherited map field to null.
  - **Expected behavior**: the resulting field is absent.
- **Test name**: explicit body modes differ.
  - **Scenario**: apply identical-looking body text under `replace` and `sections` modes.
  - **Expected behavior**: each follows only its declared algorithm.
- **Test name**: invalid section anchor fails.
  - **Scenario**: section overlay refers to absent or duplicate full heading path.
  - **Expected behavior**: composition returns a location-rich diagnostic.

### Integration Contract Tests

- **Test name**: exact capability rule governs loss.
  - **Scenario**: normalized asset uses an advisory capability key and a near-matching unrelated key also exists.
  - **Expected behavior**: composition succeeds only with an exact acknowledgment and reason.
- **Test name**: native gap policy is complete.
  - **Scenario**: imported native gap has replace, exclude, source-only, and missing-policy variants.
  - **Expected behavior**: only explicit valid variants complete composition.

### Boundary Tests

- **Test name**: overlay chain rejected.
  - **Scenario**: source declares more than one target layer or inheritance parent.
  - **Expected behavior**: composition fails before modifying content.
- **Test name**: support deletion cannot escape asset.
  - **Scenario**: deletion uses `..`, absolute path, glob, or directory path.
  - **Expected behavior**: composition rejects it.

### Behavior Tests

- **Test name**: overlay result is deterministic.
  - **Scenario**: equivalent frontmatter maps and support files arrive in varied enumeration order.
  - **Expected behavior**: normalized package bytes and asset order are stable.
- **Test name**: preamble remains repository policy.
  - **Scenario**: target preamble differs across two bundles using same adapter.
  - **Expected behavior**: each normalized skill includes only its bundle-configured preamble; adapter behavior is unchanged.
