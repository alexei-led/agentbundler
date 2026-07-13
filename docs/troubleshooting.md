# Troubleshooting

Start with the exact diagnostic. Agentbundler fails closed for invalid source,
unsupported capability use, and output drift.

## `MANIFEST_NOT_FOUND`

**Symptom:**

```text
MANIFEST_NOT_FOUND: ...
```

**Fix:** run from a directory containing `agentbundle.json`, or pass its parent
with `--root`:

```sh
agentbundler build --root /path/to/project
```

`--root` is resolved relative to the current working directory. Without it,
Agentbundler searches the current directory and its parents.

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

**Fix:** run `agentbundler build`, then inspect the generated tree. If the
source or manifest changed, a stale output is expected. If a hand-written file
was added below `output`, remove it or move it outside the compiler-owned tree.

Use JSON output after the manifest is discoverable:

```sh
agentbundler check --json
```

## `unsupported capability` or native-gap errors

Current renderers emit one package of skills only. Agents, hooks, scripts,
native resources, and arbitrary custom capability uses are not rendered. A
composition policy can classify or resolve a native gap, but it cannot make an
unsupported asset appear in the output.

If the asset is not needed for the target, exclude it in the source/package
selection or use a target that supports the required representation. Otherwise,
keep the richer source for a future adapter and publish only the skill subset.

## Frontmatter parse errors

Frontmatter must be a JSON object between the first two `---` lines:

```md
---
{ "name": "demo", "description": "A valid object" }
---

# Body
```

Common causes:

- YAML syntax instead of JSON;
- a trailing comma;
- duplicate JSON keys;
- a non-object value;
- an invalid UTF-8 bundle Markdown file.

Vendor-specific keys are not validated by Agentbundler. The target runtime still
needs to support them.

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
agentbundler build --target pi --package team-skills
```

Targets are `claude`, `codex`, `pi`, `copilot`, `grok`, and `cursor`. Duplicate
selectors fail validation.

## Symlink or unsafe-path errors

Source, sidecar, and output paths must stay inside their configured roots and
must be regular files/directories. Absolute paths, `..`, backslashes, empty path
components, duplicate separators, and symlinks are rejected intentionally.

## Target does not recognize the output

First confirm the generated target-relative path in
[targets and CLI](targets-and-cli.md). Then verify the target's current runtime
documentation. Agentbundler creates files; it does not install, enable, or
register an agent plugin.

If the path is correct but the feature is ignored, inspect frontmatter keys and
runtime requirements. An emitted file is not a guarantee that the vendor
supports every key or plugin feature.
