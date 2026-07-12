# Build Provenance

**Path**: `internal/artifact/provenance/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module adds deterministic evidence of how output was formed. Without it, users cannot explain which compiler, adapter revisions, inputs, acknowledgments, and output hashes produced a generated tree without introducing a misleading dependency lockfile.

## Functional Responsibilities

- Hash normalized configuration, inputs, and planned output.
- Record compiler version and adapter format revisions.
- Record explicit advisory acknowledgments and output executable intent.
- Create one provenance planned file outside native package roots.

## Subdomain Classification

**Supporting.** Traceability is important for trust and drift diagnosis but does not define target semantics. Volatility is moderate.

## Encapsulated Knowledge

- Provenance schema and canonical serialization.
- Inclusion and exclusion lists for deterministic evidence.
- Hash ordering and path normalization.
- The distinction between provenance and a dependency lockfile.

## Public Contract

<!-- contract: RelativePath, PackageID, ByteSequence, SourceLocation, TargetID, PlannedFile, NativeCheck, TargetPlan, BuildPlan — restated from internal/compiler/model/module.md (subset: minimal recursively closed contract) -->
```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Int?, column: Int? }
TargetID = claude | codex | pi | copilot | grok | cursor
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan] }
```

```text
append-provenance(BuildPlan, compiler-version) -> BuildPlan
```

The returned plan includes one compiler-owned provenance file at `<output-root>/.agentbundler/build.json`, outside native target package roots. It records no timestamp, absolute path, host identity, Git commit, network result, or self-hash.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent delegates plan augmentation.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/artifact` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: normative `append-provenance` operation above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module reads and augments build plans.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: restated build-plan contract above.

## Change Vectors

- Add a schema version or adapter revision field.
- Improve diagnostic provenance for acknowledgments.
- Change hash algorithm only through an explicit schema revision.

## Constraints and Invariants

- Provenance must be reproducible from identical inputs and compiler revision.
- `.agentbundler/build.json` is a reserved path relative to the configured output root; no literal `dist` path is assumed.
- It is not an Agentbundler dependency lockfile and does not resolve packages.
- Its own hash is omitted to avoid recursion.
- It cannot include secrets from source or environment.

## Test Specification

### Unit Tests

- **Test name**: identical plan has identical provenance.
  - **Scenario**: append provenance twice from equivalent inputs.
  - **Expected behavior**: bytes match exactly.
- **Test name**: nondeterministic fields excluded.
  - **Scenario**: vary time, cwd, hostname, and Git state.
  - **Expected behavior**: provenance bytes are unchanged.

### Integration Contract Tests

- **Test name**: provenance is outside native roots.
  - **Scenario**: augment a multi-target plan.
  - **Expected behavior**: native package file lists contain no compiler provenance path.

### Boundary Tests

- **Test name**: provenance cannot self-hash.
  - **Scenario**: enumerate output hashes during augmentation.
  - **Expected behavior**: provenance entry is absent from its own hash list.

### Behavior Tests

- **Test name**: acknowledgment evidence is retained.
  - **Scenario**: plan contains an accepted advisory capability.
  - **Expected behavior**: provenance records asset, target, capability, and reason.
