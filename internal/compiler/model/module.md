# Normalized Model

**Path**: `internal/compiler/model/` — the module's code is everything in this folder and its transparent subfolders
**Parent**: `internal/compiler`
**Submodules**: none (leaf)

## Purpose

This module owns the immutable language of compilation: source declarations, discovered inventories, normalized assets, diagnostics, target capabilities, target/build plans, and native verification plans. Without it, importers, composition, adapters, and artifact operations would share private structs and create circular or target-specific dependencies.

## Functional Responsibilities

- Define the stable in-process representation accepted and returned by compiler subsystems.
- Define capability and diagnostic classifications.
- Define the complete declarative output and external-check plans.
- Enforce that model values are relative-path, deterministic data with no filesystem or process behavior.

## Subdomain Classification

**Core.** The normalized model is the central product language. New asset kinds and target capabilities will change it, so it has high volatility. It must nevertheless remain small because every high-volatility branch shares it.

## Encapsulated Knowledge

- Exact field meanings and valid states for every normalized value.
- The distinction between source inventory, composed package, target plan, complete build plan, and observed filesystem state.
- The closed capability-state policy and diagnostic severity vocabulary.
- The rule that a plan describes work but never performs work.

## Public Contract

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
TargetProfile = project | package
TargetComposition = { target: TargetID, profile: TargetProfile?, skillPreamble: String?, capabilities: [CapabilityRule], nativeGaps: [NativeGapPolicy] }
BundleSourceConfig = { packages: [RelativePath] }
ClaudePluginSourceConfig = { pluginRoot: RelativePath }
SkillsRepositorySourceConfig = { package: PackageID, roots: [RelativePath], metadata: PackageMetadata }
SourceManifest = { version: Integer, kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath, composition: [TargetComposition], bundle: BundleSourceConfig?, claudePlugin: ClaudePluginSourceConfig?, skillsRepository: SkillsRepositorySourceConfig? }
SourceAsset = { identity: AssetID, kind: AssetKind, targets: [TargetID]?, base: AssetContent, capabilityUses: [CapabilityUse], overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, profile: TargetProfile?, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }

Diagnostic = { code: String, severity: Severity, location: SourceLocation?, message: String }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath?, location: SourceLocation }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan], compilerFiles: [PlannedFile] }
```

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/compiler/model`

The Go Contract Projection in `docs/tech-stack.md` defines the exported Go representation of every public model type. This package additionally exports:

```go
func NewRelativePath(value string) (RelativePath, error)
func NewPackageID(value string) (PackageID, error)
func NewAssetID(value string) (AssetID, error)
func NewCapabilityKey(value string) (CapabilityKey, error)
func DecodeSourceManifestJSON(data []byte) (SourceManifest, []Diagnostic)
func ValidateSourceManifest(manifest SourceManifest) []Diagnostic
func ValidateSourceInventory(inventory SourceInventory) []Diagnostic
func ValidateTargetComposition(input TargetComposition) []Diagnostic
func ValidateNormalizedPackage(pkg NormalizedPackage) []Diagnostic
func ValidateBuildPlan(plan BuildPlan) []Diagnostic
```

All constructors reject empty, NUL-containing, absolute, escaping, or otherwise invalid scalar values. Aggregate values crossing a module boundary must pass their corresponding validator; validators perform no filesystem, process, clock, network, or environment access. `DecodeSourceManifestJSON` rejects malformed JSON, duplicate keys, unknown fields, duplicate target IDs, an empty target list, invalid enum values, and values that violate the public contract.

## Integrations

- **Counterpart**: `internal/compiler`
  - **Direction**: compiler orchestration creates and transforms model values.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: this module's public types only; no compiler workflow leaks here.
- **Counterpart**: `internal/target`
  - **Direction**: adapters consume normalized packages and return target plans.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: high.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: `NormalizedPackage`, capability rules, diagnostics, `TargetPlan`, and `NativeCheck`.
