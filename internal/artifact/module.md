# Artifact Services

**Path**: `internal/artifact/` — the module's code is everything in this folder and its transparent subfolders, excluding child module folders
**Parent**: repository root
**Submodules**: `write`, `compare`, `provenance`, `nativeverify`

## Purpose

This module is the only owner of generated-output effects and observations. It validates declarative plans, writes output atomically, detects exact drift, adds deterministic provenance, and optionally runs native checks. Without it, adapters would perform inconsistent I/O and build reproducibility would be impossible to enforce centrally.

## Functional Responsibilities

- Validate the complete selected build plan for containment, collisions, case-fold conflicts, reserved names, and target-root ownership.
- Add deterministic provenance outside native package roots.
- Materialize the whole selected build through staging and one rollback-safe transaction.
- Compare the complete selected build exactly to existing output without writing.
- Run optional native verification only after a current exact comparison.

## Subdomain Classification

**Supporting.** This module is shared operational infrastructure rather than vendor semantics. Its functional behavior is stable, though cross-platform filesystem handling creates moderate implementation volatility.

## Encapsulated Knowledge

- Generated-root containment and path-safety rules.
- Atomic staging, replacement, and rollback behavior by supported platform.
- Exact drift definition: path, bytes, executable intent, and allowed file set.
- Provenance serialization and exclusion of nondeterministic data.
- Process invocation containment for optional native checks.

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

<!-- contract: ProvenanceInput, ProvenanceInputFile, ProvenanceAcknowledgment, AdapterRevision — restated from internal/artifact/provenance/module.md (subset: omits append-provenance) -->
```text
ProvenanceInput = {
  compilerVersion: String,
  configuration: ByteSequence,
  inputs: [ProvenanceInputFile],
  acknowledgments: [ProvenanceAcknowledgment],
  adapterRevisions: [AdapterRevision]
}
ProvenanceInputFile = { path: RelativePath, sha256: String }
ProvenanceAcknowledgment = { asset: String, target: TargetID, key: String, reason: String }
AdapterRevision = { target: TargetID, revision: Integer }
```

```text
write(BuildPlan, output-root) -> [Diagnostic]
compare(BuildPlan, output-root) -> [Diagnostic]
provenance(BuildPlan, ProvenanceInput) -> BuildPlan + [Diagnostic]
verify([NativeCheck], output-root) -> [Diagnostic]
```

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/artifact`

```go
package artifact

import "github.com/alexei-led/agentbundler/internal/compiler/model"

type ProvenanceInputFile struct {
    Path   model.RelativePath
    SHA256 string
}

type ProvenanceAcknowledgment struct {
    Asset  string
    Target model.TargetID
    Key    string
    Reason string
}

type AdapterRevision struct {
    Target   model.TargetID
    Revision int
}

type ProvenanceInput struct {
    CompilerVersion  string
    Configuration    []byte
    Inputs           []ProvenanceInputFile
    Acknowledgments  []ProvenanceAcknowledgment
    AdapterRevisions []AdapterRevision
}

