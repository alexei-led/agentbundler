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
detect-drift(BuildPlan, output-root) -> [Diagnostic]
```

No diagnostic means exact current output. Drift diagnostics are ordered by normalized relative path and classify missing, changed, or extra entries. This operation never treats parse-equivalent structured text as current when bytes differ.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent delegates output observation.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: normative `detect-drift` operation above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module reads planned output entries.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated output-plan contract above.

## Change Vectors

- Improve diagnostics or executable-intent support.
- Add a platform-specific generated-root safety check.

## Constraints and Invariants

- `check` is read-only, including on failure.
- It does not invoke Git or external parsers.
- Extra files are drift even if they are valid target-native files.
- It does not recurse through symlinks.

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

### Behavior Tests

- **Test name**: exact structured-file comparison.
  - **Scenario**: output JSON or TOML parses equivalently but has different formatting bytes.
  - **Expected behavior**: comparator reports changed drift.
