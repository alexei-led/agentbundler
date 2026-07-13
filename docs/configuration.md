# Configuration and source formats

Agentbundler reads one strict JSON file named `agentbundle.json`. Unknown
fields, duplicate keys, unsafe paths, and invalid target names fail validation.

## Manifest shape

The common fields are:

- `version`: use `1`. The current decoder also accepts omission.
- `kind`: `skills-repository`, `bundle`, or `claude-plugin`.
- `root`: source root, relative to the manifest directory.
- `targets`: one or more supported target IDs.
- `output`: dedicated generated-output directory.
- `composition`: optional target-wide policy, once per target at most.
- Source block: exactly one block matching `kind`.

The smallest useful `skills-repository` manifest is:

```json
{
  "version": 1,
  "kind": "skills-repository",
  "root": "source",
  "targets": ["pi"],
  "output": "generated",
  "skillsRepository": {
    "package": "team-skills",
    "roots": ["skills"],
    "metadata": {}
  }
}
```

Paths are normalized relative paths. Absolute paths, `..`, backslashes, empty
components, duplicate separators, NUL bytes, and symlinks in source/sidecar
paths are rejected.

## Choose a source kind

### `skills-repository`

Use it for ordinary skill folders. It recursively finds `SKILL.md` below each
listed root. Other regular files become support files. Here `<root>` means the
manifest's source root, not one individual skills directory. Centralized
sidecars use `<root>/.agentbundler/assets/skill/<name>/…`.

### `bundle`

Use it for explicit package membership or several named packages. Package JSON
lists exact asset paths; entries are not globs. Sidecars live beside each bundle
asset under `<asset>/.agentbundler/…`.

> **Build warning:** `build` replaces the complete configured output directory,
> even when selecting one target or package. Keep `output` dedicated.

### `claude-plugin`

Use it to import an existing Claude plugin. The importer reads
`.claude-plugin/plugin.json`, skills, direct agents/hooks, and native gaps.
Centralized sidecars use `.agentbundler/assets/<kind>/<name>/…` inside the plugin.

#### Example skills-repository layout

```text
source/
├── skills/explain-query/SKILL.md
└── .agentbundler/
    └── assets/skill/explain-query/targets/pi.json
```

The directory name containing `SKILL.md` becomes the skill name. Duplicate skill
names across roots fail.

#### Example bundle layout

```text
bundle/
├── packages/team.json
└── src/skills/explain-query/SKILL.md
```

Manifest:

```json
{
  "version": 1,
  "kind": "bundle",
  "root": "bundle",
  "targets": ["pi"],
  "output": "generated",
  "bundle": { "packages": ["packages/team.json"] }
}
```

Package file:

```json
{
  "id": "team-skills",
  "metadata": {},
  "assets": [
    "src/skills/explain-query",
    { "path": "src/agents/reviewer.md", "targets": ["claude", "codex"] },
    { "path": "src/resources/templates", "targets": ["claude", "codex", "pi"] }
  ]
}
```

Bundle package assets use exact forms such as `src/skills/<name>`,
`src/agents/<name>.md`, `src/resources/<name>`, `src/hooks/<name>.json`, and
`src/plugins/<target>/<file>`. The `src/` prefix is optional. Asset target
lists are exact allow-lists. Portable resource directories render under
`resources/` in package profiles; target-native resources remain explicit gaps.

#### Example manifest

```json
{
  "version": 1,
  "kind": "claude-plugin",
  "root": "source",
  "targets": ["claude"],
  "output": "generated",
  "claudePlugin": { "pluginRoot": "plugin" }
}
```

Expected input:

```text
source/plugin/
├── .claude-plugin/plugin.json
└── skills/explain-query/SKILL.md
```

Agents and hooks are recognized during import so the compiler can report an
explicit unsupported capability. They are not rendered by current target
adapters. Unrecognized regular files in a Claude plugin are treated as Claude
native gaps, not portable support files.

## Package selection

A manifest may describe more than one bundle package. Current renderers render
one package per target plan. Select a package explicitly when needed:

```sh
agentbundler build --package team-skills
agentbundler check --target codex --package team-skills
```

Selectors must name manifest-declared targets or imported package IDs. Repeated
selectors are allowed but duplicate values fail.

Next: [customization](customization.md) or [targets and CLI](targets-and-cli.md).
