# Pi Adapter

**Path**: `internal/target/pi/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf; `runtime/` is a contained TypeScript implementation owned by this module)

## Purpose

This module renders Pi package-profile output and owns the one approved embedded runtime boundary. It translates typed portable hooks through Pi's extension API without making generated packages depend on `agbun`, Bun, or an external hook runner.

## Functional Responsibilities

- Render one explicit aggregate Pi package with package metadata, skills, agents, and dependencies.
- Own, test, embed, and copy a dependency-free TypeScript hook runtime.
- Emit one versioned hook descriptor and one thin package adapter registered once in `package.json#pi.extensions`.
- Map portable hook events, matchers, decisions, timeouts, cancellation, ordering, and failure policy to the Pi extension API.

## Subdomain Classification

**Core.** Pi package and extension contracts are independently volatile.

## Encapsulated Knowledge

- `package.json#pi` resource declarations and Pi's `jiti` TypeScript loader.
- Pi lifecycle and tool-event ordering, mutable tool input, cancellation signals, and cleanup.
- Aggregate dependency/resource collision policy and embedded runtime paths.

## Public Contract

<!-- contract: TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/compiler/model/module.md -->

```text
render(PiAdapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

Version-1 package mode for hooks is `aggregate`. The request must provide explicit aggregate identity and metadata. The adapter never infers aggregation from package count.

The package root contains:

```text
package.json
skills/<name>/SKILL.md
agents/<name>.md
hooks/hooks.v1.json
<private runtime directory>/<embedded TypeScript files>
<one thin package adapter>.ts
<hook payload files>
```

`package.json#pi.extensions` contains exactly the thin adapter path. Runtime helper modules are imported by that adapter, not independently registered. The adapter may preserve existing `pi.subagents` metadata where required by the supported agent form.

Portable mappings use Pi extension events including `session_start`, idempotent `session_shutdown`, `input`/`before_agent_start`, `tool_call`, `tool_result`, `turn_end`/`agent_end`, and compaction events. `tool_call` preflight is sequential even when sibling tools later execute concurrently. Input rewrite mutates `event.input` only after runtime validation because Pi does not revalidate it.

The runtime implements the portable exec/shell process protocol, package-file resolution, matchers, deterministic hook order, bounded output, timeout, cancellation, and fail-open/fail-closed translation. Hook subprocesses inherit only `PATH`; on Windows they also inherit `PATHEXT`, `SYSTEMROOT`, `WINDIR`, and `COMSPEC` (matched case-insensitively). All other ambient variables, including credentials, are omitted. Shutdown closes dispatch and drains terminated child processes before resolving; Pi session shutdown includes session-end hooks in that final drain. Unsupported event or decision cells fail before output.

Primary evidence: installed `@earendil-works/pi-coding-agent` 0.80.7 `docs/packages.md`, `docs/extensions.md`, `README.md`, and `examples/extensions/`, checked 2026-07-15. See `docs/vendor-package-contracts.md`.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: registry exposes the Pi adapter and semantic capabilities.
- **Counterpart**: `internal/target/packageoutput`
  - **Direction**: reuses target-neutral payload/path mechanics; Pi owns aggregation and runtime serialization.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes render requests and returns declarative plans.

## Internal Design

Runtime source lives below `internal/target/pi/runtime/`. Go embeds only reviewed files below this module and emits their exact bytes. The runtime has standalone Bun tests and strict typecheck but no runtime npm dependencies. Generated packages are loaded directly by Pi's supported TypeScript loader.

Aggregation merges dependency maps only when equal values agree. Duplicate dependency versions, package/asset/hook identities, or output paths fail with every origin. The one generated descriptor and adapter are package-owned native payload, not compiler provenance.

## Constraints and Invariants

- Aggregate identity/metadata are explicit; separate mode does not silently become aggregate mode.
- Exactly one extension entry, descriptor, and runtime copy exist in one aggregate artifact.
- Generated output does not scan Pi install directories or require a global runner singleton.
- Generated output does not require Bun, TypeScript, `agbun`, network access, or a separately installed Agent Bundler runtime.
- Embedded runtime bytes are deterministic compiler inputs. Installed Pi versions, absolute source paths, time, environment, and network are not.
- Pi has no production native validator. Install/load smoke tests are test-only, opt-in, and use temporary settings/config roots.

## Test Specification

- Aggregate identity, metadata, dependencies, skills, agents, hooks, and paths merge deterministically or fail with complete collision evidence.
- Package JSON registers exactly one thin adapter and its bytes import the one embedded runtime.
- Cross-language fixtures keep Go descriptor serialization and TypeScript schema-v1 decoding aligned.
- Runtime tests cover every supported event, blocking, rewrite validation, order, fail policy, path containment, environment filtering, broken stdin pipes, timeout, cancellation, output bounds, and idempotent shutdown with no active child process.
