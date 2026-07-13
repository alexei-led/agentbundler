# Compiler Orchestration

**Path**: `internal/compiler/` — the module's code is everything in this folder and its transparent subfolders, excluding child module folders
**Parent**: repository root
**Submodules**: `model`, `source`, `composition`

## Purpose

This module coordinates one compilation without owning source-topology details, overlay algorithms, vendor semantics, or filesystem effects. Without it, the command would know every subsystem and target adapters would need to manage output lifecycle.

## Functional Responsibilities

- Select one explicit source importer.
- Compose selected source packages for selected targets.
- Obtain target adapters from the target registry.
- Render immutable target plans and assemble one whole-selection build plan.
- Route plans to write or compare artifact operations according to build mode.
- Aggregate ordered diagnostics into one compilation result.

## Subdomain Classification

**Core.** Orchestration changes when source modes, capability policy, or target adapter flows change. It is high volatility, but its workflow must remain small and explicit.

## Encapsulated Knowledge

- The only legal compilation sequence: import, compose, render, artifact action.
- Selector interpretation and target defaulting.
- The difference between `build` writing the complete selected build plan and `check` comparing that same build plan.
- Diagnostic aggregation and early termination rules.

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
BuildMode = build | check
CompileRequest = { workspaceRoot: absolute cleaned directory path, manifest: SourceManifest, targets: [TargetID], packages: [PackageID], mode: BuildMode, nativeVerify: Boolean }
CompilationResult = { plan: BuildPlan, diagnostics: [Diagnostic], drift: Boolean, nativeVerificationFailed: Boolean }
compile(CompileRequest) -> CompilationResult
```

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/compiler`

```go
package compiler

import "github.com/alexei-led/agentbundler/internal/compiler/model"

type BuildMode string

const (
    BuildModeBuild BuildMode = "build"
    BuildModeCheck BuildMode = "check"
)

type CompileRequest struct {
    WorkspaceRoot string
    Manifest      model.SourceManifest
    Targets       []model.TargetID
    Packages      []model.PackageID
    Mode          BuildMode
    NativeVerify  bool
}

type CompilationResult struct {
    Plan                     model.BuildPlan
    Diagnostics              []model.Diagnostic
    Drift                    bool
    NativeVerificationFailed bool
}

func Compile(request CompileRequest) CompilationResult
```

`Compile` never mutates `request` or returned model values. It validates that `WorkspaceRoot` is an existing cleaned absolute directory before invoking a collaborator. `WorkspaceRoot` is operational context: source paths and the generated-output root are resolved beneath it, while all model paths remain relative. A request with no selectors uses manifest-declared derived targets and all selected packages. Any error diagnostic prevents rendering dependent output. `nativeVerify` is valid only for `check`.

## Integrations

- **Counterpart**: `internal/compiler/source`
  - **Direction**: this module asks source importers to create an inventory.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: import — restated from internal/compiler/source/module.md -->
```text
import(SourceManifest, workspace-root) -> SourceInventory + [Diagnostic]
```

- **Counterpart**: `internal/compiler/composition`
  - **Direction**: this module asks composition to resolve source inventory for each target.
  - **Strength**: model.
  - **LCA / Rank / Distance**: `internal/compiler` / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: compose — restated from internal/compiler/composition/module.md -->
```text
compose(SourceInventory, TargetComposition) -> [NormalizedPackage] + [Diagnostic]
```

- **Counterpart**: `internal/target`
  - **Direction**: this module requests an adapter and rendered plan.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: render — restated from internal/target/module.md (subset: rendering operation) -->
```text
render(Adapter, [NormalizedPackage]) -> TargetPlan + [Diagnostic]
```

- **Counterpart**: `internal/artifact`
  - **Direction**: this module delegates plan materialization, comparison, provenance, and optional native checks.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**:

<!-- contract: ProvenanceInput, ProvenanceInputFile, ProvenanceAcknowledgment, AdapterRevision — restated from internal/artifact/provenance/module.md (subset: omits append-provenance) -->
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
```

<!-- contract: write, compare, provenance, verify — restated from internal/artifact/module.md -->
```text
write(BuildPlan, output-root) -> [Diagnostic]
compare(BuildPlan, output-root) -> [Diagnostic]
provenance(BuildPlan, ProvenanceInput) -> BuildPlan + [Diagnostic]
verify([NativeCheck], output-root) -> [Diagnostic]
```

## Internal Design

The compiler reads no asset file directly. It selects an importer from `source`, resolves one adapter and its capability rules per selected target, passes the inventory and rules to `composition`, and renders all selected packages for that target in one invocation. It assembles every `TargetPlan` into one `BuildPlan`, then asks `artifact` to add provenance before choosing write or compare. Native checks are executed only after a successful exact comparison.

## Change Vectors

- Add a compilation stage only when it is shared by multiple source modes or targets.
- Change selector behavior or error aggregation.
- Add a new artifact action after it has a clear plan-based contract.

## Constraints and Invariants

- This module must not import a vendor adapter leaf, importer leaf, filesystem API, process API, or serializer directly.
- It may depend only on the public facades of `model`, `source`, `composition`, `target`, and `artifact`.
- Build and check use the same imported, composed, rendered `BuildPlan`; they differ only in final artifact action.
- A collision or path-safety error anywhere in the selected build prevents every selected output replacement.

## Test Specification

### Unit Tests

- **Test name**: orchestration order is fixed.
  - **Scenario**: instrument importer, composition, adapter, and artifact fakes.
  - **Expected behavior**: calls occur import, compose, render, provenance, then write or compare.
- **Test name**: errors short-circuit dependent work.
  - **Scenario**: source or composition returns an error diagnostic.
  - **Expected behavior**: no affected adapter render or artifact action occurs.

### Integration Contract Tests

- **Test name**: build and check share one whole-selection plan.
  - **Scenario**: run both modes with several targets and packages through identical fake collaborators.
  - **Expected behavior**: rendered `BuildPlan` values are identical; only the artifact operation differs.
- **Test name**: native verification follows drift success.
  - **Scenario**: `check --native` has both current and drifted outputs.
  - **Expected behavior**: native checks run only when comparison has no drift.

### Boundary Tests

- **Test name**: native verify is rejected for build.
  - **Scenario**: set `nativeVerify` with build mode.
  - **Expected behavior**: request validation fails before importing sources.
- **Test name**: selector cannot escape manifest scope.
  - **Scenario**: request an undeclared target or package.
  - **Expected behavior**: compiler returns a diagnostic without calling collaborators.

### Behavior Tests

- **Test name**: multi-package deterministic compilation.
  - **Scenario**: compile multiple packages and targets in differing source enumeration orders.
  - **Expected behavior**: plan order, diagnostics, and provenance inputs are stable.
- **Test name**: native-source gap blocks derived target.
  - **Scenario**: an importer inventories an unresolved native-only component.
  - **Expected behavior**: composition emits an error and the compiler does not render that derived target.
