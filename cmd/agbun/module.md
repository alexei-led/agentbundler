# Command Interface

**Path**: `cmd/agbun/` — the module's code is everything in this folder and transparent subfolders
**Parent**: repository root
**Submodules**: none (leaf)

## Purpose

This module exposes the only executable entry point. It translates command-line input into a compile request and presents deterministic human or JSON results. Without it, compiler internals would leak through scripts, CI, and agent skills.

## Functional Responsibilities

- Parse `build` and `check` commands and their documented flags.
- Locate an explicit manifest by upward search or `--root`.
- Construct a compiler request.
- Render concise human diagnostics or one versioned JSON result.
- Map compilation outcomes to stable exit statuses.

## Subdomain Classification

**Supporting.** Command UX changes less often than target semantics but is a public integration surface. Volatility is moderate.

## Encapsulated Knowledge

- Flag grammar, help text, and usage errors.
- Standard-output versus standard-error rules.
- JSON result envelope version.
- Exit-status mapping and no-manifest adoption guidance presentation.

## Public Contract

<!-- contract: RelativePath, PackageID, AssetID, ByteSequence, SourceLocation, InputFile, PackageMetadata, SourceKind, TargetID, AssetKind, CapabilityKey, CapabilityState, Severity, AssetContent, BodyMode, SectionPatch, BodyPatch, FilePatch, TargetOverlay, NativeGap, Acknowledgment, CapabilityUse, CapabilityRule, NativeGapAction, NativeGapPolicy, TargetComposition, BundleSourceConfig, ClaudePluginSourceConfig, SkillsRepositorySourceConfig, SourceManifest, SourceAsset, SourcePackage, SourceInventory, NormalizedAsset, NormalizedPackage, Diagnostic, PlannedFile, NativeCheck, TargetPlan, BuildPlan — restated from internal/compiler/module.md (subset: compiler input and result model closure) -->
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

<!-- contract: BuildMode, CompileRequest, CompilationResult, compile — restated from internal/compiler/module.md -->
```text
BuildMode = build | check
CompileRequest = { workspaceRoot: absolute cleaned directory path, manifest: SourceManifest, targets: [TargetID], packages: [PackageID], mode: BuildMode, nativeVerify: Boolean }
CompilationResult = { plan: BuildPlan, diagnostics: [Diagnostic], drift: Boolean, nativeVerificationFailed: Boolean }
compile(CompileRequest) -> CompilationResult
```

### Command Grammar and Manifest

```text
agbun build [--root DIR] [--target TARGET]... [--package PACKAGE]... [--json]
agbun check [--root DIR] [--target TARGET]... [--package PACKAGE]... [--native] [--json]
agbun --help
```

- The verb is the first argument. Other than the standalone `--help`, flags follow the verb and positional arguments are invalid.
- Flags use separate values only: `--root DIR`, `--target TARGET`, and `--package PACKAGE`; `--flag=value` is a usage error.
- `--root` is non-repeatable. Its directory is resolved against the supplied working directory, cleaned, made absolute, and must exist. With it, the command reads exactly `DIR/agentbundle.json` and never searches ancestors.
- Without `--root`, the command searches from the supplied working directory through its ancestors for the first `agentbundle.json`, stopping at the filesystem root. Its containing directory is `workspaceRoot`.
- `--target` and `--package` may repeat; their values retain argument order. Duplicate selector values are usage errors. Omitted selectors become empty lists so compiler defaulting applies.
- `--native` is valid only for `check`; it sets `nativeVerify` to true. `--json` is valid for both verbs. `--help` writes usage to standard output and exits zero without compiling.
- Unknown verbs or flags, missing flag values, duplicate `--root`, duplicate selector values, and positional arguments are usage errors: the command writes one `USAGE` diagnostic to standard error, exits one, and never calls the compiler.

`agentbundle.json` is UTF-8 JSON with exactly these fields:

```json
{
  "version": 1,
  "kind": "bundle",
  "root": "bundle",
  "targets": ["claude"],
  "output": ".agentbundler"
}
```

Every shown field is required. `version` is exactly `1`; unknown fields, duplicate JSON object keys, duplicate targets, and an empty target list are source errors. `kind`, `root`, `targets`, and `output` must satisfy the restated `SourceManifest` contract. `root` and `output` are relative to `workspaceRoot`; `root` identifies a descendant source directory and `output` identifies a descendant generated-output directory. A missing manifest emits `MANIFEST_NOT_FOUND`; if the workspace contains `.claude-plugin/plugin.json` or a root `SKILL.md`, its human diagnostic also includes the exact starter object above, with `kind` changed respectively to `claude-plugin` or `skills-repository`.

