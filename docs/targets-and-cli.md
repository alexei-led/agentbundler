<!-- markdownlint-disable MD013 -->

# Targets and CLI reference

**Agent Bundler** writes one complete target subtree under `output/<target>/`.
Paths below are target-relative. Package output is deterministic and offline;
installation and publication remain external operations.

## Package output

Separate mode keeps one package at the flat target root. Two or more packages
use package-ID roots and add a target-wide catalog when the target supports one
and `distribution` is set. Pi hook packages use explicit aggregate mode instead.
Antigravity requires package profile and separate mode and never emits a
catalog.

| Target          | Installable package paths                                                                                                  | Catalog path                       |
| --------------- | -------------------------------------------------------------------------------------------------------------------------- | ---------------------------------- |
| Antigravity CLI | `plugin.json`, `skills/`, supported `agents/*.md`, explicit native resources                                               | none                               |
| Claude Code     | `.claude-plugin/plugin.json`, `skills/`, `agents/`, `hooks/hooks.json`, hook payloads                                      | `.claude-plugin/marketplace.json`  |
| Codex           | `.codex-plugin/plugin.json`, `skills/`, `hooks/hooks.json`, hook payloads                                                  | `.agents/plugins/marketplace.json` |
| Pi              | `package.json`, `skills/`, optional `agents/`, `hooks/hooks.v1.json`, `extensions/agentbundler-hooks.ts`, embedded runtime | none                               |
| Copilot CLI     | `plugin.json`, `skills/`, `agents/*.agent.md`, `hooks.json`, hook payloads                                                 | `.github/plugin/marketplace.json`  |
| Cursor          | `.cursor-plugin/plugin.json`, `skills/`, `agents/*.md`, `hooks/hooks.json`, hook payloads                                  | `.cursor-plugin/marketplace.json`  |
| Grok Build      | Claude-compatible `.claude-plugin/plugin.json`, `skills/`, `agents/`, `hooks/hooks.json`, hook payloads                    | `.claude-plugin/marketplace.json`  |

Project profiles remain narrower: Claude uses `.claude/skills`; Codex uses
`.codex-plugin/plugin.json`, a root `skills/` tree, and `.codex/agents/*.toml`;
Pi uses `.pi/skills`; Copilot uses `.github/skills`; Grok uses
`.grok/{skills,resources}`; and Cursor keeps its project layout. The Grok
package profile is distinct from `.grok/skills`. Antigravity has no project
profile. Its package name must match `^[A-Za-z0-9_-]+$`; `plugin.json` contains
only `name` and optional string `description`. One selected package is flat, and
multiple packages use package-ID roots.

The Pi aggregate package contains one generated hook-adapter registration plus
any declared author-owned native extension entries. Its thin adapter imports a
dependency-free TypeScript runtime embedded in `agbun`; the generated package
needs neither Bun, TypeScript, npm dependencies, nor the `agbun` executable at
load time. Generated agents remain listed in `pi.subagents.agents`, but Agent
Bundler does not bundle or register `pi-subagents`. Install `pi-subagents` as a
standalone Pi extension separately when those agent tools are needed.

## Portable hook cells

Support is semantic. Similar vendor event names are not enough. An unsupported
cell fails before output is written; an advisory cell requires a source
acknowledgment.

| Cell             | Claude              | Codex                           | Pi aggregate                      | Copilot CLI              | Cursor                          | Grok                |
| ---------------- | ------------------- | ------------------------------- | --------------------------------- | ------------------------ | ------------------------------- | ------------------- |
| Command `exec`   | native              | native                          | native                            | advisory: Bash form only | advisory: quoted command string | native              |
| Explicit `shell` | native              | native                          | native                            | native                   | native                          | native              |
| Tool matcher     | native categories   | Bash/MCP subset                 | native categories                 | native categories        | documented native-name subset   | native categories   |
| Async            | passive hooks       | unsupported                     | passive hooks                     | notification only        | unsupported                     | unsupported         |
| Block            | pre-tool translator | pre-tool translator             | pre-tool                          | pre-tool translator      | pre-tool translator             | pre-tool translator |
| Rewrite input    | pre-tool translator | unsupported                     | pre-tool                          | pre-tool translator      | pre-tool translator             | unsupported         |
| Fail closed      | advisory pre-tool   | advisory pre-tool               | runtime-enforced                  | advisory pre-tool        | pre-tool/prompt-submit only     | advisory pre-tool   |
| Package agents   | native              | unsupported; use project agents | metadata; install separately      | native                   | native                          | native              |

