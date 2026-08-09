# Agent Bundler

[![CI](https://github.com/alexei-led/agentbundler/actions/workflows/ci.yml/badge.svg)](https://github.com/alexei-led/agentbundler/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/alexei-led/agentbundler?sort=semver&display_name=tag)](https://github.com/alexei-led/agentbundler/releases)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/github/license/alexei-led/agentbundler)](LICENSE)

> **One source → target-specific coding-agent layouts**
>
> Claude Code · Codex · Pi · Copilot · Grok Build · Cursor · Antigravity CLI · Agent Plugins

Define coding-agent assets once. Build the target-specific trees each agent
expects. Import and build conformant Agent Plugins 1.0.0 packages with full
semantic preservation.

## Why this exists

`SKILL.md` is useful common ground, but it does not define a shared metadata
format, plugin/package layout, or a common way to ship agents, hooks, and
scripts. Each coding agent adds its own conventions. The same instructions
also do not work equally well across models: wording that helps an OpenAI model
may need a different version for an Anthropic model, and vice versa.

Maintaining copied skills and plugin files for every target is tedious. Copies
drift. Small target or model differences become manual release work.

**Agent Bundler** keeps one source of truth, then lets you customize the parts that
actually need to differ: frontmatter, Markdown sections, support files, and
short target-wide preambles. It renders the result into the directory layout
expected by Claude Code, Codex, Pi, GitHub Copilot, Grok Build, Cursor, and
Antigravity CLI.

```text
canonical source + manifest
          │
          ▼
   import → customize → render
          │
          ├── Claude Code   .claude-plugin/ + hooks/ + skills/ + commands/ + agents/
          ├── Codex         .codex-plugin/ + hooks/ + skills/
          ├── Pi            package.json + hook runtime + declared TS extensions
          ├── Copilot CLI   plugin.json + hooks.json + skills/ + agents/
          ├── Grok Build    Claude-compatible plugin + Grok hook roots
          ├── Cursor        .cursor-plugin/ + hooks/ + skills/ + agents/
          ├── Antigravity   plugin.json + skills/ + agents/ + explicit native resources
          └── Agent Plugins plugin.json + skills/ + mcp.json + extensions/ + package files
```

## Current scope

Package profiles produce **skills, agents, portable resources, lifecycle
command hooks, payload files, and deterministic catalogs** in each vendor's
native layout. The standard `agent-plugin` source imports conformant
Agent Plugins 1.0.0 packages; the `agent-plugins` target emits their
plugin.json, skills, MCP config, extension trees, and package files
with full semantic round-trip preservation. Portable user-invoked commands are a separate asset kind; Claude
emits `commands/<name>.md`, while unverified targets fail explicitly instead of
dropping the command. Claude, Codex, Copilot CLI, Cursor, Grok, and Antigravity
CLI use separate plugin roots. Antigravity requires package profile and
separate mode, emits no catalog, supports only agents with `name` and
`description`, and rejects portable commands and hooks. Explicit Antigravity-native rules, MCP configuration, hooks, and
scripts can be copied as opaque native resources. Pi can merge several logical
packages into one explicit aggregate package with its typed hook runtime and
explicitly declared native TypeScript extensions. Project profiles remain
available for their narrower target layouts.

Skills provide reusable instructions, hooks react to lifecycle events, commands
provide explicit user-invoked entry points, and native resources preserve opaque
target-owned behavior. Hook and command portability is semantic, not name-based.
Unsupported behavior fails with an exact diagnostic; Agent Bundler never
silently weakens a security hook or drops an asset.

**Agent Bundler** is a compiler, not an agent runtime, installer, marketplace, or
publisher. It creates target-ready files; your project or release workflow
decides where to install or publish them. Run repository-owned vendor smoke tests
before publishing; see [Targets and CLI](docs/targets-and-cli.md).

## Idempotence

Idempotence is a required product quality, separate from deterministic output.
With unchanged source bytes, manifest, selectors, and compiler version, a
successful `build` must leave a current output tree untouched: no replacement,
mtime change, or filesystem watcher churn. `check` must remain write-free, and
`package` must preserve an existing archive when its deterministic bytes match.
Real drift may still trigger the complete atomic replacement needed to remove
stale output safely.

The current writer and archive implementation produce identical bytes but still
replace unchanged filesystem objects. This is a known product gap, not the
intended steady-state contract. Until content-aware no-op writes land, run
`check` first and invoke `build` only when it reports drift. See
[Idempotence quality contract](docs/targets-and-cli.md#idempotence-quality-contract).

## Install

Homebrew is the default install:

```sh
brew install alexei-led/tap/agentbundler
```

Or install with Go 1.26+:

```sh
go install github.com/alexei-led/agentbundler/cmd/agbun@latest
```

## Start with an existing bundle

Discover the installed CLI, then build from a directory containing
`agentbundle.json`:

```sh
agbun --version
agbun --help
agbun check
agbun build
```

Use `agbun help build`, `agbun help check`, and `agbun help targets` for the
full command, safety, and target-ID reference.

`build` replaces the configured output directory. Use a dedicated generated
directory, not a working project root. `check` is read-only; add `--native` to
run only declared safe validators for Claude, Grok, and Antigravity after drift
passes. Opt-in root vendor discovery can route repository installs into the
generated target trees without symlinks; see
[repository-root compatibility](docs/repository-root-compatibility.md). For a
complete first bundle, see the [quick start](docs/quickstart.md).
For a tested Antigravity CLI plugin, see the
[Conductor-shaped example](examples/antigravity-conductor/README.md). For a
multi-package hook example that builds all seven targets, see
[`testdata/cc-thingz-hooks`](testdata/cc-thingz-hooks).

## What can be customized

A target overlay can:

- merge or remove JSON frontmatter keys, including target-defined metadata
  such as tools;
- replace the whole Markdown body;
- replace the body under an exact heading path;
- add, replace, or delete support files;
- prepend a short target-wide policy through composition.

See [customization](docs/customization.md) for the exact sidecar format and
examples.

## Documentation

- [Documentation index](docs/README.md)
- [Quick start](docs/quickstart.md)
- [Agent skill](skills/agentbundler/SKILL.md)

## License

[MIT](LICENSE)