func Write(plan model.BuildPlan, outputRoot string) []model.Diagnostic
func Compare(plan model.BuildPlan, outputRoot string) []model.Diagnostic
func Provenance(plan model.BuildPlan, input ProvenanceInput) (model.BuildPlan, []model.Diagnostic)
func Verify(checks []model.NativeCheck, outputRoot string) []model.Diagnostic
```

`write` is the only operation that mutates generated output. `compare` emits drift diagnostics for missing, changed, or extra entries. `provenance` returns a plan augmented with one compiler-owned metadata file, or an unchanged plan with deterministic diagnostics. `verify` is valid only after exact comparison succeeds.

## Integrations

- **Counterpart**: `internal/compiler`
  - **Direction**: compiler orchestration delegates final plan actions.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: the normative artifact operations above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module validates and consumes build plans and native checks.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated plan, file, check, and diagnostic contracts above.
- **Counterpart**: `internal/artifact/write`
  - **Direction**: parent delegates staged output replacement.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: replace-output — restated from internal/artifact/write/module.md -->
```text
replace-output(BuildPlan, output-root) -> [Diagnostic]
```

- **Counterpart**: `internal/artifact/compare`
  - **Direction**: parent delegates exact output observation.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: DriftKind, Drift, detect-drift — restated from internal/artifact/compare/module.md -->
```text
DriftKind = missing | changed | extra
Drift = { kind: DriftKind, path: RelativePath }
detect-drift(BuildPlan, output-root) -> [Drift]
```

- **Counterpart**: `internal/artifact/provenance`
  - **Direction**: parent delegates deterministic metadata augmentation.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: ProvenanceInput, ProvenanceInputFile, ProvenanceAcknowledgment, AdapterRevision, append-provenance — restated from internal/artifact/provenance/module.md -->
```text
ProvenanceInput = {
  compilerVersion: String,
  configuration: ByteSequence,
  inputs: [ProvenanceInputFile],
  acknowledgments: [ProvenanceAcknowledgment],
  adapterRevisions: [AdapterRevision]
}
ProvenanceInputFile = { path: RelativePath, sha256: String }
ProvenanceAcknowledgment = { asset: String, target: TargetID, key: String, reason: String }
AdapterRevision = { target: TargetID, revision: Integer }
append-provenance(BuildPlan, ProvenanceInput) -> BuildPlan
```

- **Counterpart**: `internal/artifact/nativeverify`
  - **Direction**: parent delegates optional external native checks.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: NativeVerificationResult, run-native-checks — restated from internal/artifact/nativeverify/module.md -->
```text
NativeVerificationResult = { success: Boolean, diagnostics: [Diagnostic] }
run-native-checks([NativeCheck], output-root) -> NativeVerificationResult
```

## Internal Design

The parent validates the whole `BuildPlan` before passing it to any child. It maps `compare.DriftKind` to locationless `DRIFT_MISSING`, `DRIFT_CHANGED`, or `DRIFT_EXTRA` model diagnostics in child order. It maps a provenance error to one `PROVENANCE_INVALID` error diagnostic and converts the facade provenance input to the child input unchanged. Provenance is appended before write or compare. `write` stages every selected target tree and replaces all selected output only after the complete staging tree validates. `compare` reads output directly and never writes. `nativeverify` receives only declared commands and the generated output root; it cannot alter the plan or source tree; its child diagnostics pass through unchanged.

## Change Vectors

- Improve cross-platform atomic replacement.
- Add a plan-level metadata field required by several artifact operations.
- Improve drift diagnostics.
- Add a supported native checker invocation pattern.

## Constraints and Invariants

- Output symlinks are forbidden.
- Artifact paths cannot be absolute, escape their output root, collide after case folding, or use reserved platform names.
- Build output excludes source-owned native trees.
- Provenance is reserved at `<output-root>/.agentbundler/build.json` and contains no timestamp, hostname, absolute path, Git state, or self-hash.
- On Windows, an executable planned file is rejected with `ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED` before any child operation. On non-Windows, executable intent means at least one POSIX execute bit; non-executable means no execute bits.
- Native checks are optional, non-hermetic, and never influence generated bytes.

## Test Specification

### Unit Tests

- **Test name**: path safety rejects unsafe plans.
  - **Scenario**: validate absolute, escaping, case-fold-colliding, and reserved-name paths.
  - **Expected behavior**: plan validation returns deterministic diagnostics.
- **Test name**: provenance has no nondeterminism.
  - **Scenario**: augment equivalent plans under different machines and times.
  - **Expected behavior**: provenance bytes are identical.

### Integration Contract Tests

- **Test name**: whole build stages before replacement.
  - **Scenario**: one planned file in the final selected target fails during staging.
  - **Expected behavior**: every existing selected output tree remains unchanged.
- **Test name**: compare and write share plan validation.
  - **Scenario**: submit invalid plan to both operations.
  - **Expected behavior**: both reject identical safety violations.

### Boundary Tests

- **Test name**: check never writes.
  - **Scenario**: run compare against missing, changed, and current output trees.
  - **Expected behavior**: timestamps and content remain untouched in every case.
- **Test name**: native verify follows current output only.
  - **Scenario**: compare finds drift with native checks requested.
  - **Expected behavior**: no process is started.
- **Test name**: Windows executable intent is rejected.
  - **Scenario**: on Windows, submit an executable planned file to write and compare.
  - **Expected behavior**: both return `ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED`; no child operation runs.

### Behavior Tests

- **Test name**: exact drift report.
  - **Scenario**: output has one missing, one changed, and one extra file.
  - **Expected behavior**: diagnostics classify each difference exactly.
- **Test name**: cross-platform output fixture.
  - **Scenario**: write and compare a representative plan on Linux, macOS, and Windows.
  - **Expected behavior**: all supported metadata rules behave consistently.
