# Pi Adapter

**Path**: `internal/target/pi/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Pi-native package output. Without it, Pi package metadata, skills, prompts, themes, and extension resources would be mistaken for portable agent or hook runtime semantics.

## Functional Responsibilities

- Render Pi package metadata and supported skill trees.
- Copy Pi-native extensions, prompts, themes, and declared resources as target-native content.
- Declare that Pi core does not provide the same portable agent and declarative hook contracts as other targets.
- Provide optional Pi native verification where a stable command is available.

## Subdomain Classification

**Core.** Pi is a primary target whose extension/package model evolves independently. Volatility is high.

## Encapsulated Knowledge

- Pi package layout and metadata.
- Pi-native resource placement.
- The boundary between portable skills and extension-provided agent or hook behavior.
- Pi-specific capability limitations and diagnostics.

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
NativeGap = { component: String, location: SourceLocation, target: TargetID? }
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
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, overlays: [TargetOverlay] }
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

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Pi render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `pi`. It uses the parent deterministic renderer baseline at `formatRevision: 1` until verified Pi metadata and resource-path facts are recorded. Capability rules are `asset.skill=native`, `asset.agent=unsupported`, `asset.hook=unsupported`, and `asset.native-resource=native`. It preserves opaque native resources and never synthesizes agent or hook runtime.

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

- Pi package metadata and resource directories.
- Pi extension APIs or built-in subagent/hook features.
- Pi native verification behavior.

## Constraints and Invariants

- No generated universal hook runner or subagent runtime is allowed.
- Pi-native extension code is opaque target content, not normalized portable behavior.
- A portable hook or agent unsupported by Pi core fails unless explicit policy and Pi-native replacement resource exist.
- The adapter must not inject repository-specific Pi instruction policy.

## Test Specification

### Unit Tests

- **Test name**: Pi package metadata is deterministic.
  - **Scenario**: render equivalent package metadata from different map order.
  - **Expected behavior**: native package metadata bytes match.
- **Test name**: unsupported portable agent fails.
  - **Scenario**: render a portable agent without Pi-native replacement.
  - **Expected behavior**: diagnostic identifies the unsupported capability.

### Integration Contract Tests

- **Test name**: skill and native extension coexist.
  - **Scenario**: render a package with a portable skill and Pi extension resource.
  - **Expected behavior**: plan contains both in their native locations.
- **Test name**: extension is not treated as portable hook implementation.
  - **Scenario**: package a Pi hook-runner extension.
  - **Expected behavior**: provenance identifies it as target-native resource only.

### Boundary Tests

- **Test name**: native resource collision fails.
  - **Scenario**: Pi resource path collides with adapter-generated package metadata.
  - **Expected behavior**: adapter returns a collision diagnostic.

### Behavior Tests

- **Test name**: Pi package golden tree.
  - **Scenario**: render a supported skill and extension fixture.
  - **Expected behavior**: output is a valid Pi-native package tree with no compiler runtime files.
