# Normalized Model

**Path**: `internal/compiler/model/` — the module's code is everything in this folder and its transparent subfolders
**Parent**: `internal/compiler`
**Submodules**: none (leaf)

## Purpose

This module owns the immutable, target-neutral language of compilation: source declarations, typed hooks, discovered inventories, normalized assets, render requests, diagnostics, and declarative output/check plans. Without it, importers, composition, adapters, and artifact operations would share private or vendor-shaped values.

## Functional Responsibilities

- Define and validate source, composition, render, and build-plan values.
- Represent hook semantics and file executable intent before target rendering.
- Define semantic capability and diagnostic classifications.
- Keep model values deterministic, relative-path-only data with no I/O or process behavior.

## Subdomain Classification

**Core.** This is the shared high-volatility product language and therefore remains small and target-neutral.

## Encapsulated Knowledge

- Valid states for portable hooks, files, package modes, and distribution input.
- The distinction between source inventory, normalized packages, target render input, target plan, and build plan.
- Exact capability matching and the closed native/equivalent/advisory/unsupported policy.
- The rule that plans describe work but never perform work.

## Public Contract

```text
RelativePath = normalized non-empty path below its declared root
PackageID = stable package identity
AssetID = stable asset identity in the form kind/name
ByteSequence = immutable UTF-8 or binary file content
SourceLocation = { path: RelativePath, line: Integer?, column: Integer? }
InputFile = { path: RelativePath, sha256: String }
PackageMetadata = Map<String, JsonValue>
DistributionMetadata = Map<String, JsonValue>

SourceKind = bundle | claude-plugin | skills-repository
TargetID = antigravity | claude | codex | pi | copilot | grok | cursor
AssetKind = skill | agent | hook | resource | native-resource
CapabilityKey = canonical non-empty identifier
CapabilityState = native | equivalent | advisory | unsupported
Severity = error | warning | information
TargetProfile = project | package
TargetPackageMode = separate | aggregate

FileContent = { bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
AssetContent = { frontmatter: Map<String, JsonValue>, body: String, files: Map<RelativePath, FileContent> }
BodyMode = replace | sections
SectionPatch = { headingPath: [String], body: String }
BodyPatch = { mode: BodyMode, text: String?, sections: [SectionPatch] }
FilePatch = { path: RelativePath, content: FileContent }
NativeResourceOptions = { piExtensions: [RelativePath] }

HookEvent = session-start | session-end | prompt-submit | pre-tool | post-tool | post-tool-failure | stop | notification | pre-compact | post-compact
HookToolCategory = command | read | write | edit | search | web | task | mcp | other
HookMatcher = { tools: [HookToolCategory] }
HookHandlerMode = exec | shell
HookArgument = { literal: String } | { packageFile: RelativePath }
HookCommand = { mode: HookHandlerMode, program: String?, arguments: [HookArgument], shellCommand: String? }
HookFailurePolicy = open | closed
HookDescriptor = {
  identity: AssetID,
  location: SourceLocation,
  event: HookEvent,
  matcher: HookMatcher?,
  handler: HookCommand,
  timeoutMilliseconds: Integer,
  asynchronous: Boolean,
  failurePolicy: HookFailurePolicy,
  environment: [String],
  order: Integer
}

TargetOverlay = { target: TargetID, frontmatterPatch: Map<String, JsonValue>?, bodyPatch: BodyPatch?, files: [FilePatch], deletedFiles: [RelativePath], acknowledgments: [Acknowledgment] }
NativeGap = { component: String, asset: AssetID?, location: SourceLocation, target: TargetID? }
Acknowledgment = { asset: AssetID, target: TargetID, key: CapabilityKey, reason: String }
CapabilityUse = { key: CapabilityKey, location: SourceLocation }
CapabilityRule = { key: CapabilityKey, state: CapabilityState }
NativeGapAction = replace | exclude | source-only
NativeGapPolicy = { component: String, action: NativeGapAction, replacement: AssetID? }
AggregatePackage = { identity: PackageID, metadata: PackageMetadata }
TargetComposition = { target: TargetID, profile: TargetProfile?, packageMode: TargetPackageMode?, aggregate: AggregatePackage?, skillPreamble: String?, capabilities: [CapabilityRule], nativeGaps: [NativeGapPolicy] }
BundleSourceConfig = { packages: [RelativePath] }
ClaudePluginSourceConfig = { pluginRoot: RelativePath }
SkillsRepositorySourceConfig = { package: PackageID, roots: [RelativePath], metadata: PackageMetadata }
SourceManifest = { version: Integer, kind: SourceKind, root: RelativePath, targets: [TargetID], output: RelativePath, distribution: DistributionMetadata?, composition: [TargetComposition], bundle: BundleSourceConfig?, claudePlugin: ClaudePluginSourceConfig?, skillsRepository: SkillsRepositorySourceConfig? }
SourceAsset = { identity: AssetID, kind: AssetKind, targets: [TargetID]?, base: AssetContent, hook: HookDescriptor?, capabilityUses: [CapabilityUse], overlays: [TargetOverlay] }
SourcePackage = { identity: PackageID, metadata: PackageMetadata, assets: [SourceAsset] }
SourceInventory = { packages: [SourcePackage], nativeGaps: [NativeGap], inputs: [InputFile] }
NormalizedAsset = { identity: AssetID, kind: AssetKind, content: AssetContent, hook: HookDescriptor?, native: NativeResourceOptions?, capabilityUses: [CapabilityUse] }
NormalizedPackage = { identity: PackageID, metadata: PackageMetadata, target: TargetID, profile: TargetProfile?, assets: [NormalizedAsset], acknowledgments: [Acknowledgment] }
TargetRenderInput = { packages: [NormalizedPackage], distribution: DistributionMetadata, packageMode: TargetPackageMode, aggregate: AggregatePackage? }

Diagnostic = { code: String, severity: Severity, location: SourceLocation?, message: String, hint: String?, asset: AssetID?, field: String?, targets: [TargetID]? }
PlannedFile = { path: RelativePath, bytes: ByteSequence, executable: Boolean, origin: [SourceLocation] }
NativeCheck = { program: String, arguments: [String], workingDirectory: RelativePath?, location: SourceLocation }
TargetPlan = { target: TargetID, packages: [PackageID], files: [PlannedFile], nativeChecks: [NativeCheck] }
BuildPlan = { targets: [TargetPlan], compilerFiles: [PlannedFile] }
```

