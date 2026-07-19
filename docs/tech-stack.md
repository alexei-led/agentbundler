# Development conventions

This is for maintainers and coding agents. User docs live in [README](README.md).

## Stack

- Go compiler and CLI. Use the standard library unless a dependency is clearly
  better; current parsing exception: `gopkg.in/yaml.v3`.
- Dependency-free TypeScript exists only in `internal/target/pi/runtime/`. Bun
  is development tooling there, never generated-package runtime.
- Go embeds the Pi hook runtime source and copies it into generated Pi packages.
  Generated packages never invoke `agbun`.

## Boundaries

- Keep production code in its existing `cmd/` or `internal/` module.
- Import exported package contracts; do not reach into sibling private files.
- Preserve deterministic output: no network, clock, environment, or random
  value may affect generated artifacts.
- Keep generated packages target-native. Do not add installers, bootstrap code,
  or inferred third-party dependencies.

## Tests and checks

Add a test only for material behavior, a boundary, or a regression. Prefer a
small table when cases share setup; do not test private implementation details.

```sh
gofmt -w $(git ls-files -- '*.go')
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
(
  cd internal/target/pi/runtime
  bun install --frozen-lockfile
  bun run typecheck
  bun test
)
scripts/check-acceptance-fixture
scripts/check-architecture
```

Vendor smoke tests are opt-in and require isolated vendor configuration:

```sh
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
```

## Contract mapping

`module.md` files state module ownership and public contracts. In Go, a contract
owner exports the relevant types/functions from its package; consumers import
that package rather than recreating look-alike types. JSON-facing model values
remain explicit structs, typed strings, slices, maps, and `[]byte` as appropriate.
