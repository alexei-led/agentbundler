# Agent Plugins support architecture

## Overview

Target-state redesign for first-class Agent Plugins 1.0.0 support in Agentbundler.
The feature adds a semantic Agent Plugins source importer and a dedicated
`agent-plugins` output target. It preserves the standard's portable semantics
without making existing vendor targets pretend to implement the standard.

Scope:

- `plugin.json` manifest validation and encoding.
- Agent Skills under the standard fixed layout.
- `mcp.json` with typed stdio, Streamable HTTP, and SSE entries.
- Reverse-domain client-extension namespaces and their package files.
- Regular package files, executable bits, provenance, deterministic rendering,
  and safe root-contained filesystem handling. Contained source symlinks are
  materialized as regular output entries; filesystem-link identity is not
  preserved.
- Capability diagnostics when converting to targets that cannot represent a
  portable component.
- Build and archive safety needed before accepting arbitrary plugin roots.

Non-goals:

- Running MCP servers or plugin code.
- Installing plugins, marketplaces, registries, signing, or trust policy.
- Defining hooks, agents, commands, rules, LSP, permissions, or authentication
  as new portable components.
- Claiming runtime/client conformance.
- Implementing upstream proposals that are not part of the pinned 1.0.0 spec.

## Source inputs and drift notes

- Requirements: user-approved semantic model immediately; dedicated source and
  target; typed MCP; lossless extensions and package files; safe containment;
  strict capability failures; no execution or installation.
- External standard: `https://agent-plugins.org/specification`, the canonical
  versioned specification and schemas in
  `https://github.com/agentplugins/agent-plugins-spec`, the conformance guide,
  compatible-client matrix, governance document, and vendor documentation.
- Prior research artifact: `.pi-subagents/artifacts/outputs/e22ce2eb/research.md`.
- Existing intent: `docs/architecture.md`, `docs/configuration.md`,
  `docs/targets-and-cli.md`, `docs/vendor-package-contracts.md`.
- Existing implementation checked: yes. The compiler pipeline is
  `source -> model -> composition -> target -> artifact`; source and target
  layers are already separated by `.archfit.yaml`.
- Existing tests and gates checked: `internal/compiler/*_test.go`, source and
  target tests, `.github/workflows/ci.yml`, and `.archfit.yaml`.
- GitNexus evidence: `SourceManifest` has 108 upstream references and
  `Compile` has 26; changes to the manifest/model must be staged and tested as
  a high-blast-radius change.

Drift and evidence limits:

- The website labels 1.0.0 Working Draft while the versioned repository spec
  says Published. Compatibility profile `agent-plugins/1.0.0-bd383552` pins
  commit `bd383552095128f6effe895b9257cfd580a6d179`.
- Pinned spec digest: `97a658b7dca3ce1b4c2266b95da300fa51d9dc4ade59d73168e5f9104272da18`.
- Pinned plugin schema URL:
  `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`; digest:
  `0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883`.
- Pinned MCP schema URL:
  `https://agent-plugins.org/schemas/1.0.0/mcp.schema.json`; digest:
  `6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb`.
- Existing source importers reject all symlinks. The Agent Plugins adapter uses
  a narrower profile: follow only paths proven by a plugin-root `os.Root` to
  remain inside that root, detect directory cycles with `os.SameFile`, read
  through opened descriptors, and materialize results as regular output
  entries. External resolution, special files, and unsupported platform
  behavior fail before rendering.
- No `.archfit` rule currently covers Agent Plugins boundaries. New rules below
  are recommended until wired into CI.
- Architecture fitness is enforced in CI with pinned archfit v1.6.0 through
  `scripts/check-architecture`. The local unversioned development binary uses a
  newer CLI/config schema and is not valid evidence. Running
  `go run github.com/alexei-led/archfit/cmd/archfit@v1.6.0 check --config .archfit.yaml --require-tools --progress none`
  at design time passed with 0 blocking findings and 16 advisories.

## Domain and volatility map

Core means differentiating and likely to change. Supporting means necessary but
not differentiating. Generic means solved infrastructure.