Decision-bearing hooks use a canonical subprocess stdin/stdout protocol and a
target-owned translator. Claude, Copilot, Cursor, and Pi translate block and
rewrite input. Codex and Grok translate block but reject rewrite input because
their documented protocol has no lossless rewrite result. Advisory pre-tool
fail-closed mappings require a source acknowledgment; no target claims a
blanket failure-policy equivalence.

Event subsets also differ. Codex does not provide the required portable
`session-end`, `notification`, or `post-tool-failure` equivalents. Copilot has no
documented `post-compact`. Cursor has no equivalent `notification` or
`post-compact`. Pi has no equivalent notification event. Claude and Grok expose
the full initial event-name set, but only events with matching timeout and
failure behavior are accepted. Target validators return exact per-hook
diagnostics for narrower cases.

### Antigravity capability boundary

Antigravity supports portable skills, resources, and agents whose frontmatter
contains exactly non-empty string `name` and `description`. Skills are
discovered as slash commands; Agent Bundler does not emit legacy `commands/`.
Every portable `asset.hook` and `hook.*` cell is unsupported, including event,
matcher, command, timeout, async, ordering, failure-policy, block, and rewrite
semantics.

A bundle may explicitly declare an Antigravity-only `asset.native-resource`
under `src/plugins/antigravity/<component>/`. Its complete symlink-free tree is
copied to the plugin root, allowing paths such as `rules/*.md`,
`mcp_config.json`, `hooks.json`, and support scripts without parsing or semantic
validation. These files are trusted code/configuration. `agy plugin validate`
validates plugin shape; it is not a sandbox and does not make raw hooks or MCP
configuration portable.

HTTP, prompt-handler, agent-handler, and MCP-tool-handler hooks are not part of
the initial portable command-hook contract. Pi-native extension trees are
supported only with an explicit `piExtensions` resource declaration; other
undeclared target-native resources remain explicit gaps. Hooks and extensions
execute trusted package code; validation is not a sandbox.

## Build and check

```text
agbun build [--root DIR] [--target TARGET]... [--package PACKAGE]... [--json]
agbun check [--root DIR] [--target TARGET]...
  [--package PACKAGE]... [--native] [--json]
```

Examples:

```sh
agbun build
agbun check
# WARNING: build replaces the complete output directory, even for one target.
agbun build --target pi
agbun check --target codex --package core-tools
agbun check --native
```

`--root` points to the directory containing `agentbundle.json`. Without it,
**Agent Bundler** searches the current directory and its parents. `--target` and
`--package` may be repeated; selectors must be declared and unique. Selecting a
single package in separate mode uses the historical flat package root.

`build` stages and replaces the complete configured output directory. `check`
compares the plan without writing and exits `2` for missing, changed, extra,
non-regular, or symlinked output. Neither command uses the network.

When `compatibility.rootManifests` is configured, the same commands also manage
owned repository-root vendor discovery files and the Pi `package.json` merge.
`package` requires them to be current but still archives only the native target
root under the configured output directory. See
[repository-root compatibility](repository-root-compatibility.md).

`check --native` runs only target-declared, offline, non-mutating validators
after drift passes:

- Antigravity: `agy plugin validate .` in each plugin root;
- Claude: `claude plugin validate --strict <root>`;
- Grok: `grok plugin validate <root>`.

A missing declared validator, including `agy`, is a native-verification failure,
not a skip. Codex, Pi, Copilot, and Cursor have no production native validator
because their useful load/install checks are mutating, model-backed, or
incomplete.

### Idempotence quality contract

Idempotence is a required product quality. It is stronger than deterministic
bytes. With unchanged source bytes, manifest, target/package selectors, and
compiler version:

- `build` must compare the complete planned tree with current output and perform
  no staging, replacement, chmod, or timestamp update when they match;
- `check` must perform no writes whether output is current, drifted, or invalid;
- `package` must preserve an existing archive and its timestamp when the
  deterministic archive bytes match;
- real path, byte, type, executable-mode, or stale-file drift may trigger the
  existing complete atomic replacement and cleanup path.

