# Quick start

This tutorial builds one skill for all currently supported targets. It uses the
simplest source kind, `skills-repository`.

> **Before you build:** `build` replaces the configured `output` directory.
> Use a dedicated directory such as `generated/`.

## 1. Create the source tree

```text
team-skills/
├── agentbundle.json
└── source/
    └── skills/
        └── explain-query/
            ├── SKILL.md
            └── references.md
```

Create `source/skills/explain-query/SKILL.md`:

```md
---
{ "name": "explain-query", "description": "Explain SQL queries clearly" }
---

# Explain a query

Explain what a query does. Identify correctness and performance risks.

## Examples

Use a small query and explain it line by line.
```

Create `source/skills/explain-query/references.md`:

```md
# Query notes

Use `EXPLAIN` before making performance claims.
```

Frontmatter is a YAML object between the first two `---` lines. Values must be
JSON-compatible. This is
Agentbundler's portable input format, not a claim that every agent recognizes
every possible key.

## 2. Add a manifest

Create `agentbundle.json` beside `source/`:

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

`root` is relative to the manifest. `roots` is relative to `root`. Each listed
root must contain at least one `SKILL.md` somewhere below it.

## 3. Build

Run from `team-skills/`:

```sh
agentbundler build
```

Expected result:

```text
build: ok
```

The target trees are created below `generated/`:

```text
generated/
├── .agentbundler/build.json
├── claude/.claude/skills/explain-query/
├── codex/.codex-plugin/plugin.json
├── codex/skills/explain-query/
├── pi/.pi/skills/explain-query/
├── copilot/.github/skills/explain-query/
├── grok/.grok/skills/explain-query/
├── cursor/.cursor-plugin/plugin.json
└── cursor/skills/explain-query/
```

Each skill directory contains `SKILL.md` and `references.md`.

## 4. Check the output

```sh
agentbundler check
```

Expected result:

```text
check: current
```

`check` does not write. It reports changed, missing, extra, non-regular, or
symlinked output entries as drift.

## 5. Use one target tree

Give your target project the contents of the matching generated directory. For
example, `generated/pi/` contains `.pi/skills/explain-query/`. The generated
path is a file-layout contract; confirm the target agent's runtime behavior in
[its official documentation](targets-and-cli.md#vendor-documentation).

Next: [customize one target](customization.md), or choose a different source
format in [configuration](configuration.md).
