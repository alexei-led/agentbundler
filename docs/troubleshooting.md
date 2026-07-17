# Troubleshooting

Start with the exact diagnostic. **Agent Bundler** fails closed for invalid source,
unsupported capability use, and output drift.

## `MANIFEST_NOT_FOUND`

**Symptom:**

```text
MANIFEST_NOT_FOUND: ...
```

**Fix:** run from a directory containing `agentbundle.json`, or pass its parent
with `--root`:

```sh
agbun build --root /path/to/project
```

`--root` is resolved relative to the current working directory. Without it,
**Agent Bundler** searches the current directory and its parents.

## `build` replaces files you expected to keep

`build` owns the complete configured `output` directory. It stages and replaces
that directory, including when a target or package selector produces only a
partial plan.

Use a dedicated output such as `generated/`. Do not point `output` at a project
root containing hand-written agent configuration. Move deployment or copying to
a separate release step.

## `check` reports drift

**Symptom:** `check` exits `2` or reports missing, changed, extra, non-regular,
or symlinked entries.

**Fix:** run `agbun build`, then inspect the generated tree. If the
source or manifest changed, a stale output is expected. If a hand-written file
was added below `output`, remove it or move it outside the compiler-owned tree.

Use JSON output after the manifest is discoverable:

```sh
agbun check --json
```

## `unsupported capability` or native-gap errors

Current renderers emit skills, portable resources, supported native agents,
typed command hooks with payloads, and deterministic catalogs. An error usually
means the selected target cannot preserve the hook's event, matcher, decision,
timeout, async, or failure behavior, or an advisory conversion lacks an exact
acknowledgment. Target-native resources and non-command hook handlers remain
unsupported. A policy can classify a gap but cannot make a semantic mismatch
safe.

If the asset is not needed for the target, use an asset target allow-list. If an
advisory conversion is acceptable, add the exact key/reason acknowledgment to
the target sidecar. Never acknowledge an unsupported security decision; change
the hook or target instead.

## Invalid Antigravity plugin name or description

Antigravity package IDs become `plugin.json#name` and must match
`^[A-Za-z0-9_-]+$`. Use only letters, digits, hyphen, and underscore. If package
metadata includes `description`, it must be a string. The generated manifest has
no version, catalog, or extra metadata fields.

## `unsupported-agent-field`

Security-sensitive agent fields are target-specific. For example,
`sandbox_mode` is a Codex agent field and must not appear in shared base
frontmatter. Put it in the agent's Codex sidecar:

```json
{
  "frontmatterPatch": {
    "sandbox_mode": "read-only"
  }
}
```

For a bundle agent file, save this as
`<agent-directory>/.agentbundler/targets/codex.json`. Alternatively, exclude the
agent from targets that cannot preserve the field's semantics. Antigravity
agents are narrower: frontmatter must contain exactly non-empty string `name`
and `description`, with no additional fields.

## Frontmatter parse errors

Frontmatter must be a YAML object between the first two `---` lines:

```md
---
name: demo
description: A valid object
---

# Body
```

Common causes:

- duplicate YAML keys;
- aliases, custom tags, timestamps, or non-finite numbers;
- a non-object value;
- an invalid UTF-8 bundle Markdown file.

Known security-sensitive target fields fail when another target cannot preserve
their semantics. Other vendor-specific keys are not exhaustively validated; the
target runtime still needs to support them.

## Section patch does not apply

For `bodyPatch.mode = "sections"`:

- use an exact hierarchical ATX path such as `["Examples", "PostgreSQL"]`;
- use heading levels 1 through 6;
- do not use Setext headings;
- do not place the anchor only inside a fenced code block;
- make sure the path exists exactly once;
- do not submit overlapping section patches.

The heading remains; only its body is replaced.

## File overlay does not win

Overlay paths are relative to the asset directory. A path cannot be both in
`files` and `deletedFiles`. A filesystem file under the overlay's `files/`
directory wins over a JSON `files` value at the same path. Deleting a missing
file is a no-op.

## Package or target selection fails

Selectors must match values declared or imported by the manifest:

```sh
agbun build --target pi --package team-skills
```

Targets are `antigravity`, `claude`, `codex`, `pi`, `copilot`, `grok`, and
`cursor`. Duplicate selectors fail validation.

## Antigravity hook or native-resource failures

Portable hooks are unsupported for Antigravity. Exclude them with an exact
asset target allow-list. If vendor-native behavior is required, declare a
non-empty Antigravity-only `asset.native-resource` under
`src/plugins/antigravity/<component>/`. A raw `hooks.json`, `mcp_config.json`,
or script is copied without semantic validation and remains trusted input.

## Missing `agy` or native validation failure

`agbun check --native` requires `agy` for each generated Antigravity CLI plugin.
A missing executable or nonzero `agy plugin validate` result exits as native
verification failure. Install the documented validator, run
`agy plugin validate <plugin-root>` in isolated config roots, and inspect its
output. Validation is not installation or a sandbox. Do not probe mutating
`agy plugin install`, `link`, `enable`, `disable`, or `uninstall` commands in the
normal user environment; Antigravity CLI 1.1.3 can parse help flags as actions.

## Symlink or unsafe-path errors

Source, sidecar, and output paths must stay inside their configured roots and
must be regular files/directories. Absolute paths, `..`, backslashes, empty path
components, duplicate separators, and symlinks are rejected intentionally.

## Target does not recognize the output

First confirm the generated target-relative path in
[targets and CLI](targets-and-cli.md). Then verify the target's current runtime
documentation. **Agent Bundler** creates files; it does not install, enable, or
register an agent plugin.

If the path is correct but the feature is ignored, inspect frontmatter keys and
runtime requirements. An emitted file is not a guarantee that the vendor
supports every key or plugin feature.
