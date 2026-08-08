# Plan: first-class Agent Plugins 1.0.0 support

## Overview

Implement the approved semantic Agent Plugins architecture without a big-bang
rewrite. The execution order fixes unsafe filesystem boundaries first, adds the
pinned wire/model seam, implements source ingestion, implements the standard
output/archive path, and finishes with repository-wide verification and docs.

This is an implementation plan. An engineer, mutator agent, or task runner
executes it after approval. The architect does not apply production changes.

## Source artifact

Approved design: `docs/agent-plugins-architecture.md`.

Design approval evidence: final independent architecture review returned
`PASS — ready for architecture-plan` after capability-ceiling and pinned-archfit
corrections.

Primary design references:

- Modules: `internal/agentplugins`, `internal/compiler/model`,
  `internal/compiler/source/agentplugin`, `internal/target/agentplugins`,
  `internal/artifact`, target registry, composition, and CLI/docs.
- Contracts: C1 pinned wire contract; C2 source-to-model; C3
  model/composition-to-target; C4 layout guard and plan-owned archives; C5
  adapter capability ceilings.
- Decisions: D1-D13, especially D2 wire/model separation, D5 strict capability
  failures, D7 contained-link materialization, D9 unknown JSON preservation,
  D10 pinned profile, D12 archive units, and D13 capability ceilings.
- Risks: output/source overlap, archive path breakout and TOCTOU, upstream
  version churn, stale capability claims, symlink identity loss, and missing
  package resource limits.
- Current evidence: `SourceManifest` has critical GitNexus blast radius (108
  upstream references); `Compile` has high blast radius (26 references). Refresh
  the stale graph before implementation with `gitnexus analyze .`.

Pinned compatibility profile:

- Profile: `agent-plugins/1.0.0-bd383552`.
- Upstream commit: `bd383552095128f6effe895b9257cfd580a6d179`.
- Spec SHA-256:
  `97a658b7dca3ce1b4c2266b95da300fa51d9dc4ade59d73168e5f9104272da18`.
- Plugin schema SHA-256:
  `0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883`.
- MCP schema SHA-256:
  `6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb`.

## Success criteria

- Build, check, and package reject source/output equality or containment before
  source traversal, including textual and symlink/junction aliases (C4).
- Archive names cannot escape the requested directory, and final archive bytes
  come only from immutable plan entries rather than a generated-filesystem walk
  (C4, D12).
- The embedded Agent Plugins profile matches the pinned spec and schema digests
  and performs no network I/O (C1, D10).
- `agent-plugin` manifests decode an explicit non-empty `agentPlugin.plugins`
  list; `agent-plugins` target registration is explicit; aggregate mode is
  rejected (D1, D4, D8).
- Manifest, skills, all MCP transports, extension namespaces, permitted unknown
  JSON, regular package files, executable intent, and provenance survive
  `SourcePackage -> NormalizedPackage -> TargetRenderInput -> BuildPlan` (C2,
  C3, D3, D9).
- Root-contained source links are materialized deliberately; external links,
  cycles, special files, and unsafe reparse behavior fail before rendering (D7).
- Target adapters remain authoritative capability ceilings; manifest policy can
  only match or narrow support (C5, D13).
- Existing seven targets, deterministic fixtures, archive contracts, race tests,
  lint, vet, and architecture gates remain green.
- Documentation says authoring/build compatibility, not runtime/client
  conformance.

## Validation Commands

Run from the repository root. Refresh GitNexus before the first task and after
large structural changes:

- `gitnexus analyze .`
- `gitnexus detect-changes --scope all --repo agentbundler`
- `test -z "$(gofmt -l $(git ls-files '*.go'))" && git diff --check`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `golangci-lint run`
- `scripts/check-acceptance-fixture`
- `tmpbin=$(mktemp -d) && GOBIN="$tmpbin" go install -ldflags "-X main.version=v1.6.0" github.com/alexei-led/archfit/cmd/archfit@v1.6.0 && PATH="$tmpbin:$PATH" ARCHFIT_VERSION_CHECK=1 scripts/check-architecture`
- `go test -run '^$' -tags=vendor_smoke ./...`

The repository's unversioned local archfit development binary is not valid for
this plan. Use pinned v1.6.0, matching CI.

## Implementation Steps

### Task 1: Secure workspace and archive destinations before source ingestion

