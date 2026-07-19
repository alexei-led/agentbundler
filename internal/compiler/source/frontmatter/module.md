# Skills Frontmatter Parser

**Path**: `internal/compiler/source/frontmatter/` — bounded YAML frontmatter parsing
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module parses the YAML frontmatter used by adopted Agent Skills documents. It is a source-import helper, not a general configuration parser.

## Functional Responsibilities

- Detect and parse bounded frontmatter blocks.
- Decode scalar and supported structured fields into importer-owned values.
- Reject malformed YAML, invalid UTF-8, and unsupported values with source diagnostics.

## Subdomain Classification

**Core.** Frontmatter interpretation affects source adoption and normalized asset metadata.

## Public Contract

```text
parse(bytes) -> Metadata + body + error
```

The parser returns metadata only. It does not traverse files, select source kinds, compose assets, render targets, or perform filesystem/process/network behavior.

## Integrations

- **Counterpart**: `internal/compiler/source` importers
  - **Direction**: bundle, Claude plugin, and skills-repository importers use the parser for Markdown metadata.
  - **Shared knowledge**: frontmatter bytes and importer-facing metadata values only.

## Constraints and Invariants

- Parsing is deterministic and independent of workspace paths, clock, environment, and vendor tools.
- Unknown or malformed frontmatter does not silently become asset content.
- The parser remains target-neutral and has no dependency on composition, target, artifact, or compatibility packages.

## Test Specification

- Valid scalar and structured frontmatter parses consistently.
- Missing delimiters, malformed YAML, invalid UTF-8, and unsupported values fail deterministically.
