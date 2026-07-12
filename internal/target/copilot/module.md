# Copilot CLI Adapter

**Path**: `internal/target/copilot/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders GitHub Copilot CLI-native plugin output. Without it, Copilot plugin manifests and component conventions would leak into portable source or other target layouts.

## Functional Responsibilities

- Render Copilot plugin metadata and supported agents, skills, hooks, MCP, extensions, and LSP resources.
- Declare per-component Copilot capability rules.
- Generate only documented Copilot-native paths and formats.

## Subdomain Classification

**Core.** Copilot CLI plugin behavior is a primary vendor contract and has high volatility.

## Encapsulated Knowledge

- Copilot CLI plugin manifest fields.
- Component paths and native metadata.
- Hook event capability mapping.
- Optional native verification command behavior.

## Public Contract

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, PackageMetadata, TargetID, AssetKind, CapabilityKey, CapabilityState, Severity, AssetContent, Acknowledgment, CapabilityUse, CapabilityRule, NormalizedAsset, NormalizedPackage, Diagnostic, PlannedFile, NativeCheck, TargetPlan — restated from internal/compiler/model/module.md (subset: minimal recursively closed contract) -->
```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
AssetID = stable asset identity in the form kind/name
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Int?, column: Int? }
PackageMetadata = Map<String, JsonValue>
TargetID = claude | codex | pi | copilot | grok | cursor
AssetKind = skill | agent | hook | native-resource
CapabilityKey = canonical non-empty identifier
CapabilityState = native | equivalent | advisory | unsupported
Severity = error | warning | information
AssetContent = { frontmatter: Map<String, JsonValue>, body: String, files: Map<RelativePath, ByteSequence> }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
CapabilityUse = { key: CapabilityKey, location: SourceLocation }
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }
Diagnostic = { code: String, severity: Severity, location: SourceLocation, message: String }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
```

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Copilot render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `copilot`. It emits only supported native plugin components; component support is declared by capability rules rather than assumed from a shared lowest common denominator.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: parent registry selects this adapter and exposes its capabilities.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/target` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: restated adapter contract above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this adapter translates normalized packages to target plans.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: high.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated normalized-package and output-plan contract above.

## Change Vectors

- Copilot plugin manifest revisions.
- Copilot CLI component and hook support.
- New documented native resources.

## Constraints and Invariants

- Do not emit a component merely because another target supports it.
- Unsupported permission, sandbox, or hook behavior must remain an error until explicitly resolved by composition policy.
- Target-native resources remain opaque unless their portable semantics are defined.

## Test Specification

### Unit Tests

- **Test name**: Copilot manifest is complete.
  - **Scenario**: render minimum supported package metadata.
  - **Expected behavior**: required plugin fields are emitted once in deterministic order.
- **Test name**: capability matrix is enforced.
  - **Scenario**: render every unsupported component cell.
  - **Expected behavior**: no unsupported file is planned.

### Integration Contract Tests

- **Test name**: mixed Copilot plugin components.
  - **Scenario**: render supported skills, agents, hooks, and MCP resources.
  - **Expected behavior**: plan paths and manifest entries agree.

### Boundary Tests

- **Test name**: duplicate native component path fails.
  - **Scenario**: two normalized assets resolve to same Copilot path.
  - **Expected behavior**: adapter returns one collision diagnostic with both origins.

### Behavior Tests

- **Test name**: Copilot golden plugin.
  - **Scenario**: render a supported fixture.
  - **Expected behavior**: generated tree follows the documented Copilot plugin layout and parses natively.