| Area | Classification | Volatility | Rationale | Open questions |
| --- | --- | --- | --- | --- |
| Agent Plugins wire contract | core | high | The feature's value is correct portable semantics and the standard is young. | When should the pinned compatibility profile be advanced? |
| Canonical compiler model | core | high | It determines what can be preserved and converted across targets. | Should future standard revisions use parallel model versions? |
| Source and target adapters | core | high | Each adapter changes with filesystem and client behavior. | Which clients deserve verified capability codecs? |
| Artifact writing and archives | supporting | medium | Stable compiler infrastructure, but security-sensitive for untrusted package roots. | Should symlinks be represented in output archives? |
| JSON/schema validation | generic/supporting | medium | Standard validation machinery is reusable, but schema updates are frequent. | Keep embedded validation dependency-free? |
| CLI and compatibility reporting | supporting | medium | User-facing control plane for selecting targets and understanding loss. | Add a standalone `validate-plugin` command now or after build support? |

Proposed deterministic labels; not approved evidence for archfit until added:

| Module | Subdomain | Volatility | Public/private boundary | Deploy unit | Status |
| --- | --- | --- | --- | --- | --- |
| `internal/agentplugins` | core format contract | high | package API public; schema internals private | agentbundler | draft |
| `internal/compiler/model` | core compiler model | high | package API public; representation private | agentbundler | existing |
| `internal/compiler/source/agentplugin` | core source adapter | high | importer entrypoint public to compiler only | agentbundler | draft |
| `internal/target/agentplugins` | core target adapter | high | renderer entrypoint public to target registry only | agentbundler | draft |
| `internal/artifact` | supporting artifact infrastructure | medium | plan/writer APIs public to compiler | agentbundler | existing |
| `internal/compatibility` | supporting repository-root contract | high | repository compatibility APIs public to compiler | agentbundler | existing; unchanged |

## Module map

