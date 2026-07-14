# Grok Build Adapter

**Path**: `internal/target/grok/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Grok Build project skills and their declared portable resources. Without it, Grok project-root conventions would become implicit behavior in portable source.

## Functional Responsibilities

- Render Grok project skills and portable resources under `.grok/`.
- Declare Grok Build capability rules and format revision.
- Keep Claude compatibility a source fact, not a reason to reuse Claude output blindly.

## Subdomain Classification

**Core.** Grok Build is a primary target with a young native plugin contract. Volatility is high.

## Encapsulated Knowledge

- Grok Build project skill and sibling resource paths.
- Format revision and optional native validation behavior.

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
SourceManifest = { version: Integer, kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath, composition: [TargetComposition], bundle: BundleSourceConfig?, claudePlugin: ClaudePluginSourceConfig?, skillsRepository: SkillsRepositorySourceConfig? }
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

<!-- contract: Adapter, render — restated from internal/target/module.md (subset: Grok render operation) -->
```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

The adapter's `target` is `grok` at `formatRevision: 3`. It renders exactly one project-profile package of `asset.skill` content to `.grok/skills/<skill>/SKILL.md` plus support files and `asset.resource` trees to `.grok/resources/<resource>/`. Agents, hooks, and native resources are unsupported; it makes no Claude-format claim.

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

- **Test name**: Grok project tree is deterministic.
  - **Scenario**: render skills, support files, and a portable resource.
  - **Expected behavior**: native paths and bytes match.
- **Test name**: unsupported capability is explicit.
  - **Scenario**: render an agent or hook.
  - **Expected behavior**: unsupported components diagnose.

### Integration Contract Tests

- **Test name**: skill/resource references remain valid.
  - **Scenario**: a generated skill references a declared sibling resource.
  - **Expected behavior**: both outputs share the `.grok` project root.

### Boundary Tests

- **Test name**: no Claude-output fallback.
  - **Scenario**: target is Grok but only a Claude-specific output representation exists.
  - **Expected behavior**: adapter returns a diagnostic instead of copying Claude files.

### Behavior Tests

- **Test name**: Grok native project golden tree.
  - **Scenario**: render a supported fixture.
  - **Expected behavior**: generated output follows Grok's project skill contract and is deterministic.
