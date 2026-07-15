# Marketplace Module

**Path**: `internal/target/marketplace/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Responsibility

Build deterministic, target-neutral marketplace catalog data from explicit distribution and package publication metadata.

## Inputs

- one separate-mode `model.TargetRenderInput`;
- non-nil distribution metadata with kebab-case `name`, `owner`, `description`, and semantic `version`;
- `owner` as a non-empty string or an object with `name` and optional `email`;
- per-package `description`, semantic `version`, `author`, `homepage`, `repository`, `license`, and non-empty `keywords`.

## Outputs

- catalog metadata;
- package entries ordered by package ID;
- source roots expressed as `.` for one flat package or the package ID for multiple packages;
- diagnostics for missing or malformed metadata and identity/path collisions.

## Invariants

- Distribution and package IDs used in catalogs are lowercase kebab-case.
- Distribution metadata rejects missing, unknown, mistyped, or malformed fields.
- Package versions are semantic versions.
- Homepage and repository values are absolute HTTP(S) URLs without credentials.
- Keywords are unique and sorted.
- The builder does not read the filesystem, environment, process state, clock, Git state, or network.
- The builder does not serialize a vendor schema. Target leaves own catalog JSON paths and shapes.
- Aggregate mode is rejected. Pi aggregation does not use marketplace output.

## Dependencies

- `internal/compiler/model`
- Go standard library