Justification: C4 and the critical self-review risks. Current model validation
checks `root` and `output` independently
(`internal/compiler/model/validation.go:63-68`), `Compile` imports source before
artifact work (`internal/compiler/compiler.go:78-80,131-142`), and archive names
are built from unchecked distribution data
(`internal/artifact/archive/archive.go:29-41`).

Files:

- `internal/artifact/layout.go` — add the opaque `WorkspaceLayoutGuard`,
  constructor, canonical containment comparison, and destination revalidation.
- `internal/artifact/layout_test.go` — equality, both containment directions,
  absent output, textual aliases, symlink/junction aliases, and mutation tests.
- `internal/artifact/artifact.go` — require guards for write, compare, and archive
  entrypoints.
- `internal/artifact/artifact_test.go` — verify every operation rejects an
  invalid or stale guard before filesystem mutation.
- `internal/artifact/archive/archive.go` — validate distribution/archive
  basenames and requested destination containment; keep current archive content
  behavior until Task 4.
- `internal/artifact/archive/archive_test.go` — traversal, separators, absolute
  names, platform-reserved forms, and destination escape regression tests.
- `internal/compiler/compiler.go` — construct Gate 0 before `source.Import` and
  pass the guard to artifact operations.
- `internal/compiler/compiler_test.go` — prove failure occurs before importer or
  output observation.
- `cmd/agbun/main.go`, `cmd/agbun/main_test.go` — construct/pass the guard on the
  direct package-command archive path and prove invalid destinations fail before
  archive mutation.
- `internal/artifact/module.md`, `internal/artifact/archive/module.md`,
  `internal/compiler/module.md`, `cmd/agbun/module.md` — record temporary and
  final boundary contracts.

Preconditions: clean worktree; refreshed GitNexus index; baseline `go test ./...`
and pinned architecture gate pass.

Postconditions: source/output overlap and archive-destination breakout are
blocked for all existing source kinds and commands without changing generated
content.

Fitness gate: existing `.archfit.yaml` rules remain enforced. No new module rule
is needed. Pinned archfit v1.6.0 must remain at 0 blocking findings.

Impact commands:

- `gitnexus impact Compile --file internal/compiler/compiler.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact Archive --file internal/artifact/artifact.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus detect-changes --scope all --repo agentbundler`

Verification commands:

- `go test ./internal/artifact/... ./internal/compiler/... -run 'Layout|Overlap|Archive|Distribution|Compile'`
- `go test ./internal/artifact/... ./internal/compiler/...`
- `go test -race ./internal/artifact/... ./internal/compiler/...`
- `go vet ./internal/artifact/... ./internal/compiler/...`
- `git diff --check`

Manual checks:

- Confirm diagnostics name both conflicting paths without leaking unrelated
  absolute workspace details.
- Confirm no test intentionally writes or deletes outside its temporary root.

- [x] Capture the baseline and add focused regression cases for every known
      overlap and archive-name escape.
- [x] Implement `WorkspaceLayoutGuard` and invoke it before `source.Import`.
- [x] Revalidate the guard at write, compare, and archive boundaries.
- [x] Validate archive/distribution basenames and destination containment.
- [x] Wire the package CLI archive caller through the guard and add pre-mutation
      command tests.
- [x] Update module contracts and run Task 1 verification commands.

### Task 2: Add the pinned wire module and canonical package representation

Justification: C1-C2; D2, D3, D9, D10. Current package models contain only
metadata and assets (`internal/compiler/model/types.go:364-405`), while the new
wire contract must stay isolated from compiler internals and all I/O.

Files:

- `internal/agentplugins/profile.go` — profile ID, upstream commit, schema URLs,
  digests, and supported selectors.
- `internal/agentplugins/manifest.go`, `mcp.go`, `decode.go`, `validate.go`,
  `encode.go` — pure wire types, duplicate-key rejection, typed transports,
  raw permitted unknown JSON, field-specific validation, and deterministic
  encoding.
- `internal/agentplugins/schemas/1.0.0/plugin.schema.json`,
  `mcp.schema.json` — exact pinned schema bytes.
- `internal/agentplugins/testdata/**`, `*_test.go`, `module.md` — normative,
  boundary, unknown-field, duplicate-key, digest, and offline fixtures.
- `internal/agentplugins/imports_test.go` — enforce an explicit pure stdlib
  allowlist; reject `os`, `io/fs`, `os/exec`, `net`, `net/http`, and other
  filesystem/process/network packages while permitting deterministic
  JSON/schema work, `net/url`, and `embed`.
