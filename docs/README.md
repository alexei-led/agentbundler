# Agent Bundler documentation

Human-facing starting points:

- [Guide](guide.md): source-to-target model.
- [Install](install.md): Homebrew, Go, and checkout installs.
- [Quick start](quickstart.md): first bundle.
- [Configuration](configuration.md): manifest and source formats.
- [Customization](customization.md): overlays, composition, and native gaps.
- [Targets and CLI](targets-and-cli.md): output paths, capabilities, commands,
  diagnostics, and vendor links.
- [Repository-root compatibility](repository-root-compatibility.md): opt-in root
  discovery files, ownership, and drift.
- [Troubleshooting](troubleshooting.md): common failures.
- [Architecture](architecture.md): compiler boundaries.
- [Release validation](release.md): checks and tag procedure.
- [Target contracts](vendor-package-contracts.md): vendor assumptions.

Keep `agentbundle.json` and source files in version control. Keep `output`
dedicated and disposable. `build` replaces it; `check` is read-only.