| Module | Responsibility | Owned knowledge | Public interface | Private internals | Owner/deploy expectation | Change vectors |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/agentplugins` | Decode, validate, and encode the pinned standard. | Wire JSON, schema IDs, field rules, transport unions, namespace syntax, standard diagnostics. | Pure Go decode/validate/encode functions and wire types. | Embedded schemas, JSON normalization, semantic validators. | Same Agentbundler binary; no filesystem or compiler imports. | Upstream spec revisions, schema changes, validation rules. |
| `internal/compiler/model` | Hold target-neutral plugin semantics. | `AgentPluginData`, MCP values, extension trees, package entries, provenance, capabilities. | Existing model types plus explicit plugin fields. | Validation and identity rules not specific to the wire format. | Same binary; model layer. | New portable components, package-mode semantics, capability model. |
| `internal/compiler/source/agentplugin` | Import one or more declared plugin roots. | Root discovery, safe filesystem reads, skill topology, package-file inventory, source diagnostics. | `InspectAgentPluginRoot(manifest, workspaceRoot, workspace)`. | Containment resolver, deterministic traversal, partial component collection. | Same binary; source stage. | Standard layout, filesystem rules, package size/depth limits. |
| `internal/compiler/source/frontmatter` | Parse and validate Agent Skills metadata. | Skill frontmatter and content rules shared by source adapters. | Existing frontmatter API, extended only when required by the standard. | YAML parsing and field diagnostics. | Same binary; source stage. | Agent Skills specification changes. |
| `internal/compiler/source` | Route explicit source kinds and normalize inventories. | Source-kind dispatch and importer lifecycle. | `Import`, source-kind registry. | Workspace validation and diagnostic sorting. | Same binary; compiler stage. | New source kinds and importer contracts. |
| `internal/compiler/composition` | Compose selected source packages without knowing filesystem or targets. | Target selection, package selection, overlays, capability uses. | Existing composition interfaces plus plugin data preservation. | Merge and selector rules. | Same binary; stage layer. | Multi-package composition and collision policy. |
| `internal/target/agentplugins` | Render one normalized package to a conforming Agent Plugins directory. | Manifest emission, `skills/`, `mcp.json`, extension namespaces, package files. | Target adapter and renderer registration. | Stable ordering, serialization, path mapping, target-specific checks. | Same binary; target stage. | 1.0.0 output rules and future standard profiles. |
| `internal/target` | Register and select target adapters. | Target IDs, target capabilities, package mode. | Existing target registry. | Adapter lookup and shared render validation. | Same binary; target stage. | Target matrix and capability declarations. |
| `internal/artifact` | Validate, stage, write, compare, and archive immutable plans. | Output containment, atomic replacement, archive destination safety. | Artifact APIs consume a validated layout guard and plan-owned archive units. | Staging journals and writer implementation. | Same binary; supporting stage. | Filesystem platform behavior, archive formats. |
| `cmd/agbun` and docs | Expose source/target selection and explain support. | Manifest decoding, command UX, user-facing diagnostics. | Existing CLI plus target/source names and validation command if approved. | Flag wiring and help text. | Composition root only. | CLI compatibility and documentation. |

The standard adapter owns translation between the wire contract and the
compiler model. The compiler model never imports a source or target adapter.
The source adapter never imports a target adapter. This is the primary modularity
invariant. `internal/compatibility` remains limited to repository-root
compatibility files; it does not own target representability.

## Canonical data contract

The source manifest contract is explicit and non-discovering:

```json
{
  "version": 1,
  "kind": "agent-plugin",
  "root": "plugins",
  "targets": ["agent-plugins"],
  "output": "generated",
  "composition": [],
  "agentPlugin": {
    "plugins": ["deploy-tools", "review-tools"]
  }
}
```

`agentPlugin.plugins` is a non-empty, duplicate-free list of plugin roots
relative to `root`. Roots are normalized and processed in lexical order. Each
root must contain `plugin.json`; its validated `name` becomes `PackageID`.
Duplicate and case-fold-equivalent names fail the source. The
`agent-plugins` target supports separate package mode only and always emits
`agent-plugins/<plugin-name>/...`; aggregate mode fails before rendering.

The model adds explicit package-owned values:

```text
SourceManifest.AgentPlugin        *AgentPluginSourceConfig
SourcePackage.AgentPlugin         *AgentPluginData
NormalizedPackage.AgentPlugin     *AgentPluginData
TargetRenderInput.Packages[*]      carries NormalizedPackage.AgentPlugin

AgentPluginSourceConfig
  Plugins []RelativePath

AgentPluginData
  Profile             AgentPluginProfile
  Manifest            AgentPluginManifest
  MCPServers          []MCPServer
  Extensions          []ClientExtension
  PackageFiles        []PackageFile
  UnknownManifest     map[string]JSONValue
  UnknownMCP          map[string]JSONValue

MCPServer
  Name                string
  Transport           stdio | streamable-http | sse
  Stdio               *StdioMCPServer
  Remote              *RemoteMCPServer
  Unknown             map[string]JSONValue
