# Atomic Output Writer

**Path**: `internal/artifact/write/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module materializes the complete selected `BuildPlan` through staging and rollback-safe replacement. Without it, partial builds and stale generated files could leave output in an indeterminate state.

## Functional Responsibilities

- Create a private staging tree below the configured generated-output parent.
- Write planned bytes and executable intent in deterministic path order.
- Verify staged plan completeness.
- Replace all selected output trees as one transaction, using atomic directory exchange where platform support permits and a rollback-safe journal otherwise.
- Remove stale generated entries only as part of replacement.

## Subdomain Classification

**Generic.** Atomic filesystem replacement is a solved infrastructure concern. Functional volatility is low; platform-specific implementation volatility is moderate.

## Encapsulated Knowledge

- Staging naming, cleanup, and replacement mechanics.
- Executable-bit application rules per platform.
- Rollback-safe behavior when a directory swap cannot be atomic.
- Error cleanup that preserves last known good output.

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

```text
replace-output(BuildPlan, output-root) -> [Diagnostic]
```

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/artifact/write`

```go
package write

import "github.com/alexei-led/agentbundler/internal/compiler/model"

func ReplaceOutput(plan model.BuildPlan, outputRoot string) []model.Diagnostic
```

`outputRoot` is a cleaned absolute path. A target file's destination is `outputRoot / target / PlannedFile.path`; a compiler file's destination is `outputRoot / PlannedFile.path`. The operation assumes parent plan validation has succeeded. It stages the entire output root, including compiler files, then either replaces the full selected generated output or reports failure while preserving the prior output root. It never writes source-owned files.

Write and recovery failures emit a locationless error diagnostic with code `ARTIFACT_WRITE_FAILED` and a message containing the failed operation and wrapped OS error. A Windows executable-intent rejection emits `ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED`. Diagnostics are emitted in operation order and do not expose absolute paths outside the configured output root.

### Fallback Replacement Journal

When atomic directory exchange is unavailable, one private journal, staging directory, and backup directory are created adjacent to `outputRoot`, never inside it. The journal records the output path, staging path, backup path, whether an old root existed, and one phase: `prepared`, `old-moved`, or `new-installed`. It is closed after staging validation, then the old root is renamed to backup and `old-moved` persisted, then staging is renamed to output and `new-installed` persisted. Backup and journal are removed only after `new-installed`; supported directory/file sync operations are performed after each phase and rename.

Before staging, an existing journal is recovered idempotently: `prepared` removes staging and retains the old root; `old-moved` removes any replacement root and restores backup when present; `new-installed` retains the replacement root and removes backup. If recovery cannot establish its required state, return a diagnostic, leave the journal, and start no replacement. An interruption before `new-installed` restores the prior state; an interruption after it retains the complete new state. Durability is limited by platforms without directory syncing.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent delegates staged replacement.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: normative `replace-output` operation above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module reads planned output entries.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated output-plan contract above.

## Change Vectors

- Improve platform-specific replacement fallback.
- Clarify executable intent on Windows.
- Add staging integrity checks.

## Constraints and Invariants

- The writer never follows output symlinks.
- It creates directories only below validated output root, except its private staging, journal, and backup siblings adjacent to that root.
- On Windows, executable intent `true` is invalid and parent validation emits `ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED`. On non-Windows, true means at least one execute bit and false means no execute bits.
- A write failure cannot leave a mixed old/new generated output root or delete any prior selected generated entry.
- File modification times are not part of output semantics.

## Test Specification

### Unit Tests

- **Test name**: staged paths follow sorted plan order.
  - **Scenario**: write plan entries in varied input order.
  - **Expected behavior**: staging write order and final tree are deterministic.
- **Test name**: executable intent is applied on POSIX.
  - **Scenario**: plan has executable and non-executable files.
  - **Expected behavior**: executable files have at least one execute bit and non-executable files have none.
- **Test name**: fallback journal recovers each interruption phase.
  - **Scenario**: construct interrupted `prepared`, `old-moved`, and `new-installed` journals for existing and absent roots, then invoke `ReplaceOutput`.
  - **Expected behavior**: recovery follows the state table and the next replacement starts only after recovery succeeds.

### Integration Contract Tests

- **Test name**: successful replacement removes stale output.
  - **Scenario**: existing generated tree has an extra old file.
  - **Expected behavior**: final tree contains only planned files.

### Boundary Tests

- **Test name**: staging failure preserves all selected trees.
  - **Scenario**: induce a write error in one target before replacement.
  - **Expected behavior**: every selected existing output tree remains unchanged.

### Behavior Tests

- **Test name**: replacement is idempotent.
  - **Scenario**: write an identical plan twice.
  - **Expected behavior**: final tree content and executable intent remain identical.
