# Targets and CLI reference

**Agent Bundler** writes a complete target subtree under `output/<target>/`. The
paths below are target-relative.

## Target layouts

Project profiles keep the lightweight roots used by each agent. Package profiles
produce installable target roots:

- **Claude Code:** project `.claude/skills/<name>/`; package
  `.claude-plugin/plugin.json`, `skills/`, `agents/`, and `resources/`.
- **Codex:** package `.codex-plugin/plugin.json`, `skills/`, standalone
  `agents/*.toml`, and `resources/`.
- **Pi:** project `.pi/skills/<name>/`; package `package.json`, `skills/`,
  `resources/`, and—when agent assets are selected—`agents/`. **Agent Bundler**
  registers package agents through `pi.subagents.agents`, which requires
  [`pi-subagents`](https://github.com/nicobailon/pi-subagents).
- **GitHub Copilot:** `.github/skills/<name>/` and declared portable resources
  under `.github/resources/<name>/` in the project profile.
- **Grok Build:** `.grok/skills/<name>/` and declared portable resources under
  `.grok/resources/<name>/` in the project profile.
- **Cursor:** package `.cursor-plugin/plugin.json`, `skills/`, and `resources/`.

Every skill output contains `SKILL.md` plus composed regular support files. When
frontmatter exists, the renderer writes it as compact JSON between `---` lines.
Package resources are portable directory trees under `resources/<name>/`.
**Agent Bundler** accepts ordinary YAML frontmatter and normalizes it to JSON for
output; `agentbundle.json` itself remains strict JSON.

The target directory is a layout contract, not proof of vendor feature parity.
Copy or package the matching target subtree according to the target's current
runtime documentation.

## Vendor documentation

- [Claude Code plugins](https://code.claude.com/docs/en/plugins) and
  [skills](https://code.claude.com/docs/en/skills)
- [Codex skills](https://developers.openai.com/codex/skills)
- [Pi skills](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md),
  [packages](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/packages.md),
  [extensions](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md),
  and [pi-subagents](https://github.com/nicobailon/pi-subagents)
- [GitHub Copilot agent skills](https://docs.github.com/copilot/concepts/agents/about-agent-skills)
- [Grok Build skills, plugins, and marketplaces](https://docs.x.ai/build/features/skills-plugins-marketplaces)
- [Cursor rules](https://docs.cursor.com/en/context/rules) and
  [Agent Skills release notes](https://cursor.com/changelog/2-4)
- [Agent Skills specification](https://agentskills.io/specification)

These runtimes change independently of **Agent Bundler**. Check their docs before
adding vendor-specific frontmatter or packaging assumptions.

## Help

Ask the CLI for the command or option details before scripting against it:

```sh
agbun help
agbun help build
agbun help check
```

`-h` and `--help` also work at the top level and after `build` or `check`.
Help exits `0` and does not require a manifest.

## Commands

```text
agbun build [--root DIR] [--target TARGET]... [--package PACKAGE]... [--json]
agbun check [--root DIR] [--target TARGET]...
  [--package PACKAGE]... [--native] [--json]
```

Examples:

```sh
agbun build
agbun check
# WARNING: build replaces the complete output directory, even for one target.
# Keep output in a dedicated generated directory.
agbun build --target pi
agbun check --target codex --package team-skills
agbun check --json
```

`--root` points to the directory containing `agentbundle.json`. Without it,
**Agent Bundler** searches the current directory and its parents. `--target` and
`--package` may be repeated; selectors must be declared and unique. Current
renderers require exactly one selected package per target plan. Use one package
selector when building a target; multiple distinct packages are not aggregated.

## Output ownership

`build` stages and replaces the complete configured output directory. `check`
compares the expected plan without writing. Neither command uses the network.

`generated/.agentbundler/build.json` records the configuration digest, input and
output hashes, acknowledgments, and output file details. It is compiler metadata,
not an input file.

## Machine-readable results

With a valid manifest, `--json` writes one result object to stdout. Diagnostics
are normally written to stderr. The result contains:

- `version`
- `command`
- `diagnostics`
- `drift`
- `nativeVerificationFailed`

Manifest-discovery failures occur before the result object is created and are
reported as ordinary diagnostics.

## Exit statuses

- `0`: success; `check` found current output.
- `1`: source, validation, capability, render, or write failure.
- `2`: output drift: missing, changed, extra, non-regular, or symlinked entries.
- `3`: native verification failure.

`--native` is valid only with `check`. Built-in target adapters currently
declare no native checks, so it adds no checks today.

## Current limitations

Current target renderers intentionally support:

- one package per target plan;
- `skill` assets and portable `resource` directory trees, including sibling
  project resources for Copilot and Grok;
- Claude Markdown agents, Codex standalone TOML agents, and Pi subagent
  Markdown agents in package profiles;
- regular support files;
- YAML frontmatter with JSON-compatible values and Markdown bodies;
- target overlays for frontmatter, heading blocks, files, and deletions.

They do not currently render:

- hook assets;
- scripts as a separate portable asset type;
- target-native resources;
- arbitrary custom capability uses;
- multi-package aggregation;
- full vendor plugin or extension manifests beyond the generated Codex and
  Cursor skill manifests.
