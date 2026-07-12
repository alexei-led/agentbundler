# agentbundler

Compile portable coding-agent assets into deterministic target packages.

## Usage

```sh
go run ./cmd/agentbundler build --root PATH

go run ./cmd/agentbundler check --root PATH --json

go run ./cmd/agentbundler check --root PATH --native
```

The command reads `agentbundle.json`, supports `build` and read-only `check`, and writes generated output only for `build`. Use repeated `--target` and `--package` flags to select subsets. JSON results go to stdout; human diagnostics go to stderr.

## Verification

```sh
go test ./...
go vet ./...
```
