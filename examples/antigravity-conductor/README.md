# Conductor-shaped Antigravity CLI plugin

This example compiles one portable skill and one Antigravity-only rule into a
native Antigravity CLI plugin. It demonstrates the shape of
[Conductor 0.3.0][conductor] without copying its implementation or importing an
arbitrary Antigravity plugin repository.

The source was informed by Conductor at pinned commit
[`fb6212e8faee3f9ecb69f0ee19bd5b2a0765bb0a`][conductor-pinned]. The skill and
rule in this example are original, minimal demonstration content.

## Build and check

Run these commands from this example directory:

```sh
cd examples/antigravity-conductor
go run ../../cmd/agbun build --root .
go run ../../cmd/agbun check --root .
agy plugin validate generated/antigravity
```

The generated plugin is flat because the source has one package:

```text
generated/antigravity/
├── plugin.json
├── README.md
├── skills/conductor-setup/SKILL.md
└── rules/conductor_antigravity.md
```

Validation checks plugin structure. It does not install the plugin or sandbox
native files.

## Optional local installation

Installation is an explicit user action outside Agent Bundler. After reviewing
the generated files, run:

```sh
agy plugin install "$(pwd)/generated/antigravity"
```

Agent Bundler never installs, links, enables, disables, or uninstalls the
plugin.

## Boundaries

- Portable skills are compiled to `skills/`; Antigravity discovers their slash
  commands. Agent Bundler does not generate legacy `commands/`.
- This example preserves `rules/conductor_antigravity.md` as an explicit,
  target-native resource. Agent Bundler does not interpret its behavior.
- Portable hooks are unsupported. Explicit native `hooks.json`, MCP
  configuration, and scripts can be preserved, but remain trusted native input.
- Antigravity plugin manifests have no generated version or marketplace
  catalog in the verified contract.
- Direct import of an arbitrary Antigravity plugin repository is not supported.

[conductor]: https://github.com/gemini-cli-extensions/conductor
[conductor-pinned]: https://github.com/gemini-cli-extensions/conductor/tree/fb6212e8faee3f9ecb69f0ee19bd5b2a0765bb0a
