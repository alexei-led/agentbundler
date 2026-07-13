# agentbundler

Compile portable coding-agent assets into deterministic vendor-native target trees.

## Usage

```sh
go run ./cmd/agentbundler build --root PATH

go run ./cmd/agentbundler check --root PATH --json

go run ./cmd/agentbundler check --root PATH --native
```

The command reads `agentbundle.json`, supports `build` and read-only `check`, and writes generated output only for `build`. Use repeated `--target` and `--package` flags to select subsets. JSON results go to stdout; human diagnostics go to stderr.

The current lossless subset is skill-only and accepts one source package per target. It emits native skill roots for Claude (`.claude/skills/`), Copilot (`.github/skills/`), Pi (`.pi/skills/`), and Grok (`.grok/skills/`), and native plugin roots for Codex (`.codex-plugin/plugin.json`, `skills/`) and Cursor (`.cursor-plugin/plugin.json`, `skills/`). Agents, hooks, native resources, and multi-package aggregation are rejected until their portable model and native contract are implemented.

## Verification

```sh
go test ./...
go vet ./...
```
