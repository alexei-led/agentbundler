# Antigravity Target Adapter

**Path**: `internal/target/antigravity/`
**Parent**: `internal/target`

## Purpose

Render package-profile, separate-mode output as Antigravity CLI plugin roots at format revision 1.

## Contract

The adapter emits a strict root `plugin.json` containing only `name` and an optional string `description`. It emits portable skills under `skills/`, agents under `agents/` only for the verified `name`/`description` frontmatter subset, portable resources under `resources/`, and exact explicit native-resource files at their plugin-root paths.

Portable hooks and every `hook.*` capability are unsupported. Raw target-native `hooks.json`, `mcp_config.json`, rules, and support files may pass through only as explicit native resources. Pi extension declarations are rejected. The adapter has no catalog and no manifest version field.

Every successfully rendered plugin root declares the non-mutating local check `agy plugin validate .`. Rendering does not invoke the CLI, install plugins, or read user configuration.
