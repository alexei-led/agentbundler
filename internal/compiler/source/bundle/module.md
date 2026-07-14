# Bundle Source Importer

**Path**: `internal/compiler/source/bundle/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module imports **Agent Bundler**'s clean canonical bundle layout. Without it, owned repositories such as migrated `cc-thingz` and `architect` would need compatibility scripts or target-specific build logic.

## Functional Responsibilities

- Read `agentbundle.json` and canonical source paths.
- Discover explicit package manifests, skills, agents, hooks, and target-native resources.
- Preserve direct asset target overlays beside canonical assets.
- Build package membership from explicit manifests rather than globs.

### Canonical Layout and Schema

```text
agentbundle.json
packages/<package>.json
src/skills/<name>/SKILL.md
src/agents/<name>.md
src/hooks/<name>.json
src/plugins/<target>/<name>/...
<asset-directory>/.agentbundler/asset.json
<asset-directory>/.agentbundler/targets/<target>.json
<asset-directory>/.agentbundler/targets/<target>/files/...
```

`agentbundle.json` uses the shared version-1 JSON `SourceManifest` schema and `bundle.packages` lists exact package-manifest paths. A package manifest is `{ "id": String, "metadata": Object, "assets": [String | AssetEntry] }`, where `AssetEntry` is `{ "path": RelativePath, "targets": [TargetID]? }`. Each entry is a skill directory (`src/skills/<name>` or `skills/<name>`), an agent file (`src/agents/<name>.md` or `agents/<name>.md`), a portable resource directory (`src/resources/<name>` or `resources/<name>`), an exact hook file, or a target-native resource. Target allow-lists are exact and fail closed. A skill entry resolves its directory and `SKILL.md`; file entries use their containing directory as the sidecar directory. Unknown fields, duplicate keys, duplicate asset paths, and invalid identities are errors.

An asset sidecar is `asset.json` with exactly `{ "capabilities": [String] }`, where each capability string becomes a `CapabilityUse` at the sidecar path. A target sidecar is `<asset-directory>/.agentbundler/targets/<target>.json` with exactly `{ "frontmatterPatch": Object?, "bodyPatch": Object?, "files": Object?, "deletedFiles": [String], "acknowledgments": [Object] }`; absent fields are omitted. `files` maps asset-relative slash paths to UTF-8 strings or base64 objects `{ "base64": String }`; the sibling `files/` tree, when present, overrides same-path JSON entries. Sidecars and support files cannot contain symlinks or paths outside the asset.

## Subdomain Classification

**Core.** The canonical layout is the primary authoring model and will evolve with portable asset semantics. It is high volatility.

## Encapsulated Knowledge

- Canonical path conventions for `src/skills`, `src/agents`, `src/resources`, `src/hooks`, and `src/plugins`.
- Explicit multi-package composition rules.
- Direct-overlay path resolution and support-file containment.
- The distinction between portable assets and opaque target-native resources.

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
AssetKind = skill | agent | hook | resource | native-resource
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
SourceAsset = { identity: AssetID, kind: AssetKind, targets: [TargetID]?, base: AssetContent, capabilityUses: [CapabilityUse], overlays: [TargetOverlay] }
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
inspect-bundle(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

The importer accepts only `kind: bundle`. It decodes UTF-8 JSON with duplicate-key and unknown-field rejection. Markdown frontmatter is optional; when present it starts with a first-line `---`, ends at the next `---` line, and contains exactly one JSON object. The remaining bytes are the exact body. Package membership is explicit; unlisted canonical assets are ignored. All walks are sorted and do not cross symlinks. Portable source remains author-owned; generated output never becomes another source root.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: the parent selects this importer for `bundle` manifests.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler/source` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: the normative `inspect-bundle` operation above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module constructs source inventory values.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 2 / 2.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: only the restated model subset above.

## Change Vectors

- Add a portable asset kind.
- Clarify canonical manifest fields.
- Add a direct target overlay capability.

## Constraints and Invariants

- Package manifests, not directory inference, define package membership.
- A target resource cannot overwrite an adapter-generated file.
- Overlay files remain inside their owning asset directory.
- Unknown manifest fields and duplicate JSON object keys are errors.

## Test Specification

### Unit Tests

- **Test name**: explicit membership beats discovery.
  - **Scenario**: source contains an unlisted skill.
  - **Expected behavior**: it is not included in any package inventory.
- **Test name**: direct overlay is associated with one asset.
  - **Scenario**: add an overlay path outside an asset root.
  - **Expected behavior**: import fails with a contained-path diagnostic.

### Integration Contract Tests

- **Test name**: multi-package membership.
  - **Scenario**: list one skill in two package manifests.
  - **Expected behavior**: both packages reference the same normalized source asset identity.
- **Test name**: portable and native resources coexist.
  - **Scenario**: package contains skills and a target-native resource tree.
  - **Expected behavior**: inventory preserves both with distinct asset kinds.

### Boundary Tests

- **Test name**: duplicate YAML key fails.
  - **Scenario**: manifest repeats a configuration key.
  - **Expected behavior**: no inventory is emitted.
- **Test name**: support symlink fails.
  - **Scenario**: asset support tree contains a symlink.
  - **Expected behavior**: import rejects it.

### Behavior Tests

- **Test name**: owned repository replacement fixture.
  - **Scenario**: import a migrated multi-package fixture modeled on cc-thingz.
  - **Expected behavior**: all intended assets and overlays appear in deterministic inventory order.
- **Test name**: single-package fixture.
  - **Scenario**: import a migrated fixture modeled on architect.
  - **Expected behavior**: its agent, skills, and package metadata compose without project-specific behavior.
