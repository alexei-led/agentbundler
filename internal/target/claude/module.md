# Claude Adapter

**Path**: `internal/target/claude/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Claude Code-native package output. Without it, Claude plugin manifests, skills, agents, hooks, and marketplace metadata would leak into portable source and other adapters.

## Functional Responsibilities

- Render Claude plugin package trees and target-wide metadata where required.
- Preserve skills and agents in Claude-native Markdown forms.
- Render supported hook configuration and target-native resources.
- Declare Claude capability rules and format revision.

## Subdomain Classification

**Core.** Claude package behavior is a primary target contract and changes with vendor features. Volatility is high.

## Encapsulated Knowledge

- Claude plugin directory and manifest layout.
- Claude-specific frontmatter fields and hook event/matcher representation.
- Claude marketplace/index requirements.
- Native validation commands available to optional verification.

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

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Claude render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `claude`. Its target plan includes only deterministic Claude-native files and optional native checks under generated output. It does not edit an adopted canonical Claude source tree.

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

- Claude plugin manifest revisions.
- New or changed Claude skill, agent, hook, MCP, or extension capabilities.
- Marketplace/index layout changes.

## Constraints and Invariants

- Claude instruction policy is author source or target overlay content, never adapter-injected prose.
- Unsupported Claude semantics return diagnostics rather than silently filtering fields.
- Generated files must not collide with target-native resources.
- Native verification is optional and never changes output bytes.

## Test Specification

### Unit Tests

- **Test name**: Claude manifest serialization is stable.
  - **Scenario**: render equivalent package metadata in different map orders.
  - **Expected behavior**: manifest bytes are identical.
- **Test name**: unsupported field is diagnosed.
  - **Scenario**: normalized content carries an undeclared Claude capability.
  - **Expected behavior**: render returns a capability diagnostic.

### Integration Contract Tests

- **Test name**: skills agents and hooks render to native locations.
  - **Scenario**: render a supported mixed package fixture.
  - **Expected behavior**: plan paths match the Claude plugin contract.
- **Test name**: marketplace metadata is target-wide.
  - **Scenario**: render multiple Claude packages.
  - **Expected behavior**: shared target metadata is coherent and package entries are deterministic.

### Boundary Tests

- **Test name**: canonical source rebuild stays under output.
  - **Scenario**: compile a `claude-plugin` source with Claude selected.
  - **Expected behavior**: the Claude target plan rebuilds the native package only under configured generated output and contains no source-owned path.

### Behavior Tests

- **Test name**: Claude package golden tree.
  - **Scenario**: render a canonical fixture.
  - **Expected behavior**: generated tree parses as native Claude package metadata and preserves declared asset behavior.
