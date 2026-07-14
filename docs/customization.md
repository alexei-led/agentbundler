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
    "examples/query.sql": { "base64": "U0VMRUNUICogRlJPTSB0ZWFtOwo=" }
  },
  "deletedFiles": ["references/legacy.md"]
}
```

A path cannot appear in both `files` and `deletedFiles`. Deleting a missing file
is a no-op. Use base64 for raw bytes.

For a tree-backed replacement:

```text
.agentbundler/assets/skill/explain-query/targets/pi/
└── files/
    └── references.md
```

The tree-backed `references.md` wins over the JSON value at the same path.

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
profiles render Claude, Codex, and Pi agents; Pi agents use `pi-subagents` and
therefore require that runtime package. Current renderers still accept one
package per target plan.

Next: [target layouts and CLI](targets-and-cli.md).
