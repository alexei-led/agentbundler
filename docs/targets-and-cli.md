<!-- markdownlint-disable MD013 -->

# Targets and CLI

Agent Bundler writes one complete subtree under `output/<target>`. Output is
deterministic and offline. Installation and publication are external actions.

## Output

| Target | Package root | Catalog |
| --- | --- | --- |
| Agent Plugins | `plugin.json`, `skills/`, `mcp.json` (when MCP servers are present), `extensions/`, package files | none |
| Antigravity | `plugin.json`, `skills/`, supported `agents/`, explicit native resources | none |
| Claude | `.claude-plugin/plugin.json`, `skills/`, `commands/`, `agents/`, `hooks/` | `.claude-plugin/marketplace.json` |
| Codex | `.codex-plugin/plugin.json`, `skills/`, `hooks/` | `.agents/plugins/marketplace.json` |
| Pi | `package.json`, `skills/`, optional `agents/`, hooks, generated adapter | none |
| Copilot | `plugin.json`, `skills/`, `agents/`, `hooks.json` | `.github/plugin/marketplace.json` |
| Cursor | `.cursor-plugin/plugin.json`, `skills/`, `agents/`, `hooks/` | `.cursor-plugin/marketplace.json` |
| Grok | Claude-compatible root | `.claude-plugin/marketplace.json` |

The `agent-plugins` target is separate-only and always emits one plugin root per
package. Package archives use plan-owned archive units, not a filesystem walk.
Agent Plugins has no catalog.

Pi packages include the generated hook adapter and explicit author native
extensions. Agents remain in `pi.subagents.agents`; Agent Bundler does not bundle
or register `pi-subagents`. Install that extension separately when needed.

## Capability boundaries

Portable hooks and user-invoked commands are separate capabilities. Unsupported
behavior fails; advisory behavior needs an acknowledgment.

| Capability | Claude | Codex | Pi | Copilot | Cursor | Grok | Antigravity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| User-invoked command | native | unsupported | unsupported | unsupported | unsupported | unsupported | unsupported |
| Hook command `exec` | native | native | native | advisory | advisory | native | unsupported |
| Explicit shell | native | native | native | native | native | native | unsupported |
| Async | passive | unsupported | passive | notification only | unsupported | unsupported | unsupported |
| Block | native mapping | native mapping | native | native mapping | native mapping | native mapping | unsupported |
| Rewrite input | native mapping | unsupported | native | native mapping | native mapping | unsupported | unsupported |
| Package agents | native | project agents only | metadata; install separately | native | native | native | narrow subset |

Antigravity supports portable skills, resources, and agents with exact
non-empty `name` and `description` frontmatter. Portable commands and hooks are unsupported.
Use an explicit Antigravity native resource for vendor hooks, rules, MCP config,
or scripts; those files are trusted and validator checks are not a sandbox.

## Commands

```text
agbun build [--root DIR] [--target TARGET]... [--package PACKAGE]... [--json]
agbun check [--root DIR] [--target TARGET]... [--package PACKAGE]... [--native] [--json]
agbun package --out DIR
```

```sh
agbun build
agbun check
agbun build --target pi
agbun check --target codex --package core-tools
agbun check --native
```

`--root` points to the directory containing `agentbundle.json`; otherwise the
CLI searches parents. Repeated selectors must be unique and declared.

**`build` replaces the complete configured output directory**, including when a
selector is used. Keep `output` dedicated. `check` never writes and exits `2`
for missing, changed, extra, non-regular, or symlinked output. Neither command
uses the network.

With `compatibility.rootManifests`, build/check also manage owned root discovery
files and the Pi root merge. `package` requires current compatibility state but
archives only target-native output. See
[repository-root compatibility](repository-root-compatibility.md).

`check --native` runs declared offline validators only after drift passes:
Antigravity `agy plugin validate`, Claude `claude plugin validate --strict`, and
Grok `grok plugin validate`. A missing declared validator fails. Codex, Pi,
Copilot, and Cursor have no production non-mutating validator.

## Verification and installation

Use generated roots in temporary integration environments. Do not install
plugins into normal user configuration from automation.

```sh
agy plugin validate generated/antigravity
claude plugin validate --strict generated/claude
grok plugin validate generated/grok/core-tools
scripts/check-acceptance-fixture
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
```

Vendor smoke tests are opt-in and require temporary HOME/config/cache roots.
They may need installed CLIs, credentials, or model-backed sessions.

## Help, JSON, and exits

```sh
agbun --version
agbun version
agbun --help
agbun help build
agbun help targets
```

`--json` writes `version`, `command`, `diagnostics`, `drift`, and
`nativeVerificationFailed` to stdout. Normal diagnostics use stderr.

- `0`: success.
- `1`: source, validation, capability, render, or write failure.
- `2`: drift.
- `3`: native validation failure.

`output/.agentbundler/build.json` records hashes, acknowledgments, adapter
revisions, compiler version, and output files. Released binaries report their
injected tag.

Primary links and maintained assumptions:
[Target contracts](vendor-package-contracts.md).
