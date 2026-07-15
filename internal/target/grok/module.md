# Grok Build Adapter

**Path**: `internal/target/grok/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders the Grok Build project profile and a separately generated Grok-tested installable plugin. The package profile uses Grok's documented Claude Code compatibility while retaining Grok-owned hook roots and capability limits.

## Functional Responsibilities

- Preserve project skills under `.grok/skills/` for the direct project profile.
- Render a Claude-compatible package tree with Grok-specific command-root handling.
- Map only documented Grok hook events, decisions, matchers, timeouts, and failure behavior.
- Render `.claude-plugin/marketplace.json` and declare the official local plugin validator.

## Subdomain Classification

**Core.** Grok's plugin, compatibility, and hook behavior is independently volatile.

## Encapsulated Knowledge

- Grok project paths and Claude-compatible plugin discovery.
- `GROK_PLUGIN_ROOT`, `GROK_PLUGIN_DATA`, hook events, explicit deny, timeout, and fail-open behavior.
- `grok plugin validate` declaration.

## Public Contract

<!-- contract: TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/compiler/model/module.md -->

```text
render(GrokAdapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

The project profile remains `.grok/skills/<name>/SKILL.md`. Separate package-profile roots contain:

```text
.claude-plugin/plugin.json
skills/<name>/SKILL.md
agents/<name>.md
hooks/hooks.json
<hook payload files>
```

The tree is generated and tested for Grok; it is not a byte-copy of Claude output. Package-file command arguments use `GROK_PLUGIN_ROOT`, not an ambient source path. Catalog-enabled installable output is format revision 5.

Documented portable events are session start/end, prompt submit, pre/post tool, post-tool failure, stop, notification, pre-compact, and post-compact. Matchers are regexes over mapped tool names. `PreToolUse` is the only blocking event and supports explicit deny; Grok documents no input-rewrite decision. Timeout/crash/malformed-output failures are fail open, so `hook.failure.closed` is unsupported. Grok does not document asynchronous command handlers, so `hook.async` is unsupported. Claude-compatible agents render as Markdown, but Claude-only `sandbox_mode` is rejected.

The adapter declares `grok plugin validate <plugin-root>` as a `NativeCheck` for every generated plugin root. Grok documents no separate offline marketplace validator, so the Claude-compatible catalog is covered by golden/schema tests. Rendering does not execute validators.

Primary sources: <https://docs.x.ai/build/features/skills-plugins-marketplaces> and <https://docs.x.ai/build/features/hooks>, accessed 2026-07-15. See `docs/vendor-package-contracts.md`.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: registry exposes the adapter and exact capabilities.
- **Counterpart**: `internal/target/packageoutput`
  - **Direction**: shares rooting, payload copying, executable propagation, and collision handling.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes render requests and returns declarative plans/checks.

## Constraints and Invariants

- `.grok/skills` is never conflated with the installable Claude-compatible plugin profile.
- Similar Claude behavior does not authorize unsupported Grok rewrite, closed-failure, or handler cells.
- Shell and exec conversion preserves its declared semantic mode or fails.
- Catalog generation and native validation do not publish, install, trust, authenticate, or mutate Grok configuration.
- Native checks are official, offline, local-tree, and non-mutating; process execution stays in artifact native verification after no drift.
- Hook-free version-1 input stays compatible except an explicit format-revision/path correction.

## Test Specification

- Project and package goldens prove distinct roots and exact plugin/catalog paths.
- Event/matcher/timeout/block/fail-open/root-variable cases are covered.
- Grok validator declarations are exact and render remains process-free.
