# Multi-package distributions

Status: package profiles implement the first stage. Hooks and executable native
resources remain deferred.

## Contract

A target package profile receives one or more normalized packages. Each package
is self-contained. With one package, existing flat target paths remain stable.
With multiple packages, every package owns a directory named by its validated ID:

```text
<target>/<package-id>/{manifest,README.md,skills,agents,resources}
```

Package order is not significant. The compiler sorts packages and files by stable
identity before rendering. Duplicate package IDs and output paths fail before write;
artifact validation also rejects case-folded collisions and reserved paths.

## Ownership

- Core owns package selection, composition, capability checks, provenance, and
  output collision validation.
- Target adapters own manifests, file names, and vendor-specific metadata.
- The source repository owns marketplaces, publication, credentials, install policy,
  and vendor smoke tests.
- Output is compiler-owned and must use a dedicated directory.

## Deferred generic capabilities

- Structured hooks with explicit per-target event mappings.
- Executable/native resource metadata and Pi extension package registration.
- Package dependency DAGs and native dependency semantics.
- Target marketplace/channel manifests.

These require synthetic target-neutral tests before any repository cutover. Do not
silently drop security, sandbox, permission, executable, or dependency semantics.
