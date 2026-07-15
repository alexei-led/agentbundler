# Atomic Output Writer

**Path**: `internal/artifact/write/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module materializes a complete selected build plan through staging and rollback-safe replacement.

## Functional Responsibilities

- Stage every planned byte under a private sibling tree in deterministic path order.
- Apply explicit executable intent.
- Validate staged completeness and replace the selected output as one transaction.
- Recover interrupted fallback journal phases without mixing old and new trees.

## Subdomain Classification

**Generic.** Atomic filesystem replacement is stable infrastructure with platform-specific volatility.

## Public Contract

<!-- contract: BuildPlan, PlannedFile, Diagnostic — restated from internal/compiler/model/module.md -->

```text
replace-output(BuildPlan, output-root) -> [Diagnostic]
```

A target file destination is `<output-root>/<target>/<planned path>`; a compiler file destination is `<output-root>/<planned path>`. The parent validates the complete plan before this operation.

On POSIX, `PlannedFile.executable: true` applies at least one execute bit and false clears all execute bits. On Windows, true is rejected as `ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED` before replacement. A script invoked through an interpreter can remain non-executable and therefore build on Windows.

When atomic directory exchange is unavailable, a private sibling journal records `prepared`, `old-moved`, or `new-installed`. Recovery is idempotent: phases before installation restore the prior tree; the final phase keeps the complete replacement and removes backup state.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent validates and delegates the only generated-output mutation.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model-owned plan bytes, paths, and executable flags.

## Constraints and Invariants

- Source-owned files are never written. Output symlinks are never followed.
- Staging, journal, and backup paths are private siblings of the configured generated root; planned files stay below validated output roots.
- Executable intent from source hook payloads is applied exactly, not inferred from shebang, suffix, or handler mode.
- A failure cannot leave mixed target trees or delete the last complete output.
- Modification time is not semantic. The writer performs no process execution, publication, installation, network access, or vendor configuration change.

## Test Specification

- Reordered plans produce the same final bytes and modes.
- POSIX executable/non-executable files receive exact intent; Windows rejects true before mutation.
- Every fallback journal phase recovers to one complete state.
- Staging or replacement failure preserves the prior complete output.
- A second identical write is idempotent.
