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
replace-output(BuildPlan, output-root) -> [Diagnostic]
```

The operation assumes parent plan validation has succeeded. It either replaces every selected generated tree or reports failure while preserving every prior selected tree. It never writes source-owned files.

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
- It creates directories only below validated output root.
- A write failure cannot leave a mixed old/new selection or delete any prior selected generated tree.
- File modification times are not part of output semantics.

## Test Specification

### Unit Tests

- **Test name**: staged paths follow sorted plan order.
  - **Scenario**: write plan entries in varied input order.
  - **Expected behavior**: staging write order and final tree are deterministic.
- **Test name**: executable intent is applied.
  - **Scenario**: plan has executable and non-executable files.
  - **Expected behavior**: supported platform metadata matches intent.

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
