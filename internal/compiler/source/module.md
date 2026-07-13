# Source Import

**Path**: `internal/compiler/source/` — the module's code is everything in this folder and its transparent subfolders, excluding child module folders
**Parent**: `internal/compiler`
**Submodules**: `bundle`, `claudeplugin`, `skillrepo`

## Purpose

This module selects the explicit source-topology importer and produces one complete inventory. Without it, the compiler would contain vendor-layout parsing, clean-bundle parsing, and adoption detection logic.

## Functional Responsibilities

- Validate the manifest's declared source kind.
- Route to exactly one child importer.
- Normalize importer results into one source inventory.
- Detect known layouts only to print starter-manifest guidance when no manifest exists.
- Preserve every unrecognized native component as an explicit native gap.

## Subdomain Classification

**Core.** Source adoption is a product differentiator and will change as real repository topologies are adopted. It is high volatility.

## Encapsulated Knowledge

- The three supported source kinds and their selection rules.
- The rule that a build never infers an importer from incidental files.
- The distinction between a detected candidate layout and a committed source declaration.
- The rule that all imported components are inventoried, including unsupported native-only ones.

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

```text
import(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

`import` validates an existing cleaned absolute `workspace-root`, resolves `SourceManifest.root` beneath it, and chooses the importer only from `SourceManifest.kind`. If no manifest is present, the command may report recognized candidate layouts and print a starter manifest, but this operation is not called. An importer error still reports every safely discoverable source location.

## Integrations

- **Counterpart**: `internal/compiler/source/bundle`
  - **Direction**: this module delegates clean canonical bundle parsing.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler/source` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: inspect-bundle — restated from internal/compiler/source/bundle/module.md -->
```text
inspect-bundle(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

- **Counterpart**: `internal/compiler/source/claudeplugin`
  - **Direction**: this module delegates existing Claude-plugin import.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler/source` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: inspect-claudeplugin — restated from internal/compiler/source/claudeplugin/module.md -->
```text
inspect-claudeplugin(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

- **Counterpart**: `internal/compiler/source/skillrepo`
  - **Direction**: this module delegates existing generic skills-repository import.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler/source` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: inspect-skillrepo — restated from internal/compiler/source/skillrepo/module.md -->
```text
inspect-skillrepo(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

## Internal Design

The parent performs no topology-specific traversal beyond manifest discovery. Each child returns a complete inventory using the same model. The parent converts no native-only component into a portable asset; it preserves it in `nativeGaps` for composition policy.

## Change Vectors

- Add a source kind after repeated real adoption evidence.
- Improve detected-layout guidance.
- Clarify importer-level diagnostics or source-location precision.

## Constraints and Invariants

- The source kind is explicit and committed in `agentbundle.json`.
- No child importer calls target adapters, writes output, or executes native tools.
- The importer root is contained by the manifest repository root.
- Every filesystem walk is sorted and rejects source symlinks and path escape.

## Test Specification

### Unit Tests

- **Test name**: explicit kind selects one importer.
  - **Scenario**: load one manifest for each source kind.
  - **Expected behavior**: exactly its matching child importer is called.
- **Test name**: absent manifest does not auto-adopt.
  - **Scenario**: inspect a recognized Claude layout with no manifest.
  - **Expected behavior**: starter-manifest guidance is returned without an inventory.

### Integration Contract Tests

- **Test name**: child inventories share one shape.
  - **Scenario**: import equivalent skills from all source modes.
  - **Expected behavior**: composition receives comparable `SourceInventory` values.
- **Test name**: native gaps survive routing.
  - **Scenario**: child importer reports a vendor-only component.
  - **Expected behavior**: parent preserves it unchanged in the returned inventory.

### Boundary Tests

- **Test name**: unknown source kind is rejected.
  - **Scenario**: manifest declares an unsupported source kind.
  - **Expected behavior**: no child importer runs.
- **Test name**: importer root cannot leave repository.
  - **Scenario**: manifest root uses an absolute or escaping path.
  - **Expected behavior**: validation fails before traversal.

### Behavior Tests

- **Test name**: stable source inventory.
  - **Scenario**: create source directories in arbitrary filesystem order.
  - **Expected behavior**: returned packages, assets, gaps, and inputs are sorted deterministically.
- **Test name**: source topology isolation.
  - **Scenario**: import the same logical skills through bundle and skills-repository modes.
  - **Expected behavior**: later composition sees no topology-specific branching requirement.
