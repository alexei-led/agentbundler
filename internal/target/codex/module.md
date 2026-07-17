# Codex Adapter

**Path**: `internal/target/codex/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders OpenAI Codex project profiles and installable plugin packages using the current verified plugin and lifecycle-hook contracts.

## Functional Responsibilities

- Render project skills and `.codex/agents/*.toml` where that existing project profile is selected.
- Render installable `.codex-plugin/plugin.json`, skills, `hooks/hooks.json`, payloads, and already-modeled `.mcp.json` resources.
- Render `.agents/plugins/marketplace.json` for separate distribution.
- Declare exact capabilities and the absence of a production native validator.

## Subdomain Classification

**Core.** Codex plugin, trust, hook, and subagent contracts change independently.

## Encapsulated Knowledge

- Codex plugin/default hook paths and `${PLUGIN_ROOT}` command references.
- Hook trust, concurrency, event, matcher, decision, timeout, and async limitations.
- Project-only subagent TOML paths.

## Public Contract

<!-- contract: Adapter, TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/target/module.md and internal/compiler/model/module.md -->

```text
render(CodexAdapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

Installable package roots contain only verified plugin components:

```text
.codex-plugin/plugin.json
skills/<name>/SKILL.md
hooks/hooks.json
<payload files>
.mcp.json                 when already modeled
```

`hooks/hooks.json` is Codex's default plugin hook path. `.codex-plugin/plugin.json#hooks` may override it with a plugin-relative path, path array, inline hook object, or inline-object array; Agent Bundler uses the default unless a future requirement needs an override.

Current official plugin docs do not define a plugin agent component or root. Package-profile `asset.agent` is therefore removed before plugin serialization and emitted separately at target-root `.codex/agents/<name>.toml`. Existing project-profile agents use the same path. These profiles are not claimed as plugin contents.

Verified initial hook cells:

- native/equivalent where exact payload behavior matches: command exec/shell, events `session-start`, `prompt-submit`, `pre-tool`, `post-tool`, `stop`, `pre-compact`, `post-compact`, and tool-category matcher;
- `hook.decision.block` is native through the target-owned pre-tool translator; `hook.decision.rewrite-input` remains unsupported because Codex has no lossless input-rewrite result;
- unsupported: `session-end`, `notification`, `post-tool-failure` unless current docs add an exact event, and async (`async` is parsed but command handlers are skipped);
- `hook.failure.closed` is advisory for pre-tool hooks through the translator and requires an exact acknowledgment; other events reject it. Ordering/concurrency-dependent behavior remains unsupported unless Codex preserves the requested contract.

No stable official offline non-mutating validator covers required plugin/hook behavior, so production `NativeCheck` is empty. Marketplace add/list and hook trust/load smoke tests are test-only, opt-in, and use temporary `CODEX_HOME`.

Primary sources: <https://developers.openai.com/codex/plugins>, <https://developers.openai.com/codex/build-plugins>, <https://developers.openai.com/codex/hooks>, and <https://developers.openai.com/codex/subagents>, accessed 2026-07-15. See `docs/vendor-package-contracts.md`.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: registry exposes this adapter.
- **Counterpart**: `internal/target/packageoutput`
  - **Direction**: uses shared package/payload mechanics with Codex-owned serializers.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes model render requests and returns plans.

## Constraints and Invariants

- Do not invent plugin-root `agents/`; keep generated `.codex/agents` profiles separate from installable plugin roots.
- Do not use the stale root `hooks.json` claim for plugin output; default is `hooks/hooks.json`.
- Matching command hooks may launch concurrently; portable order is supported only when native behavior preserves it.
- Non-managed plugin hooks require user trust; generation does not grant or mutate trust.
- Unsupported async, event, decision, or failure semantics return diagnostics with no partial package.
- The format revision increments for the verified plugin-path/contract correction.

## Test Specification

- Golden package trees use the verified default hook path; agent assets render only in the separate target-root project profile.
- Project agent behavior remains unchanged and separately tested.
- Trust, concurrency, async, unsupported event/failure, collisions, hook-free regression, catalog, and deterministic cases are covered.
