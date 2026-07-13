# Exact Drift Comparator

**Path**: `internal/artifact/compare/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module observes all selected generated output against one `BuildPlan` without modifying it. Without it, CI and developers would rely on Git state or semantic parsers that can hide meaningful generated drift.

## Functional Responsibilities

- Enumerate existing generated output below a validated root.
- Compare exact relative paths, bytes, and executable intent.
- Report missing, changed, and extra entries deterministically.
- Perform no write, cleanup, or timestamp mutation.

## Subdomain Classification

**Generic.** Exact tree comparison is a stable infrastructure concern. Functional volatility is low.

## Encapsulated Knowledge

- Exact drift definition and classification.
- Safe generated-root enumeration.
- Cross-platform executable-intent comparison.
- Stable ordering and concise diagnostic formatting inputs.

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
DriftKind = missing | changed | extra
Drift = { kind: DriftKind, path: RelativePath }
detect-drift(BuildPlan, output-root) -> [Drift]
```

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/artifact/compare`

```go
package compare

import "github.com/alexei-led/agentbundler/internal/compiler/model"

type DriftKind string

const (
    DriftMissing DriftKind = "missing"
    DriftChanged DriftKind = "changed"
    DriftExtra   DriftKind = "extra"
)

type Drift struct {
    Kind DriftKind
    Path model.RelativePath
}

func DetectDrift(plan model.BuildPlan, outputRoot string) []Drift
```

`outputRoot` is an existing cleaned absolute generated-output directory. For each `TargetPlan`, the destination of a planned file is `outputRoot / target / PlannedFile.path`; `target` is the target ID's canonical string. A compiler file's destination is `outputRoot / PlannedFile.path`. `Drift.Path` is that normalized destination path relative to `outputRoot`, for example `claude/.claude-plugin/plugin.json`. An empty result means exact current output. Drift entries are ordered by path then kind and classify missing, changed, or extra entries exactly once. This operation never treats parse-equivalent structured text as current when bytes differ, follows no symlink, and performs no mutation. A symlink at an expected planned-file path is one `changed` drift. A symlink at an unplanned path is one `extra` drift and is never traversed. A symlink at an unplanned ancestor of planned files is `extra` at the symlink path; each blocked planned descendant is `missing` at its planned destination. These rules take precedence over byte and executable comparison.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent delegates output observation.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: normative `DriftKind`, `Drift`, and `detect-drift` contract above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module reads planned output entries.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated output-plan contract above; the Go API imports its normative `model.BuildPlan` type.

## Change Vectors

- Improve diagnostics or executable-intent support.
- Add a platform-specific generated-root safety check.

## Constraints and Invariants

- `check` is read-only, including on failure.
- It does not invoke Git or external parsers.
- Extra files are drift even if they are valid target-native files.
- It does not recurse through symlinks.
- On Windows, any planned file with `executable: true` is invalid input. The parent artifact validator rejects it with `ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED` before calling this comparator. If this leaf is called directly with such a plan, it reports one `Drift{Kind: DriftChanged, Path: planned destination}` without reading output. On non-Windows, executable intent matches whether at least one POSIX execute bit is set; non-executable intent matches no execute bits.

## Test Specification

### Unit Tests

- **Test name**: drift classes are distinct.
  - **Scenario**: compare plans with missing, changed, and extra entries.
  - **Expected behavior**: diagnostics classify each exactly once.
- **Test name**: ordering is stable.
  - **Scenario**: filesystem enumeration order varies.
  - **Expected behavior**: diagnostic order is normalized by path.

### Integration Contract Tests

- **Test name**: writer output is current.
  - **Scenario**: write a plan then compare it.
  - **Expected behavior**: comparator emits no drift.

### Boundary Tests

- **Test name**: comparator has no mutations.
  - **Scenario**: snapshot tree metadata before current and drifted comparisons.
  - **Expected behavior**: snapshots remain unchanged.
- **Test name**: symlink drift is classified without traversal.
  - **Scenario**: compare planned-file, unplanned-file, and blocking-directory symlinks, including dangling links.
  - **Expected behavior**: expected links are `changed`, unplanned links are `extra`, blocked descendants are `missing`, and no link target is read.

### Behavior Tests

- **Test name**: exact structured-file comparison.
  - **Scenario**: output JSON or TOML parses equivalently but has different formatting bytes.
  - **Expected behavior**: comparator reports changed drift.
