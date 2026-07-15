# Configuration and source formats

**Agent Bundler** reads one strict JSON file named `agentbundle.json`. Unknown
fields, duplicate keys, unsafe paths, and invalid target names fail validation.

## Manifest shape

The common fields are:

- `version`: use `1`. The current decoder also accepts omission.
- `kind`: `skills-repository`, `bundle`, or `claude-plugin`.
- `root`: source root, relative to the manifest directory.
- `targets`: one or more supported target IDs.
- `output`: dedicated generated-output directory.
- `distribution`: optional target-wide JSON metadata for generated catalogs.
- `composition`: optional target-wide policy, once per target at most. It may
  declare `packageMode` and an `aggregate` package.
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
    { "path": "src/agents/reviewer.md", "targets": ["claude", "codex", "pi"] },
    { "path": "src/resources/templates", "targets": ["claude", "codex", "pi"] }
  ]
}
```

Bundle package assets use exact forms such as `src/skills/<name>`,
`src/agents/<name>.md`, `src/resources/<name>`, `src/hooks/<name>.json`, and
`src/plugins/<target>/<file>`. The `src/` prefix is optional. Asset target
lists are exact allow-lists. Portable resource directories render under
`resources/` in package profiles; target-native resources remain explicit gaps.

Pi package metadata may include a `dependencies` object with package-name keys
and non-empty string versions. **Agent Bundler** writes it only to Pi
`package.json`; use it to ship runtime package prerequisites such as
`pi-subagents` alongside generated subagents.

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

## Distribution and package mode

`distribution` is target-wide metadata. It is not copied from an arbitrary
source package. Values must be JSON values. Catalog renderers define and validate
the fields they consume; no catalog is published or installed by a build.

Each target composition may set `packageMode`:

- `separate`: render independent package roots. This is the version-1 default
  when `packageMode` is omitted.
- `aggregate`: render one explicitly declared package. In version 1 this is
  valid only for target `pi` with profile `package`.

An `aggregate` object is valid only with `packageMode: "aggregate"`. It requires
both `identity` and `metadata`; `metadata` must be present even when it is `{}`.
Package dependency maps must contain non-empty string versions. Equal versions
may merge, while conflicting versions fail validation.

Separate package example:

```json
{
  "version": 1,
  "kind": "bundle",
  "root": "bundle",
  "targets": ["claude"],
  "output": "generated",
  "distribution": {
    "name": "Team tools",
    "owner": "platform"
  },
  "composition": [
    {
      "target": "claude",
      "profile": "package",
      "packageMode": "separate"
    }
  ],
  "bundle": { "packages": ["packages/team.json"] }
}
```

Explicit Pi aggregate example:

```json
{
  "version": 1,
  "kind": "bundle",
  "root": "bundle",
  "targets": ["pi"],
  "output": "generated",
  "distribution": {
    "name": "Team tools",
    "owner": "platform"
  },
  "composition": [
    {
      "target": "pi",
      "profile": "package",
      "packageMode": "aggregate",
      "aggregate": {
        "identity": "team-tools",
        "metadata": {
          "version": "1.0.0",
          "description": "Shared team tools"
        }
      }
    }
  ],
  "bundle": {
    "packages": ["packages/core.json", "packages/review.json"]
  }
}
```

Aggregate mode is explicit. It is never inferred from the number of packages.
The Pi aggregate renderer is added with Pi hook package support; until then the
configuration is validated but rendering reports that aggregation is not yet
implemented.

## Package selection

A manifest may describe more than one bundle package. Separate package mode
renders every selected package in one target plan. Select packages explicitly
when needed:

```sh
agbun build --package team-skills
agbun check --target codex --package team-skills
```

Selectors must name manifest-declared targets or imported package IDs. Repeated
selectors are allowed but duplicate values fail.

Next: [customization](customization.md) or [targets and CLI](targets-and-cli.md).
