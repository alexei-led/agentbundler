# Tech Stack

This file is the normative implementation stack for every module in the **Agent Bundler** design tree.

## Language and Runtime

- **Compiler language**: Go 1.26.
- **Contained runtime island**: dependency-free TypeScript under `internal/target/pi/runtime/`, owned by the Pi target module. Its tested source is embedded in `agbun` and copied into generated Pi packages.
- **Product form**: standalone command-line compiler. Generated packages never require or invoke `agbun`.
- **Pi loader compatibility**: Pi loads generated `.ts` extension modules through its supported `jiti` loader. Generated Pi packages require Pi, not Bun, TypeScript, a transpile step, or a separately installed Agent Bundler runtime.
- **Repository layout**: Go packages follow the documented `cmd/` and `internal/` tree. The TypeScript island remains inside the existing `target` architecture boundary.

## Module and Dependency Management

- **Package manager**: Go Modules, with a root `go.mod`.
- **Module import path**: `github.com/alexei-led/agentbundler`. Cross-module imports use the full path `github.com/alexei-led/agentbundler/<module-path>`; for example, `internal/artifact` is imported as `github.com/alexei-led/agentbundler/internal/artifact`.
- **Dependencies**: Prefer the Go standard library. When the standard library does not provide a complete, well-tested primitive, use a mature open-source dependency instead of reimplementing it. Record the dependency and its boundary in this document and keep its use narrow. Current exception: `gopkg.in/yaml.v3` for YAML parsing and serialization.
- **Provenance configuration normalization**: use `encoding/json.Compact` only. It removes insignificant JSON whitespace but does not sort object keys or normalize number or string spellings. Provenance must not claim RFC 8785 conformance.
- **Build tooling**: `go build` for the CLI and the standard Go toolchain. Go embedding copies the Pi runtime source bytes; it does not generate the runtime as a Go string template.
- **TypeScript development tooling**: Bun is used only in `internal/target/pi/runtime/` for dependency install, strict typecheck, and tests. Exact development dependencies and `bun.lock` are committed. Runtime dependencies are empty.

## Testing

- **Framework and runner**: the standard library `testing` package and `go test`.
- **Test style**: behavior-focused, table-driven tests where cases share setup and assertions; test public module seams rather than private implementation details.
- **When to test**: add focused tests when they reduce material risk — non-trivial logic, boundary handling, module contracts, composition, or a regression. Do not add tests merely to satisfy a category or count.
- **TDD**: optional. Start with a failing test only when it usefully clarifies intended behavior or guards a bug; otherwise implement and validate directly.
- **Focused verification**: from the repository root, strip the module path's trailing slash and run `go test ./<module-path>`; for example, `internal/artifact/compare/` uses `go test ./internal/artifact/compare`, and the root module uses `go test .`.
- **Pi runtime verification**: run `(cd internal/target/pi/runtime && bun install --frozen-lockfile && bun run typecheck && bun test)`. Do not use `bunx`; the pinned local TypeScript tool is exposed by the runtime package's `typecheck` script.
- **Full verification**: run `go test ./...` before final acceptance, plus the Pi runtime command once that runtime exists.

## Quality Checks

- **Formatting**: `gofmt` on all Go source and test files.
- **Static analysis**: `go vet ./...`.
- **Architecture analysis**: Archfit analyzes both Go and TypeScript. TypeScript below `internal/target/pi/runtime/` belongs to the existing `target` module and may not create a cross-layer edge.
- **Verification rule**: run the relevant tests when present or warranted, plus formatting, static analysis, and `scripts/check-architecture` before final acceptance.

## Go Contract Projection

The language-neutral contracts in `module.md` files are normative. In Go they map as follows:

- A module's package path is `github.com/alexei-led/agentbundler/<module-path>` and its package name is the final directory name. The repository root package is `agentbundler`.
- A contract type is owned by the module that defines it normatively. A consumer imports that package when its own `module.md` has a marked restatement of the type; it never recreates a look-alike type.
- A contract operation becomes an exported Go function whose name is the operation name converted to PascalCase: `compile` → `Compile`, `detect-drift` → `DetectDrift`, and `run-native-checks` → `RunNativeChecks`.
- `String`, `Boolean`, `Integer`, `ByteSequence`, `Map<String, JsonValue>`, `[T]`, `T?`, and records map to `string`, `bool`, `int`, `[]byte`, `map[string]any`, `[]T`, `*T`, and exported-field structs. Contract enums are named `string` types with constants. A record field uses its contract name converted to PascalCase.
- An operation returning `T + [Diagnostic]` maps to `(T, []model.Diagnostic)`. An operation returning `[Diagnostic]` maps to `[]model.Diagnostic`. `model` means `github.com/alexei-led/agentbundler/internal/compiler/model`.
- Operational filesystem inputs such as `workspace-root` and `output-root` are cleaned absolute `string` paths. They are never model values, serialized output, or generated content.

## Conventions

- Keep production code inside the module folder described by its `module.md`.
- Use only the contracts documented by the module tree; do not reach into sibling or child internals. Importing a contract owner's public Go package through a marked restatement is allowed; importing its private files is not.
- Preserve deterministic behavior and the no-network/no-clock/no-environment-output constraints from the design documents.
- A fresh checkout may fetch pinned Pi development tools only during `bun install --frozen-lockfile`. After setup, product builds/checks, generated output, Go tests, and runtime tests do not use network responses as input.
- Bun and TypeScript are development checks only. Neither may appear as a generated-package runtime dependency.
