# Customization

Keep canonical content in one place. Add a target overlay only when metadata,
body, support files, or capability policy differs.

- **Asset overlay:** one asset for one target.
- **Composition:** target-wide package, skill, and capability policy.

The compiler applies canonical content, `frontmatterPatch`, `deletedFiles`, JSON
`files`, tree-backed files, `bodyPatch`, then `skillPreamble`. Tree-backed files
win over JSON values at the same path.

## Overlay location

Skills repository:

```text
source/.agentbundler/assets/skill/explain-query/targets/pi.json
```

Bundle skill:

```text
bundle/src/skills/explain-query/.agentbundler/targets/pi.json
```

Claude-plugin imports use the skills-repository location inside the plugin root.
Sidecars customize portable assets only; they do not make raw vendor files
portable.

## Overlay fields

```json
{
  "frontmatterPatch": {
    "description": "Pi-specific description",
    "metadata": { "draft": null }
  },
  "bodyPatch": {
    "mode": "sections",
    "sections": [
      {
        "headingPath": ["Examples", "PostgreSQL"],
        "body": "Use EXPLAIN ANALYZE.\n"
      }
    ]
  },
  "files": {
    "references.md": "Pi-specific reference\n",
    "scripts/check.sh": { "text": "#!/bin/sh\nexit 0\n", "executable": true }
  },
  "deletedFiles": ["references/legacy.md"]
}
```

`frontmatterPatch` merges objects, replaces other values, and deletes `null`
keys. Frontmatter must be JSON-compatible; vendor-specific keys are the
author's responsibility.

`bodyPatch.mode` is `replace` or `sections`. A section uses an exact,
unique hierarchical ATX heading path. Fenced-code headings are ignored; missing,
duplicate, or overlapping paths fail.

`files` paths are relative to the asset directory. A string is non-executable
UTF-8; an object uses exactly one of `text` or `base64` and optional
`executable`. A path cannot be both added and deleted. For larger or binary
files, put replacements in:

```text
.agentbundler/assets/skill/explain-query/targets/pi/files/
```

## Composition

A composition entry can add a preamble to every skill for a target:

```json
{
  "composition": [
    {
      "target": "pi",
      "skillPreamble": "Report commands before conclusions."
    }
  ]
}
```

It can also replace that target's capability rules and native-gap policy.
Capability states are `native`, `equivalent`, `advisory`, and `unsupported`.
Advisory use needs an exact asset acknowledgment; unsupported use fails.
Declaring capability rules replaces defaults, so list the complete intended
policy.

```json
{
  "target": "pi",
  "acknowledgments": [
    {
      "asset": "skill/explain-query",
      "target": "pi",
      "key": "asset.skill.some-feature",
      "reason": "Pi omits this optional behavior."
    }
  ]
}
```

Native gaps can `exclude`, remain `source-only`, or `replace` an imported
component with another asset. Policy does not make unsupported hooks, scripts,
or native resources portable.

Pi agents are recorded under `pi.subagents.agents`; install `pi-subagents`
separately when its tools are needed. Antigravity-native files are explicit
bundle resources under `src/plugins/antigravity/<component>/`; they are copied
without interpretation and remain trusted vendor configuration.

Next: [targets and CLI](targets-and-cli.md).
