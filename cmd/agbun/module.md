# Agent Bundler Command

**Path**: `cmd/agbun/` — the module's code is everything in this folder and transparent subfolders
**Parent**: repository root
**Submodules**: none (leaf)

## Purpose

This module is the composition root and presentation boundary for the closed `agbun build` and `agbun check` command surface.

## Functional Responsibilities

- Parse workspace, target, package, output-mode, and native-check options.
- Strictly load the source manifest and invoke compiler orchestration.
- Present stable human or versioned JSON results and exit categories.
- Ensure `check` stays read-only and native verification is opt-in.

## Subdomain Classification

**Generic.** CLI presentation is stable and does not own compiler semantics.

## Public Contract

```text
build [--root path] [--target id] [--package id] [--json]
check [--root path] [--target id] [--package id] [--native] [--json]
```

Exit status is zero on success, one on source/capability/render/write failure, two on drift, and three on optional native verification failure. `--native` is valid only for check.

## Integrations

- **Counterpart**: repository root
  - **Direction**: implements the public command and exit contract.
- **Counterpart**: `internal/compiler`
  - **Direction**: builds one request and delegates all import/compose/render/artifact sequencing.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: displays model diagnostics without defining look-alike model types.

## Constraints and Invariants

- The command does not import source, composition, target, or artifact leaves directly.
- It does not interpret typed hooks, package modes, distribution metadata, vendor paths, embedded runtime bytes, or capability cells.
- `check` never writes. `check --native` can run only checks already declared in the compiled plan after no drift.
- No command publishes, installs, submits, authenticates, fetches packages, or changes vendor configuration.
- Human output goes to stderr; JSON mode emits one versioned stdout object.

## Test Specification

- Only build/check and their legal options parse.
- Selector and manifest errors produce stable diagnostics and exit one.
- Drift maps to exit two and starts no native checks.
- Native validation failure maps to exit three without changing output.
- Existing hook-free version-1 builds preserve their command behavior.
