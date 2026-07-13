# Native Verification

**Path**: `internal/artifact/nativeverify/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module runs optional installed-harness checks after exact drift comparison. Without it, users lack an opt-in compatibility signal; placing it here prevents environment-dependent behavior from contaminating deterministic compilation.

## Functional Responsibilities

- Validate declared native-check commands and working directories.
- Execute checks without shell interpolation.
- Capture exit status and bounded diagnostic output.
- Report unavailable tools and failed checks distinctly.

## Subdomain Classification

**Generic.** Process execution is a solved boundary concern. Functional volatility is low, while vendor command availability is moderate external volatility.

## Encapsulated Knowledge

- Safe process creation without a shell.
- Working-directory containment beneath generated output.
- Output capture bounds and diagnostic conversion.
- Difference between absent tool, nonzero check, and compiler error.

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
NativeVerificationResult = { success: Boolean, diagnostics: [Diagnostic] }
run-native-checks([NativeCheck], output-root) -> NativeVerificationResult
```

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/artifact/nativeverify`

```go
package nativeverify

import "github.com/alexei-led/agentbundler/internal/compiler/model"

const MaxOutputBytesPerStream = 32 * 1024

type Result struct {
    Success     bool
    Diagnostics []model.Diagnostic
}

func RunNativeChecks(checks []model.NativeCheck, outputRoot string) Result
```

`outputRoot` is an existing cleaned absolute generated-output directory. An empty check list returns `Result{Success: true}`. Checks run sequentially in declared order; one failure does not suppress later checks. If `workingDirectory` is absent, the selected working directory is `outputRoot`; otherwise it is `outputRoot / workingDirectory`. Before every process start, the runner resolves `outputRoot` and the selected working directory through symlinks. The resolved directory must exist, be a directory, and remain under resolved `outputRoot`; a failed validation starts no process.

The runner invokes the `PATH`-resolved program directly with its declared argument vector, empty stdin, unchanged inherited environment, and no shell. It captures stdout and stderr independently, retains the first `MaxOutputBytesPerStream` bytes of each while draining overflow, and renders invalid UTF-8 bytes as `\\xNN`. A nonzero result includes both retained streams and, for an overflowed stream, appends `[truncated after 32768 bytes]`. Exit status zero is successful. A start failure, signal termination, nonzero exit, invalid declaration, or invalid directory makes `Success` false. A truncation warning alone does not.

The operation executes only declared checks after exact comparison reports current output. It never modifies source or output and never affects generated bytes.

### Diagnostic Codes

```text
NATIVE_VERIFY_INVALID_CHECK              error: invalid program or argument; no process starts
NATIVE_VERIFY_OUTPUT_ROOT_UNAVAILABLE    error: output root cannot resolve to an existing directory; no processes start
NATIVE_VERIFY_WORKING_DIRECTORY_ESCAPE   error: resolved directory is outside output root; no process starts for that check
NATIVE_VERIFY_WORKING_DIRECTORY_UNAVAILABLE error: directory cannot resolve, exist, or be a directory; no process starts for that check
NATIVE_VERIFY_TOOL_UNAVAILABLE           error: PATH cannot resolve the declared executable
NATIVE_VERIFY_START_FAILED               error: executable resolved but process creation failed
NATIVE_VERIFY_FAILED                     error: process started but signaled or exited nonzero; includes bounded evidence
NATIVE_VERIFY_OUTPUT_TRUNCATED           warning: stdout, stderr, or both exceeded the per-stream limit
```

Every emitted diagnostic uses `NativeCheck.location`. A failure diagnostic precedes its truncation warning for the same check.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent delegates optional external verification.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: normative `NativeVerificationResult` and `run-native-checks` contract above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module reads declared native checks.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated native-check and diagnostic contract above.

## Change Vectors

- Improve tool discovery diagnostics.
- Add a bounded output-capture policy.
- Adapt to native checker command changes.

## Constraints and Invariants

- The runner performs no shell invocation, network request, environment mutation, or interactive stdin exchange. Adapter-declared native verifiers are trusted non-interactive tools: the runner cannot prevent a child process from making network requests or showing an OS-level credential UI.
- Checks run only under `check --native` after no drift.
- Program and arguments come only from adapter-declared `NativeCheck` values.
- Process output is diagnostic evidence, not generated content.

## Test Specification

### Unit Tests

- **Test name**: no shell interpolation.
  - **Scenario**: check argument contains shell metacharacters.
  - **Expected behavior**: process receives it literally.
- **Test name**: bounded output is explicit.
  - **Scenario**: one stream exceeds 32768 bytes.
  - **Expected behavior**: retained evidence is bounded and exactly one `NATIVE_VERIFY_OUTPUT_TRUNCATED` warning is emitted.
- **Test name**: omitted working directory uses output root.
  - **Scenario**: run a fake checker with no working directory that reports its current directory.
  - **Expected behavior**: its current directory is the resolved output root.
- **Test name**: missing program is distinct.
  - **Scenario**: declared program is unavailable.
  - **Expected behavior**: diagnostic identifies unavailable native verifier.

### Integration Contract Tests

- **Test name**: current output enables checks.
  - **Scenario**: exact compare succeeds and one fake native check exits successfully.
  - **Expected behavior**: check result is successful.
- **Test name**: nonzero native check maps to exit category.
  - **Scenario**: fake native checker exits nonzero.
  - **Expected behavior**: compiler result marks native verification failure.

### Boundary Tests

- **Test name**: invalid working directory fails.
  - **Scenario**: native check directory escapes output root.
  - **Expected behavior**: no process starts.

### Behavior Tests

- **Test name**: drift suppresses verification.
  - **Scenario**: comparator reports drift and native checks are requested.
  - **Expected behavior**: no native checker is invoked.
