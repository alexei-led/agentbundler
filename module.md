# Agent Bundler

**Path**: repository root — the module's code is everything in this folder and transparent subfolders, excluding child module folders
**Parent**: none (root)
**Submodules**: `cmd/agbun`, `internal/compiler`, `internal/target`, `internal/artifact`

## Purpose

**Agent Bundler** is a standalone Go compiler for coding-agent packages. It turns an explicit source repository into deterministic, target-native package trees without becoming a package manager, installer, registry, or universal runtime. Without this module, each repository must maintain target-specific compiler logic and cannot prove that generated output is current.

## Functional Responsibilities

- Expose the `build` and `check` product operations.
- Accept clean **Agent Bundler** bundles and low-friction adopted repositories.
- Compile supported assets for Claude, Codex, Pi, Copilot CLI, Grok Build, Cursor CLI, and Antigravity CLI.
- Preserve native semantics where possible and fail on unsupported or unacknowledged semantic loss.
- Produce reproducible generated trees, installable package roots, deterministic target catalogs, and provenance outside native package roots.
- Preserve idempotence: an equivalent `build` or `package` run must not replace, rewrite, chmod, or retimestamp current output.

## Subdomain Classification

**Core.** Portable asset normalization, target capability preservation, and deterministic compilation are the product's differentiating behavior and will evolve as harnesses evolve. This module is therefore high volatility.

## Encapsulated Knowledge

- The product boundary: compiler only; no dependency resolution, installation, publication, registry, APM integration, or external adapter SDK.
- The source-mode policy: `bundle`, `claude-plugin`, and `skills-repository` are explicit committed choices, never implicit build behavior.
- The ownership policy: native-source files remain author-owned; derived output is disposable and compiler-owned.
- The hook policy: hooks are typed first-class assets whose payload bytes, executable intent, events, matchers, decisions, timeout, async, and failure semantics must survive or fail explicitly.
- The target policy: a target is supported only for exact capability cells that its adapter declares and tests.
- The sole runtime exception: the Pi adapter owns a dependency-free TypeScript hook runtime, embeds its tested source in `agbun`, and emits it with a thin Pi extension adapter. Generated packages never call or require `agbun`.

## Public Contract

```text
build [--root path] [--target id] [--package id] [--json]
  validates, compiles, and atomically replaces selected derived output.

check [--root path] [--target id] [--package id] [--native] [--json]
  validates, compiles without writing, and reports exact generated-output drift.
```

`build` and `check` are the only user-facing verbs. Normal diagnostics use stable codes and source locations. JSON output is one versioned result object on standard output; human diagnostics use standard error. Exit status is zero on success, one for source/capability/render/write failure, two for drift, and three for optional native verification failure.

Idempotence is a required quality contract, not an optimization. Given unchanged source bytes, manifest, selectors, and compiler version, `build` must leave a current output tree untouched and `package` must preserve matching archives. Real drift may use complete atomic replacement to retain stale-file cleanup and rollback safety. The current writer and archive path are byte-deterministic but still replace unchanged filesystem objects; this is a known implementation gap until content-aware no-op writes are added.

## Integrations

- **Counterpart**: `cmd/agbun`
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
├── cmd/agbun        command parsing and presentation
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
- Strengthen deterministic and idempotent output, provenance, or native verification.

## Constraints and Invariants

- Normal builds use no network, clock, hostname, locale, absolute source path, Git state, installed vendor version, or mutable environment as an output input.
- Equivalent successful runs are filesystem no-ops: unchanged output files, directories, and archives retain identity and timestamps. This required invariant is not yet satisfied by the current writer/archive implementation.
- The compiler never silently drops or weakens security, permission, sandbox, hook, executable, decision, timeout, failure, or capability semantics.
- Generated output contains no **Agent Bundler** executable/runtime dependency. The embedded Pi payload is target-owned generated source loaded by Pi, not a call back into `agbun`.
- Deterministic marketplace/catalog generation is artifact creation only. Production code never publishes, submits, authenticates, installs, changes vendor configuration, or fetches packages.
- `check` never writes. `check --native` may run only declared official offline non-mutating validators after exact no-drift comparison.
- Native target package roots contain only target-native files, including the Pi-owned runtime payload; compiler provenance is outside them.
- Version-1 hook-free manifests and package layouts remain compatible. Aggregate mode is explicit, never inferred, and in version 1 is limited to Pi package profile.
- `internal/target` has six vendor leaves plus cohesive shared rendering leaves. Vendor schemas, hook mappings, root variables, catalogs, and validator declarations remain isolated in vendor adapters.

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
  - **Scenario**: run in a directory containing a detected supported layout but no `agentbundle.json`.
  - **Expected behavior**: no compilation occurs; the diagnostic prints the exact starter manifest.
- **Test name**: invalid selector is rejected.
  - **Scenario**: select a target or package not declared by the manifest.
  - **Expected behavior**: failure identifies the selector and valid alternatives.

### Behavior Tests

- **Test name**: deterministic complete compilation.
  - **Scenario**: build the same fixture from two distinct absolute paths.
  - **Expected behavior**: output trees and provenance bytes are identical.
- **Test name**: unchanged build is a filesystem no-op.
  - **Scenario**: run `build` twice with identical source bytes, manifest, selectors, and compiler version.
  - **Expected behavior**: the second run creates no staging transaction and preserves every output path, byte, mode, file identity, and timestamp.
- **Test name**: unchanged package preserves archives.
  - **Scenario**: run `package` twice against a current generated tree.
  - **Expected behavior**: byte-identical destination archives retain file identity and timestamps.
- **Test name**: strict semantic preservation.
  - **Scenario**: compile an asset with an unsupported security-relevant capability.
  - **Expected behavior**: compilation fails unless a valid explicit policy resolves it.
