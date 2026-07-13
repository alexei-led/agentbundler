# Agentbundler

[![CI](https://github.com/alexei-led/agentbundler/actions/workflows/ci.yml/badge.svg)](https://github.com/alexei-led/agentbundler/actions/workflows/ci.yml)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

Coding-agent assets are not portable. The same skill gets copied into several
vendor-specific directory trees, then those generated copies drift from their
source.

Agentbundler compiles one explicit source into deterministic native layouts for
Claude Code, Codex, Pi, GitHub Copilot, Grok Build, and Cursor. It applies
target-specific composition rules, renders the selected layouts, records
input/output hashes, and can prove whether generated output is current.

```text
source + agentbundle.json
        │
        ▼
 import → compose → render → build or check
        │
        └───────────────► native target trees + provenance
```

- `build` stages, verifies, and replaces the generated output tree.
- `check` compiles in memory and reports missing, changed, or extra output
  without writing.
- Unsupported capabilities fail explicitly instead of being silently dropped.

The current lossless target subset supports skills and one source package per
target. Agents, hooks, native resources, and multi-package aggregation are
rejected by the target renderers.

## Install

```sh
brew install alexei-led/tap/agentbundler
```

### Go alternative

Requires Go 1.26.

```sh
go install github.com/alexei-led/agentbundler/cmd/agentbundler@latest
```

## Start

```sh
agentbundler build --root /path/to/project
agentbundler check --root /path/to/project
```

See the [guide](docs/guide.md) for source layout, configuration, target output,
examples, and architecture.

## License

[MIT](LICENSE)
