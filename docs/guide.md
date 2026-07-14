# User guide

**Agent Bundler** exists for a boring reason: copying the same coding-agent skill
into six vendor-specific trees is easy to start and painful to maintain.

`SKILL.md` gives us a useful shared format for instructions, but it does not
settle the rest of the package contract. Agents disagree about frontmatter,
plugin manifests, extension directories, support files, and how to describe
hooks, scripts, or agents. Model behavior differs too. A prompt tuned for one
model family can be weak or noisy for another.

**Agent Bundler** treats the portable source as the canonical version and the agent
layouts as build artifacts:

```text
source + agentbundle.json
          │
          ▼
 import → compose → render → build/check
          │
          └── target-ready directories + build metadata
```

## The useful mental model

- **Source** is what you maintain: skills, Markdown, support files, metadata,
  and optional sidecars.
- **Overlay** is a small, target-scoped difference: change frontmatter, replace
  a section, or add/delete a file for Pi without forking the skill.
- **Composition** is target-wide policy: a preamble, capability classification,
  acknowledgment, or native-gap decision.
- **Target output** is a target-specific skill tree that you can hand to an
  agent project or package in a plugin repository. It is not a complete vendor
  plugin unless that target renderer emits the required manifest and files.
- **Provenance** records input and output hashes so `check` can find drift.

The output is compiler-owned. Keep it in a dedicated directory. Do not edit it
by hand or point `output` at a project root containing unrelated agent files.

## What Agent Bundler does today

The current adapters render skills, portable package resources, and supported
native agent forms into target-native project or installable package layouts.
Installable profiles can render multiple self-contained packages under
package-owned roots. They do not install an agent, run a model, publish a
marketplace package, or promise that every target supports every frontmatter key.

The source model can recognize richer assets and native gaps. The current target
renderers still reject hooks, scripts, and target-native resources. Unsupported
capabilities fail explicitly in the compiler.

## Choose your next page

- New project: [Quick start](quickstart.md)
- Existing source: [Configuration and source formats](configuration.md)
- Target-specific instructions: [Customization](customization.md)
- Generated paths and vendor behavior: [Targets and CLI](targets-and-cli.md)
- Build failures or drift: [Troubleshooting](troubleshooting.md)
- Implementation and extension points: [Architecture](architecture.md)
