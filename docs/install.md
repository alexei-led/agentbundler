# Install

## Homebrew

```sh
brew install alexei-led/tap/agentbundler
```

Update it with:

```sh
brew update
brew upgrade agentbundler
```

The formula name follows the repository identifier; it installs the `agbun`
executable.

## Go

Go 1.26 or newer:

```sh
go install github.com/alexei-led/agentbundler/cmd/agbun@latest
```

The installed binary is placed in Go's configured `GOBIN` (or `GOPATH/bin`).
Make sure that directory is on `PATH`.

## Build from a checkout

```sh
git clone https://github.com/alexei-led/agentbundler.git
cd agentbundler
go build -o ./bin/agbun ./cmd/agbun
./bin/agbun --help
./bin/agbun version
./bin/agbun --version
```

## Verify the CLI

```sh
agbun --version
agbun --help
agbun help targets
```

`agbun version` and `agbun --version` must print the same value. Release binaries
report their injected `vMAJOR.MINOR.PATCH`; development checkouts normally report
`agbun-dev`.

The command supports `build` and `check`. See the
[CLI reference](targets-and-cli.md) for all selectors and exit statuses, and
[release validation](release.md) for the embedded Pi runtime and release-build
checks.

**Agent Bundler** is a local compiler. It does not need network access to
build or check a bundle.
