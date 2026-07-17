# Agent Bundler

[![CI](https://github.com/alexei-led/agentbundler/actions/workflows/ci.yml/badge.svg)](https://github.com/alexei-led/agentbundler/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/alexei-led/agentbundler?sort=semver&display_name=tag)](https://github.com/alexei-led/agentbundler/releases)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/github/license/alexei-led/agentbundler)](LICENSE)

> **One source → target-specific coding-agent layouts**
>
> Claude Code · Codex · Pi · Copilot · Grok Build · Cursor

Define coding-agent assets once. Build the target-specific trees each agent
expects.

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
expected by Claude Code, Codex, Pi, GitHub Copilot, Grok Build, and Cursor.

```text
canonical source + manifest
          │
          ▼
   import → customize → render
          │
          ├── Claude Code   .claude-plugin/ + hooks/ + skills/ + agents/
          ├── Codex         .codex-plugin/ + hooks/ + skills/
          ├── Pi            package.json + typed hook runtime + declared TypeScript extensions
          ├── Copilot CLI   plugin.json + hooks.json + skills/ + agents/
          ├── Grok Build    Claude-compatible plugin + Grok hook roots
          └── Cursor        .cursor-plugin/ + hooks/ + skills/ + agents/
```

## Current scope

Package profiles produce **skills, agents, portable resources, command hooks,
payload files, and deterministic catalogs** in each vendor's native layout.
Claude, Codex, Copilot CLI, Cursor, and Grok use separate plugin roots. Pi can
merge several logical packages into one explicit aggregate package with its
typed hook runtime and explicitly declared native TypeScript extensions.
Project profiles remain available for their narrower target layouts.

Hook portability is semantic, not name-based. An unsupported event, matcher,
decision, timeout, async mode, or failure policy fails with an exact diagnostic;
Agent Bundler never silently weakens a security hook or drops an asset.

**Agent Bundler** is a compiler, not an agent runtime, installer, marketplace, or
publisher. It creates target-ready files; your project or release workflow
decides where to install or publish them. Run repository-owned vendor smoke tests
before publishing; see [Targets and CLI](docs/targets-and-cli.md).

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
run only declared safe validators for Claude and Grok after drift passes.
For a complete first bundle, see the [quick start](docs/quickstart.md). For a
multi-package hook example that builds all six targets, see
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

- [Docs index](docs/README.md)
- [User guide: how to think about **Agent Bundler**](docs/guide.md)
- [Install](docs/install.md)
- [Quick start](docs/quickstart.md)
- [Configuration and source formats](docs/configuration.md)
- [Target customization](docs/customization.md)
- [Targets and CLI reference](docs/targets-and-cli.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Architecture](docs/architecture.md)
- [Release validation](docs/release.md)
- [**Agent Bundler** Agent Skill](skills/agentbundler/SKILL.md)

## License

[MIT](LICENSE)
