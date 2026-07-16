# Cursor Adapter

**Path**: `internal/target/cursor/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Cursor project profiles and installable plugins using Cursor's current plugin and hook contracts.

## Functional Responsibilities

- Preserve the existing direct project profile where selected.
- Render `.cursor-plugin/plugin.json`, skills, agents, hooks, and payload files for package profiles.
- Translate supported portable hook semantics without treating similar event names as equivalent by default.
- Render target-wide `.cursor-plugin/marketplace.json` for separate packages.

## Subdomain Classification

**Core.** Cursor plugin and hook contracts are independently volatile.

## Encapsulated Knowledge

- Cursor plugin component discovery and marketplace schema.
- `hooks/hooks.json`, command-string handlers, tool matchers, timeout, and event-specific failure behavior.
- The absence of an official production native validator.

## Public Contract

<!-- contract: TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/compiler/model/module.md -->

```text
render(CursorAdapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

Separate package roots contain:

```text
.cursor-plugin/plugin.json
skills/<name>/SKILL.md
agents/<name>.md
hooks/hooks.json
<hook payload files>
```

Cursor auto-discovers those component roots. Package-file handler arguments are rendered as contained package-relative command paths. Since Cursor command hooks are command strings, a canonical exec hook is equivalent only where deterministic quoting preserves every literal/package-file argument; adopted shell remains explicit shell.

Verified portable events include session start/end, prompt submit (`beforeSubmitPrompt`), generic pre/post tool, post-tool failure, stop, and pre-compact. Generic tool matchers use only exact documented names: `Shell`, `Read`, `Write`, `Task`, and `MCP:<tool_name>`; broader portable edit, search, and web categories are rejected because Cursor does not expose a lossless matcher. Cursor has native allow/deny and updated-input results, but Agent Bundler rejects portable block and rewrite capabilities until a target-owned process-protocol translator exists. Session lifecycle and pre-compact behavior is observational where documented. No current `postCompact` event exists.

The hook entry `timeout` is seconds. Exit 0 uses JSON output, exit 2 blocks an applicable action, and other failures default fail open. Documented `failClosed: true` is emitted only for blocking pre-tool and prompt-submit hooks; the adapter cannot make a blanket closed-failure claim. Prompt handler hooks are outside the initial command-hook contract.

Primary sources: <https://cursor.com/docs/plugins>, <https://cursor.com/docs/reference/plugins>, and <https://cursor.com/docs/hooks>, accessed 2026-07-15. See `docs/vendor-package-contracts.md`.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: registry exposes the adapter and exact capabilities.
- **Counterpart**: `internal/target/packageoutput`
  - **Direction**: shares package roots, payload copying, executable propagation, and collision handling.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes render requests and returns declarative plans.

## Constraints and Invariants

- Package profile is Cursor's native plugin tree; project-profile `.cursor/` output remains a separate mode.
- Unsupported post-compact, async, unsafe exec conversion, or closed-failure semantics fail with no partial plan.
- Plugin scripts/payloads stay contained and retain executable intent.
- Catalog generation is deterministic artifact output only; it does not submit to Cursor Marketplace or install a plugin.
- Cursor exposes no stable official offline non-mutating plugin validator. The `--plugin-dir` smoke is test-only, requires `CURSOR_API_KEY`, and uses temporary home/config/cache/workspace roots; without credentials it skips rather than touching normal Cursor state.
- Verified native path changes increment the target format revision while hook-free version-1 input stays compatible.

## Test Specification

- Golden package trees cover plugin, skills, agents, hook file, payloads, and catalog paths.
- Tests cover event/matcher/timeout/fail-open/closed constraints and deterministic order.
- Decision capability tests prove rejection before output until protocol translation exists.
- Unsupported semantic cells and collisions return no partial plan.
