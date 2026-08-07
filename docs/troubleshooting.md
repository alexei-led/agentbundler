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

## Pi

- Generated Pi agents are listed in `pi.subagents.agents`. Install standalone
  `pi-subagents` separately when those tools are needed.
- Agent Bundler preserves only explicit author dependencies. Do not expect it to
  infer a third-party runtime, peer dependency, extension registration, or
  `node_modules` tree.

For target paths and commands, see [Targets and CLI](targets-and-cli.md). For
source formats, see [Configuration](configuration.md).
