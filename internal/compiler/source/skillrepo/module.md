# Skills Repository Source Importer

**Path**: `internal/compiler/source/skillrepo/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module adopts an existing Agent Skills collection with no file moves. Without it, a repository containing useful `SKILL.md` directories would need to adopt multi-package bundle structure merely to compile for other targets.

## Functional Responsibilities

- Read explicit skill roots from the source manifest.
- Discover `SKILL.md` directories under those roots in deterministic order.
- Produce one normalized package containing all discovered skills.
- Import optional `.agentbundler/` derived-target adaptations.
- Reject topology ambiguity rather than guessing a package grouping.

### Manifest and Discovery Schema

`skillsRepository` is required and has `{ "package": String, "roots": [RelativePath], "metadata": Object }`. Each root is resolved below the manifest root. A version-1 `agentbundle.json` with `kind: skills-repository` therefore supplies package identity, metadata, and explicit roots; no root is inferred.

For each declared root, recursively find `SKILL.md` without crossing symlinks. The directory containing `SKILL.md` is the asset root and its basename is the identity `skill/<basename>`. The file is parsed with optional JSON-subset frontmatter delimited by first-line and closing `---`; frontmatter is an object and the remaining bytes are the exact body. Every regular file below the asset root except `.agentbundler/` is a base support file. Duplicate identities, invalid frontmatter, or non-regular entries are diagnostics. `.agentbundler/assets/skill/<name>/asset.json` is exactly `{ "capabilities": [String] }`; each target sidecar is `targets/<target>.json` with the shared overlay fields, and `targets/<target>/files/...` overrides same-path JSON file entries.

## Subdomain Classification

**Supporting.** It reduces adoption friction for generic skills repositories. Its functional behavior is narrow and changes less often than portable composition, so volatility is moderate.

## Encapsulated Knowledge

- Accepted explicit skill-root conventions.
- One-repository-to-one-package composition.
- Discovery, containment, duplicate identity, and support-file rules.
- The separation between native skills content and derived-target adaptation sidecars.

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
SourceManifest = { version: Integer, kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath, composition: [TargetComposition], bundle: BundleSourceConfig?, claudePlugin: ClaudePluginSourceConfig?, skillsRepository: SkillsRepositorySourceConfig? }
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
inspect-skillrepo(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

The importer accepts only `kind: skills-repository`. It decodes UTF-8 JSON with duplicate-key and unknown-field rejection, validates the package configuration, sorts roots and discovered paths, and hashes every imported regular input with SHA-256. Every declared root is explicit. The resulting inventory has one package; repositories needing several package compositions use bundle mode.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: the parent selects this importer for skills-repository manifests.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler/source` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: the normative `inspect-skillrepo` operation above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module constructs source packages and assets.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 2 / 2.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: only the restated model subset above.

## Change Vectors

- Support a real, repeated existing skills-root convention.
- Improve detection guidance for repositories without manifests.
- Add a defined optional sidecar adaptation field.

## Constraints and Invariants

- Roots are explicit; `skills/`, `.agents/skills/`, and a single-skill root are conventions for guidance, not implicit selection.
- Skill identity collisions across roots are errors.
- This source mode never infers several package groups.
- Source skills and support files are not rewritten.

## Test Specification

### Unit Tests

- **Test name**: explicit roots only.
  - **Scenario**: repository contains a second unlisted skill tree.
  - **Expected behavior**: it is absent from inventory.
- **Test name**: duplicate identity fails.
  - **Scenario**: two roots contain the same skill identity.
  - **Expected behavior**: import returns a location-rich error diagnostic.

### Integration Contract Tests

- **Test name**: one repository produces one package.
  - **Scenario**: discover several skill directories from declared roots.
  - **Expected behavior**: inventory contains one package with all discovered skills.
- **Test name**: sidecar remains separate.
  - **Scenario**: add derived-target data under `.agentbundler/`.
  - **Expected behavior**: source skill files are unchanged and the adaptation is associated by identity.

### Boundary Tests

- **Test name**: empty root fails.
  - **Scenario**: declared root contains no `SKILL.md`.
  - **Expected behavior**: import reports the empty declared root.
- **Test name**: path escape fails.
  - **Scenario**: root or sidecar path leaves the repository.
  - **Expected behavior**: import rejects it before scanning.

### Behavior Tests

- **Test name**: generic skills adoption fixture.
  - **Scenario**: import `skills/*/SKILL.md` without moving files.
  - **Expected behavior**: a deterministic one-package inventory is created.
- **Test name**: detection guidance is explicit.
  - **Scenario**: run without a manifest in a likely skills repository.
  - **Expected behavior**: the diagnostic prints a starter manifest with candidate roots, but performs no build.
