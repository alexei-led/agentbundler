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

<!-- contract: RelativePath, PackageID, ByteSequence, SourceLocation, TargetID, Severity, Diagnostic, PlannedFile, NativeCheck, TargetPlan, BuildPlan — restated from internal/compiler/model/module.md (subset: minimal recursively closed contract) -->
```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Int?, column: Int? }
TargetID = claude | codex | pi | copilot | grok | cursor
Severity = error | warning | information
Diagnostic = { code: String, severity: Severity, location: SourceLocation, message: String }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan] }
```

```text
write(BuildPlan, output-root) -> [Diagnostic]
compare(BuildPlan, output-root) -> [Diagnostic]
provenance(BuildPlan, compiler-version) -> BuildPlan
verify([NativeCheck], output-root) -> [Diagnostic]
```

`write` is the only operation that mutates generated output. `compare` emits drift diagnostics for missing, changed, or extra entries. `provenance` returns a plan augmented with one compiler-owned metadata file. `verify` is valid only after exact comparison succeeds.

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

<!-- contract: detect-drift — restated from internal/artifact/compare/module.md -->
```text
detect-drift(BuildPlan, output-root) -> [Diagnostic]
```

- **Counterpart**: `internal/artifact/provenance`
  - **Direction**: parent delegates deterministic metadata augmentation.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: append-provenance — restated from internal/artifact/provenance/module.md -->
```text
append-provenance(BuildPlan, compiler-version) -> BuildPlan
```

- **Counterpart**: `internal/artifact/nativeverify`
  - **Direction**: parent delegates optional external native checks.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: run-native-checks — restated from internal/artifact/nativeverify/module.md -->
```text
run-native-checks([NativeCheck], output-root) -> [Diagnostic]
```

## Internal Design

The parent validates the whole `BuildPlan` before passing it to any child. Provenance is appended before write or compare. `write` stages every selected target tree and replaces all selected output only after the complete staging tree validates. `compare` reads output directly and never writes. `nativeverify` receives only declared commands and the generated output root; it cannot alter the plan or source tree.

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

### Behavior Tests

- **Test name**: exact drift report.
  - **Scenario**: output has one missing, one changed, and one extra file.
  - **Expected behavior**: diagnostics classify each difference exactly.
- **Test name**: cross-platform output fixture.
  - **Scenario**: write and compare a representative plan on Linux, macOS, and Windows.
  - **Expected behavior**: all supported metadata rules behave consistently.
