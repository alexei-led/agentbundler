# Configuration

Agent Bundler reads strict JSON from `agentbundle.json`. Unknown or duplicate
fields, invalid target IDs, unsafe paths, and symlinks in source/sidecar paths
fail. Paths are normalized relative paths: no absolute paths, `..`, backslashes,
empty components, or NUL bytes.

## Manifest

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

Common fields:

- `version`: `1`; omission remains accepted.
- `kind`: `skills-repository`, `bundle`, or `claude-plugin`.
- `root`: source root relative to the manifest.
- `targets`: one or more of `antigravity`, `claude`, `codex`, `pi`, `copilot`,
  `grok`, or `cursor`.
- `output`: dedicated generated directory. `build` replaces all of it.
- `distribution`: optional catalog metadata for compatible separate package
  targets.
- `composition`: optional target package/profile policy.
- `compatibility`: optional root discovery configuration; see
  [repository-root compatibility](repository-root-compatibility.md).
- Exactly one source block matching `kind`.

## Source kinds

### Skills repository

Use ordinary skill folders. Agent Bundler finds `SKILL.md` recursively below
`skillsRepository.roots`; each containing directory is one skill. Other files
in that directory are support files. Duplicate skill names fail.

```text
source/
├── skills/explain-query/SKILL.md
└── .agentbundler/assets/skill/explain-query/targets/pi.json
```

### Bundle

Use explicit packages or asset membership. Package assets are exact paths, not
globs.

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

```json
{
  "id": "team-skills",
  "metadata": {},
  "assets": [
    "src/skills/explain-query",
    { "path": "src/agents/reviewer.md", "targets": ["claude", "pi"] }
  ]
}
```

Assets use paths such as `src/skills/<name>`, `src/agents/<name>.md`,
`src/hooks/<name>`, `src/resources/<name>`, or
`src/plugins/<target>/<file-or-directory>`. The `src/` prefix is optional.
`targets` is an exact allow-list. Portable resources render under `resources/`;
target-native resources require an explicit native-resource declaration.

### Claude plugin

Use an existing plugin as source:

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

The importer reads `.claude-plugin/plugin.json`, skills, direct agents/hooks,
and known native gaps. Unsupported portable behavior reports a capability
diagnostic rather than guessing.

## Native resources and hooks

A Pi native extension tree needs `.agentbundler/asset.json`:

```json
{
  "capabilities": ["asset.native-resource"],
  "piExtensions": ["extensions/team-tools.ts"]
}
```

Agent Bundler copies the tree and registers only listed contained
`extensions/*.ts` or `extensions/*.js` entries. It does not infer entrypoints or
install dependencies.

An Antigravity native resource uses the same capability declaration, but the
bundle asset must use exactly `"targets": ["antigravity"]`. Its contents are
trusted raw vendor configuration or code.

A portable hook is a directory with strict `hook.json`; remaining regular files
are its payload. Use `exec` with literal and `packageFile` arguments unless
shell syntax is intentional. Decision or rewrite hooks require the matching
capability in `.agentbundler/asset.json`. Target-specific capability loss needs
an acknowledgment sidecar. See `testdata/cc-thingz-hooks` for the complete
seven-target example.

Pi package `metadata.dependencies` is copied exactly. Agents remain declared in
`pi.subagents.agents`; Agent Bundler never infers `pi-subagents` or other
third-party runtime dependencies.

## Composition and distribution

`packageMode` defaults to `separate`. `aggregate` is explicit and currently
valid only for Pi package output. It requires an `identity` and `metadata`.
Equal explicit dependency versions merge; conflicts fail.

`distribution` creates a local deterministic catalog for Claude, Codex,
Copilot, Cursor, and Grok in separate package mode. It never publishes or
changes vendor configuration. Catalog metadata requires a kebab-case `name`,
owner, description, and semantic version. Package publication metadata requires
description, semantic version, author, HTTP(S) homepage/repository, license,
and keywords.

Antigravity always uses separate package output. Pi and Antigravity have no
catalog.

## Selection

Select declared targets or bundle package IDs when needed:

```sh
agbun build --package team-skills
agbun check --target codex --package team-skills
```

Root compatibility requires every configured compatibility target and all
packages; `--package` is rejected for that mode.

Next: [customization](customization.md) or
[targets and CLI](targets-and-cli.md).
