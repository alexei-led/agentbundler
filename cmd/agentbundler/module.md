# Command Interface

**Path**: `cmd/agentbundler/` — the module's code is everything in this folder and transparent subfolders
**Parent**: repository root
**Submodules**: none (leaf)

## Purpose

This module exposes the only executable entry point. It translates command-line input into a compile request and presents deterministic human or JSON results. Without it, compiler internals would leak through scripts, CI, and agent skills.

## Functional Responsibilities

- Parse `build` and `check` commands and their documented flags.
- Locate an explicit manifest by upward search or `--root`.
- Construct a compiler request.
- Render concise human diagnostics or one versioned JSON result.
- Map compilation outcomes to stable exit statuses.

## Subdomain Classification

**Supporting.** Command UX changes less often than target semantics but is a public integration surface. Volatility is moderate.

## Encapsulated Knowledge

- Flag grammar, help text, and usage errors.
- Standard-output versus standard-error rules.
- JSON result envelope version.
- Exit-status mapping and no-manifest adoption guidance presentation.

## Public Contract

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, InputFile, PackageMetadata, SourceKind, TargetID, AssetKind, CapabilityKey, CapabilityState, Severity, AssetContent, TargetOverlay, NativeGap, Acknowledgment, CapabilityUse, CapabilityRule, SourceManifest, SourceAsset, SourcePackage, SourceInventory, NormalizedAsset, NormalizedPackage, Diagnostic, PlannedFile, NativeCheck, TargetPlan, BuildPlan — restated from internal/compiler/module.md (subset: compiler input and result model closure) -->
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
TargetOverlay = { target: TargetID, content: AssetContent?, deletedFiles: [RelativePath], acknowledgments: [Acknowledgment] }
NativeGap = { component: String, location: SourceLocation, target: TargetID? }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
CapabilityUse = { key: CapabilityKey, location: SourceLocation }
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
SourceManifest = { kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath }
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }
Diagnostic = { code: String, severity: Severity, location: SourceLocation, message: String }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan] }
```

<!-- contract: BuildMode, CompileRequest, CompilationResult, compile — restated from internal/compiler/module.md -->
```text
BuildMode = build | check
CompileRequest = { manifest: SourceManifest, targets: [TargetID], packages: [PackageID], mode: BuildMode, nativeVerify: Boolean }
CompilationResult = { plan: BuildPlan, diagnostics: [Diagnostic], drift: Boolean, nativeVerificationFailed: Boolean }
compile(CompileRequest) -> CompilationResult
```

The command never reads source assets directly. Human text goes to standard error except successful concise status; JSON mode writes exactly one versioned result object to standard output. Exit status is zero for success, one for source/capability/render/write failure, two for drift, and three for native verification failure.

## Integrations

- **Counterpart**: repository root
  - **Direction**: this module implements the root CLI contract.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: restated `build` and `check` operations above.
- **Counterpart**: `internal/compiler`
  - **Direction**: this module invokes compiler orchestration once per command.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: restated compile request and result contract above.

## Change Vectors

- Improve diagnostics or JSON envelope while preserving versioned compatibility.
- Add a flag only when it changes an existing operation rather than creating a new verb.
- Clarify detected-layout adoption guidance.

## Constraints and Invariants

- There are no additional verbs: no `validate`, `lint`, `clean`, `dry-run`, `init`, `watch`, `pack`, or `publish`.
- `check` is read-only by contract; no flag may weaken this.
- The command does not know source topology, overlay semantics, target layout, or filesystem replacement algorithms.
- Unknown flags and verbs fail before compilation.

## Test Specification

### Unit Tests

- **Test name**: two-command grammar.
  - **Scenario**: parse each valid command and invalid extra verb.
  - **Expected behavior**: only `build` and `check` construct requests.
- **Test name**: native flag scope.
  - **Scenario**: supply `--native` to build and check.
  - **Expected behavior**: build rejects it; check sets native verification.

### Integration Contract Tests

- **Test name**: CLI request mapping.
  - **Scenario**: invoke each command with target, package, root, and JSON flags.
  - **Expected behavior**: compiler fake receives the corresponding request.
- **Test name**: exit-status mapping.
  - **Scenario**: compiler fake returns success, ordinary error, drift, and native-verification failure.
  - **Expected behavior**: process exits zero, one, two, and three respectively.

### Boundary Tests

- **Test name**: missing manifest presents guidance.
  - **Scenario**: run in a detected source layout without manifest.
  - **Expected behavior**: no compile call; stable diagnostic includes starter manifest.
- **Test name**: JSON remains machine-only.
  - **Scenario**: command emits a diagnostic in JSON mode.
  - **Expected behavior**: standard output contains one JSON object and standard error contains no competing human report.

### Behavior Tests

- **Test name**: CI check workflow.
  - **Scenario**: execute `check --json` against current and drifted fixture output.
  - **Expected behavior**: result is machine-readable, stable, and distinguishes drift from source error.
