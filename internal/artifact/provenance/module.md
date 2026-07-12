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

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, InputFile, PackageMetadata, SourceKind, TargetID, AssetKind, CapabilityKey, CapabilityState, Severity, AssetContent, BodyMode, SectionPatch, BodyPatch, FilePatch, TargetOverlay, NativeGap, Acknowledgment, CapabilityUse, CapabilityRule, NativeGapAction, NativeGapPolicy, TargetComposition, BundleSourceConfig, ClaudePluginSourceConfig, SkillsRepositorySourceConfig, SourceManifest, SourceAsset, SourcePackage, SourceInventory, NormalizedAsset, NormalizedPackage, Diagnostic, PlannedFile, NativeCheck, TargetPlan, BuildPlan — restated from internal/compiler/model/module.md -->
```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
AssetID = stable asset identity in the form kind/name
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Int?, column: Int? }
InputFile = { path: RelativePath, sha256: String }
PackageMetadata = Map<String, JsonValue>

SourceKind = bundle | claude-plugin | skills-repository
TargetID = claude | codex | pi | copilot | grok | cursor
AssetKind = skill | agent | hook | native-resource
CapabilityKey = canonical non-empty identifier
CapabilityState = native | equivalent | advisory | unsupported
Severity = error | warning | information

AssetContent = { frontmatter: Map<String, JsonValue>, body: String, files: Map<RelativePath, ByteSequence> }
BodyMode = replace | sections
SectionPatch = { headingPath: [String], body: String }
BodyPatch = { mode: BodyMode, text: String?, sections: [SectionPatch] }
FilePatch = { path: RelativePath, bytes: ByteSequence }
TargetOverlay = { target: TargetID, frontmatterPatch: Map<String, JsonValue>?, bodyPatch: BodyPatch?, files: [FilePatch], deletedFiles: [RelativePath], acknowledgments: [Acknowledgment] }
NativeGap = { component: String, asset: AssetID?, location: SourceLocation, target: TargetID? }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
CapabilityUse = { key: CapabilityKey, location: SourceLocation }
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
NativeGapAction = replace | exclude | source-only
NativeGapPolicy = { component: String, action: NativeGapAction, replacement: AssetID? }
TargetComposition = { target: TargetID, skillPreamble: String?, capabilities: [CapabilityRule], nativeGaps: [NativeGapPolicy] }
BundleSourceConfig = { packages: [RelativePath] }
ClaudePluginSourceConfig = { pluginRoot: RelativePath }
SkillsRepositorySourceConfig = { package: PackageID, roots: [RelativePath], metadata: PackageMetadata }
SourceManifest = { kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath, composition: [TargetComposition], bundle: BundleSourceConfig?, claudePlugin: ClaudePluginSourceConfig?, skillsRepository: SkillsRepositorySourceConfig? }
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, capabilityUses: [CapabilityUse], overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }

Diagnostic = { code: String, severity: Severity, location: SourceLocation?, message: String }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath?, location: SourceLocation }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan], compilerFiles: [PlannedFile] }
```

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

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/artifact/provenance`

```go
package provenance

import (
    "encoding/json"

    "github.com/alexei-led/agentbundler/internal/compiler/model"
)

type InputFile struct {
    Path   model.RelativePath
    SHA256 string
}

type Acknowledgment struct {
    Asset  string
    Target model.TargetID
    Key    string
    Reason string
}

type AdapterRevision struct {
    Target   model.TargetID
    Revision int
}

type Input struct {
    CompilerVersion  string
    Configuration    json.RawMessage
    Inputs           []InputFile
    Acknowledgments  []Acknowledgment
    AdapterRevisions []AdapterRevision
}

func Append(plan model.BuildPlan, input Input) (model.BuildPlan, error)
```

`Append` validates its inputs without filesystem access, never mutates `plan` or `input`, and returns a deep-copied plan. It adds exactly one non-executable `PlannedFile` at `.agentbundler/build.json` to `BuildPlan.compilerFiles`, with no origins. It fails when that path already exists in a target or compiler file, when an input is malformed, or when evidence does not cover every selected target exactly once. The returned plan includes one compiler-owned provenance file at `<output-root>/.agentbundler/build.json`, outside native target package roots. It records no timestamp, absolute path, host identity, Git commit, network result, secret, or self-hash.

### Provenance JSON Schema and Canonicalization

The file is UTF-8 JSON with these members, in the shown order:

```json
{
  "schemaVersion": 1,
  "compiler": { "version": "string" },
  "configuration": { "sha256": "64 lowercase hexadecimal characters" },
  "inputs": [
    { "path": "string", "sha256": "64 lowercase hexadecimal characters" }
  ],
  "acknowledgments": [
    {
      "asset": "string",
      "target": "claude",
      "key": "string",
      "reason": "string"
    }
  ],
  "outputs": [
    {
      "target": "claude",
      "adapterRevision": 1,
      "files": [
        {
          "path": "string",
          "sha256": "64 lowercase hexadecimal characters",
          "executable": false
        }
      ]
    }
  ]
}
```

Every object has no unlisted members. `schemaVersion` is exactly `1`; all strings shown as non-empty are non-empty; every digest is a lowercase SHA-256 hexadecimal digest. `Input.Configuration` must be exactly one valid non-empty JSON object with no trailing bytes after whitespace; arrays, scalars, null, malformed JSON, and duplicate object keys are invalid. `configuration.sha256` is SHA-256 of `encoding/json.Compact` applied to that object. This is deterministic compact JSON, not RFC 8785: object-member order and lexical number/string spellings remain significant. Each input digest is the validated supplied digest. Each output digest is SHA-256 of the exact `PlannedFile.bytes` for the corresponding native target file.

Before encoding the provenance JSON, order arrays by ascending UTF-8 byte sequence: inputs by path; acknowledgments by `(asset, target, key, reason)`; outputs by target; and files by path. `PlannedFile.origin`, `NativeCheck`, compiler-owned files, the operational output root, and `.agentbundler/build.json` itself are excluded from output evidence. `AdapterRevisions` contains exactly one entry for every selected target and no others. The provenance file therefore never hashes itself.

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
- `.agentbundler/build.json` is a reserved compiler-file path relative to the operational output root; no literal `dist` path is assumed.
- It is not an Agentbundler dependency lockfile and does not resolve packages.
- Its own hash is omitted to avoid recursion.
- It cannot include secrets from source or environment.

## Test Specification

### Unit Tests

- **Test name**: canonical schema and hash vectors.
  - **Scenario**: append provenance twice from equivalent configurations and shuffled inputs, acknowledgments, targets, and files.
  - **Expected behavior**: bytes match exactly and the fixed `encoding/json.Compact`/SHA-256 vector; reordering JSON object members may change the hash.
- **Test name**: append does not mutate and rejects collisions.
  - **Scenario**: append to one plan, then append to a plan already containing `.agentbundler/build.json`.
  - **Expected behavior**: the original is unchanged, the first result has one compiler file, and the collision returns an error.
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
