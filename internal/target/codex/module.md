# Codex Adapter

**Path**: `internal/target/codex/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders OpenAI Codex CLI-native package output. Without it, Codex agent TOML conversion and Codex-specific package layout would contaminate portable authoring or other target adapters.

## Functional Responsibilities

- Render Codex-native skills and package resources.
- Convert supported normalized agents from Markdown representation to deterministic Codex TOML.
- Render supported hooks and native resources.
- Declare Codex capability rules and format revision.

## Subdomain Classification

**Core.** Codex is a primary target with moving agent and hook contracts. Volatility is high.

## Encapsulated Knowledge

- Codex package paths and manifest layout.
- Narrow Markdown-agent to TOML conversion and escaping rules.
- Codex-supported metadata aliases and native hook representation.
- Optional Codex native verification invocation.

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

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Codex render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `codex`. Agent conversion is equivalent only for fields and body semantics that the Codex format represents; all other cases are governed by composition capability policy.

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

- Codex agent TOML schema or escaping rules.
- Codex skill, hook, and package layout changes.
- New native verification support.

## Constraints and Invariants

- TOML output has defined key order, UTF-8 encoding, newline, and escaping behavior.
- The adapter never invents unsupported agent fields or silently drops one.
- Native resources remain opaque files unless their semantics are separately modeled.
- Formatting is deterministic and belongs to this adapter, not to artifact services.

## Test Specification

### Unit Tests

- **Test name**: multiline TOML escaping.
  - **Scenario**: render agent bodies with quotes, control characters, and multiline text.
  - **Expected behavior**: output is valid deterministic TOML preserving content.
- **Test name**: key order is fixed.
  - **Scenario**: render equivalent agent metadata in different map orders.
  - **Expected behavior**: bytes are identical.

### Integration Contract Tests

- **Test name**: normalized agent converts to Codex agent.
  - **Scenario**: render a fully supported agent fixture.
  - **Expected behavior**: target plan contains a valid native TOML agent at the Codex path.
- **Test name**: unsupported agent semantics remain errors.
  - **Scenario**: render a capability cell outside Codex support.
  - **Expected behavior**: no partial agent file is planned.

### Boundary Tests

- **Test name**: malformed source body cannot create invalid TOML.
  - **Scenario**: pass a body requiring an unsupported representation.
  - **Expected behavior**: diagnostic names source location and target capability.

### Behavior Tests

- **Test name**: Codex mixed package golden tree.
  - **Scenario**: render supported skills, agents, hooks, and native resources.
  - **Expected behavior**: all generated files are native Codex forms and parse successfully.
