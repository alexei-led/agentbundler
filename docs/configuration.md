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
- `kind`: `skills-repository`, `bundle`, `claude-plugin`, or `agent-plugin`.
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
- Agent Plugin sources require `targets: ["agent-plugins"]`.

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
    "src/commands/resume-from.md",
    { "path": "src/agents/reviewer.md", "targets": ["claude", "pi"] }
  ]
}
```

Assets use paths such as `src/skills/<name>`, `src/agents/<name>.md`,
`src/commands/<name>.md`, `src/hooks/<name>`, `src/resources/<name>`, or
`src/plugins/<target>/<file-or-directory>`. The `src/` prefix is optional.
`targets` is an exact allow-list. Portable resources render under `resources/`;
target-native resources require an explicit native-resource declaration.

A portable command is user-invoked, unlike a lifecycle hook. Its Markdown file
name is the stable kebab-case command name and its frontmatter requires a
non-empty string `description`:

```markdown
---
description: Resume from a saved handoff.
---
Resume the session from the supplied handoff.
```

Flat command sidecars use `src/commands/<name>.md.agentbundler/`.
`asset.json` declares per-command capabilities and
`targets/<target>.json` patches frontmatter and body. Claude documents commands
as simple Markdown files, so the initial mapping rejects support files and file
patches instead of inventing
a payload layout. Use a portable resource or explicit native resource until a
target's command support-file contract is verified. Claude emits
`commands/<name>.md` for package profiles and
`.claude/commands/<name>.md` for project profiles. Codex, Pi, Copilot, Cursor,
Grok, and Antigravity currently reject `asset.command` with an explicit
capability diagnostic and produce no command artifact.

### Agent Plugin

Import one or more Agent Plugins 1.0.0 packages. Each declared plugin root
carries a `plugin.json` manifest, optional `mcp.json` servers, extension
namespaces, portable skills, and regular package files. The importer preserves
full semantic content: manifest fields, MCP transports, extension key-value
pairs, permitted unknown JSON, and links materialized as source locations.

```json
{
  "version": 1,
  "kind": "agent-plugin",
  "root": "source",
  "targets": ["agent-plugins"],
  "output": "generated",
  "agentPlugin": {
    "plugins": ["my-plugin"]
  }
}
```

`agentPlugin.plugins` lists one or more plugin-root directories relative to
`root`. Each directory must contain `plugin.json` and may contain `mcp.json`,
skills, extensions, and regular package files. The `agent-plugins` target is
the only supported target for this source kind.

Duplicate or case-fold-clashing plugin paths and plugin names are rejected.
External symlinks, cycles, and special files fail before traversal. Contained
symlinks are resolved and their bytes are materialized; the symlink path is
recorded as the source location for provenance.

Importer limits: 10,000 entries, 64 MiB per file, 256 MiB total, depth 64,
and 1,024 UTF-8 bytes per relative path.

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

The importer reads `.claude-plugin/plugin.json`, skills, direct agents,
portable top-level `commands/*.md` files with description frontmatter, hooks,
and known native gaps. Command files outside the portable subset remain native
gaps. Unsupported portable behavior reports a capability diagnostic rather
than guessing.

## Skills, hooks, commands, and native resources

A skill is reusable instruction content that the host may select. A hook reacts
to a lifecycle event and can run a typed command handler. A command is an
explicit slash-command entry point selected by the user. A native resource is
opaque target-owned configuration or code; use it only when no portable asset
kind represents the behavior.

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
