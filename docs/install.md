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

## Go

Go 1.26 or newer:

```sh
go install github.com/alexei-led/agentbundler/cmd/agentbundler@latest
```

The installed binary is placed in Go's configured `GOBIN` (or `GOPATH/bin`).
Make sure that directory is on `PATH`.

## Build from a checkout

```sh
git clone https://github.com/alexei-led/agentbundler.git
cd agentbundler
go build -o ./bin/agentbundler ./cmd/agentbundler
./bin/agentbundler --help
```

## Verify the CLI

```sh
agentbundler --help
```

The command supports `build` and `check`. See the [CLI reference](targets-and-cli.md)
for all selectors and exit statuses.

Agentbundler is a local compiler. It does not need network access to build or
check a bundle.
