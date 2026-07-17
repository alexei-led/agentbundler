# Agent Bundler documentation

Start here if you already know the project and need the right page.

- Understand the problem and compiler model: [User guide](guide.md)
- Install or update the CLI: [Install](install.md)
- Build your first bundle: [Quick start](quickstart.md)
- Define the manifest and source kind: [Configuration](configuration.md)
- Make one target differ: [Customization](customization.md)
- See output paths, flags, and exits: [Targets and CLI](targets-and-cli.md)
- Diagnose failures or stale output: [Troubleshooting](troubleshooting.md)
- Understand packages and the pipeline: [Architecture](architecture.md)
- Run and interpret the release gates: [Release validation](release.md)
- Review pinned vendor paths and limits: [Vendor package contracts](vendor-package-contracts.md)
- Build an Antigravity CLI plugin: [Conductor-shaped example](../examples/antigravity-conductor/README.md)

## The short version

**Agent Bundler** imports a canonical source, applies target-specific overlays,
and renders complete target trees. `build` writes them. `check` compares them. Keep
`agentbundle.json`, source files, and sidecars in version control; keep the
configured output directory separate and disposable.

Current renderers support skills, portable resources, selected native agent
forms, typed command hooks with payloads, explicit Pi aggregation, and
vendor-specific catalogs where supported. Antigravity CLI is package-only: it
has no generated catalog, accepts a narrow portable-agent subset, rejects
portable hooks, and can preserve explicit native resources without interpreting
them. See the [portable hook cells](targets-and-cli.md#portable-hook-cells)
before relying on blocking, rewrite, async, or failure semantics across targets.