- **Counterpart**: `internal/artifact`
  - **Direction**: artifact services consume the complete build plan and emit diagnostics or provenance data.
  - **Strength**: model.
  - **LCA / Rank / Distance**: root / 2 / 2.
  - **Volatility**: moderate.
  - **Balanced?**: yes, at the model-distance limit.
  - **Shared knowledge**: `BuildPlan`, `TargetPlan`, `PlannedFile`, `NativeCheck`, `Diagnostic`, and source locations.

## Change Vectors

- Add a precisely specified asset kind.
- Add a target capability state only if the existing four states cannot describe a repeated real case.
- Add target-plan or build-plan metadata needed by multiple independent artifact operations.
- Make a source location more precise for better diagnostics.

## Constraints and Invariants

- No model type carries an absolute path, open file handle, process handle, clock, environment map, or target-specific private struct.
- Lists have deterministic order before crossing a module boundary.
- A source asset with a non-empty `targets` list is included only for those exact targets. The allow-list is fail-closed and does not support globs or deny rules.
- `resource` is a portable package-root directory tree. It is distinct from `native-resource`, which remains target-specific and requires native-gap policy.
- A `PlannedFile` path is relative and cannot contain a path escape.
- A `NativeCheck.program` is a non-empty executable name containing no NUL, `/`, or `\\`; it is resolved through `PATH`, never as a filesystem path. Every argument contains no NUL. An absent `workingDirectory` means the generated target root; a present value is relative to that root. `location` identifies the adapter declaration that produced the check.
- A `BuildPlan` contains all selected targets plus compiler-owned files and is the indivisible write/check transaction. Each compiler-file path is relative to the generated-output root; each target file path is relative to its target root. No full destination path may occur more than once.
- `BodyPatch` has exactly one active payload: replace has non-nil text and zero sections; sections has nil text and one or more unique non-empty heading paths. Overlay file paths are unique and cannot occur in deleted files.
- Target compositions and native-gap policies are unique by target and component. `replace` requires a syntactically valid replacement `AssetID`; `exclude` and `source-only` forbid one. `ValidateTargetComposition` enforces only this syntactic invariant because it has no inventory input. `compose(SourceInventory, TargetComposition)` resolves the relational invariant: every replacement ID must name an asset in that inventory. Capability states require exact acknowledgments for semantic loss; unsupported is always an error.
- Within a `NormalizedPackage`, every asset identity is unique. Within one target render input, every package identity is unique.
- Capability matching uses exact `CapabilityKey` equality; there are no wildcards, predicates, or expressions.
- `advisory` and `unsupported` are not successful states until composition resolves them explicitly.
- This module imports no sibling module.

## Test Specification

### Unit Tests

- **Test name**: relative path rejects escape.
  - **Scenario**: construct model values with absolute paths, `..`, or empty path segments.
  - **Expected behavior**: construction fails before values reach artifact services.
- **Test name**: capability states are closed.
  - **Scenario**: decode a state outside the four declared values.
  - **Expected behavior**: validation emits a source diagnostic.

### Integration Contract Tests

- **Test name**: normalized package is adapter-safe.
  - **Scenario**: pass a composed package to a test adapter.
  - **Expected behavior**: the adapter can render a plan without filesystem access or source topology knowledge.
- **Test name**: build plan is artifact-safe.
  - **Scenario**: pass a multi-target planned file set to write and compare fakes.
  - **Expected behavior**: both use the same deterministic plan representation.

### Boundary Tests

- **Test name**: native check cannot contain an absolute working directory.
  - **Scenario**: construct a native check outside the generated target root.
  - **Expected behavior**: the model rejects it.
- **Test name**: acknowledgment is exact.
  - **Scenario**: attach an acknowledgment without target, asset, capability, or reason.
  - **Expected behavior**: validation fails.

### Behavior Tests

- **Test name**: model serialization is deterministic.
  - **Scenario**: serialize equivalent values built in different insertion orders.
  - **Expected behavior**: normalized order produces the same bytes.
- **Test name**: model remains target-neutral.
  - **Scenario**: inspect public model fields for vendor-private configuration.
  - **Expected behavior**: no vendor-specific field exists outside target-native resource content.
