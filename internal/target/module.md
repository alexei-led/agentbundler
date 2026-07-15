# Target Adapter Registry

**Path**: `internal/target/` — the module's code is everything in this folder and its transparent subfolders, excluding child module folders
**Parent**: repository root
**Submodules**: `claude`, `codex`, `pi`, `copilot`, `grok`, `cursor`, `skills`, `plugin`, `packageoutput`, `marketplace`

## Purpose

This module owns the closed built-in adapter registry and the pure target-render boundary. Vendor paths, schemas, event mappings, command roots, catalogs, and validator declarations stay in vendor leaves.

## Functional Responsibilities

- Register and resolve six built-in adapters.
- Expose adapter format revisions and exact semantic capability rules.
- Render one explicit target-wide request to one declarative plan.
- Keep rendering free of filesystem, process, network, clock, Git, locale, and environment access.

## Subdomain Classification

**Core.** Vendor contracts and hook capabilities are independently volatile.

## Encapsulated Knowledge

- Registry membership and adapter interface.
- Exact capability-state policy and target format revisions.
- Target-wide render purity and no dynamic adapter loading.

## Public Contract

<!-- contract: TargetID, TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/compiler/model/module.md -->

```text
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
resolve(TargetID) -> Adapter + [Diagnostic]
capabilities(Adapter) -> [CapabilityRule]
render(Adapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

`TargetRenderInput` contains ordered packages, common distribution metadata, explicit package mode, and optional explicit aggregate identity/metadata. Render is pure and returns complete files plus safe optional `NativeCheck` declarations. It never infers mode from package count.

## Native Renderer Contract

Every adapter emits a vendor-native supported subset. Portable hooks use exact semantic cells in addition to `asset.hook`:

```text
hook.command.exec
hook.command.shell
hook.event.session-start
hook.event.session-end
hook.event.prompt-submit
hook.event.pre-tool
hook.event.post-tool
hook.event.post-tool-failure
hook.event.stop
hook.event.notification
hook.event.pre-compact
hook.event.post-compact
hook.matcher.tool-category
hook.decision.block
hook.decision.rewrite-input
hook.async
hook.failure.closed
```

A leaf may mark only proven cells native/equivalent; advisory needs exact acknowledgment, unsupported fails. Adapters defensively reject any asset or semantic cell composition should have blocked. No hook, executable intent, package, catalog entry, or security decision is silently omitted or weakened.

`separate` renders stable independent package roots and is the compatibility default. In source version 1, `aggregate` is accepted only by Pi package profile. Pi aggregation produces one explicitly named installable package; it is not inferred and is not a global singleton claim.

Package profiles render verified native forms:

- Claude: `.claude-plugin/plugin.json`, skills, agents, `hooks/hooks.json`, payloads.
- Codex: `.codex-plugin/plugin.json`, skills, default `hooks/hooks.json`, payloads, and `.mcp.json` only when already modeled. Plugin agents are unsupported until an official component path exists.
- Pi: one explicit package root with `package.json`, skills, agents, one generated hook descriptor, one thin extension, and the embedded runtime.
- Copilot CLI: root `plugin.json`, skills, `agents/*.agent.md`, root `hooks.json`, payloads.
- Cursor: `.cursor-plugin/plugin.json`, skills, agents, `hooks/hooks.json`, payloads.
- Grok: project profile keeps `.grok/skills`; package profile is a separately generated Grok-tested Claude-compatible plugin with Grok command-root behavior.

Target-wide catalogs are deterministic artifacts at the leaf-owned paths pinned in `docs/vendor-package-contracts.md`. The shared `marketplace` leaf strictly validates common distribution/package metadata and orders target-neutral entries; vendor leaves own JSON schemas. Rendering a catalog does not publish, register, authenticate, install, fetch, or update configuration.

## Integrations

- **Counterpart**: `internal/compiler`
  - **Direction**: orchestration resolves adapters, supplies capability rules, and renders requests.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model-owned render requests and returns model-owned plans.
- **Counterpart**: vendor leaves
  - **Direction**: registry delegates schema/mapping/render behavior by target ID.

## Internal Design

The registry is an explicit map, not installed-tool discovery. Shared package output may sort, root, copy payload files, preserve origins/modes, and detect collisions. Vendor leaves alone serialize native hook manifests, map events/decisions, choose command roots, serialize catalogs, and declare native checks.

## Change Vectors

- Add a built-in target after primary contract verification.
- Change a verified capability or native path with a format revision.
- Add a narrow shared renderer seam with no vendor names.

## Constraints and Invariants

- No external adapter SDK, dynamic loading, target-to-target calls, output writes, or process invocation.
- Native output path/schema claims cite primary evidence in the leaf and `docs/vendor-package-contracts.md`.
- `NativeCheck` declarations are limited to official offline non-mutating validators. Initially only Claude strict validation and Grok plugin validation qualify.
- Install/load/list smoke tests for Codex, Copilot, Cursor, and Pi are test-only, opt-in, and use temporary vendor configuration roots.
- Hook-free version-1 inputs retain output unless a verified native correction changes a leaf revision.
- Embedded Pi runtime bytes are an allowed deterministic adapter input; installed vendor versions are not.

## Test Specification

- Registry membership and format revisions are closed and positive.
- Every supported/unsupported semantic cell has fixture coverage.
- Separate and Pi aggregate requests render deterministically under reordered input.
- Target mismatch, collision, unacknowledged advisory, and unsupported semantics return no partial plan.
