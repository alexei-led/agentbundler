<!-- markdownlint-disable MD013 -->

# Target contracts

Maintained vendor assumptions used by adapters. Agent Bundler renders files; it
does not install, publish, authenticate, or modify vendor state.

| Target | Package root | Catalog | Native validator |
| --- | --- | --- | --- |
| Agent Plugins | `plugin.json`, `skills/`, `mcp.json` (when present), `extensions/`, package files | none | none |
| Antigravity | `plugin.json`, `skills/`, optional `agents/` and native resources | none | `agy plugin validate <root>` |
| Claude | `.claude-plugin/plugin.json`, `skills/`, `commands/`, `agents/`, `hooks/` | `.claude-plugin/marketplace.json` | `claude plugin validate --strict <root>` |
| Codex | `.codex-plugin/plugin.json`, `skills/`, `hooks/` | `.agents/plugins/marketplace.json` | none |
| Pi | `package.json`, `pi`, `skills/`, `hooks/` | none | none |
| Copilot | `plugin.json`, `skills/`, `agents/`, `hooks.json` | `.github/plugin/marketplace.json` | none |
| Cursor | `.cursor-plugin/plugin.json`, `skills/`, `agents/`, `hooks/` | `.cursor-plugin/marketplace.json` | none |
| Grok | Claude-compatible plugin root | `.claude-plugin/marketplace.json` | `grok plugin validate <root>` |

The Agent Plugins target emits the portable `agent-plugins.org` standard format.
Vendor `plugin.json` manifests (Claude, Codex, Copilot, Cursor) are
vendor-specific layouts that happen to share the same filename. They are not
interchangeable with Agent Plugins output.

## Boundaries

- Package output is target-native and deterministic. Root compatibility is
  opt-in and never enters target archives.
- Codex project agents are `.codex/agents/*.toml`; marketplace installation does
  not install them. Root compatibility may copy canonical profiles there.
- Claude Code discovers user-invoked command Markdown at `commands/<name>.md`
  in generated plugin roots and `.claude/commands/<name>.md` in project output.
- Claude Code 2.1.210 auto-loads `hooks/hooks.json` from generated plugin roots.
  Claude manifests omit `hooks` for that standard path to avoid duplicate loading.
- Grok reads Claude marketplaces. Claude and Grok root compatibility cannot be
  enabled together.
- Pi agents remain metadata in `pi.subagents.agents`. Agent Bundler compiles
  author-owned files only: it does not infer or bundle `pi-subagents`, peers,
  dependency closures, `bundledDependencies`, `node_modules`, or third-party
  extension registrations. Users install `pi-subagents` separately when needed.
- Pi `pi.extensions` entries come from explicit author `piExtensions` resources
  plus the generated Agent Bundler hook adapter. Explicit dependencies are
  preserved; implicit dependencies are not created.
- Portable commands are emitted only for Claude's verified Markdown layouts.
  Other targets fail `asset.command` before output.
- Portable hooks are translated only where the target preserves the requested
  event and decision semantics. Unsupported cells fail; advisory losses need an
  acknowledgment.
- Native resources are explicit pass-throughs. Validation does not sandbox them.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
scripts/check-acceptance-fixture
scripts/check-architecture
```

Vendor smoke tests are opt-in and must use temporary HOME/config/cache roots:

```sh
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
```

Primary references:

- [Antigravity plugins](https://antigravity.google/docs/cli/plugins)
- [Claude plugins](https://code.claude.com/docs/en/plugins-reference)
- [Codex plugins](https://developers.openai.com/codex/plugins)
- [Pi packages](https://github.com/earendil-works/pi/tree/main/packages/coding-agent/docs)
- [Copilot plugins](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference)
- [Cursor plugins](https://cursor.com/docs/plugins)
- [Grok plugins and marketplaces](https://docs.x.ai/build/features/skills-plugins-marketplaces)
