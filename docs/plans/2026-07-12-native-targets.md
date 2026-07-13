# Native Target Output

## Decision

Agentbundler will emit vendor-native output. The first release is a **lossless supported subset**:

- Render only asset kinds and capability uses whose native representation is documented and represented by the normalized model.
- Reject unsupported or insufficiently modeled assets. Do not retain agentbundler sidecars in a native target tree.
- Reject multiple source packages per target until the vendor's native aggregation contract is implemented.
- Codex and Cursor output plugin roots. Claude, Copilot, Pi, and Grok output project skill roots for the initial skill-only slice.
- The target-neutral `packages/`, `asset.json`, `content.md`, and `package-index.json` format is removed only as each adapter gains its native replacement.

## Evidence

| Target  | Initial native layout                                                                        | Evidence                                              |
| ------- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Claude  | `.claude/skills/<skill>/SKILL.md`                                                            | Claude Code Skills documentation                      |
| Codex   | `.codex-plugin/plugin.json`, `skills/`, and only separately modeled native plugin components | `cc-thingz/dist/codex/plugins/*` working distribution |
| Copilot | `.github/skills/<skill>/SKILL.md`                                                            | GitHub Copilot agent-skills documentation             |
| Cursor  | `.cursor-plugin/plugin.json`, `skills/`, `agents/`, `hooks/` when modelled                   | `cursor/plugins` official examples and schema         |
| Grok    | `.grok/skills/<skill>/SKILL.md`                                                              | Grok Build Skills documentation                       |
| Pi      | `.pi/skills/<skill>/SKILL.md`                                                                | Pi Skills documentation                               |

## Contracts

1. Every adapter validates the normalized package and rejects more than one package before rendering.
2. The initial skill renderer emits the source frontmatter, Markdown body, and support files without transformation beyond deterministic serialization of frontmatter.
3. Target-specific metadata is required where its native plugin manifest requires it. It is not synthesized from opaque package metadata without validation.
4. Agent, hook, and native-resource assets remain unsupported until their target-native schemas and model representation are implemented. Cursor and Codex are the exceptions only after their plugin component schemas are added.
5. Each adapter has a deterministic golden-tree test, an unsupported-subset test, and a duplicate/path-collision test. Tests validate public planned files, not private helpers.

## Delivery Order

1. Define shared native-skill serialization and package/metadata validation at an acyclic target submodule boundary.
2. Implement native skill layouts for Claude, Copilot, Pi, and Grok.
3. Implement Codex and Cursor plugin manifests and skills, then add agents/hooks only after their source schemas are modeled.
4. Replace target-neutral documentation and baseline tests with native-contract fixtures.
5. Run build/check/drift, full test, race, and module-alignment gates.

## Deferred

- Multi-package vendor aggregation.
- Vendor-native resources, MCP, LSP, extensions, and themes.
- Hook configuration from the generic `asset.hook` model.
- Native validation commands where a supported local validator is not available.
