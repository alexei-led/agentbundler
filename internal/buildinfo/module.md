# Build Information

**Path**: `internal/buildinfo/` — compiler version lookup
**Parent**: repository root
**Submodules**: none (leaf)

## Purpose

This module exposes the build version used in CLI output and deterministic compiler provenance.

## Functional Responsibilities

- Return the injected release version when available.
- Provide the repository fallback version for local builds.

## Subdomain Classification

**Generic.** Build metadata is operational support and does not own compiler semantics.

## Public Contract

```text
Version() -> string
```

The value is consumed by the CLI and provenance generation. It does not participate in source import, composition, target rendering, artifact effects, or external process behavior.

## Integrations

- **Counterpart**: `cmd/agbun`
  - **Direction**: presents the version to users.
- **Counterpart**: `internal/compiler`
  - **Direction**: records the version in compiler-owned provenance.

## Constraints and Invariants

- No filesystem, network, clock, Git, or vendor-tool calls are made by this package.
- Version lookup remains side-effect free and has one stable fallback.

## Test Specification

- Injected release metadata wins over the fallback.
- Missing or malformed build metadata returns the documented fallback.
