# Bundle Source Importer

**Path**: `internal/compiler/source/bundle/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module imports Agentbundler's clean canonical bundle layout. Without it, owned repositories such as migrated `cc-thingz` and `architect` would need compatibility scripts or target-specific build logic.

## Functional Responsibilities

- Read `agentbundle.yaml` and canonical source paths.
- Discover explicit package manifests, skills, agents, hooks, and target-native resources.
- Preserve direct asset target overlays beside canonical assets.
- Build package membership from explicit manifests rather than globs.

## Subdomain Classification

**Core.** The canonical layout is the primary authoring model and will evolve with portable asset semantics. It is high volatility.

## Encapsulated Knowledge

- Canonical path conventions for `src/skills`, `src/agents`, `src/hooks`, and `src/plugins`.
- Explicit multi-package composition rules.
- Direct-overlay path resolution and support-file containment.
- The distinction between portable assets and opaque target-native resources.

## Public Contract

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, InputFile, PackageMetadata, SourceKind, TargetID, AssetKind, CapabilityKey, Severity, AssetContent, TargetOverlay, NativeGap, Acknowledgment, SourceManifest, SourceAsset, SourcePackage, SourceInventory, Diagnostic — restated from internal/compiler/model/module.md (subset: minimal recursively closed contract) -->
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
Severity = error | warning | information
AssetContent = { frontmatter: Map<String, JsonValue>, body: String, files: Map<RelativePath, ByteSequence> }
TargetOverlay = { target: TargetID, content: AssetContent?, deletedFiles: [RelativePath], acknowledgments: [Acknowledgment] }
NativeGap = { component: String, location: SourceLocation, target: TargetID? }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
SourceManifest = { kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath }
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
Diagnostic = { code: String, severity: Severity, location: SourceLocation, message: String }
```

```text
inspect-bundle(SourceManifest) -> SourceInventory + [Diagnostic]
```

The importer accepts only `kind: bundle`. Package membership is explicit. Portable source remains author-owned; generated output never becomes another source root.

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
- Unknown manifest fields and duplicate YAML keys are errors.

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
