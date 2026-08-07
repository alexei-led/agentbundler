<!-- markdownlint-disable MD013 -->

# Release validation

Run from a clean checkout. Release tags are `vMAJOR.MINOR.PATCH` on `master`.
The release workflow validates, builds six OS/architecture binaries, creates the
GitHub release, and dispatches the Homebrew update.

## Required local gate

```sh
gofmt -l $(git ls-files -- '*.go')
git diff --check
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

Vendor smoke tests are opt-in. Run them only with isolated vendor config roots:

```sh
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
```

They may require installed CLIs, credentials, or a model-backed session. A skip
or failure is a vendor-environment result, not proof that deterministic rendering
is broken.

## Version and tag

Release builds inject the tag into `internal/buildinfo.releaseVersion`:

```sh
go build -ldflags='-X github.com/alexei-led/agentbundler/internal/buildinfo.releaseVersion=vX.Y.Z' ./cmd/agbun
```

Both commands must print the same version:

```sh
agbun version
agbun --version
```

Release steps:

```sh
git status --short --branch
git add -A
git commit -m '...'
git push origin master
git tag -a vX.Y.Z -m 'vX.Y.Z'
git push origin vX.Y.Z
```

Verify the tag-triggered workflow and release:

```sh
gh run list --workflow release.yml --limit 1
gh release view vX.Y.Z
```

Do not commit generated output, vendor configuration, credentials, or release
artifacts. Migration of consuming repositories is a separate task.
