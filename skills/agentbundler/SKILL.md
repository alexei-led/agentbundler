---
name: agentbundler
description: Build and verify coding-agent skill bundles with **Agent Bundler**. Use when configuring agentbundle.json, creating or updating skills-repository, bundle, or Claude-plugin sources, selecting targets or packages, applying target overlays, generating output, checking drift, or diagnosing **Agent Bundler** CLI failures.
license: MIT
compatibility: Requires the `agbun` CLI on PATH. Commands are local and do not need network access. Use a dedicated generated output directory.
---

# Agent Bundler

Use this skill to operate the `agentbundler` CLI and maintain its source bundle.
Treat `agentbundle.json` and the configured source root as the source of truth.
Treat the configured output directory as compiler-owned build output.

## Start with help

Explore the installed CLI before guessing syntax:

```sh
agbun --version
agbun --help
agbun help build
agbun help check
agbun help targets
```

Help and version commands do not require `agentbundle.json`. Use the version in
`generated/.agentbundler/build.json` to confirm which compiler produced output.

The CLI has two build commands:

- `build` compiles and replaces the complete configured output directory.
- `check` compares the expected plan with output and does not write files.

Do not use `build` against a project root containing hand-written agent files.
Use a dedicated output such as `generated/`.

## Locate the project

1. Look for `agentbundle.json` in the current directory and its parents.
2. If the bundle is elsewhere, pass its parent with `--root DIR`.
3. Read the manifest before changing source or configuration.
4. Do not invent target IDs, package IDs, source paths, or overlay paths.

The supported target IDs are:

```text
claude  codex  pi  copilot  grok  cursor
```

## Configure `agentbundle.json`

Use strict JSON. Unknown fields, duplicate keys, unsafe paths, and invalid
values fail validation. Use `version: 1` even though the current decoder
accepts an omitted version.

Minimal skills repository:

```json
{
  "version": 1,
  "kind": "skills-repository",
  "root": "source",
  "targets": ["claude", "codex", "pi", "copilot", "grok", "cursor"],
  "output": "generated",
  "skillsRepository": {
    "package": "team-skills",
    "roots": ["skills"],
    "metadata": {
      "description": "Team coding skills",
      "version": "1.0.0"
    }
  }
}
```

Source kinds:

- `skills-repository`: recursively finds `SKILL.md` below each declared root.
  Other regular files in a skill directory become support files.
- `bundle`: package JSON lists exact asset paths. Use this for explicit package
  membership or multiple package IDs.
- `claude-plugin`: imports `.claude-plugin/plugin.json` and recognized Claude
  plugin assets.

Current renderers emit skills, portable resources, supported native agents,
typed command hooks with payloads, and deterministic catalogs. Claude, Codex,
Copilot, Cursor, and Grok package profiles support separate package-ID roots. Pi
supports an explicit aggregate package with one registered embedded hook runtime.
Renderers do not publish or install packages. `check --native` runs only declared
safe Claude and Grok validators after drift passes.

## Create a skill source

A skill directory must contain `SKILL.md`:

```text
source/
└── skills/
    └── explain-query/
        ├── SKILL.md
        └── references.md
```

**Agent Bundler** accepts YAML frontmatter between the first two `---` lines. Values
must be JSON-compatible; `agentbundle.json` remains strict JSON:

```md
---
{ "name": "explain-query", "description": "Explain SQL queries" }
---

# Explain a query

Explain the query, then identify correctness and performance risks.
```

The renderer writes present frontmatter as compact JSON. Do not assume a
vendor-specific frontmatter key is supported just because **Agent Bundler** accepts
it; verify it in the target agent's documentation.

## Apply target-specific changes

For a `skills-repository`, put a per-skill overlay here:

```text
source/.agentbundler/assets/skill/explain-query/targets/pi.json
```

An overlay can:

- recursively merge frontmatter; `null` removes a key;
- replace the entire body;
- replace the body under an exact hierarchical ATX heading path;
- add or replace support files;
- delete support files with `deletedFiles`.

Example:

```json
{
  "frontmatterPatch": {
    "description": "Explain SQL queries using Pi conventions",
    "metadata": { "audience": "developers", "draft": null }
  },
  "bodyPatch": {
    "mode": "sections",
    "sections": [
      {
        "headingPath": ["Explain a query", "Examples", "PostgreSQL"],
        "body": "Use EXPLAIN ANALYZE for PostgreSQL examples.\n"
      }
    ]
  },
  "files": { "references.md": "Pi-specific reference text\n" },
  "deletedFiles": ["references/legacy.md"]
}
```

Section replacement keeps the heading and replaces its body until the next
heading at the same or higher level. The path must occur exactly once. Fenced
code-block headings and Setext headings are not anchors. Missing, duplicate,
or overlapping patches fail.

A filesystem file under the overlay's `files/` directory wins over a JSON
`files` value at the same path. Paths are asset-relative. A path cannot be in
both `files` and `deletedFiles`. Deleting a missing file is a no-op.

Target overlays do not inherit from another target. Composition is target-wide
and can prepend a `skillPreamble`; it also classifies capabilities and native
gaps. A composition entry replaces that target's default capability rules, so
list every required rule when declaring `capabilities`.

## Build and verify

Build all targets declared by the manifest:

```sh
agbun build
```

Build selected output. Remember that `build` still replaces the complete output
directory, including output for targets not selected:

```sh
agbun build --root ./plugin --target pi
```

Check without writing:

```sh
agbun check
agbun check --target pi --package team-skills
agbun check --json
```

Use `--native` only with `check`:

```sh
agbun check --native
```

Use `--json` when another tool needs structured results. With a discoverable
manifest it writes one result object containing `version`, `command`,
`diagnostics`, `drift`, and `nativeVerificationFailed`. Manifest-discovery
failures occur before that object is created.

Exit statuses:

- `0`: success; `check` found current output.
- `1`: source, validation, capability, render, or write failure.
- `2`: output drift.
- `3`: native verification failure.

Normal workflow after source/config changes:

1. Inspect the manifest and target/package selection.
2. Run `agbun check` to see whether output is stale.
3. Run `agbun build` using the dedicated output directory.
4. Run `agbun check` again.
5. For a distributed package, run repository-owned vendor smoke tests. `agbun
check --native` runs only declared checks; built-in adapters do not invoke
   optional vendor CLIs.
6. Inspect generated target paths and report any unsupported source assets.

## Diagnose failures

- `MANIFEST_NOT_FOUND`: run from the manifest directory or pass `--root`.
- `DRIFT_*`: output is missing, changed, extra, non-regular, or symlinked; run
  `build` only after confirming the output directory is disposable.
- Frontmatter errors: use a JSON object, no trailing commas, and no duplicate
  keys.
- Section patch errors: use the exact heading path once; do not use Setext
  headings or anchors inside fenced code.
- Unsupported capability/native-gap errors: check the target hook event,
  matcher, decision, timeout, async, and failure-policy cells. Advisory
  conversions require an exact acknowledgment. A policy cannot make an
  unsupported security decision or target-native resource safe.
- Target not recognizing files: confirm the generated target path, then check
  the target agent's current runtime documentation. **Agent Bundler** creates files;
  it does not install, enable, or register an agent plugin. Validate a published
  package with the repository's target-specific vendor smoke tests.

When reporting a failure, include the command, exit status, relevant diagnostic,
manifest path, selected target/package, and whether output was changed.
