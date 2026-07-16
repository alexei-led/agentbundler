# Copilot CLI Adapter

**Path**: `internal/target/copilot/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module renders GitHub Copilot CLI project profiles and installable plugins with target-owned hook and marketplace semantics.

## Functional Responsibilities

- Preserve existing project-profile skills under `.github/skills/`.
- Render package roots with `plugin.json`, skills, agents, hooks, and payload files.
- Map supported portable hook events, tool categories, commands, timeouts, decisions, ordering, and failure behavior.
- Render target-wide `.github/plugin/marketplace.json` in separate mode.

## Subdomain Classification

**Core.** Copilot plugin and hook contracts are independently volatile.

## Encapsulated Knowledge

- Root plugin manifest, component paths, `${PLUGIN_ROOT}`, and marketplace schema.
- Copilot CLI PascalCase/VS Code-compatible hook events, command fields, matchers, decision output, timeout, and failure behavior.
- The absence of an official production native validator.

## Public Contract

<!-- contract: TargetRenderInput, TargetPlan, CapabilityRule, Diagnostic — restated from internal/compiler/model/module.md -->

```text
render(CopilotAdapter, TargetRenderInput) -> TargetPlan + [Diagnostic]
```

Separate package roots contain:

```text
plugin.json
skills/<name>/SKILL.md
agents/<name>.agent.md
hooks.json
<hook payload files>
```

The manifest points to the root `hooks.json`. Package-file command arguments use `${PLUGIN_ROOT}` through a target-owned representation. Copilot command hooks are shell command fields (`bash`, `powershell`, or `command`). Explicit shell hooks keep the cross-platform `command` field; portable exec hooks emit only the Bash command, with Bash quoting for every literal and package-file argument. PowerShell is not emitted, so exec hooks do not run on Windows hosts.

Verified portable events render with the documented PascalCase/VS Code-compatible names: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`, `Notification`, and `PreCompact`. `PreToolUse` uses Claude-compatible matcher names. Copilot has native block and rewrite results, but Agent Bundler rejects those portable capabilities until a target-owned process-protocol translator exists. Hook entries execute in order. Notification is inherently async and cannot block. There is no documented `PostCompact` event.

Failure behavior is event-specific: most command failures and timeouts fail open; pre-tool command crashes/nonzero exits fail closed while pre-tool timeout still fails open. Portable `hook.failure.closed` is therefore unsupported. Pre-tool behavior and conversion of portable exec arguments into Copilot's cross-platform shell `command` field are advisory and require exact acknowledgments. HTTP and prompt handlers are outside the initial command-hook contract.

Primary sources: <https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference> and <https://docs.github.com/en/copilot/reference/hooks-reference>, accessed 2026-07-15. See `docs/vendor-package-contracts.md`.

## Integrations

- **Counterpart**: `internal/target`
  - **Direction**: registry exposes the adapter and capability cells.
- **Counterpart**: `internal/target/packageoutput`
  - **Direction**: shares rooting, payload copying, mode propagation, and collision handling.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: consumes render requests and returns declarative plans.

## Constraints and Invariants

- Vendor hook schema, event names, root variable, catalog bytes, and decision mapping remain in this leaf.
- Unsupported post-compact, async blocking, exec quoting, or failure semantics return diagnostics with no partial plan.
- Executable intent on payloads is preserved even when a handler invokes them through an interpreter.
- Marketplace generation never registers, installs, publishes, or changes Copilot configuration.
- No official stable offline non-mutating validator is declared. Install/list smoke tests are test-only, opt-in, isolate `HOME`, `COPILOT_HOME`, and cache paths, and hash normal Copilot configuration trees before and after execution.
- Native output changes increment the explicit format revision; hook-free version-1 inputs remain compatible.

## Test Specification

- Golden project and package trees cover exact manifest, agent, hook, payload, and catalog paths.
- Event/matcher/timeout/order/failure cases prove exact capability states.
- Decision capability tests prove rejection before output until protocol translation exists.
- Bash execution tests run on Unix for quoted literals, spaces, and package-file arguments.
- Unsupported post-compact, unsafe exec conversion, and failure-policy combinations yield no partial plan.
