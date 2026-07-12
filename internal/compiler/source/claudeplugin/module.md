# Claude Plugin Source Importer

**Path**: `internal/compiler/source/claudeplugin/` — the module's code is everything in this folder and transparent subfolders
**Parent**: `internal/compiler/source`
**Submodules**: none (leaf)

## Purpose

This module adopts one existing Claude plugin without moving its files. Without it, repositories such as `fractal-modularity` would need to migrate a valid native package before they could generate derived targets.

## Functional Responsibilities

- Parse one local `.claude-plugin/plugin.json` root.
- Accept marketplace metadata only when it identifies that same local plugin root.
- Import known portable Claude components: skills, agents, and declared hooks.
- Inventory Claude-only components as native gaps.
- Preserve the native source tree as author-owned and untouched.

## Subdomain Classification

**Supporting.** This importer makes adoption low-friction but does not define portable semantics. Claude's format may change, so implementation volatility is moderate.

## Encapsulated Knowledge

- Claude plugin and marketplace path rules.
- The one-local-plugin-root limit.
- The mapping from known Claude components to normalized assets.
- The inventory rules for unrecognized or nonportable Claude features.

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
inspect-claudeplugin(SourceManifest) -> SourceInventory + [Diagnostic]
```

The importer accepts only `kind: claude-plugin`. It never edits Claude source. When Claude is selected, the Claude adapter rebuilds a deterministic native package under the configured generated-output root. Derived-target overlays and native-gap policies live in `.agentbundler/` beside, not inside, the Claude-native tree.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: the parent selects this importer for Claude plugin manifests.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/compiler/source` / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: the normative `inspect-claudeplugin` operation above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: this module constructs a target-neutral inventory and native gaps.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 2 / 2.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: only the restated model subset above.

## Change Vectors

- Claude changes plugin manifest or component conventions.
- A Claude component receives a defined portable mapping.
- Existing Claude marketplace adoption needs a richer but still explicit root rule.

## Constraints and Invariants

- The importer supports one local plugin root, not a multi-plugin marketplace.
- A missing `agentbundle.yaml` may produce guidance but cannot trigger auto-adoption.
- Unknown Claude-native content is never silently discarded during derived compilation.
- Claude may be selected like any other target; its generated package is adapter-rendered under output and never written back to source.

## Test Specification

### Unit Tests

- **Test name**: one plugin root only.
  - **Scenario**: marketplace metadata names several plugin roots.
  - **Expected behavior**: import rejects the unsupported multi-root declaration.
- **Test name**: marketplace root must match plugin root.
  - **Scenario**: marketplace metadata points outside the declared source root.
  - **Expected behavior**: import fails with a containment diagnostic.

### Integration Contract Tests

- **Test name**: native source remains untouched.
  - **Scenario**: import and build Claude plus derived targets from a Claude plugin fixture.
  - **Expected behavior**: all plans point under generated output; no source-tree file is planned for write.
- **Test name**: Claude-only component becomes gap.
  - **Scenario**: plugin contains an unrecognized native component.
  - **Expected behavior**: inventory includes a `NativeGap` with source location.

### Boundary Tests

- **Test name**: absent plugin manifest fails.
  - **Scenario**: declare Claude plugin source without `.claude-plugin/plugin.json`.
  - **Expected behavior**: no inventory is emitted.
- **Test name**: source symlink fails.
  - **Scenario**: plugin component path traverses through a symlink.
  - **Expected behavior**: import rejects it.

### Behavior Tests

- **Test name**: fractal-modularity adoption fixture.
  - **Scenario**: import a fixture with Claude plugin metadata, skills, and hooks.
  - **Expected behavior**: recognized components are normalized and canonical Claude files remain author-owned.
- **Test name**: starter manifest guidance.
  - **Scenario**: run source discovery in a Claude plugin repository without a manifest.
  - **Expected behavior**: diagnostic prints a complete explicit Claude-plugin starter manifest.
