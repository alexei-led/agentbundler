# Skills Repository Source Importer

**Path**: `internal/compiler/source/skillrepo/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module adopts an existing Agent Skills collection from explicit roots without moving source files or guessing package groups.

## Functional Responsibilities

- Discover `SKILL.md` assets below manifest-declared roots in deterministic order.
- Return one source package with exact bodies, frontmatter, support-file bytes, executable intent, origins, overlays, and input hashes.
- Reject ambiguous identities, unsafe traversal, and malformed sidecars.

## Subdomain Classification

**Supporting.** Generic skills adoption is narrower and less volatile than canonical bundle import.

## Public Contract

<!-- contract: SourceManifest, SourceInventory, FileContent, Diagnostic — restated from internal/compiler/model/module.md -->

```text
inspect-skillrepo(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

`skillsRepository` declares one package identity, metadata, and explicit roots. Each directory containing `SKILL.md` is one `skill/<basename>` asset. Every regular support file below it, excluding `.agentbundler/`, becomes `FileContent` with exact bytes, source origin, and observed executable intent. Target overlay file replacements use the same executable-aware `FileContent` shape.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: selected only for `kind: skills-repository`.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: constructs model-owned inventory and file values.

## Constraints and Invariants

- Roots and one package identity are explicit; discovery never creates package groups.
- Walks are sorted, contained, and symlink-free. Duplicate identities and paths fail with source locations.
- Executable intent is preserved even though skills usually do not execute support files.
- This importer creates no hooks from opaque files and performs no composition, rendering, artifact action, process, network, publication, or installation.
- Hook/distribution/package-mode optional fields do not change existing hook-free version-1 skills manifests.

## Test Specification

- Listed roots only are imported into one deterministic package.
- Duplicate identity, empty roots, escapes, symlinks, malformed frontmatter, and sidecar collisions fail.
- Support-file bytes, origins, input hashes, and executable intent survive import unchanged.
