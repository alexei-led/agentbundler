# Claude Adapter

**Path**: `internal/target/claude/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders Claude Code-native project and installable-plugin output, including typed command hooks, payloads, catalog metadata, and safe strict-validator declarations.

## Functional Responsibilities

- Render `.claude-plugin/plugin.json`, skills, agents, resources, hooks, and payload files.
- Map portable hooks to verified Claude events, tool matchers, commands, timeouts, async flags, and decisions.
- Render target-wide `.claude-plugin/marketplace.json` in separate mode.
- Declare exact capability states, format revision, and official native checks.

## Subdomain Classification

**Core.** Claude plugin and hook contracts are primary and independently volatile.

## Encapsulated Knowledge

- Claude plugin paths, schemas, `${CLAUDE_PLUGIN_ROOT}`, and marketplace format.
- Claude hook event/matcher/decision behavior and timeout units.
- Official strict validation command.

## Public Contract

<!-- contract: Adapter, TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/target/module.md and internal/compiler/model/module.md -->

```text
render(ClaudeAdapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

Package-profile separate roots contain:

```text
.claude-plugin/plugin.json
skills/<name>/SKILL.md
agents/<name>.md
hooks/hooks.json
<payload files>
```

When distribution metadata is present, the target root also contains one
`.claude-plugin/marketplace.json`. Its entries use `./` for one flat package or
`./<package-id>` for multi-package roots and are ordered by package ID.

Package-file command arguments render with `${CLAUDE_PLUGIN_ROOT}` and contained payload paths. The target format revision increments from the hook-free revision when these native bytes or catalog output are enabled.

Verified initial semantic cells:

- native/equivalent: `asset.hook`, `hook.command.exec`, explicit adopted `hook.command.shell`, events `session-start`, `session-end`, `prompt-submit`, `pre-tool`, `post-tool`, `post-tool-failure`, `stop`, `notification`, `pre-compact`, `post-compact`, and tool-category matchers;
- native only for passive compatible events: `hook.async`;
- unsupported until a target-owned translator exists: `hook.decision.block` and `hook.decision.rewrite-input`; Claude's native decision output is not the portable author-payload protocol;
- unsupported unless a concrete mapping proves crash and timeout behavior: `hook.failure.closed`;
- unsupported: HTTP, prompt, agent, and MCP-tool handlers in the initial portable command-hook contract.

A similarly named event is not enough: unsupported matcher, mutation, async, timeout, or failure semantics fail through exact capability diagnostics.

The adapter declares `claude plugin validate --strict <plugin-root>` as a `NativeCheck` for each generated installable root. When a catalog is present, one strict check at `.` validates the marketplace and all local plugin roots together. Rendering does not invoke the process.

Primary sources: <https://code.claude.com/docs/en/plugins-reference> and <https://code.claude.com/docs/en/hooks>, accessed 2026-07-15. See `docs/vendor-package-contracts.md`.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: registry exposes this adapter.
- **Counterpart**: `internal/target/packageoutput`
  - **Direction**: uses shared rooting/payload mechanics with Claude-owned serialization callbacks.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes render input and returns plan/diagnostics.

## Constraints and Invariants

- Native hook manifest is `hooks/hooks.json`; no target-neutral interchange file is emitted.
- Arbitrary adopted shell remains explicit shell; canonical exec is not rendered through an implicit shell when native exec form preserves it.
- Closed-failure security policy is never presented as equivalent to explicit deny behavior.
- Native checks are offline, non-mutating declarations and run only after exact no-drift comparison.
- Catalog generation is deterministic artifact creation, never publication or installation.
- Hook-free version-1 output remains stable except an explicit format-revision/native-path correction.

## Test Specification

- Golden trees cover hook-free and mixed hook packages, command roots, event/matcher/timeout/async mappings, payload modes, collisions, and catalogs.
- Decision capability tests prove rejection before output until protocol translation exists.
- Unsupported semantic cells produce no partial plan.
- Official strict validator declarations are exact and process-free at render time.