- `internal/compiler/model/types.go` — `SourceKindAgentPlugin`,
  `AgentPluginSourceConfig`, `AgentPluginData`, MCP/extension/package types, and
  new portable capability keys. Do not add target/archive behavior yet.
- `internal/compiler/model/clone.go`, `validation.go`,
  `normalized_validation.go`, `render_input.go`, `json.go`, `model_test.go` —
  complete deep clone, validation, sorting, and strict Agentbundler source-config
  decoding.
- `.archfit.yaml` — add `internal/agentplugins` as a core/high-volatility model
  layer module and forbid imports from every `internal/compiler/**` package
  including model, plus target, artifact, and cmd.
- `internal/compiler/model/module.md` — document package-owned portable data.

Preconditions: Task 1 committed and green; exact upstream schema bytes verified
against the design digests.

Postconditions: the pure pinned format contract and complete package-owned model
representation exist. No composition, source routing, target registration, or
archive behavior changes yet.

Fitness gate: a deliberate import from `internal/agentplugins` to any
`internal/compiler/**`, target, artifact, or cmd package must fail pinned archfit.
The import-allowlist test must fail on filesystem, process, or network imports.
The implemented module passes both gates.

Impact commands:

- `gitnexus impact SourceManifest --file internal/compiler/model/types.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact SourcePackage --file internal/compiler/model/types.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus detect-changes --scope all --repo agentbundler`

Verification commands:

- `go test ./internal/agentplugins/...`
- `go test ./internal/compiler/model -run 'AgentPlugin|SourceManifest|Clone|UnknownJSON'`
- `go test ./internal/compiler/model`
- `tmpbin=$(mktemp -d) && GOBIN="$tmpbin" go install -ldflags "-X main.version=v1.6.0" github.com/alexei-led/archfit/cmd/archfit@v1.6.0 && PATH="$tmpbin:$PATH" ARCHFIT_VERSION_CHECK=1 scripts/check-architecture`
- `git diff --check`

Manual checks:

- Compare every embedded file digest with the approved design.
- Confirm `internal/agentplugins` has no compiler or I/O-capable imports.
- Confirm raw unknown JSON is preserved as a value, not interpreted as compiler
  semantics.

- [x] Vendor the exact pinned schemas and add profile/digest/offline tests.
- [x] Implement pure manifest/MCP decode, validate, and deterministic encode.
- [x] Add the package-owned model, deep-clone/validation/sort path, source config,
      and portable capability keys.
- [x] Add and prove archfit plus stdlib import-allowlist boundaries.
- [x] Update module docs and run Task 2 verification commands.

### Task 3: Implement and register the full Agent Plugin source adapter

Justification: C2, C5; D1, D3, D6-D11, D13. The source dispatcher currently
supports only bundle, Claude plugin, and skills repository
(`internal/compiler/source/source.go:34-47`), existing importers reject all
symlinks, and manifest policy can currently replace adapter capabilities
(`internal/compiler/compiler.go:297-304`).

Files:

- `internal/compiler/source/agentplugin/agentplugin.go` — explicit multi-root
  import, identity handling, component failure scopes, deterministic inventory.
- `internal/compiler/source/agentplugin/packagefs.go` — resolved plugin-root
  `os.Root`, descriptor-relative traversal, `os.SameFile` cycle detection,
  special-file rejection, materialization, quotas, modes, digests, and source
  locations.
- `internal/compiler/source/agentplugin/skills.go` — immediate-child Agent Skills
  discovery using the existing frontmatter contract.
- `internal/compiler/source/agentplugin/mcp.go`, `extensions.go` — map pinned wire
  values and package trees into `AgentPluginData`.
- `internal/compiler/source/agentplugin/*_test.go`, `testdata/**` — minimal/full,
  multiple roots, collisions, nested-skill, all MCP transports, unknown JSON,
  contained/external links, cycles, special files, quotas, and no-execution
  fixtures.
- `internal/compiler/source/source.go`, `source_test.go` — register and route
  `SourceKindAgentPlugin`.
- `internal/compiler/composition/composition.go`, `composition_test.go`,
  `module.md` — deep-copy plugin data through package selection without merging
  and enforce component capability uses.
- `internal/compiler/compiler.go`, `compiler_test.go` — merge manifest rules
  under adapter ceilings; reject unknown keys, upgrades, and native/equivalent
  substitution.
- `internal/target/target.go`, `target_test.go` — define the closed portable
  capability catalog and normalize omitted rules to unsupported.
