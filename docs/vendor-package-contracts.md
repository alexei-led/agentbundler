# Vendor Package Contracts

This note pins the native package and hook contracts used by Agent Bundler. Primary sources were checked on 2026-07-15. A target adapter must fail an unlisted semantic cell rather than infer parity from a similar name.

## Contract summary

| Target             | Installable package root                                                         | Hook file                                                               | Catalog path                                     | Production native validator              |
| ------------------ | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------ | ---------------------------------------- |
| Claude Code        | `.claude-plugin/plugin.json`, `skills/`, `agents/`                               | `hooks/hooks.json`                                                      | `.claude-plugin/marketplace.json`                | `claude plugin validate --strict <root>` |
| OpenAI Codex       | `.codex-plugin/plugin.json`, `skills/`, optional `.mcp.json`                     | `hooks/hooks.json`                                                      | `.agents/plugins/marketplace.json`               | None                                     |
| Pi                 | `package.json`, declared `pi.extensions`, `pi.skills`, and package resources     | Agent Bundler `hooks/hooks.v1.json` consumed by one generated extension | None; install the package root with `pi install` | None                                     |
| GitHub Copilot CLI | root `plugin.json`, `skills/`, `agents/*.agent.md`                               | root `hooks.json`                                                       | `.github/plugin/marketplace.json`                | None                                     |
| Cursor             | `.cursor-plugin/plugin.json`, `skills/`, `agents/`                               | `hooks/hooks.json`                                                      | `.cursor-plugin/marketplace.json`                | None                                     |
| Grok Build         | Grok-tested Claude-compatible `.claude-plugin/plugin.json`, `skills/`, `agents/` | `hooks/hooks.json`                                                      | `.claude-plugin/marketplace.json`                | `grok plugin validate <root>`            |

Catalog paths are generated artifacts only. Agent Bundler does not register, publish, submit, install, authenticate, fetch, or change vendor configuration.

Repository vendor smokes are opt-in under the `vendor_smoke` build tag. A shared
test-only harness requires exact CLI names, uses positive subprocess deadlines,
bounds combined output to 32 KiB, supplies temporary HOME/config/cache roots,
and verifies normal configuration digests after mutating tests. CI runs only the
safe local-tree validators, pinned to Claude Code 2.1.210 and Grok Build 0.2.101;
the Grok Linux binary is checksum-pinned. Codex, Pi, Copilot, and Cursor
install/load smokes are never production native checks.

## Claude Code

