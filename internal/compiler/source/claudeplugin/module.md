# Claude Plugin Source Importer

**Path**: `internal/compiler/source/claudeplugin/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module adopts one existing Claude Code plugin without moving or rewriting author-owned files. It translates verified Claude hook declarations into target-neutral typed hooks while preserving shell compatibility explicitly.

## Functional Responsibilities

- Parse one local `.claude-plugin/plugin.json` root and matching local marketplace entry.
- Import skills, agents, hooks, payload files, and recognized portable resources.
- Inventory unrecognized Claude-only components as native gaps.
- Preserve source bytes, executable intent, locations, target allow-lists, and ownership.

### Native Layout and Mapping

```text
agentbundle.json
.claude-plugin/plugin.json
.claude-plugin/marketplace.json
skills/<name>/SKILL.md
agents/<name>.md
hooks/hooks.json
<payload files referenced by hooks>
.agentbundler/assets/<kind>/<name>/...
```

Claude's default plugin hook file is `hooks/hooks.json`; `.claude-plugin/plugin.json#hooks` may instead contain an inline hook object or one or more contained `./`-prefixed plugin-relative paths. Declaring the manifest field replaces default-file discovery. The importer does not treat arbitrary undeclared `hooks/<name>.json` files as native hooks.

Known Claude command hooks map event, matcher, timeout, async flag, explicit decision semantics, and command to `HookDescriptor`. A portable tool category is adopted only when the matcher contains that category's complete Claude tool-name expansion; a partial expansion fails instead of widening on later target rendering. Native `command` plus `args` stays exec form. A statically unambiguous plugin-root payload argument, or a legacy interpreter plus one plugin-root script reference, becomes a package-file argument with imported payload. Other command strings remain explicit `shell` mode only when they contain no Claude path placeholder; the importer never pretends to parse arbitrary shell syntax into safe argv. HTTP, prompt, agent, MCP-tool, and unmodeled condition handlers remain native gaps until their portable semantics are approved.

The native contract was verified against <https://code.claude.com/docs/en/plugins-reference> and <https://code.claude.com/docs/en/hooks>, accessed 2026-07-15. Target output details are pinned in `docs/vendor-package-contracts.md`.

## Subdomain Classification

**Supporting.** Claude adoption is a topology boundary whose vendor schema changes independently.

## Encapsulated Knowledge

- Claude plugin, hook, and marketplace discovery rules.
- Claude event/matcher/command parsing at the adoption boundary.
- The distinction between provable package-file references and arbitrary shell.

## Public Contract

<!-- contract: SourceManifest, SourceInventory, SourceAsset, FileContent, HookDescriptor, CapabilityUse, NativeGap, Diagnostic — restated from internal/compiler/model/module.md -->

```text
inspect-claudeplugin(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

Every recognized hook produces exact semantic capabilities in addition to `asset.hook`. Unsupported handler kinds or unprovable semantics become explicit gaps or diagnostics; no hook is discarded.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: selected only for `kind: claude-plugin`.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: constructs target-neutral inventories, hooks, file content, capabilities, and gaps.

## Constraints and Invariants

- One contained local plugin root only; marketplace source must resolve to it.
- Unknown fields, duplicate keys, malformed UTF-8, source symlinks, and path escapes fail.
- Native source remains untouched. Generated Claude output is rebuilt only under configured output.
- Shell stays shell. Vendor root variables never enter normalized model values.
- A payload is imported only when its reference is statically provable and contained; no whole-tree copying is used to hide ambiguity.
- Existing hook-free adopted plugins remain compatible.

## Test Specification

- Official default and manifest-selected hook layouts import.
- Legacy shell, simple script, complex shell, invalid schema, missing payload, source mode, and no-source-write cases are covered.
- Round-trip capability tests prove arbitrary shell is never labeled exec and partial native matcher categories never widen.