### Go Boundary

The command imports only the compiler facade and model contracts:

```go
import (
    "github.com/alexei-led/agentbundler/internal/compiler"
    "github.com/alexei-led/agentbundler/internal/compiler/model"
)

type compileFunc func(compiler.CompileRequest) compiler.CompilationResult

func run(args []string, workingDirectory string, stdout io.Writer, stderr io.Writer, compile compileFunc) int
```

The executable obtains `workingDirectory` from `os.Getwd`; tests provide a temporary absolute directory. It reads and decodes the manifest but never reads source assets. For every valid invocation it calls the injected function exactly once with `WorkspaceRoot` set to the manifest directory and `Mode` set from the verb.

### Rendering

Human diagnostics use one standard-error line each:

```text
[path[:line[:column]]:] severity[code]: message
```

The location prefix is omitted when no location is available. Warnings and information diagnostics do not make an otherwise successful outcome fail. A diagnostic-free `build` writes exactly `build: ok\n` to standard output; a current `check` writes exactly `check: current\n`.

In JSON mode, standard output is exactly one newline-terminated object and standard error is empty:

```json
{
  "version": 1,
  "command": "build",
  "diagnostics": [
    {
      "code": "string",
      "severity": "error",
      "location": { "path": "string", "line": null, "column": null },
      "message": "string"
    }
  ],
  "drift": false,
  "nativeVerificationFailed": false
}
```

Members appear in the shown order and diagnostics retain compiler order. `location` is `null` when unavailable; otherwise absent line and column values are `null`. The command never serializes `BuildPlan`.

Exit status is zero only when there is no error diagnostic, `drift` is false, and `nativeVerificationFailed` is false. Ordinary errors exit one, drift exits two, and native-verification failure exits three; precedence is native verification, then drift, then ordinary errors.

The command never reads source assets directly. Human text goes to standard error except successful concise status; JSON mode writes exactly one versioned result object to standard output. Exit status is zero for success, one for source/capability/render/write failure, two for drift, and three for native verification failure.

## Integrations

- **Counterpart**: repository root
  - **Direction**: this module implements the root CLI contract.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: moderate.
  - **Balanced?**: yes.
  - **Shared knowledge**: restated `build` and `check` operations above.
- **Counterpart**: `internal/compiler`
  - **Direction**: this module invokes compiler orchestration once per command.
  - **Strength**: contract.
  - **LCA / Rank / Distance**: root / 1 / 1.
  - **Volatility**: high.
  - **Balanced?**: yes.
  - **Shared knowledge**: restated compile request and result contract above.

## Change Vectors

- Improve diagnostics or JSON envelope while preserving versioned compatibility.
- Add a flag only when it changes an existing operation rather than creating a new verb.
- Clarify detected-layout adoption guidance.

## Constraints and Invariants

- There are no additional verbs: no `validate`, `lint`, `clean`, `dry-run`, `init`, `watch`, `pack`, or `publish`.
- `check` is read-only by contract; no flag may weaken this.
- The command does not know source topology, overlay semantics, target layout, or filesystem replacement algorithms.
- Unknown flags and verbs fail before compilation.

## Test Specification

### Unit Tests

- **Test name**: two-command grammar.
  - **Scenario**: parse each valid command and invalid extra verb.
  - **Expected behavior**: only `build` and `check` construct requests.
- **Test name**: native flag scope.
  - **Scenario**: supply `--native` to build and check.
  - **Expected behavior**: build rejects it; check sets native verification.

### Integration Contract Tests

- **Test name**: CLI request mapping.
  - **Scenario**: invoke each command with target, package, root, and JSON flags.
  - **Expected behavior**: compiler fake receives the corresponding request.
- **Test name**: exit-status mapping.
  - **Scenario**: compiler fake returns success, ordinary error, drift, and native-verification failure.
  - **Expected behavior**: process exits zero, one, two, and three respectively.

### Boundary Tests

- **Test name**: missing manifest presents guidance.
  - **Scenario**: run in a detected source layout without manifest.
  - **Expected behavior**: no compile call; stable diagnostic includes starter manifest.
- **Test name**: JSON remains machine-only.
  - **Scenario**: command emits a diagnostic in JSON mode.
  - **Expected behavior**: standard output contains one JSON object and standard error contains no competing human report.

### Behavior Tests

- **Test name**: CI check workflow.
  - **Scenario**: execute `check --json` against current and drifted fixture output.
  - **Expected behavior**: result is machine-readable, stable, and distinguishes drift from source error.