- Native layout: `.claude-plugin/plugin.json` is the manifest; `skills/`, `agents/`, `hooks/`, and payload files remain at the plugin root. The default plugin hook file is `hooks/hooks.json`, or `plugin.json#hooks` may provide an inline object or one or more contained `./`-prefixed plugin-relative paths.
- Package paths: exec-form command hooks use `command` plus `args`; package-file arguments use `${CLAUDE_PLUGIN_ROOT}`. Shell-form `command` remains explicit shell behavior.
- Portable events with direct native events: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`, `Notification`, `PreCompact`, and `PostCompact`.
- Match and decisions: tool-event matchers can select native tool names. `PreToolUse` can explicitly deny a tool call and can return updated input. Async command hooks are usable only where the portable hook is passive; an async hook cannot preserve a blocking or rewrite decision.
- Limits: command `timeout` is seconds. A general command crash is not the same as portable fail-closed behavior; `hook.failure.closed` stays unsupported unless an exact event mapping proves it. HTTP, prompt, agent, and MCP-tool handlers are outside the initial command-hook contract.
- Validation: `claude plugin validate --strict <root>` is official, offline for a local tree, and non-mutating. It is the only Claude command allowed in production native verification. The `vendor_smoke` test runs it with temporary `HOME`, `CLAUDE_CONFIG_DIR`, and `XDG_CONFIG_HOME` roots and blocked proxy endpoints.
- Offline fire boundary: installed Claude Code 2.1.210 exposes plugin validation and session-time plugin loading, but no command that fires a hook directly. `--plugin-dir` loads hooks only as part of a Claude session, which requires a model request. Agent Bundler therefore does not claim an installed-CLI fire smoke; the hermetic subprocess protocol test covers stdin, decisions, exit status, timeout, and output limits without a network or model session.
- Sources: [plugin reference](https://code.claude.com/docs/en/plugins-reference) and [hooks reference](https://code.claude.com/docs/en/hooks).

## OpenAI Codex

- Native layout: `.codex-plugin/plugin.json` is required. Installable plugins may contain `skills/`, `hooks/`, `.mcp.json`, `.app.json`, and assets. The verified plugin contract does **not** define a plugin agent component; project custom agents remain separate at `.codex/agents/*.toml`.
- Hook path: the default plugin hook file is `hooks/hooks.json`. `plugin.json#hooks` can replace it with one path, several paths, inline hook objects, or an array of inline objects. This corrects the older root-`hooks.json` and plugin-`agents/` assumptions.
- Package paths: plugin hook commands receive `PLUGIN_ROOT` and `PLUGIN_DATA`, plus Claude-compatible aliases.
- Portable event candidates: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `PreCompact`, and `PostCompact`. Codex also exposes `PermissionRequest`, `SubagentStart`, and `SubagentStop`, which are not initial portable events. There is no verified `SessionEnd`, `Notification`, or `PostToolUseFailure` equivalent in the current hook list.
- Match and decisions: matchers are effective for tool and compaction events but ignored for `UserPromptSubmit` and `Stop`. `PreToolUse` can deny supported Bash, `apply_patch`, and MCP calls and can rewrite supported input shapes. Interception is not a complete enforcement boundary. Matching command hooks launch concurrently, so portable ordering is unsupported where behavior depends on serialized execution.
- Limits: `timeout` is seconds, default 600. Only command handlers run. Prompt and agent handlers are parsed but skipped. `async` is parsed but async command hooks are skipped. Non-managed plugin hooks require user review and trust.
- Validation: no official stable offline non-mutating validator covers the required package and hook behavior. Marketplace add/list, install/load, and trust checks are test-only, opt-in, and use temporary `CODEX_HOME`/configuration.
- Sources: [plugins](https://developers.openai.com/codex/plugins), [build plugins](https://developers.openai.com/codex/build-plugins), [hooks](https://developers.openai.com/codex/hooks), and [subagents](https://developers.openai.com/codex/subagents).

## Pi

- Evidence pinned: installed `@earendil-works/pi-coding-agent` 0.80.7, `docs/packages.md`, `docs/extensions.md`, `README.md`, and `examples/extensions/`.
- Native layout: package resources are declared in root `package.json#pi`. `pi.extensions` accepts paths to `.ts`/`.js` extensions and `pi.skills` accepts skill roots. Paths are package-relative. Pi can install a local package root directly with `pi install <path>`.
- Loader/runtime: Pi loads TypeScript extensions through `jiti`, so generated packages need no compilation step, Bun, TypeScript package, or Agent Bundler executable. Agent Bundler emits one tested dependency-free runtime plus one thin adapter and lists exactly that adapter in `pi.extensions`.
- Event mapping: portable session start/end, prompt submit, pre/post tool, stop/passive turn completion, and compaction map through Pi extension events such as `session_start`, idempotent `session_shutdown`, `input`/`before_agent_start`, sequential-preflight `tool_call`, `tool_result`, `turn_end`/`agent_end`, and `session_before_compact`/`session_compact`.
- Match and decisions: `tool_call` can block. Its mutable input can implement rewrite only after the runtime validates the replacement because Pi does not revalidate after mutation. Parallel tool calls are preflighted sequentially and then may execute concurrently. Extension callbacks receive cancellation signals where the Pi API exposes them.
- Limits: Pi has no declarative hook JSON contract, timeout, failure, environment, or process-output policy. Those semantics belong to the embedded Agent Bundler runtime. Hook subprocesses inherit only `PATH`, plus `PATHEXT`, `SYSTEMROOT`, `WINDIR`, and `COMSPEC` on Windows; ambient credentials and all other variables are omitted. The generated descriptor is `hooks/hooks.v1.json`, not a Pi-native manifest.
- Validation: Pi has no stable offline non-mutating package validator. The opt-in `vendor_smoke` test runs installed Pi 0.80.7 `pi install -l` and `pi list` from a temporary project, sets `PI_CODING_AGENT_DIR` to a temporary directory, and byte-checks that the normal user settings path is unchanged. A second smoke imports the generated adapter through Pi's real `jiti` extension loader and proves one extension/handler registration plus adapter-path schema diagnostics. Tool-call execution remains in the deterministic fake-runtime tests because firing it through the CLI requires an active model turn.

## GitHub Copilot CLI

- Native layout: `plugin.json` is at the plugin root. Default component roots are `skills/` and `agents/`; agents use `*.agent.md`. The manifest points hooks to root `hooks.json`. Copilot also recognizes `hooks/hooks.json`, but Agent Bundler uses the documented root form.
- Package paths: `${PLUGIN_ROOT}` refers to plugin files. Hook command entries are shell commands (`bash`, `powershell`, or cross-platform `command`), so canonical exec arguments may be rendered only where the adapter can prove equivalent quoting on each emitted platform form.
- Portable events: Copilot accepts both camelCase native names and PascalCase/VS Code-compatible names. Agent Bundler emits `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`, `Notification`, and `PreCompact` to retain Claude-compatible payloads. There is no `PostCompact` event in the documented list.
- Match and decisions: `PreToolUse` can allow, deny, ask, or replace arguments; portable block and rewrite-input map there. PascalCase `PreToolUse` uses the documented Claude-compatible matcher names and semantics. Hook entries of one event execute in order. Notification is inherently asynchronous and never blocks.
- Limits: `timeoutSec` is seconds, default 30. Most failures are fail-open. A `preToolUse` command crash/nonzero exit is fail-closed, but its timeout is fail-open; therefore portable closed-failure is supported only when the requested failure cases exactly match, never as a blanket claim. HTTP and prompt hooks are outside the initial command-hook contract.
- Validation: no stable official offline non-mutating plugin validator is documented. The opt-in smoke verified Copilot CLI 1.0.70 local marketplace add, direct local plugin install, and list under temporary `HOME`, `COPILOT_HOME`, and cache roots. It hashes the normal Copilot configuration and cache trees before and after to prove they are unchanged.
- Sources: [CLI plugin reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference) and [hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference).

## Cursor

- Native layout: `.cursor-plugin/plugin.json` is required. Default component roots include `skills/`, `agents/`, and `hooks/hooks.json`. Multi-plugin repositories use `.cursor-plugin/marketplace.json`.
- Package paths: plugin scripts are package-relative. Command hook entries are command strings; explicit shell stays shell, and canonical exec may be rendered only through a proven equivalent representation.
- Portable events: `sessionStart`, `sessionEnd`, `beforeSubmitPrompt`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `stop`, and `preCompact`. The hook API also exposes event-specific shell, MCP, file, subagent, Tab, and workspace hooks. There is no documented portable `postCompact` equivalent.
- Match and decisions: generic tool hooks match documented `Shell`, `Read`, `Write`, `Grep`, `Delete`, `Task`, and `MCP:<tool_name>` names. Only portable tool categories with exact native names are enabled. `preToolUse` supports `permission: allow|deny` and `updated_input`; `beforeSubmitPrompt` can continue or block. Documented `failClosed: true` is emitted only for applicable blocking events. Session lifecycle and compaction hooks are observational where documented.
- Limits: `timeout` is seconds. Exit 0 uses JSON output, exit 2 blocks an applicable action, and other failures default fail-open. Prompt handler hooks are outside the initial command-hook contract.
- Validation: Cursor documents no stable official offline non-mutating plugin validator. Cursor Agent 2026.07.09-a3815c0 exposes noninteractive `--plugin-dir` only as part of a model-backed `--print` session. The opt-in smoke therefore runs only with `CURSOR_API_KEY`, isolates home/config/cache/workspace roots, and verifies a private skill token; otherwise it skips with the exact offline boundary.
- Sources: [plugins](https://cursor.com/docs/plugins), [plugin reference](https://cursor.com/docs/reference/plugins), and [hooks](https://cursor.com/docs/hooks).

## Grok Build

- Native layout: Grok states full Claude Code compatibility and reads Claude marketplaces, plugins, skills, agents, and hooks. Agent Bundler therefore emits a Grok-tested Claude-compatible tree with `.claude-plugin/plugin.json`, `skills/`, `agents/`, and `hooks/hooks.json`. The direct project profile remains separate at `.grok/skills/`.
- Package paths: plugin hook processes receive `GROK_PLUGIN_ROOT` and `GROK_PLUGIN_DATA`. Generated Grok commands use those variables rather than assuming Claude's root variable.
- Portable events: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`, `Notification`, `PreCompact`, and `PostCompact` have documented events.
- Match and decisions: matchers are regular expressions over mapped tool names. `PreToolUse` is the only blocking event and supports explicit deny. Grok documents no input-rewrite decision.
- Limits: timeout is seconds, default 5. Exit 0 allows and exit 2 denies. Timeouts, crashes, and malformed output fail open, so portable closed-failure and rewrite-input are unsupported. HTTP handlers are outside the initial command-hook contract.
- Validation: `grok plugin validate <root>` validates a local plugin manifest without installing it. It is the only Grok command allowed in production native verification.
- Sources: [skills, plugins, and marketplaces](https://docs.x.ai/build/features/skills-plugins-marketplaces) and [hooks](https://docs.x.ai/build/features/hooks).