The current implementation only partially meets this contract. `check` is
write-free, and generated trees and archives are byte-deterministic. However,
`build` currently replaces the complete output tree on every successful run,
and `package` replaces byte-identical archives. This unnecessary rewrite is a
known product gap. Until content-aware no-op writes are implemented, run
`agbun check` first and run `agbun build` only after drift exit status `2`.
Compilation or observation failures use status `1` and must not be treated as a
reason to overwrite output.

## Installation and validation examples

Build first. Then use the exact generated target root in an isolated integration
or release job:

```sh
# Official non-mutating validators.
agy plugin validate generated/antigravity
claude plugin validate --strict generated/claude
grok plugin validate generated/grok/core-tools

# Optional external installation of a validated one-package plugin.
agy plugin install "$(pwd)/generated/antigravity"

# Pi aggregate package; isolate both project and user configuration.
package_root=$(cd generated/pi && pwd -P)
smoke_root=$(mktemp -d)
trap 'rm -rf "$smoke_root"' EXIT
mkdir -p "$smoke_root/workspace" "$smoke_root/home"
(
  cd "$smoke_root/workspace"
  HOME="$smoke_root/home" \
  PI_CODING_AGENT_DIR="$smoke_root/pi-agent" \
  XDG_CONFIG_HOME="$smoke_root/config" \
  XDG_CACHE_HOME="$smoke_root/cache" \
  PI_OFFLINE=1 \
  pi install "$package_root" -l --approve
)

# Repository test suite: mutating/load smokes are opt-in and isolate config.
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
```

The `agy plugin install` command is an optional user action, not part of
`agbun`; do not run it in automation or against normal user state.

The opt-in smokes use temporary `HOME` and vendor config/cache roots, bounded
subprocesses and output, exact missing-CLI skips, and post-run digests of normal
config paths. Cursor's load smoke also requires `CURSOR_API_KEY`; without it the
test names the unsupported offline boundary. No smoke publishes a package or
uses the developer's normal vendor configuration.

For the hermetic all-target acceptance case:

```sh
scripts/check-acceptance-fixture
```

It copies `testdata/cc-thingz-hooks` into two temporary roots, builds all seven
targets, runs read-only `check`, and compares every generated byte.

## Help and version

Help and version commands do not require a manifest:

```sh
agbun --version
agbun version
agbun --help
agbun help build
agbun help check
agbun help targets
```

`-h` and `--help` work at the top level and after `build` or `check`. Help exits
`0`; invalid command or help-topic errors point back to `agbun help`.

## Machine-readable results

With a valid manifest, `--json` writes one object to stdout with `version`,
`command`, `diagnostics`, `drift`, and `nativeVerificationFailed`. Diagnostics
normally go to stderr. A diagnostic omits `location` when unavailable; a present
location has `path`, `line`, and `column`.

Exit statuses:

- `0`: success; `check` found current output;
- `1`: source, validation, capability, render, or write failure;
- `2`: output drift;
- `3`: native verification failure.

`generated/.agentbundler/build.json` records configuration, input/output hashes,
acknowledgments, adapter revisions, compiler version, and output file details.
Released binaries report their injected tag; development builds report the Go
module version when available, otherwise `agbun-dev`.

`agbun package --out DIR` archives current target roots without rebuilding.
Repository-root compatibility wrappers and the development `package.json` are
never included in these target-native archives. Antigravity uses the default deterministic
`<distribution.name>-antigravity.tar.gz` archive and has no marketplace root.

## Vendor documentation

- [Antigravity CLI plugins](https://antigravity.google/docs/cli/plugins)
- [Antigravity CLI features](https://antigravity.google/docs/cli/features)
- [Antigravity CLI releases](https://github.com/google-antigravity/antigravity-cli/releases)
- [Pinned Conductor source](https://github.com/gemini-cli-extensions/conductor/tree/fb6212e8faee3f9ecb69f0ee19bd5b2a0765bb0a)
- [Claude Code plugins and hooks](https://code.claude.com/docs/en/plugins-reference)
- [Codex plugins and hooks](https://developers.openai.com/codex/plugins)
- [Pi packages and extensions](https://github.com/earendil-works/pi/tree/main/packages/coding-agent/docs)
- [Copilot CLI plugins and hooks](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference)
- [Cursor plugins and hooks](https://cursor.com/docs/plugins)
- [Grok Build skills, plugins, and marketplaces](https://docs.x.ai/build/features/skills-plugins-marketplaces)
- [Pinned contract notes](vendor-package-contracts.md)

These runtimes change independently. Re-check their primary docs before adding
vendor-specific assumptions.
