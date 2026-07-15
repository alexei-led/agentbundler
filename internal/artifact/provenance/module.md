# Build Provenance

**Path**: `internal/artifact/provenance/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/artifact`
**Submodules**: none (leaf)

## Purpose

This module appends deterministic compiler-owned evidence for configuration, source inputs, acknowledgments, adapter revisions, output bytes, and executable intent.

## Functional Responsibilities

- Hash compact configuration, imported inputs, and exact target output bytes.
- Record compiler version, target adapter revisions, acknowledgments, and each output file's executable flag.
- Add one non-executable `.agentbundler/build.json` outside target package roots.

## Subdomain Classification

**Supporting.** Provenance is stable shared traceability infrastructure.

## Public Contract

<!-- contract: BuildPlan, PlannedFile, TargetID — restated from internal/compiler/model/module.md -->

```text
ProvenanceInput = {
  compilerVersion: String,
  configuration: ByteSequence,
  inputs: [ProvenanceInputFile],
  acknowledgments: [ProvenanceAcknowledgment],
  adapterRevisions: [AdapterRevision]
}
ProvenanceInputFile = { path: RelativePath, sha256: String }
ProvenanceAcknowledgment = { asset: String, target: TargetID, key: String, reason: String }
AdapterRevision = { target: TargetID, revision: Integer }
append-provenance(BuildPlan, ProvenanceInput) -> BuildPlan
```

The schema remains version 1. Each output record includes target, adapter revision, sorted file path, SHA-256 of exact bytes, and `executable: Boolean`. The returned plan is a deep copy and adds exactly one non-executable compiler file at `.agentbundler/build.json`.

Configuration hashing uses `encoding/json.Compact`, not RFC 8785. Input, acknowledgment, target, and file arrays use specified UTF-8 lexical ordering. Planned origins and native checks are not serialized.

## Integrations

- **Counterpart**: `internal/artifact`
  - **Direction**: parent validates the plan and delegates augmentation before write/compare.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: reads and deep-copies model-owned plans without target knowledge.

## Constraints and Invariants

- Executable intent is recorded exactly and cannot be dropped or inferred from file bytes.
- No timestamp, hostname, absolute path, source root, Git state, installed vendor/tool version, locale, network result, secret, native-check output, or self-hash appears.
- Embedded Pi runtime bytes are ordinary target output bytes and are hashed exactly; their installed development environment is not evidence.
- Provenance is not a dependency lockfile, registry record, publication receipt, or install record.

## Test Specification

- Shuffled equivalent inputs/plans serialize identically.
- Changing output bytes or executable intent changes the corresponding evidence.
- Appending never mutates input and rejects reserved-path collisions.
- The provenance file remains outside target-native package roots and never hashes itself.