- `internal/compiler/source/agentplugin/module.md`,
  `internal/compiler/source/module.md` — document importer ownership and
  materialization semantics.

Preconditions: Task 2 committed and green. The importer limits are fixed by the
approved design: 10,000 entries, 64 MiB per file, 256 MiB total file bytes,
depth 64, and 1,024 UTF-8 bytes per relative path.

Postconditions: `kind: "agent-plugin"` imports full pinned semantics and carries
it through composition. Existing targets render ordinary skills through their
current asset path. Every new portable capability—stdio, Streamable HTTP, SSE,
extensions, permitted unknown JSON, and package files—defaults to unsupported;
no first-release vendor codec opts in. Manifest policy may only retain or narrow
that ceiling. The `agent-plugins` output target is not registered until Task 4.

Fitness gate: existing `source_no_composition`, `source_no_target`, and
`source_no_artifact` rules must pass. The source adapter may import only
`internal/agentplugins`, compiler model, and source-local/frontmatter packages.
Existing composition boundary rules remain passing after plugin-data
propagation.

Impact commands:

- `gitnexus impact Import --file internal/compiler/source/source.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact SourceInventory --file internal/compiler/model/types.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact Compose --file internal/compiler/composition/composition.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact Compile --file internal/compiler/compiler.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus detect-changes --scope all --repo agentbundler`

Verification commands:

- `go test ./internal/compiler/source/agentplugin -run 'AgentPlugin|Manifest|Skill|MCP|Extension|Symlink|Containment|Quota'`
- `go test ./internal/compiler/source/...`
- `go test ./internal/compiler/... ./internal/target -run 'AgentPlugin|Capability|CompositionPolicy'`
- `go test -race ./internal/compiler/source/... ./internal/compiler/composition`
- `tmpbin=$(mktemp -d) && GOBIN="$tmpbin" go install -ldflags "-X main.version=v1.6.0" github.com/alexei-led/archfit/cmd/archfit@v1.6.0 && PATH="$tmpbin:$PATH" ARCHFIT_VERSION_CHECK=1 scripts/check-architecture`
- `git diff --check`

Manual checks:

- Inspect diagnostics for plugin, component, skill, and MCP-server failure scope.
- Confirm static and runtime-spy tests prove MCP commands, bare executables, and
  remote URLs are never executed, resolved, connected to, or fetched (D6).
- Confirm link materialization is visible in diagnostics/provenance and never
  silently described as byte-for-byte filesystem round-trip.

- [ ] Implement explicit plugin-root import and duplicate/case-fold identity
      rejection.
- [ ] Implement bounded root-contained traversal and link materialization.
- [ ] Import skills, typed MCP, extension namespaces, permitted unknown JSON,
      package files, modes, inputs, and provenance.
- [ ] Propagate package plugin data through composition without merging.
- [ ] Register source routing, normalize omitted portable capabilities to
      unsupported, enforce adapter ceilings, and add existing-target failure
      tests.
- [ ] Update source/composition module docs and run Task 3 verification commands.

### Task 4: Implement the standard target and plan-owned package archives

Justification: C3-C5; D1, D4, D5, D8, D11-D13. Existing plugin renderers are
vendor-specific, `TargetPlan` lacks package archive roots, and current archive
code re-walks generated output (`internal/artifact/archive/archive.go:50-130`).

Files:

- `internal/target/agentplugins/agentplugins.go` — adapter ID, format revision,
  authoritative capabilities, separate-only package mode, and renderer.
- `internal/target/agentplugins/manifest.go`, `mcp.go`, `files.go` — map canonical
  semantics through `internal/agentplugins` into stable plugin roots.
- `internal/target/agentplugins/*_test.go`, `testdata/**`, `module.md` — minimal,
  full, semantic round-trip, unknown JSON, collision, deterministic plan, and
  unsupported-mode coverage.
- `internal/compiler/model/types.go`, `validation.go`, `model_test.go` — add and
  validate `ArchiveUnit` on `TargetPlan`.
- `internal/target/target.go`, `target_test.go`, `catalog_test.go`,
  `internal/target/module.md` — register `agent-plugins`, capabilities, revision,
  and target catalog behavior; centrally default empty archive-unit lists from
  existing renderers to one `.` unit.
- `internal/artifact/archive/archive.go` — replace generated-output walking with
  `TargetPlan.Files` plus `ArchiveUnits`; strip unit roots and write deterministic
  entries from planned bytes/modes.
