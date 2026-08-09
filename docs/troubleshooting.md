# Troubleshooting

Run `agbun check --json` first. It reports stable diagnostic codes and does not
write files.

## Manifest or source errors

- `MANIFEST_NOT_FOUND`: run from the repository root or pass `--root DIR`.
- Invalid JSON, unknown fields, duplicate keys, or bad IDs: fix the named
  manifest/source location. Agent Bundler intentionally does not guess.
- Unsafe path or symlink diagnostic: replace the path with a contained regular
  file/directory. Absolute paths, traversal, backslashes, and symlinks are not
  accepted.
- Package or target selector failure: use IDs declared by the manifest. Root
  compatibility requires every configured compatibility target and all packages.

## Output and drift

- `build` replaces the entire configured `output` directory. Keep source files
  outside it and use `check` before rebuilding.
- Drift means output is missing, changed, extra, non-regular, or symlinked.
  Inspect the diagnostic, then run `build` if replacing generated output is
  intended.
- Root compatibility drift is separate from `output` drift. Do not hand-edit
  owned marketplace files, Codex profiles, or merged Pi fields; change source
  configuration and rebuild.

## Capability and overlays

- Unsupported capability/native-gap diagnostics mean the selected target cannot
  preserve requested semantics. Remove the target, use an explicit native
  resource, or provide an acknowledged advisory mapping.
- Antigravity agents require exact non-empty string `name` and `description`.
  Portable commands and hooks are unsupported there.
- A Markdown section patch must name one exact ATX heading path. Missing,
  duplicate, or overlapping paths fail.
- Tree-backed overlay files override JSON `files` entries at the same path.

## Vendor validation

- `check --native` validates only Antigravity, Claude, and Grok after output is
  current. Install the required validator or omit `--native`.
- Validator success checks package shape, not scripts, hooks, MCP configuration,
  or other trusted native resources.
- Vendor smoke tests are opt-in. Run them only with temporary HOME/config/cache
  roots; missing CLIs, credentials, or model-backed behavior are vendor
  environment limits, not compiler output proof.

## Agent Plugins

- `AGENT_PLUGINS_PROFILE_MISMATCH`: the schema selector in `plugin.json` or
  `mcp.json` does not match the pinned profile `agent-plugins/1.0.0-bd383552`.
  Only the pinned spec version is supported.
- `AGENT_PLUGINS_DUPLICATE_KEY`: JSON input contains a duplicate object key.
  Remove the duplicate key from the upstream file.
- `AGENT_PLUGINS_INVALID_NAME`: the plugin name violates the 1–64 lowercase
  letters/digits/hyphens/periods rule. Fix the name in `plugin.json`.
- `AGENT_PLUGINS_LINK_EXTERNAL`: a symlink points outside the plugin root.
  Replace it with a contained regular file or remove it.
- `AGENT_PLUGINS_LINK_CYCLE`: a symlink cycle was detected. Resolve the cycle.
- `AGENT_PLUGINS_SPECIAL_FILE`: a device, FIFO, socket, or other special file
  was found in the plugin root. Remove it or replace with a regular file.
- `AGENT_PLUGINS_QUOTA_EXCEEDED`: the plugin root exceeds entry, file-size,
  total-byte, depth, or path-length limits. Reduce the plugin contents.
- `AGENT_PLUGINS_AGGREGATE`: the `agent-plugins` target does not support
  aggregate package mode. Use separate mode only.
- `AGENT_PLUGINS_NO_PLUGINS`: the `agentPlugin.plugins` list is missing or
  empty. Declare at least one plugin root.

Agent Plugins 1.0.0 is a young standard. The embedded profile pins one specific
upstream commit. To advance the profile, replace the schema files in
`internal/agentplugins/schemas/1.0.0/` with the exact upstream bytes, update
the SHA-256 digests, and follow the D10 compatibility review procedure.

## Pi

- Generated Pi agents are listed in `pi.subagents.agents`. Install standalone
  `pi-subagents` separately when those tools are needed.
- Agent Bundler preserves only explicit author dependencies. Do not expect it to
  infer a third-party runtime, peer dependency, extension registration, or
  `node_modules` tree.

For target paths and commands, see [Targets and CLI](targets-and-cli.md). For
source formats, see [Configuration](configuration.md).
