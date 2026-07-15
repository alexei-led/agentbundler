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
`src/agents/<name>.md`, `src/resources/<name>`, `src/hooks/<name>`, and
`src/plugins/<target>/<file>`. The `src/` prefix is optional. Asset target
lists are exact allow-lists. Portable resource directories render under
`resources/` in package profiles; target-native resources remain explicit gaps.
The old exact `src/hooks/<name>.json` form remains compatible only for a
payload-free descriptor.

Pi package metadata may include a `dependencies` object with package-name keys
and non-empty string versions. **Agent Bundler** writes it only to Pi
`package.json`; use it to ship runtime package prerequisites such as
`pi-subagents` alongside generated subagents.

### Canonical command hook

A payload-owning hook is one directory. `hook.json` is strict JSON. Other
regular files in the directory are copied as hook payloads; walks are sorted,
contained, and symlink-free.

```text
src/hooks/command-guard/
├── hook.json
├── check.sh
├── rules.json
└── .agentbundler/
    ├── asset.json
    └── targets/copilot.json
```

Exact descriptor:

```json
{
  "event": "pre-tool",
  "matcher": { "tools": ["command"] },
  "handler": {
    "mode": "exec",
    "program": "bash",
    "arguments": [
      { "literal": "-eu" },
      { "packageFile": "check.sh" },
      { "packageFile": "rules.json" }
    ]
  },
  "timeoutMilliseconds": 5000,
  "asynchronous": false,
  "failurePolicy": "open",
  "order": 10
}
```

Use `exec` for canonical hooks. Literal and package-file arguments remain
separate until the target renders its own package-root syntax. Use `shell` only
when arbitrary shell syntax is intentional:

```json
{
  "event": "stop",
  "handler": { "mode": "shell", "shellCommand": "printf '%s\\n' done" },
  "timeoutMilliseconds": 1000,
  "asynchronous": false,
  "failurePolicy": "open",
  "order": 20
}
```

A hook that can deny or rewrite must declare that semantic capability in
`.agentbundler/asset.json`, for example:

```json
{ "capabilities": ["hook.decision.block"] }
```

Target-specific advisories require an exact acknowledgment. This example accepts
Copilot's safe exec-to-command-string conversion without changing the hook:

```json
{
  "acknowledgments": [
    {
      "asset": "hook/command-guard",
      "target": "copilot",
      "key": "hook.command.exec",
      "reason": "Copilot uses a quoted command string."
    }
  ]
}
```

Put it at
`src/hooks/command-guard/.agentbundler/targets/copilot.json`. Target allow-lists
on the package asset entry are the right way to exclude a hook from targets
that cannot preserve its semantics. See the complete six-target example in
`testdata/cc-thingz-hooks`.

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

Agents and command hooks are normalized during import and render only when the
selected target preserves their declared semantics. Unsupported event, matcher,
decision, timeout, async, shell, or failure behavior returns an exact capability
diagnostic. Unrecognized regular files in a Claude plugin remain Claude-native
gaps, not portable support files.

## Distribution and package mode

`distribution` is optional target-wide publication metadata. When it is present
on a separate package-profile build, Agent Bundler emits a deterministic local
catalog for Claude, Codex, Copilot, Cursor, and Grok. Pi emits no catalog.
Catalog generation never publishes, registers, installs, authenticates, fetches,
or changes vendor configuration.

Catalog distribution metadata is strict:

- `name`: lowercase kebab-case catalog ID;
- `owner`: non-empty string, or an object with non-empty `name` and optional
  valid `email`;
- `description`: non-empty trimmed text;
- `version`: semantic version.

No other distribution fields are accepted. Each package included in a catalog
must have lowercase kebab-case `id` and these metadata fields:

- `description` and semantic `version`;
- `author` as a non-empty string, or an object with `name` and optional `email`
  and HTTP(S) `url`;
- absolute HTTP(S) `homepage` and `repository` URLs without credentials;
- non-empty `license`;
- a non-empty array of unique string `keywords`.

Keywords and catalog entries are sorted. A single package is rendered at the
flat catalog source `.`; two or more packages use their package IDs as source
roots. Target serializers add the native `./` prefix or local-source object when
the vendor schema requires it. Duplicate identities, source roots, or generated
catalog paths fail instead of overwriting output.

Generated catalog paths are:

```text
claude  .claude-plugin/marketplace.json
codex   .agents/plugins/marketplace.json
copilot .github/plugin/marketplace.json
cursor  .cursor-plugin/marketplace.json
grok    .claude-plugin/marketplace.json
pi      no catalog
```

Each target composition may set `packageMode`:

- `separate`: render independent package roots. This is the version-1 default
  when `packageMode` is omitted.
- `aggregate`: render one explicitly declared package. In version 1 this is
  valid only for target `pi` with profile `package`.

An `aggregate` object is valid only with `packageMode: "aggregate"`. It requires
both `identity` and `metadata`; `metadata` must be present even when it is `{}`.
Package dependency maps must contain non-empty string versions. Equal versions
may merge, while conflicting versions fail validation.

Separate catalog example:

```json
{
  "version": 1,
  "kind": "bundle",
  "root": "bundle",
  "targets": ["claude"],
  "output": "generated",
  "distribution": {
    "name": "team-tools",
    "owner": {
      "name": "Platform Team",
      "email": "plugins@example.com"
    },
    "description": "Shared developer tools",
    "version": "2.0.0"
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

The referenced package file must provide publication metadata, for example:

```json
{
  "id": "team-tools",
  "metadata": {
    "description": "Shared developer workflows",
    "version": "1.2.3",
    "author": "Platform Team",
    "homepage": "https://example.com/team-tools",
    "repository": "https://github.com/example/team-tools",
    "license": "MIT",
    "keywords": ["agents", "tools"]
  },
  "assets": ["src/skills/explain-query"]
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

Aggregate mode is explicit and never inferred from package count. The Pi
aggregate renderer emits one installable package root and no marketplace file.

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
