# Target Adapter Registry

**Path**: `internal/target/` — the module's code is everything in this folder and its transparent subfolders, excluding child module folders
**Parent**: repository root
**Submodules**: `claude`, `codex`, `pi`, `copilot`, `grok`, `cursor`

## Purpose

This module owns the built-in adapter boundary and target selection. Without it, compilation would scatter vendor identifiers, capabilities, layouts, and serializers across source composition and artifact handling.

## Functional Responsibilities

- Register the six built-in adapters explicitly.
- Resolve an adapter by target identifier.
- Expose each adapter's format revision and capability rules.
- Render all selected normalized packages for one target to one declarative target plan, including target-wide metadata.
- Keep target adapters pure: no direct filesystem, process, network, clock, or environment access.

## Subdomain Classification

**Core.** Vendor formats and capabilities are independently volatile. This module is high volatility and is intentionally decomposed into one leaf per vendor.

## Encapsulated Knowledge

- Built-in target registry membership.
- Adapter interface and purity requirements.
- Target capability states and format revisions.
- The rule that adding a target is an in-tree adapter change, not an external plugin load.

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
NativeGap = { component: String, location: SourceLocation, target: TargetID? }
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
SourceAsset = { identity: AssetID, kind: AssetKind, base: AssetContent, overlays: [TargetOverlay] }
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
Adapter = { target: TargetID, formatRevision: Integer, capabilities: [CapabilityRule] }
resolve(TargetID) -> Adapter + [Diagnostic]
capabilities(Adapter) -> [CapabilityRule]
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

`resolve` returns one built-in adapter or a diagnostic. `render` is pure and returns all generated native files, plus optional native checks. An adapter may return a diagnostic for a capability the composition layer should already have rejected; this is a defensive consistency check, not an alternate loss policy.

## Deterministic Renderer Baseline

Until a verified target-primary layout is documented, every built-in adapter uses this target-neutral interchange baseline at `formatRevision: 1`. It is not a claim of vendor-runtime acceptance. Each adapter declares one capability rule for each key: `asset.skill`, `asset.agent`, `asset.hook`, and `asset.native-resource`.

For each accepted package, the renderer emits:

```text
packages/<package-segment>/package.json
packages/<package-segment>/assets/<asset-kind>/<asset-segment>/asset.json
packages/<package-segment>/assets/<asset-kind>/<asset-segment>/content.md
packages/<package-segment>/assets/<asset-kind>/<asset-segment>/files/<source-relative-path>
package-index.json
```

`package.json` is canonical JSON `{ "identity": PackageID, "metadata": PackageMetadata, "target": TargetID }`. `asset.json` is canonical JSON `{ "capabilityUses": [CapabilityUse], "frontmatter": Map<String, JsonValue>, "identity": AssetID, "kind": AssetKind }`. `content.md` is the exact UTF-8 body; support files are copied byte-for-byte. `package-index.json` is `{ "format": "agentbundler-target-bundle", "formatRevision": 1, "packages": [PackageID], "target": TargetID }`. JSON has UTF-8, sorted object keys, no insignificant whitespace, and one trailing newline.

Package and asset segments percent-encode UTF-8 bytes as uppercase `%HH`, leaving only ASCII letters, digits, `-`, `_`, and `.` literal. Sort packages by identity, assets by identity, capability uses by key then source location, files by path, and target-plan files by path. All baseline files are non-executable and native checks are empty. A target mismatch, non-native/equivalent capability, invalid identity, or duplicate output path returns diagnostics and no plan files.

## Integrations

- **Counterpart**: `internal/compiler`
  - **Direction**: compiler orchestration resolves adapters, obtains capability rules, and requests rendering.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: the normative adapter operations above.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: adapters consume all normalized packages selected for their target and return one target plan.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: high.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: the restated model subset above.
- **Counterpart**: adapter leaves
  - **Direction**: this module dispatches to one child by target identifier.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: `internal/target` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: `Adapter` and its three operations.

## Internal Design

The registry is a closed explicit map from `TargetID` to child adapter. It does not scan installed tools, load user modules, or infer target behavior from files. Each child adapter owns its native layout and serialization. The parent owns only selection and the common adapter contract.

## Change Vectors

- Add a built-in target after its native source and output contracts are verified.
- Revise an adapter format revision.
- Add or remove a documented target capability.
- Split a vendor adapter only when it has independently changing internal concerns.

## Constraints and Invariants

- No external adapter SDK or dynamic module loading exists in the initial design.
- Adapters cannot write output; only `internal/artifact` executes the complete build plan.
- Adapters cannot weaken `unsupported` or `advisory` policy.
- Every native output path must be target-native and complete enough for the target's documented package contract.
- The registry must reject duplicate target identifiers.

## Test Specification

### Unit Tests

- **Test name**: registry is closed.
  - **Scenario**: resolve every declared and an undeclared target identifier.
  - **Expected behavior**: each declared target resolves once; undeclared target returns a diagnostic.
- **Test name**: adapter format revision is present.
  - **Scenario**: inspect every registered adapter.
  - **Expected behavior**: each has a positive explicit format revision.

### Integration Contract Tests

- **Test name**: composition receives adapter capability rules.
  - **Scenario**: resolve each adapter during compilation.
  - **Expected behavior**: compiler passes its rules unchanged to composition.
- **Test name**: target-wide render returns one plan.
  - **Scenario**: execute each adapter with several packages and an instrumented filesystem/process boundary.
  - **Expected behavior**: no boundary is accessed while one coherent target plan and diagnostics are returned.

### Boundary Tests

- **Test name**: duplicate adapter identifier fails.
  - **Scenario**: construct registry with two adapters for one target.
  - **Expected behavior**: registry construction fails.
- **Test name**: target mismatch fails.
  - **Scenario**: pass normalized packages whose target differs from the resolved adapter target.
  - **Expected behavior**: render returns a diagnostic without files.

### Behavior Tests

- **Test name**: all target-wide adapter fixtures render deterministically.
  - **Scenario**: render every supported multi-package capability fixture twice with different package and insertion order.
  - **Expected behavior**: plans are byte-for-byte and path-for-path identical.
- **Test name**: unsupported cell remains explicit.
  - **Scenario**: render each documented unsupported asset-target combination.
  - **Expected behavior**: diagnostics identify the capability and target; no partial plan is returned.