```

`StdioMCPServer` owns `command`, `args`, `env`, and `cwd`.
`RemoteMCPServer` owns `url` and `headers`. `command` is one bare token or a
plugin-relative `./` path and never receives placeholder expansion. `args`,
`env` values, and `cwd` receive only single-pass `${PLUGIN_ROOT}` and
`${PLUGIN_DATA}` expansion at runtime; the compiler validates their permitted
forms but does not expand them. Arguments, environment values, headers, and URLs
remain opaque strings except for their field-specific schema rules.
`PLUGIN_ROOT` and `PLUGIN_DATA` environment keys are reserved and rejected in
source configuration.

`ClientExtension` owns a reverse-domain namespace, an opaque JSON manifest
value, and namespaced package files. `PackageFile` owns a contained relative
path, bytes, executable intent, digest, and source location. It excludes files
owned by the manifest, MCP, skills, and extension contracts, preventing two
model values from owning one output path.

Every package-level value has a validation and deep-clone function. Composition
copies `AgentPluginData` exactly from `SourcePackage` to `NormalizedPackage`,
applies package selection without merging, rejects aggregate mode, and records
component capability uses. Target adapters declare the maximum capability state
through `target.Adapter.Capabilities`; compiler policy merge treats those rules
as a ceiling before `composition.Compose` enforces them. Manifest policy may
repeat a rule or narrow it to advisory/unsupported, but may not introduce a key,
upgrade unsupported/advisory support, or switch between native and equivalent.

The format decoder uses a standard-specific JSON policy, not the strict
Agentbundler manifest decoder. Duplicate object members are rejected as
ambiguous. Unknown members that the pinned 1.0.0 profile says clients ignore are
stored as raw validated JSON values and reproduced value-for-value by the
`agent-plugins` target with deterministic object ordering. They have no compiler
semantics. A vendor target lacking a verified carrier reports an unsupported
capability instead of dropping them. This is a semantic JSON round trip, not a
byte-for-byte formatting round trip.

## Integration contracts

### C1: `internal/agentplugins` -> source and target adapters

- Strength: contract, because adapters consume pure wire types and functions.
- Distance: package-to-package within one binary; low runtime distance but a
  separate ownership boundary in the code structure.
- Volatility: high; the upstream standard is changing.
- Balanced: yes. High volatility is balanced by a narrow pure contract rather
  than duplicated schema knowledge.
- Contract: pinned schema version, deterministic diagnostics, no I/O, no imports
  from compiler/source, compiler/model, target, or artifact.
- Knowledge shared: wire fields and validation results. Filesystem and compiler
  composition remain outside this module.
- Balancing move: keep schema updates isolated in this package and require a
  versioned fixture suite before changing the pinned revision.
- Failure modes: schema mismatch, unsupported version, malformed JSON, semantic
  field errors. No partial output may be emitted after an error.

### C2: source adapter -> compiler model

- Strength: contract/model. The adapter maps standard concepts into explicit
  target-neutral values, not vendor-specific assets.
- Distance: adjacent stage packages in one binary.
- Volatility: high on both sides.
- Balanced: acceptable high-cohesion coupling at low distance.
- Contract: `SourceInventory` containing `SourcePackage` with package metadata,
  skills, typed plugin data, package entries, inputs, and diagnostics.
- Knowledge shared: semantic MCP and extension data; importer filesystem details
  stay private.
- Balancing move: keep root traversal and decoding private; expose no `os.Root`
  or path-walking helpers to the model.
- Failure modes: invalid plugin root, duplicate identity, containment failure,
  component-level diagnostics, unsupported standard version.

### C3: compiler model/composition -> target adapter

- Strength: model. Targets receive normalized plugin data and capability uses.
- Distance: separate stage modules within one binary.
- Volatility: high.
- Balanced: use a stable target render input and explicit capability checks.
- Contract: `TargetRenderInput`, target capability declaration, and immutable
  `BuildPlan` output.
- Knowledge shared: semantic data and target profile; target internals remain
  private.
- Balancing move: prohibit target adapters from reading source files or
  compiler composition internals.
- Failure modes: unsupported component, aggregate/package-mode conflict,
  identity collision, invalid output path, serialization failure.

### C4: compiler/artifact -> filesystem

- Strength: functional/intrusive at the artifact boundary because it controls
  replacement and archive writes.
- Distance: package to OS/filesystem; high operational consequence.
- Volatility: medium, but security impact is high.
- Balanced: keep the boundary narrow and validate immutable plans before writes.
- Contract: `internal/artifact` owns the opaque `WorkspaceLayoutGuard` and its
  constructor. Compiler Gate 0 calls that constructor before source import. It
  resolves the existing source root and the deepest existing output ancestor,
  appends clean missing output components, and rejects equality or containment in
  either direction. `artifact.Write`, `artifact.Compare`, and `artifact.Archive`
  revalidate the guard immediately before filesystem access, covering textual
  aliases and symlink/junction aliases without storing absolute paths in the
  deterministic `BuildPlan`.
- Contract: `TargetPlan` adds `ArchiveUnits []ArchiveUnit`; each unit has a safe
  basename and a target-plan-relative root. `target.Render` supplies one `.`
  archive unit when an existing adapter returns none, preserving all seven
  renderers without cross-cutting edits. The `agent-plugins` adapter declares
  one unit per plugin root. Archive code selects and strips that prefix from
  `TargetPlan.Files`; it receives no workspace or generated-output path and
  performs no second walk.
- Balancing move: lower strength by making the plan and layout guard complete
  before artifact code runs.
- Failure modes: source/output overlap, symlink escape, archive traversal,
  TOCTOU, failed atomic replacement, platform mode differences.

### C5: target adapter capability contract -> composition

- Strength: contract/model through existing `model.CapabilityRule` values.
- Distance: adjacent stage packages in one deployable.
- Volatility: high because client support changes.
- Balanced: adapters own volatile representability; composition owns generic
  enforcement. Neither imports the other.
- Contract: the target registry owns a closed catalog for skills, each MCP
  transport, extensions, preserved unknown JSON, and package files. Missing
  portable rules are normalized to `unsupported`; adapters opt in only where a
  verified codec exists. In `compositionPolicy`, normalized adapter rules are
  authoritative ceilings. Manifest rules may be identical or more restrictive
  only: native/equivalent may narrow to advisory/unsupported, advisory may
  narrow to unsupported, and unsupported cannot be upgraded. Native and
  equivalent cannot be substituted for each other. Unknown keys fail. The
  effective rules enter `TargetComposition`; `composition.Compose` rejects
  unsupported uses before rendering.
- Failure modes: missing, upgraded, substituted, or unsupported capability rules
  emit diagnostics naming the package component and target. No manifest or
  adapter may silently downgrade.

## Key flows

### F1: Import a standard plugin

1. CLI decodes the Agentbundler source manifest and passes it to the compiler.
2. Compiler Gate 0 calls `artifact.NewWorkspaceLayoutGuard` before
   `source.Import`; failure prevents any source traversal.
3. `source.Import` routes `agent-plugin` to the source adapter using
   `AgentPluginSourceConfig.Plugins`.
4. For each explicit plugin root, the adapter resolves the root within the
   workspace and opens a second `os.Root` at the resolved plugin boundary.
5. `internal/agentplugins` validates and decodes the manifest and MCP config
   against profile `agent-plugins/1.0.0-bd383552`.
6. Skills are discovered only at the standard fixed depth. Each skill is parsed
   through the shared frontmatter contract.
7. A descriptor-relative sorted traversal follows only root-contained links,
   detects directory cycles with `os.SameFile`, rejects special files, and
   materializes linked contents as regular package entries.
8. Extension directories and other package files are inventoried with relative
   paths, modes, digests, and source locations.
9. The adapter produces a deterministic `SourceInventory` and diagnostics.
10. Any error prevents compilation output; component-level diagnostics identify
    the narrowest failed boundary.

### F2: Compose and render a standard plugin

1. Composition selects packages and preserves semantic plugin data.
2. The target capability table checks whether requested target semantics are
   representable.
3. `internal/target/agentplugins` emits a stable package root containing
   `plugin.json`, optional `skills/`, optional `mcp.json`, extension namespaces,
   and package files.
4. The target returns an immutable `BuildPlan`; artifact writing occurs later.
5. The artifact writer validates output containment and atomically applies the
   plan.

### F3: Convert a standard plugin to a vendor target

1. Import and normalize the standard package.
2. Run target capability checks before rendering.
3. Map skills where the target contract is verified.
4. Map each MCP transport only through an explicit target codec.
5. Retain extensions only for a registered namespace codec; otherwise fail with
   a capability diagnostic.
6. Refuse unsupported portable semantics rather than dropping them.

### F4: Archive a generated plugin

1. The package CLI constructs or receives `WorkspaceLayoutGuard` before calling
   `artifact.Archive`; guard failure occurs before archive mutation.
2. `target.Render` defaults a missing archive-unit list to one `.` unit; the
   Agent Plugins target emits explicit per-plugin units.
3. Validate distribution and archive-unit names as basenames and verify the
   requested archive directory against `WorkspaceLayoutGuard`.
4. For each unit, select files from `TargetPlan.Files` below the unit root and
   strip the root prefix; reject missing, overlapping, or escaping entries.
5. Create deterministic archives directly from planned bytes and executable
   intent. Never inspect generated output.
6. Produce one archive for each Agent Plugin package and one target-root archive
   for existing targets unless their adapter declares otherwise.
7. Test package CLI guard propagation, mutation after planning, extraction paths,
   and deterministic bytes on Unix and Windows.

## Module test specifications

### `internal/agentplugins`

Behavior tests:

- Decode minimal and full manifests only for pinned profile
  `agent-plugins/1.0.0-bd383552`; reject every other schema selector.
- Verify the embedded spec and schema digests in a profile test.
- Preserve permitted unknown manifest/MCP members as raw valid JSON values while
  assigning them no compiler semantics; reject duplicate object keys.
- Reject invalid required fields, names, and types.
- Decode stdio, Streamable HTTP, and SSE into explicit transport structs.
- Keep `command`, arguments, environment values, headers, and URLs under their
  field-specific rules; never apply generic path validation to opaque strings.
- Reject reserved environment overrides, invalid `cwd`, invalid placeholders,
  unsafe plugin-relative paths, malformed URLs, and invalid reverse-domain
  namespaces.

Unit tests:

- Canonical JSON ordering and stable encoding.
- Per-field diagnostics with JSON path and source location.
- Schema/version selection is offline and pinned.

Boundary tests:

- Package cannot import compiler, source, target, artifact, or perform I/O.
- Unknown future schema versions fail clearly instead of silently degrading.

Fitness check:

- Add archfit forbidden dependencies from `internal/agentplugins/**` to
  `internal/compiler/**` (including model), `internal/target/**`,
  `internal/artifact/**`, and `cmd/**`.
- Add a Go import-allowlist test that rejects filesystem, process, and network
  packages such as `os`, `io/fs`, `os/exec`, `net`, and `net/http`; permit only
  the pure standard-library packages required for JSON/schema work and `embed`.

### `internal/compiler/source/agentplugin`

Behavior tests:

- Import one plugin and multiple explicit roots deterministically.
- Discover only immediate child skills.
- Preserve extension trees, licenses, MCP command payloads, modes, and inputs.
- Report malformed plugin, skill, and MCP entries at the correct scope.

Boundary tests:

- Materialize root-contained file and directory symlinks; reject external links,
  cycles, special files, and unsupported reparse behavior.
- Reject duplicate and case-fold-equivalent identities.
- Verify compiler Gate 0 rejects output/source equality and containment before
  importer entry.
- Never execute an MCP command or inspect a remote URL.
- Enforce 10,000 package entries, 64 MiB per regular file, 256 MiB total regular
  file bytes, depth 64, and 1,024 UTF-8 bytes per relative path before unbounded
  allocation. Boundary values pass; the next value fails with a scoped
  diagnostic.

Contract tests:

- Source output passes `ValidateSourceInventory` and retains all portable data.

Fitness check:

- Keep source adapter imports limited to model, agentplugins, frontmatter, and
  source filesystem helpers. No target, composition, or artifact imports.

### `internal/compiler/model` and composition

Behavior tests:

- `CloneAgentPluginData` deep-copies manifests, MCP maps/slices, raw JSON,
  extension files, package files, bytes, and source locations.
- Plugin data survives `SourcePackage -> NormalizedPackage -> TargetRenderInput`
  unchanged under source normalization and package selection.
- Separate package mode preserves plugin boundaries.
- Aggregate mode for `agent-plugins` fails before composition can merge packages.
- Capability uses survive composition and produce deterministic diagnostics from
  adapter-owned rules.
- Manifest capability rules can only match or narrow adapter ceilings; unknown
  keys, unsupported upgrades, and native/equivalent substitution fail.

Boundary tests:

- Model validation does not access the filesystem.
- Composition does not import source, target, or artifact packages.

Fitness check:

- Existing `.archfit.yaml` rules `composition_no_source`, `composition_no_target`,
  and `composition_no_artifact` remain failing gates; add plugin-specific checks
  only if the new packages expose a bypass.

### `internal/target/agentplugins`

Behavior tests:

- Emit a conforming minimal package.
- Emit full skills, MCP, extensions, regular package files, and executable bits.
- Produce identical plans across repeated runs and package selection order.
- Keep one-plugin and multi-plugin output roots stable.

Boundary tests:

- Fail before output planning when a component is unsupported.
- Reject path collisions between skills, MCP payloads, extensions, and package
  files.
- Never read source roots or execute commands.

Fitness check:

- Add an archfit forbidden dependency from `internal/target/**` to
  `internal/compiler/source/**`; existing rule already covers this.

### `internal/artifact`

Behavior tests:

- Create Gate 0 layout guards for existing and absent output roots, textual
  aliases, symlink/junction aliases, equality, and both containment directions.
- Revalidate layout guards immediately before write, compare, and archive.
- Reject archive names containing separators, traversal, absolute paths, or
  platform-reserved basename forms.
- Validate `ArchiveUnit` coverage and non-overlap.
- Archive immutable plan bytes and reproduce deterministic output after the
  generated filesystem is mutated or removed.

Boundary tests:

- Verify staging and final roots remain contained after symlink creation.
- Exercise failure during replacement and journal cleanup.

Fitness check:

- Add regression tests to the existing artifact CI suite. If archfit supports
  path-policy rules, encode the artifact/source separation; otherwise retain the
  test as the enforced check.

### CLI and compatibility

Behavior tests:

- Decode the exact `agentPlugin.plugins` JSON contract, reject missing or empty
  lists, and route explicit `agent-plugin` sources.
- Register target ID `agent-plugins` and reject aggregate package mode.
- Capability diagnostics identify target, package component, and remediation.
- Help and docs distinguish standard support from runtime conformance.

Contract tests:

- Check output with the official non-mutating validators where available.
- Keep the pinned standard revision and client capability matrix visible in
  provenance.

## Design decisions and trade-offs

- **D1: Add both source and target.** Chosen because users need to import existing
  standard packages and generate them. A target-only implementation cannot
  validate or convert them safely. A source-only implementation cannot make the
  standard a first-class output.
- **D2: Keep wire types separate from compiler model.** Chosen to prevent the
  upstream schema from leaking through every compiler layer. Trade-off: explicit
  translation code. Revisit only if the model and wire contract remain stable
  across two major standard revisions.
- **D3: Preserve portable data as typed semantics plus opaque extension entries.**
  Chosen to avoid treating MCP and extensions as generic assets. Trade-off: more
  model and renderer code; this is required to prevent silent loss.
- **D4: Dedicated standard target, never a vendor plugin target alias.** Chosen
  because similarly named vendor manifests have incompatible schemas.
- **D5: Strict capability failures.** Chosen because silent loss makes a portable
  package appear valid while changing behavior. Offer an explicit future
  `--allow-lossy` mode only after a loss report contract exists.
- **D6: No runtime execution.** Chosen because compilation must remain safe and
  deterministic. MCP runtime tests belong to clients, not Agentbundler.
- **D7: Materialize contained symlinks.** Follow file and directory links only
  through a resolved plugin-root `os.Root`; detect cycles; reject external,
  special, or unsupported entries; emit regular files/directories only. Chosen
  because current plans and archives have no safe link-entry contract. This is
  semantic content preservation, not filesystem-link identity preservation.
- **D8: Separate output package per plugin.** Chosen because the standard defines
  a plugin package boundary and multi-plugin composition semantics are unsettled.
- **D9: Preserve permitted unknown JSON values.** The standard decoder accepts
  unknown members only where the pinned profile permits, rejects duplicate keys,
  stores raw JSON values, and reproduces them deterministically. Chosen to avoid
  silent transformer loss without assigning future semantics.
- **D10: Pin an immutable compatibility profile.** Profile
  `agent-plugins/1.0.0-bd383552` fixes the upstream commit, schema URLs, and
  SHA-256 digests. Advancing it requires an explicit compatibility review,
  fixture delta, adapter revision bump, and provenance update.
- **D11: Keep capability ownership in target adapters and composition.** Chosen
  to preserve existing dependency direction and `.archfit` rules;
  `internal/compatibility` is not expanded.
- **D12: Archive plan-owned units.** `TargetPlan.ArchiveUnits` defines package
  roots; artifact code consumes only plan entries and distribution metadata,
  never the generated filesystem.
- **D13: Adapter capabilities are ceilings.** Manifest composition policy may
  retain or narrow an adapter state, never upgrade or change its semantics.
  Chosen because current `compositionPolicy` otherwise lets configuration bypass
  adapter-declared support (`internal/compiler/compiler.go:297-304`).

## Self-review

| Issue | Severity | Evidence/rationale | Resolution |
| --- | --- | --- | --- |
| Current IR has no place for MCP, extensions, or package-level files. | critical | `internal/compiler/model/types.go:364-405` only models assets and native options. | Canonical data contract now names package-owned types, validation, clone path, composition propagation, and aggregate rejection. |
| Capability ownership initially violated repository boundaries. | critical | `.archfit.yaml:190-204` forbids compatibility-to-target/composition dependencies. | Capability rules remain adapter-owned and are enforced by existing composition; `internal/compatibility` is excluded. |
| Manifest policy can currently replace adapter capabilities. | high | `internal/compiler/compiler.go:297-304`. | D13 makes adapter states ceilings and adds merge validation plus regression tests. |
| Existing source importers reject symlinks. | high | Source helpers and bundle inspector reject `os.ModeSymlink`. | Standard-specific policy now follows only contained links and materializes them; link identity is an explicit non-goal. |
| Existing artifact writer re-reads generated paths. | critical | `internal/artifact/artifact.go:70-79`; `internal/artifact/archive/archive.go:21-130`. | `ArchiveUnit` and plan-only archive input are now explicit contracts; the workspace/output walk is removed. |
| Source and output roots are not globally checked for overlap. | critical | `internal/compiler/model/validation.go:63-68`; compile imports before output safety. | Compiler Gate 0 creates a layout guard before import; artifact operations revalidate it. |
| Standard profile was not reproducible. | high | Website/repository status conflict and no stable tag. | Commit, schema URLs, SHA-256 digests, profile ID, and update procedure are specified. |
| Unknown JSON could be silently removed. | high | Ignore-on-read is unsafe for a transformer round trip. | Preserve raw valid values where the pinned profile permits; reject duplicates; require capability support for other targets. |
| A new shared wire module could become a dependency hub. | medium | High-volatility schema is consumed by source and target. | Keep it pure, small, and enforce forbidden dependencies before implementation. |

## Open risks

- Upstream 1.0.0 may change before a stable tag. Owner: format adapter owner.
  Advance the pinned profile only through D10's compatibility review.
- Client capability claims may become stale. Owner: each target adapter owner.
  The registry defaults every omitted portable capability to unsupported;
  require a dated evidence source and codec test before opting in.
- Package limits may reject unusually large valid packages. Owner: maintainers.
  The first profile uses 10,000 entries, 64 MiB per file, 256 MiB total, depth
  64, and 1,024 UTF-8 path bytes. Revisit only with measured fixtures and a
  bounded-memory design.
- Materialization changes filesystem-link identity. Owner: source/target adapter
  owners. Diagnostics and docs must state this; future link preservation needs a
  new plan-entry kind and cross-platform artifact review.
- A standalone plugin validation command changes CLI scope. Owner: maintainers.
  The first implementation exposes validation through `check`; add a command
  only if direct validation without a bundle manifest is required.

## Handoff

- Recommended next step: `architecture-plan`.
- Implementation notes: execute Gate 0 first; then add the archfit boundary,
  pinned format module, and complete model clone path before source/target
  adapters. Keep one writer per task and run the existing seven-target acceptance
  matrix after each compiler/model change.
- Acceptance signals: pinned fixtures validate offline; permitted unknown JSON
  and regular package bytes survive import/composition/render; contained links
  are materialized deliberately; unsupported conversions fail explicitly;
  archive output comes only from plan entries; source/output guards pass alias
  tests; plans are deterministic; existing targets remain green; archfit
  boundaries remain enforced.
