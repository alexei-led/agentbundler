# Exact Drift Comparator

**Path**: `internal/artifact/compare/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module compares selected generated output to one declarative build plan without modifying the filesystem.

## Functional Responsibilities

- Enumerate generated output without following symlinks.
- Compare exact destination paths, bytes, and executable intent.
- Report deterministic missing, changed, and extra drift.

## Subdomain Classification

**Generic.** Exact read-only tree comparison is stable infrastructure.

## Public Contract

<!-- contract: BuildPlan, PlannedFile, RelativePath — restated from internal/compiler/model/module.md -->

```text
DriftKind = missing | changed | extra
Drift = { kind: DriftKind, path: RelativePath }
detect-drift(BuildPlan, output-root) -> [Drift]
```

A target file destination is `<output-root>/<target>/<planned path>`; a compiler file destination is `<output-root>/<planned path>`. Entries sort by path then kind. Parse-equivalent JSON/TOML/Markdown is still changed when bytes differ.

On POSIX, executable intent matches whether any execute bit is set. A planned false intent requires no execute bits. On Windows, the artifact parent rejects true executable intent before calling this leaf; a direct invalid leaf call reports changed without reading that path.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent validates the plan and maps drift to diagnostics.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: reads model-owned build plans.

## Constraints and Invariants

- Comparison is read-only in current and drifted cases: no cleanup, chmod, touch, process, network, Git, or vendor command.
- Symlinks are never traversed. An expected symlink is changed; an unplanned symlink is extra; descendants blocked by a symlink are missing.
- Executable intent from imported hook payloads survives as an exact drift dimension.
- Extra valid vendor files are still drift because the build plan owns the complete selected output.

## Test Specification

- Missing, byte-changed, mode-changed, and extra entries are classified once in stable order.
- Writer output compares current for both executable and non-executable files on POSIX.
- Symlink cases are classified without reading link targets.
- Comparison never starts native validation or mutates metadata.
