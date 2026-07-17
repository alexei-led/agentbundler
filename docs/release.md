# Release validation

A release is valid only after source, generated packages, embedded runtime bytes,
and the CLI version contract pass together. The release workflow does not
publish vendor plugins or mutate vendor configuration.

## Required gate

From the repository root:

```sh
changed_go=$(git diff --name-only -- '*.go')
test -z "$changed_go" || gofmt -d $changed_go
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
archfit check --config .archfit.yaml --require-tools --progress none
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
git diff --check
```

`vendor_smoke` is opt-in. A missing executable skips with its exact CLI name.
Cursor also skips when `CURSOR_API_KEY` is absent because it has no offline load
validator. Every subprocess is time/output bounded and uses temporary config
roots. Claude, Grok, and Antigravity production-native checks are automatic
non-mutating vendor validators. The Antigravity smoke protects the normal
`~/.gemini` tree before and after `agy plugin validate`.

## Runtime and version contract

The Pi runtime source of record is `internal/target/pi/runtime/src`. Go embeds
those exact reviewed files. `TestReleaseBuildEmbedsTestedRuntimeAndPrintsInjectedVersion`
builds a release-shaped `agbun`, generates a Pi package with that binary, and
byte-compares every emitted runtime module with the source tested by Bun.

Release builds inject the tag through:

```text
-X github.com/alexei-led/agentbundler/internal/buildinfo.releaseVersion=vMAJOR.MINOR.PATCH
```

Both commands must print the same exact line:

```sh
agbun version
agbun --version
# agbun vMAJOR.MINOR.PATCH
```

The release matrix builds Darwin, Linux, and Windows on amd64 and arm64. It
checks that every artifact contains the injected tag and executes both version
forms on Linux/amd64. Development builds report the Go module version when
available, otherwise `agbun-dev`.

## Pinned CI evidence

The CI workflow pins action commits, Bun 1.3.14, Claude Code 2.1.210, Grok
Build 0.2.101, and Antigravity CLI 1.1.3. The Antigravity Linux x64 archive is
accepted only when SHA-256
`7a7239a69b65d3cf3af7e75f27b2ff4e9cce696a7b9a9e5c37c695f1c74eec34`
passes and `agy --version` prints `1.1.3`. The validator job builds the
cc-thingz-shaped fixture and runs:

```sh
claude plugin validate --strict testdata/cc-thingz-hooks/generated/claude
grok plugin validate testdata/cc-thingz-hooks/generated/grok/core-tools
grok plugin validate testdata/cc-thingz-hooks/generated/grok/workflow-tools
agy plugin validate testdata/cc-thingz-hooks/generated/antigravity/core-tools
agy plugin validate testdata/cc-thingz-hooks/generated/antigravity/workflow-tools
```

The job uses temporary `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, and
`TMPDIR` roots and blocks network proxies during validation. These commands only
validate local trees. They do not install plugins, mutate normal vendor state,
authenticate, publish, or start model sessions.

## Release checklist

1. Run the required gate on a clean checkout.
2. Inspect all seven target trees produced by `testdata/cc-thingz-hooks`
   against `docs/vendor-package-contracts.md`, including both generated
   Antigravity package roots.
3. Confirm `git diff --check`, GitNexus change detection, and Archfit have no
   unintended dependency-direction regression.
4. Run the scoped architecture re-review across `internal/compiler/model`,
   `internal/compiler/source`, `internal/compiler/composition`,
   `internal/target`, `internal/target/pi/runtime`, and `internal/artifact`.
5. Tag only a commit on `master` with `vMAJOR.MINOR.PATCH`.
6. Let `.github/workflows/release.yml` validate, build, checksum, and create the
   GitHub release before updating the Homebrew formula.

Consuming-repository migration is separate. Passing this gate does not claim
that cc-thingz source has been migrated, installed, or behavior-compared. That
repository must still adopt canonical hook directories, configure Pi aggregate
metadata, run actual vendor installs, and remove legacy compilers only after its
own checks pass.