The canonical hook JSON uses the exact field spellings `event`, `matcher.tools`, `handler.mode`, `handler.program`, `handler.arguments`, `literal`, `packageFile`, `handler.shellCommand`, `timeoutMilliseconds`, `asynchronous`, `failurePolicy`, `environment`, and `order`. `environment` is an optional allowlist of environment variable names; the runtime still supplies only its safe baseline. `identity` and `location` are importer-assigned and are not author JSON fields. Unknown and duplicate JSON fields fail.

### Go API

**Package**: `github.com/alexei-led/agentbundler/internal/compiler/model`

The Go Contract Projection in `docs/tech-stack.md` defines exported representations. This package owns constructors and aggregate validators, including strict source-manifest decoding and validation for inventory, composition, normalized packages, target render input, and build plans.

## Integrations

- **Counterpart**: `internal/compiler`
  - **Direction**: orchestration creates and transforms model values.
  - **Strength**: model.
  - **Shared knowledge**: public model types only.
- **Counterpart**: `internal/target`
  - **Direction**: adapters consume `TargetRenderInput` and return `TargetPlan` values.
  - **Strength**: model.
  - **Shared knowledge**: normalized packages, render configuration, capabilities, diagnostics, files, and native checks.
- **Counterpart**: `internal/artifact`
  - **Direction**: artifact services consume complete plans.
  - **Strength**: model.
  - **Shared knowledge**: build plans, planned executable intent, native checks, diagnostics, and source locations.

## Internal Design

`FileContent` is the only pre-render payload shape; bytes and executable intent cannot be separated during import, overlay, or composition. A hook is one typed descriptor plus its `AssetContent.files`. Target adapters translate portable events, matchers, arguments, timeouts, decisions, and failure policy to verified native forms.

## Change Vectors

- Add a portable hook event or semantic capability after at least one target mapping is proven.
- Add common distribution metadata needed by more than one catalog.
- Add target-plan data needed by more than one artifact operation.

## Constraints and Invariants

- No type carries an absolute path, filesystem/process handle, clock, environment map, vendor root variable, or target-private schema.
- A hook-kind asset has exactly one descriptor with matching identity; non-hook assets have none.
- `exec` requires a non-empty program and forbids `shellCommand`; `shell` requires `shellCommand` and forbids program/arguments. A hook argument has exactly one of `literal` or `packageFile`; exec arguments are ordered and may repeat.
- Package-file arguments resolve only to files in the same hook payload. Paths are contained, normalized, and symlink-free at import.
- Timeout is in the inclusive range `1..600000` milliseconds. `order` is non-negative; equal orders are valid and sort by asset identity in UTF-8 byte order, then normalized source location. Asynchronous execution is allowed only for `session-end`, `post-tool`, `post-tool-failure`, `notification`, `pre-compact`, and `post-compact`; it is rejected for `session-start`, `prompt-submit`, `pre-tool`, and `stop`, and cannot carry blocking, rewrite-input, or closed-failure semantics.
- Exact semantic capabilities include `asset.hook`, `hook.command.exec`, `hook.command.shell`, each `hook.event.<event>`, `hook.matcher.tool-category`, `hook.decision.block`, `hook.decision.rewrite-input`, `hook.async`, and `hook.failure.closed`. Unsupported is an error; advisory requires an exact acknowledgment. No target may silently omit or weaken a hook.
- File executable intent survives source import, overlays, composition, rendering, provenance, comparison, and writing. Interpreter-backed payloads need not be executable. Windows keeps the explicit artifact rejection for planned executable intent.
- `separate` is the source-version-1 compatibility default. `aggregate` is valid only for Pi package profile, must be explicitly declared, and requires explicit aggregate identity and metadata. It is never inferred from package count.
- Within one target render input, package identities are unique and packages are ordered. Aggregate dependency values may merge only when equal; dependency, asset, hook, or path conflicts fail with all origins.
- Version-1 hook-free manifests remain valid. New fields are optional unless their feature is selected; target format revisions change when native output changes. Source version changes only if optional strict decoding cannot preserve compatibility.
- Generated bytes may depend only on source files, explicit manifests, adapter revision, and embedded runtime bytes. They do not depend on network, time, hostname, Git, absolute source paths, installed vendor versions, or locale.
- This module imports no sibling module.

## Test Specification

- Strict decode covers duplicate/unknown fields, hook command exclusivity, paths, timeouts, async restrictions, and package modes.
- Model tests cover valid exec and shell hooks, executable file intent, semantic capability keys, aggregate requirements, and deterministic ordering.
- Render-input validation proves only Pi package profile can aggregate and that separate remains the default.
