# Cursor CLI Adapter

**Path**: `internal/target/cursor/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Cursor CLI-native output. Without it, Cursor rules, skills, agents, and partial hook behavior would be conflated with Cursor Marketplace plugin parity or other target semantics.

## Functional Responsibilities

- Emit direct `.cursor/` bundles for Cursor CLI.
- Render documented native skills and agents.
- Render only the supported hook subset.
- Declare Cursor capability rules and format revision.

## Subdomain Classification

**Core.** Cursor CLI and plugin parity are evolving independently. Volatility is high.

## Encapsulated Knowledge

- Direct Cursor CLI `.cursor/` output topology.
- Cursor skill and subagent locations.
- The difference between documented CLI support and Marketplace plugin parity.
- Supported hook event subset and native validation limits.

## Public Contract

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, InputFile, PackageMetadata, SourceKind, TargetID, AssetKind, CapabilityKey, CapabilityState, Severity, AssetContent, BodyMode, SectionPatch, BodyPatch, FilePatch, TargetOverlay, NativeGap, Acknowledgment, CapabilityUse, CapabilityRule, NativeGapAction, NativeGapPolicy, TargetComposition, BundleSourceConfig, ClaudePluginSourceConfig, SkillsRepositorySourceConfig, SourceManifest, SourceAsset, SourcePackage, SourceInventory, NormalizedAsset, NormalizedPackage, Diagnostic, PlannedFile, NativeCheck, TargetPlan, BuildPlan — restated from internal/compiler/model/module.md -->
```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
AssetID = stable asset identity in the form kind/name
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Int?, column: Int? }
InputFile = { path: RelativePath, sha256: String }
PackageMetadata = Map<String, JsonValue>

SourceKind = bundle | claude-plugin | skills-repository
TargetID = claude | codex | pi | copilot | grok | cursor
AssetKind = skill | agent | hook | native-resource
CapabilityKey = canonical non-empty identifier
CapabilityState = native | equivalent | advisory | unsupported
Severity = error | warning | information

AssetContent = { frontmatter: Map<String, JsonValue>, body: String, files: Map<RelativePath, ByteSequence> }
BodyMode = replace | sections
SectionPatch = { headingPath: [String], body: String }
BodyPatch = { mode: BodyMode, text: String?, sections: [SectionPatch] }
FilePatch = { path: RelativePath, bytes: ByteSequence }
TargetOverlay = { target: TargetID, frontmatterPatch: Map<String, JsonValue>?, bodyPatch: BodyPatch?, files: [FilePatch], deletedFiles: [RelativePath], acknowledgments: [Acknowledgment] }
NativeGap = { component: String, asset: AssetID?, location: SourceLocation, target: TargetID? }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
CapabilityUse = { key: CapabilityKey, location: SourceLocation }
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
NativeGapAction = replace | exclude | source-only
NativeGapPolicy = { component: String, action: NativeGapAction, replacement: AssetID? }
TargetComposition = { target: TargetID, skillPreamble: String?, capabilities: [CapabilityRule], nativeGaps: [NativeGapPolicy] }
BundleSourceConfig = { packages: [RelativePath] }
ClaudePluginSourceConfig = { pluginRoot: RelativePath }
SkillsRepositorySourceConfig = { package: PackageID, roots: [RelativePath], metadata: PackageMetadata }
SourceManifest = { kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath, composition: [TargetComposition], bundle: BundleSourceConfig?, claudePlugin: ClaudePluginSourceConfig?, skillsRepository: SkillsRepositorySourceConfig? }
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, capabilityUses: [CapabilityUse], overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }

Diagnostic = { code: String, severity: Severity, location: SourceLocation?, message: String }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath?, location: SourceLocation }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan], compilerFiles: [PlannedFile] }
```

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Cursor render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `cursor`. It uses the parent deterministic renderer baseline at `formatRevision: 1` until verified Cursor CLI topology facts are recorded. Capability rules are `asset.skill=native`, `asset.agent=native`, `asset.hook=unsupported`, and `asset.native-resource=unsupported`; it does not invent `.cursor/` paths or Marketplace behavior.

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

- Cursor CLI skills, agents, and rules layout changes.
- Cursor hook subset expansion.
- Verified Cursor CLI and Marketplace parity.

## Constraints and Invariants

- Marketplace packaging is not inferred from CLI support.
- Unsupported hook triggers fail rather than generating an approximate hook.
- Direct bundle output remains target-native and has no Agentbundler runtime.

## Test Specification

### Unit Tests

- **Test name**: Cursor output root is direct CLI bundle.
  - **Scenario**: render a package.
  - **Expected behavior**: every plan path lies under the Cursor direct-bundle topology.
- **Test name**: hook subset enforcement.
  - **Scenario**: render supported and unsupported hook triggers.
  - **Expected behavior**: only supported triggers produce files.

### Integration Contract Tests

- **Test name**: skills and agents render together.
  - **Scenario**: render a supported Cursor package.
  - **Expected behavior**: direct bundle references coherent skill and agent locations.

### Boundary Tests

- **Test name**: Marketplace assumption is rejected.
  - **Scenario**: request a Marketplace-only output feature without documented CLI support.
  - **Expected behavior**: adapter returns a capability diagnostic.

### Behavior Tests

- **Test name**: Cursor CLI golden bundle.
  - **Scenario**: render a supported fixture.
  - **Expected behavior**: direct `.cursor/` output is deterministic and follows documented CLI paths.
