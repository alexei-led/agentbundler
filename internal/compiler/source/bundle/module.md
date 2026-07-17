# Bundle Source Importer

**Path**: `internal/compiler/source/bundle/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module imports Agent Bundler's canonical owned-source layout, including first-class portable hook directories and their payload files.

## Functional Responsibilities

- Strictly decode `agentbundle.json`, package manifests, hook descriptors, and sidecars.
- Build package membership from exact manifest entries.
- Import skills, agents, resources, hooks, and target-native resources.
- Capture payload bytes, executable intent, source origins, target allow-lists, capabilities, and input hashes.

### Canonical Layout and Schema

```text
agentbundle.json
packages/<package>.json
src/skills/<name>/SKILL.md
src/agents/<name>.md
src/resources/<name>/...
src/hooks/<hook-id>/hook.json
src/hooks/<hook-id>/<payload files>
src/hooks/<hook-id>/.agentbundler/targets/<target>.json
src/hooks/<legacy-name>.json
src/plugins/<target>/<name>/...
<asset-directory>/.agentbundler/asset.json
<asset-directory>/.agentbundler/targets/<target>.json
<asset-directory>/.agentbundler/targets/<target>/files/...
```

`bundle.packages` lists exact package-manifest paths. A package manifest remains `{ "id": String, "metadata": Object, "assets": [String | AssetEntry] }`, where `AssetEntry` is `{ "path": RelativePath, "targets": [TargetID]? }`. Hook entries normally identify `src/hooks/<hook-id>/`; the exact `src/hooks/<name>.json` compatibility form is accepted only for descriptor-only hooks with no payload files.

Each canonical hook directory contains exactly one strict `hook.json` using the model-owned JSON fields:

```json
{
  "event": "pre-tool",
  "matcher": { "tools": ["command"] },
  "handler": {
    "mode": "exec",
    "program": "bash",
    "arguments": [{ "literal": "-eu" }, { "packageFile": "hook.sh" }]
  },
  "timeoutMilliseconds": 10000,
  "asynchronous": false,
  "failurePolicy": "closed",
  "order": 100
}
```

Unknown or duplicate fields fail. Every other file below the hook directory, excluding `.agentbundler` sidecars, is owned payload. Payload walks are sorted, contained, and symlink-free. File modes become `FileContent.executable`; package-file arguments must resolve to imported files in that same hook.

Target sidecars use the shared overlay contract. A JSON file patch is either a string shorthand for non-executable UTF-8 bytes or a strict object containing exactly one of `text` or `base64` plus optional `executable`; its origin is the JSON sidecar. A filesystem patch replaces the complete JSON `FileContent` at the same path, observes executable mode, and uses the tree file as origin. Target sidecars may patch content and acknowledgments but may not replace the portable hook descriptor with vendor-private schema.

A native-resource path may be one file or one directory. Its target is the exact `src/plugins/<target>/<name>` path segment; it is never inferred from filenames. Antigravity package entries must declare an exact `antigravity`-only target allow-list. Earlier target-native entries retain the version-1 string shorthand; the importer records their path target so composition excludes them from other targets. The resource's `.agentbundler/asset.json` still requires `capabilities`. Antigravity trees must explicitly declare `asset.native-resource` and copy the complete contained, symlink-free tree without interpreting rules, MCP configuration, hooks, or support files. Pi extension trees additionally declare `piExtensions`, a sorted list of contained `extensions/*.ts` or `extensions/*.js` entry paths. The importer copies complete trees, including helper modules, but never infers entries or claims direct Antigravity plugin-repository import.

## Subdomain Classification

**Core.** Canonical authoring and portable hook semantics are high-volatility product behavior.

## Encapsulated Knowledge

- Canonical path conventions and strict JSON spellings.
- Explicit package membership and target allow-lists.
- Hook payload ownership, source-mode capture, and sidecar containment.

## Public Contract

<!-- contract: SourceManifest, SourceInventory, SourceAsset, FileContent, HookDescriptor, CapabilityUse, Diagnostic — restated from internal/compiler/model/module.md -->

```text
inspect-bundle(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

A hook directory maps to exactly one hook-kind `SourceAsset` with one `HookDescriptor`, deterministic payload `FileContent` values, semantic capability uses, and source input hashes. The importer adds `asset.hook` and exact uses for command form, event, matcher, decision behavior, async, and closed failure policy. It does not translate any vendor event or command-root variable.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: selected only for `kind: bundle`.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: constructs source inventory values and no look-alike types.

## Constraints and Invariants

- Package manifests, never discovery or globs, define membership.
- `hook.json` is not a payload and cannot be referenced by `packageFile`.
- Legacy exact JSON hooks cannot own sibling payloads.
- Source and sidecar symlinks, escapes, duplicate paths, duplicate identities, and unknown fields fail.
- Interpreter invocation does not imply or require executable mode; observed executable intent is still preserved.
- A target resource cannot overwrite an adapter-owned generated file.
- Pi extension entries and Antigravity native trees are explicit target-owned resources, not inferred from filenames or imports.
- The importer performs no target serialization, process execution, publication, installation, or network access.

## Test Specification

- Canonical and legacy hook forms import deterministically.
- Invalid descriptor combinations, missing package files, traversal, symlinks, duplicate fields, and mode conflicts fail with source locations.
- POSIX fixtures preserve executable intent while non-executable interpreter-backed scripts remain valid.
