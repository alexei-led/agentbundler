# Grok Build Adapter

**Path**: `internal/target/grok/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Grok Build-native plugin output. Without it, Grok plugin layout and its supported skills, agents, hooks, MCP, and LSP resources would become implicit behavior in portable source.

## Functional Responsibilities

- Render Grok-native plugin trees and required metadata.
- Map supported normalized skills, agents, hooks, and native resources.
- Declare Grok Build capability rules and format revision.
- Keep Claude compatibility a source fact, not a reason to reuse Claude output blindly.

## Subdomain Classification

**Core.** Grok Build is a primary target with a young native plugin contract. Volatility is high.

## Encapsulated Knowledge

- Grok Build plugin layout and metadata requirements.
- Grok-specific hook capability and matcher representation.
- Native resource placement for MCP and LSP features.
- Format revision and optional native validation behavior.

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

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Grok render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `grok`. It emits Grok-native output whenever documented rather than treating Claude-compatible input as a permanent output format.

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

- Grok Build plugin and marketplace format changes.
- Hook event and component support revisions.
- Evolution away from Claude compatibility toward independent native features.

## Constraints and Invariants

- Claude-compatible source does not imply byte-copying Claude output.
- Native Grok semantics are emitted only when documented and covered by a capability rule.
- Unknown Grok-only features remain target-native resources, not portable assumptions.

## Test Specification

### Unit Tests

- **Test name**: Grok manifest rendering is deterministic.
  - **Scenario**: render equivalent metadata in varied map order.
  - **Expected behavior**: native manifest bytes match.
- **Test name**: hook capability is explicit.
  - **Scenario**: render each known and unsupported portable hook trigger.
  - **Expected behavior**: supported triggers map once; unsupported triggers diagnose.

### Integration Contract Tests

- **Test name**: Grok native components agree with metadata.
  - **Scenario**: render a package with supported skill, agent, hook, MCP, and LSP content.
  - **Expected behavior**: every planned component has a coherent native reference.

### Boundary Tests

- **Test name**: no Claude-output fallback.
  - **Scenario**: target is Grok but only a Claude-specific output representation exists.
  - **Expected behavior**: adapter returns a diagnostic instead of copying Claude files.

### Behavior Tests

- **Test name**: Grok native plugin golden tree.
  - **Scenario**: render a supported fixture.
  - **Expected behavior**: generated output follows Grok's native plugin contract and is deterministic.