- `internal/artifact/archive/archive_test.go`,
  `internal/artifact/artifact_test.go` — unit coverage, overlap, empty/missing
  units, one archive per plugin, mutation-after-plan, Unix/Windows paths, and
  deterministic bytes.
- `internal/artifact/artifact.go` — remove workspace/manifest traversal inputs
  from archive API; pass distribution metadata, plan, guard, and destination.
- `internal/compiler/compiler.go`, `compiler_test.go` — route rendering,
  provenance revision, separate package roots, build/check/package integration.
- `cmd/agbun/main.go`, `main_test.go`, `module.md` — migrate the direct package
  command caller to the new archive API and test guard propagation plus one
  archive per plugin.
- `testdata/agent-plugins/**` — checked minimal and full portable fixtures.
- `scripts/check-agent-plugins-fixture` — deterministic build/check/package
  acceptance gate.

Preconditions: Task 3 committed and green; no unresolved target capability or
archive filename decision.

Postconditions: users can build, check, and package pinned Agent Plugins with
full semantic preservation; existing renderers receive one central default `.`
archive unit without individual edits; the standard target emits one explicit
unit per plugin; no archive code reads generated output.

Fitness gate: existing `target_no_source`, `target_no_composition`, and
`target_no_artifact` rules must pass. Target uses only model, pure format, and
approved target-local helpers. Artifact remains independent of source and target.

Impact commands:

- `gitnexus impact TargetPlan --file internal/compiler/model/types.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact Render --file internal/target/target.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus impact Archive --file internal/artifact/artifact.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus detect-changes --scope all --repo agentbundler`

Verification commands:

- `go test ./internal/target/agentplugins ./internal/target -run 'AgentPlugin|Registry|Catalog|Capability'`
- `go test ./internal/artifact/... -run 'Archive|ArchiveUnit|Deterministic|Traversal'`
- `go test ./internal/compiler/... -run 'AgentPlugin|Archive|Deterministic|Drift'`
- `scripts/check-agent-plugins-fixture`
- `scripts/check-acceptance-fixture`
- `go test ./...`
- `go test -race ./...`
- `tmpbin=$(mktemp -d) && GOBIN="$tmpbin" go install -ldflags "-X main.version=v1.6.0" github.com/alexei-led/archfit/cmd/archfit@v1.6.0 && PATH="$tmpbin:$PATH" ARCHFIT_VERSION_CHECK=1 scripts/check-architecture`
- `git diff --check`

Manual checks:

- Extract one generated archive and confirm `plugin.json` is at its package root.
- Compare semantic JSON values and regular package bytes between source fixture
  and generated target; formatting and source symlink identity may differ only
  as documented.
- Confirm generated output is accepted by any available official non-mutating
  validator. Record unavailable-validator gaps instead of substituting a vendor
  validator.

- [ ] Implement the full standard target and exact capability declaration.
- [ ] Register the target, stable separate package roots, and central `.` archive
      default for existing renderers.
- [ ] Replace filesystem archive walking with validated plan-owned archive units
      and migrate the package CLI caller.
- [ ] Add minimal/full deterministic acceptance fixtures and semantic
      round-trip checks.
- [ ] Run Task 4 verification commands and record official-validator coverage.

### Task 5: Final verification, documentation, and architecture handoff

Justification: all design contracts and acceptance signals. User-facing support
is incomplete until the standard profile, limitations, safe conversion behavior,
and verification evidence are documented and the entire repository remains
green.

Files:

- `README.md` — advertise Agent Plugins authoring/build support precisely.
- `docs/configuration.md` — exact `agentPlugin.plugins` manifest contract.
- `docs/guide.md`, `docs/quickstart.md` — import and build examples.
- `docs/targets-and-cli.md` — `agent-plugins` target, separate-only mode,
  build/check/package behavior, and no runtime conformance claim.
- `docs/architecture.md` — new format/source/target boundaries and plan-owned
  archive contract.
- `docs/vendor-package-contracts.md` — clarify that vendor plugin manifests are
  not aliases of Agent Plugins.
- `docs/troubleshooting.md` — schema-profile, capability, containment, link
  materialization, and archive diagnostics.
- `.github/workflows/ci.yml` — run the Agent Plugins fixture gate and keep pinned
  archfit enforcement.
- `docs/plans/agent-plugins-support.md` — record execution evidence and any
  approved deviations without rewriting completed task history.

Preconditions: Tasks 1-4 committed; all focused checks pass; no unresolved
critical/significant reviewer finding.

