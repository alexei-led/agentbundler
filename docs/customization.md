# Target customization

Keep the canonical skill in one place. Add a target overlay only when a target
needs different metadata, wording, or support files.

There are two scopes:

- **Asset overlay:** one asset for one target.
- **Composition:** one target-wide policy for all assets.

An overlay does not inherit from another target. An asset has at most one
overlay per target. The compiler applies changes in this order:

1. copy canonical frontmatter, body, and support files;
2. apply `frontmatterPatch`;
3. delete `deletedFiles`;
4. apply JSON `files` replacements/additions;
5. apply `bodyPatch`;
6. prepend `skillPreamble`, if configured.

A filesystem file in the overlay's `files/` tree wins over a JSON `files` value
at the same path. This gives a deterministic escape hatch for binary or larger
files.

## Where an overlay lives

For a `skills-repository`:

```text
source/
├── skills/explain-query/SKILL.md
└── .agentbundler/assets/skill/explain-query/
    └── targets/pi.json
```

For a `bundle` skill:

```text
bundle/src/skills/explain-query/
├── SKILL.md
└── .agentbundler/
    └── targets/pi.json
```

For a Claude plugin:

```text
plugin/
├── skills/explain-query/SKILL.md
└── .agentbundler/assets/skill/explain-query/targets/pi.json
```

Use `targets/antigravity.json` at the same sidecar location for an Antigravity
skill or agent overlay. Sidecars customize portable assets; they do not turn raw
vendor files into portable semantics.

The overlay file is JSON. The examples below are valid strict JSON.

## Frontmatter

Source frontmatter is a YAML object between the first two `---` lines. Values
must be JSON-compatible. The
patch is recursive: objects merge into objects, other values replace, and
`null` deletes a key.

Canonical `SKILL.md`:

```md
---
{
  "name": "explain-query",
  "description": "Explain SQL queries",
  "metadata": { "audience": "all", "draft": false },
}
---

# Explain a query

## Examples

### PostgreSQL

Use a small query and explain it.
```

`targets/pi.json`:

```json
{
  "frontmatterPatch": {
    "description": "Explain SQL queries with Pi conventions",
    "metadata": {
      "audience": "developers",
      "draft": null
    }
  }
}
```

The generated Pi frontmatter is equivalent to:

```json
{
  "name": "explain-query",
  "description": "Explain SQL queries with Pi conventions",
  "metadata": { "audience": "developers" }
}
```

**Agent Bundler** does not validate vendor-specific frontmatter keys. Add a key only
when the target agent documents it, and test the generated tree with that
agent.

## Replace the whole body

Use this when the target needs a genuinely different document:

```json
{
  "bodyPatch": {
    "mode": "replace",
    "text": "# Explain a query\n\nUse Pi tools and report commands first.\n"
  }
}
```

## Replace a Markdown section

Use an exact hierarchical ATX heading path to replace only one block:

```json
{
  "bodyPatch": {
    "mode": "sections",
    "sections": [
      {
        "headingPath": ["Explain a query", "Examples", "PostgreSQL"],
        "body": "Use EXPLAIN ANALYZE for PostgreSQL examples.\n"
      }
    ]
  }
}
```

The heading stays; its body is replaced until the next heading at the same or
higher level. Heading levels are 1 through 6. Headings inside fenced code
blocks are ignored. Setext headings are not recognized. The anchor must exist
exactly once. Missing, duplicate, or overlapping replacements fail the build.
This is a heading-block operation, not an arbitrary marker replacement.

## Replace, add, or delete files

Paths are relative to the skill directory:

```json
{
  "files": {
    "references.md": "Pi-specific reference text\n",
    "scripts/check.sh": { "text": "#!/bin/sh\nexit 0\n", "executable": true },
    "examples/query.sql": {
      "base64": "U0VMRUNUICogRlJPTSB0ZWFtOwo=",
      "executable": false
    }
  },
  "deletedFiles": ["references/legacy.md"]
}
```

A path cannot appear in both `files` and `deletedFiles`. Deleting a missing file
is a no-op. A string is shorthand for non-executable UTF-8 content. Use exactly
one of `text` or `base64` in the object form, with optional `executable`. Unknown
fields and invalid combinations fail.

For a tree-backed replacement:

```text
.agentbundler/assets/skill/explain-query/targets/pi/
└── files/
    └── references.md
```

The tree-backed `references.md` wins over the JSON value at the same path. This
replaces bytes, executable intent, and source origin together; filesystem mode
sets executable intent for a tree-backed file.

## Target-wide preamble

A composition entry can prepend a short policy to every skill for one target:

```json
{
  "composition": [
    {
      "target": "pi",
      "skillPreamble": "Use Pi tools and report commands first."
    }
  ]
}
```

A preamble is added after the asset body patch. Use an overlay when only one
skill differs.

## Acknowledge a deliberate capability loss

An advisory capability is accepted only with an exact acknowledgment on the
asset overlay. Put the acknowledgment beside the target overlay:

```json
{
  "target": "pi",
  "acknowledgments": [
    {
      "asset": "skill/explain-query",
      "target": "pi",
      "key": "asset.skill.some-feature",
      "reason": "Pi has no equivalent; the skill omits this optional behavior."
    }
  ]
}
```

The `asset`, `target`, and `key` must match the imported asset, selected target,
and advisory capability exactly. Keep the reason specific; it becomes part of
build provenance.

## Capabilities and native gaps

Composition also classifies source capabilities and target-native gaps.
Capability states are `native`, `equivalent`, `advisory`, and `unsupported`.
Advisory capability use requires an exact acknowledgment; unsupported use fails.

A composition entry replaces that target's default rules; it does not merge
with them. If you declare capabilities, list every rule needed for the target:

```json
{
  "composition": [
    {
      "target": "pi",
      "capabilities": [{ "key": "asset.skill", "state": "native" }],
      "nativeGaps": []
    }
  ]
}
```

Native-gap policies use a component name, an action, and an optional
replacement asset:

```json
{
  "target": "pi",
  "nativeGaps": [
    {
      "component": "plugin/resource.bin",
      "action": "replace",
      "replacement": "skill/explain-query"
    }
  ]
}
```

Use `exclude` when the gap should not enter the target package, `source-only`
when it should remain documented but not emitted, or `replace` with an existing
asset as fallback. Every applicable source gap needs one matching policy.

Policy does not make a hook, script, or native resource renderable. Package
profiles render target-supported agent files. Pi records them under
`pi.subagents.agents` but does not bundle an execution extension; install
`pi-subagents` separately if those tools are needed. Antigravity agents accept
only exact non-empty string `name` and `description` frontmatter.

Antigravity-native files use an explicit bundle resource under
`src/plugins/antigravity/<component>/` with a local
`.agentbundler/asset.json` declaring `asset.native-resource` and an exact
Antigravity package allow-list. Rules, raw `mcp_config.json`, raw `hooks.json`,
and scripts are copied without interpretation. They remain trusted explicit
portability gaps: portable hooks are unsupported, and `agy plugin validate` is
not a sandbox.

Next: [target layouts and CLI](targets-and-cli.md).
