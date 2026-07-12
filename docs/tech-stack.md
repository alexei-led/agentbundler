# Tech Stack

This file is the normative implementation stack for every module in the Agentbundler design tree.

## Language and Runtime

- **Language**: Go 1.26.
- **Product form**: standalone command-line compiler.
- **Repository layout**: Go packages follow the documented `cmd/` and `internal/` tree.

## Module and Dependency Management

- **Package manager**: Go Modules, with a root `go.mod`.
- **Module import path**: `github.com/alexei-led/agentbundler`. Cross-module imports use the full path `github.com/alexei-led/agentbundler/<module-path>`; for example, `internal/artifact` is imported as `github.com/alexei-led/agentbundler/internal/artifact`.
- **Dependencies**: Go standard library only. Do not add third-party dependencies unless the design and an explicit stack decision are updated first.
- **Build tooling**: `go build` for the CLI and the standard Go toolchain.

## Testing

- **Framework and runner**: the standard library `testing` package and `go test`.
- **Test style**: behavior-focused, table-driven tests where cases share setup and assertions; test public module seams rather than private implementation details.
- **When to test**: add focused tests when they reduce material risk — non-trivial logic, boundary handling, module contracts, composition, or a regression. Do not add tests merely to satisfy a category or count.
- **TDD**: optional. Start with a failing test only when it usefully clarifies intended behavior or guards a bug; otherwise implement and validate directly.
- **Focused verification**: from the repository root, strip the module path's trailing slash and run `go test ./<module-path>`; for example, `internal/artifact/compare/` uses `go test ./internal/artifact/compare`, and the root module uses `go test .`.
- **Full verification**: run `go test ./...` before final acceptance.

## Quality Checks

- **Formatting**: `gofmt` on all Go source and test files.
- **Static analysis**: `go vet ./...`.
- **Verification rule**: run the relevant tests when present or warranted, plus formatting and static analysis before final acceptance.

## Conventions

- Keep production code inside the module folder described by its `module.md`.
- Use only the contracts documented by the module tree; do not reach into sibling or child internals.
- Preserve deterministic behavior and the no-network/no-clock/no-environment-output constraints from the design documents.
