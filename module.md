# Agentbundler

**Path**: repository root — the module's code is everything in this folder and transparent subfolders, excluding child module folders
**Parent**: none (root)
**Submodules**: `cmd/agentbundler`, `internal/compiler`, `internal/target`, `internal/artifact`

## Purpose

Agentbundler is a standalone Go compiler for coding-agent packages. It turns an explicit source repository into deterministic, target-native package trees without becoming a package manager, installer, registry, or universal runtime. Without this module, each repository must maintain target-specific compiler logic and cannot prove that generated output is current.

## Functional Responsibilities

- Expose the `build` and `check` product operations.
- Accept clean Agentbundler bundles and low-friction adopted repositories.
- Compile supported assets for Claude, Codex, Pi, Copilot CLI, Grok Build, and Cursor CLI.
- Preserve native semantics where possible and fail on unsupported or unacknowledged semantic loss.
- Produce reproducible generated trees and provenance outside native package roots.

## Subdomain Classification

**Core.** Portable asset normalization, target capability preservation, and deterministic compilation are the product's differentiating behavior and will evolve as harnesses evolve. This module is therefore high volatility.

## Encapsulated Knowledge

- The product boundary: compiler only; no dependency resolution, installation, publishing, registry, APM integration, external adapter SDK, or generated runtime shim.
- The source-mode policy: `bundle`, `claude-plugin`, and `skills-repository` are explicit committed choices, never implicit build behavior.
- The ownership policy: native-source files remain author-owned; derived output is disposable and compiler-owned.
- The target policy: a target is supported only for capability cells that its adapter declares and tests.

## Public Contract

```text
build [--root path] [--target id] [--package id] [--json]
  validates, compiles, and atomically replaces selected derived output.

check [--root path] [--target id] [--package id] [--native] [--json]
  validates, compiles without writing, and reports exact generated-output drift.
```

`build` and `check` are the only user-facing verbs. Normal diagnostics use stable codes and source locations. JSON output is one versioned result object on standard output; human diagnostics use standard error. Exit status is zero on success, one for source/capability/render/write failure, two for drift, and three for optional native verification failure.

## Integrations

- **Counterpart**: `cmd/agentbundler`
  - **Direction**: the command module implements this module's CLI contract.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: exactly the two commands, selectors, output channels, and exit-status meanings above.
- **Counterpart**: `internal/compiler`
  - **Direction**: the command module delegates compilation to the compiler facade.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: compile orchestration remains internal; this root exposes only the CLI outcome.
- **Counterpart**: `internal/target`
  - **Direction**: target adapters define supported native outputs.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: target selection is explicit; no target behavior is inferred from ambient tools.

## Internal Design

```text
root
├── cmd/agentbundler        command parsing and presentation
├── internal/compiler       source import, composition, orchestration
│   └── model               immutable normalized contracts
├── internal/target         adapter registry and vendor semantics
└── internal/artifact       output, drift, provenance, native checks
```

A command creates a compile request and invokes the compiler. The compiler imports one explicit source topology, composes overlays and policies into normalized packages, asks each target adapter for one target-wide plan, assembles one complete build plan, and gives it to artifact services. `build` writes the staged selection atomically; `check` compares the same build plan to disk. Adapter implementations never write files or invoke processes.

## Change Vectors

- Add a target adapter or revise a target format.
- Add a source importer based on a repeated adoption case.
- Add an asset capability after its portable semantics are specified.
- Strengthen deterministic output, provenance, or native verification.

## Constraints and Invariants

- Normal builds use no network, clock, hostname, locale, absolute source path, Git state, or mutable environment as an output input.
- The compiler never silently drops security, permission, sandbox, hook, or capability semantics.
- Generated output contains no Agentbundler runtime dependency.
- `check` never writes.
- Native target package roots contain only target-native files; compiler provenance is outside them.
- `internal/target` has six direct adapter submodules, one above the 4±1 cognitive guideline. This is a deliberate minor trade-off: each vendor is an independently volatile peer, and adding a grouping module would add distance without a cohesive responsibility. Revisit when a real shared adapter family emerges.
- No production implementation code is authorized until this design and a subsequent implementation plan are approved.

## Test Specification

### Unit Tests

- **Test name**: command surface is closed.
  - **Scenario**: invoke help parsing with supported and unknown verbs.
  - **Expected behavior**: only `build` and `check` are accepted; unknown verbs fail with usage.
- **Test name**: selection defaults are explicit.
  - **Scenario**: omit target and package selectors.
  - **Expected behavior**: the compiler selects only manifest-declared derived targets and packages.

### Integration Contract Tests

- **Test name**: build delegates to the compiler facade.
  - **Scenario**: compile a valid fixture through the executable entry point.
  - **Expected behavior**: selected output is written and a successful result is presented.
- **Test name**: check preserves output ownership.
  - **Scenario**: run `check` against a fixture with no drift.
  - **Expected behavior**: exit status is zero and no output file metadata changes.

### Boundary Tests

- **Test name**: no manifest emits adoption guidance.
  - **Scenario**: run in a directory containing a detected supported layout but no `agentbundle.yaml`.
  - **Expected behavior**: no compilation occurs; the diagnostic prints the exact starter manifest.
- **Test name**: invalid selector is rejected.
  - **Scenario**: select a target or package not declared by the manifest.
  - **Expected behavior**: failure identifies the selector and valid alternatives.

### Behavior Tests

- **Test name**: deterministic complete compilation.
  - **Scenario**: build the same fixture from two distinct absolute paths.
  - **Expected behavior**: output trees and provenance bytes are identical.
- **Test name**: strict semantic preservation.
  - **Scenario**: compile an asset with an unsupported security-relevant capability.
  - **Expected behavior**: compilation fails unless a valid explicit policy resolves it.