Postconditions: whole-plan validation is green, public docs match behavior, the
GitNexus change report is recorded, and a scoped post-implementation
architecture review is ready.

Fitness gate: pinned archfit v1.6.0 full gate passes with 0 blocking findings.
New Agent Plugins module and dependency rules are enforced in CI, not warn-only.

Impact commands:

- `gitnexus analyze .`
- `gitnexus detect-changes --scope all --repo agentbundler`
- `gitnexus impact Compile --file internal/compiler/compiler.go --direction upstream --depth 3 --include-tests --repo agentbundler`
- `gitnexus check --cycles --repo agentbundler`

Verification commands:

- `test -z "$(gofmt -l $(git ls-files '*.go'))"`
- `git diff --check`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `golangci-lint run`
- `go test -run '^$' -tags=vendor_smoke ./...`
- `scripts/check-acceptance-fixture`
- `scripts/check-agent-plugins-fixture`
- `tmpbin=$(mktemp -d) && GOBIN="$tmpbin" go install -ldflags "-X main.version=v1.6.0" github.com/alexei-led/archfit/cmd/archfit@v1.6.0 && PATH="$tmpbin:$PATH" ARCHFIT_VERSION_CHECK=1 scripts/check-architecture`

Manual checks:

- Confirm every compatibility claim has a dated official source.
- Confirm docs say contained links are materialized and unknown permitted JSON is
  semantically, not byte-format, preserved.
- Confirm no docs imply installation, execution, trust, registry, marketplace,
  or runtime-client conformance.
- Review final diff for accidental changes outside the approved architecture.

- [ ] Update public, module, CLI, troubleshooting, and architecture docs from
      verified behavior.
- [ ] Add the Agent Plugins acceptance gate to CI.
- [ ] Run every whole-plan validation command and record exit status/evidence.
- [ ] Refresh GitNexus and record affected processes, modules, and residual risk.
- [ ] Record the scoped `architecture-review` follow-up against C1-C5, D1-D13,
      the new archfit rules, and all acceptance signals.

## Acceptance criteria

- All Task 1-5 postconditions hold with recorded command evidence.
- Pinned Agent Plugins schemas and profile digests match the approved design.
- Minimal and full standard fixtures pass build, check, package, drift,
  deterministic, semantic round-trip, and archive extraction tests.
- Source/output overlap, archive breakout, archive TOCTOU, external link,
  cycle, special-file, capability-upgrade, duplicate-key, unknown-field, and
  no-execution/no-network regressions have focused tests (including D6).
- Every existing target and fixture remains green under normal and race tests.
- Pinned archfit v1.6.0 reports 0 blocking findings and enforces the new pure
  format boundary in CI.
- GitNexus is refreshed and `detect-changes` reports no unexplained affected
  flow.
- Public docs match implemented behavior and state all non-goals.
- Independent post-implementation `architecture-review` finds no critical or
  significant drift from `docs/agent-plugins-architecture.md`.

## Safety notes

- This plan changes high-blast-radius model, compiler, source, target, and
  artifact contracts. Execute tasks serially with one writer. Commit each task
  independently so rollback is a normal revert, not a manual partial unwind.
- Task 1 changes destructive output validation. Run it before accepting any new
  plugin source roots.
- Never test containment or archives against real project directories. Use only
  isolated temporary roots.
- The importer must never execute MCP commands, fetch MCP URLs, install
  dependencies, or infer trust.
- Keep the compatibility profile pinned. Do not silently replace embedded
  schemas from the network or implement unmerged upstream proposals.
- Contained links are materialized. Supporting preserved link identity later
  requires a new design for entry kinds, compare/write/archive behavior, and
  Windows junction/reparse semantics.
- If a task uncovers a product decision that changes package boundaries,
  capability loss policy, archive units, or security behavior, stop and amend
  the architecture before continuing.

## Re-review

After implementation, run `architecture-review` over:

- `internal/agentplugins/**` purity and pinned profile.
- `SourcePackage -> NormalizedPackage -> TargetRenderInput -> BuildPlan` data
  preservation.
- source/target/artifact dependency direction and new archfit gates.
- Gate 0 layout protection and plan-only archive behavior.
- adapter capability ceilings, strict loss diagnostics, and D6 no-execution/no-network behavior.
- acceptance fixtures, public compatibility claims, and residual standard-churn
  risk.

Compare findings directly with contracts C1-C5, decisions D1-D13, and the
acceptance criteria in this plan.
