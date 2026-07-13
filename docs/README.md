# Agentbundler documentation

Start here if you already know the project and need the right page.

- Understand the problem and compiler model: [User guide](guide.md)
- Install or update the CLI: [Install](install.md)
- Build your first bundle: [Quick start](quickstart.md)
- Define the manifest and source kind: [Configuration](configuration.md)
- Make one target differ: [Customization](customization.md)
- See output paths, flags, and exits: [Targets and CLI](targets-and-cli.md)
- Diagnose failures or stale output: [Troubleshooting](troubleshooting.md)
- Understand packages and the pipeline: [Architecture](architecture.md)

## The short version

Agentbundler imports a canonical source, applies target-specific overlays, and
renders complete target trees. `build` writes them. `check` compares them. Keep
`agentbundle.json`, source files, and sidecars in version control; keep the
configured output directory separate and disposable.

Current renderers support one package of skills and support files. See the
[limitations](targets-and-cli.md#current-limitations) before designing a
bundle around agents, hooks, scripts, or native plugin resources.
