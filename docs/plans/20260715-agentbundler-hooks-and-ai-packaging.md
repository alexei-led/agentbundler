<!-- markdownlint-disable MD013 -->

# Agent Bundler Hooks and AI Tool Packaging

## Overview

Implement first-class executable lifecycle hooks and complete installable package output for Claude Code, OpenAI Codex, Pi, GitHub Copilot CLI, Cursor, and Grok Build.

The current compiler imports `AssetKindHook` but every target adapter rejects it. Package payload files do not carry executable metadata, Pi packages cannot register TypeScript extensions, target rendering lacks an explicit distribution/aggregation contract, and native verification is mostly empty. This plan closes those gaps without turning Agent Bundler into an installer, publisher, registry, or network-dependent package manager.

The work is intentionally incremental:

1. make the normative module contracts describe the approved architecture;
2. add typed hook and file metadata to the normalized model;
3. preserve those values through source import and composition;
4. prove one complete Claude vertical slice;
5. add the bundled Pi runtime and aggregate Pi package;
6. implement Codex, Copilot, Cursor, and Grok native packages independently;
7. generate deterministic marketplace/catalog metadata;
8. prove the result with a cc-thingz-shaped fixture and optional vendor smoke tests.

## Context and source evidence

Local implementation facts:

- `internal/compiler/model/types.go` declares `AssetKindHook` and `PlannedFile.Executable`.
- `internal/target/packageoutput/packageoutput.go` handles skills, resources, and agents only; its `add` helper always emits non-executable files.
- Every target codec currently declares hook support as unsupported.
- `internal/artifact/write`, `compare`, and `provenance` already carry planned executable intent, while Windows deliberately rejects it.
- `internal/target/pi/codec.go` emits skills and `pi.subagents`, but no `pi.extensions` entry.
- `internal/compiler/source/bundle/module.md` currently describes hooks as exact JSON files, which is insufficient for scripts and support files.
- `module.md` and `internal/target/pi/module.md` currently forbid generated runtime shims. The approved Pi design changes that contract for one target-owned, self-contained runtime payload.
- `internal/target/packageoutput.RenderWithCodec` already renders several packages under stable package-ID roots. Aggregate output is a separate mode and must not be inferred from package count.
- `.archfit.yaml` enables Go only. The Pi runtime introduces a small TypeScript island under the Pi adapter and therefore requires a matching architecture/tooling update.

Primary vendor contracts, checked 2026-07-15:

- Claude Code plugin and hook references: <https://code.claude.com/docs/en/plugins-reference> and <https://code.claude.com/docs/en/hooks>.
- OpenAI Codex plugins, hooks, and subagents: <https://developers.openai.com/codex/plugins>, <https://developers.openai.com/codex/build-plugins>, <https://developers.openai.com/codex/hooks>, and <https://developers.openai.com/codex/subagents>.
- Pi package and extension references from the installed `@earendil-works/pi-coding-agent` package: `docs/packages.md`, `docs/extensions.md`, `README.md`, and `examples/extensions/`.
- GitHub Copilot CLI plugin and hook references: <https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference> and <https://docs.github.com/en/copilot/reference/hooks-reference>.
- Cursor plugin and hook references: <https://cursor.com/docs/plugins> and <https://cursor.com/docs/hooks>.
- Grok plugin and hook references: <https://docs.x.ai/build/features/skills-plugins-marketplaces> and <https://docs.x.ai/build/features/hooks>.

The executor must re-check vendor paths and schemas against those primary sources before changing each target codec. If a vendor contract has changed, update that target's `module.md` and the vendor-contract note before implementation. Do not silently guess.

## Approved architecture decisions

### D1 — Hooks are a first-class Agent Bundler asset

Agent Bundler owns typed hook normalization, payload copying, target-native manifest generation, capability diagnostics, and deterministic output. A cc-thingz post-build compiler is not an accepted final design.

Rejected option: retain hooks as opaque `native-resource` files. That preserves bytes but cannot prove event, matcher, timeout, blocking, or failure-policy semantics across targets.

### D2 — Use a typed hook descriptor

A hook asset has a typed descriptor in addition to payload files. It is not encoded in arbitrary frontmatter/body fields.

The initial portable contract supports command hooks because that is the concrete cc-thingz need. HTTP, prompt, and agent hook handlers remain future capability cells unless a target task proves and implements them explicitly.

The descriptor includes:

- stable hook ID and source location;
- portable event;
- canonical tool-category matcher;
- ordered command arguments;
- timeout;
- asynchronous/passive execution flag;
- failure policy;
- deterministic order;
- optional target allow-list inherited from the asset.

### D3 — Command arguments distinguish literals from package files

Do not put vendor root variables into the normalized model. Represent command arguments as either literal values or package-file references. Each target adapter renders its own root syntax or safe relative path.

Conceptual shape:

```text
HookDescriptor = {
  event: HookEvent,
  matcher: HookMatcher?,
  handler: HookCommand,
  timeoutMilliseconds: Integer,
  asynchronous: Boolean,
  failurePolicy: open | closed,
  order: Integer
}

HookCommand = {
  mode: exec | shell,
  program: String?,
  arguments: [HookArgument],
  shellCommand: String?
}

HookArgument = literal(String) | package-file(RelativePath)
```

`exec` and `shell` are mutually exclusive. `exec` is the canonical authoring form. `shell` exists only for explicit source declarations and adopted legacy Claude hooks; it carries an advisory or unsupported capability on targets that cannot preserve its semantics.

### D4 — Prefer interpreter invocation but preserve executable intent

Canonical cc-thingz scripts should use `program + arguments`, for example `bash` plus a package-file argument. They must not require a POSIX executable bit to run.

File mode still belongs in the model because native packages and adopted sources may require it. Preserve executable intent through import, overlays, composition, render, provenance, compare, and write. Keep the existing explicit Windows rejection for executable-only output. Interpreter-backed scripts must remain buildable on Windows.

Rejected options:

- clear executable bits silently on Windows — loses source intent;
- require every script to be executable — needlessly breaks Windows builds;
- use an implicit shell for every command — increases injection and quoting risk.

### D5 — Canonical bundle hooks are directory assets

Use this canonical layout:

```text
src/hooks/<hook-id>/hook.json
src/hooks/<hook-id>/<payload files>
src/hooks/<hook-id>/.agentbundler/targets/<target>.json
```

Package manifests list the hook directory. Continue accepting the old exact `src/hooks/<name>.json` form only as a compatibility form for descriptor-only hooks without payload files.

The canonical `hook.json` is strict JSON. Unknown and duplicate fields fail. Payload walks are sorted, contained, and symlink-free.

### D6 — Capability reporting is semantic

Keep `asset.hook`, but do not use it as the only capability. Add exact capability uses for semantics such as:

```text
hook.command.exec
hook.command.shell
hook.event.pre-tool
hook.event.post-tool
hook.event.session-start
hook.event.session-end
hook.event.prompt-submit
hook.event.stop
hook.event.notification
hook.matcher.tool-category
hook.decision.block
hook.decision.rewrite-input
hook.async
hook.failure.closed
```

A target may support some cells and reject others. Unsupported behavior is an error; advisory behavior requires an exact acknowledgment. No target may silently omit a hook or weaken a security decision.

### D7 — Target adapters receive an explicit render request

Replace the bare `render([]NormalizedPackage)` input with a target-neutral render request containing:

- ordered normalized packages;
- common distribution metadata;
- package mode `separate` or `aggregate`;
- explicit aggregate identity and metadata when aggregate mode is selected.

`separate` remains the compatibility default. In source manifest version 1,
`aggregate` is valid only for the Pi package profile; model validation rejects it
for Claude, Codex, Copilot, Cursor, and Grok. Aggregate mode must be declared in
`agentbundle.json`; it is never inferred from package count.

### D8 — Pi uses a hybrid runtime design

The Pi hook runner is:

- a standalone TypeScript implementation with its own tests;
- owned and versioned with the Pi target adapter;
- embedded into the `agbun` binary;
- copied into generated Pi output;
- loaded through one generated thin adapter registered in `package.json#pi.extensions`.

It is not generated as a large Go string template and is not a separately required user installation.

Place it below `internal/target/pi/runtime/` so `internal/target/pi` can embed it without a cross-module generated-copy step. The generated package contains the runtime source and a thin package-specific adapter. Pi's supported TypeScript loader executes it directly.

### D9 — Pi output is one explicit aggregate package

For a composition configured with `packageMode: "aggregate"`, Pi emits one package root with one `package.json`, one hook extension registration, merged skills/agents, and one generated `hooks/hooks.v1.json`.

Rules:

- aggregate identity and metadata are explicit;
- package dependency maps merge only when equal values agree;
- conflicting dependency versions fail;
- duplicate skill, agent, hook, or output paths fail with both source locations;
- the adapter registers exactly one extension entry;
- aggregation only deduplicates within that artifact; it is not advertised as a global cross-package singleton.

### D10 — Other targets render their native plugin forms

- Claude: `.claude-plugin/plugin.json`, `skills/`, `agents/`, `hooks/hooks.json`, payload files.
- Codex: `.codex-plugin/plugin.json`, `skills/`, plugin-level `agents/`, root `hooks.json`, payload files, and optional `.mcp.json` when already modeled.
- Copilot CLI: root `plugin.json`, `skills/`, `agents/*.agent.md`, root `hooks.json`, payload files.
- Cursor: `.cursor-plugin/plugin.json`, `skills/`, agents, `hooks/hooks.json`, payload files.
- Grok: generate a Grok-tested Claude-compatible plugin tree, with Grok-specific hook command root handling. Keep `.grok/skills` as a separate project profile, not as the installable plugin profile.

### D11 — Generate catalogs, but do not publish or install

Agent Bundler may deterministically generate target marketplace/catalog manifests from common source distribution metadata because those are package artifacts. It still does not publish, submit, authenticate, update user configuration, or perform network installation.

Generated catalog paths:

- Claude: `.claude-plugin/marketplace.json`.
- Codex: `.agents/plugins/marketplace.json`.
- Copilot: `.github/plugin/marketplace.json`.
- Cursor: `.cursor-plugin/marketplace.json`.
- Grok: `.claude-plugin/marketplace.json`, using its documented Claude compatibility.
- Pi: no marketplace file; the package root is installable directly through `pi install`.

### D12 — Native verification is safe and offline

`agbun check --native` may invoke only official, offline, non-mutating validators. Initially:

- Claude: `claude plugin validate --strict <root>`.
- Grok: `grok plugin validate <root>`.

Codex, Copilot, Cursor, and Pi lack an equivalent stable non-mutating validator for every required behavior. Their install/load smoke tests run only in test code or repository scripts with temporary HOME/config directories and explicit opt-in. Native verification must never mutate the developer's real configuration.

### D13 — Backward compatibility is explicit

Existing hook-free version-1 manifests and generated package layouts continue to compile. New fields are optional unless hook, aggregate, or marketplace behavior needs them. Increase target format revisions when native output changes. Increase the source manifest version only if strict decoding cannot safely add optional fields under version 1; record that decision in the model module before implementation.

### D14 — The compiler remains deterministic and offline

Generated bytes may depend on source files, explicit manifests, target adapter revision, and embedded runtime bytes. They may not depend on network responses, current time, hostname, Git state, absolute source paths, installed vendor versions, or ambient locale. A fresh checkout may fetch the pinned Pi development tools during `bun install --frozen-lockfile`; after that setup, `agbun build`, `agbun check`, and generated-output tests remain offline.

## Success criteria

- A canonical hook directory imports into a typed descriptor plus payload files with deterministic source locations.
- Hook-free manifests retain their existing output and tests unless a corrected vendor-native path deliberately changes a target format revision.
- File executable intent survives source import, overlays, composition, rendering, provenance, drift comparison, and writing.
- Every selected hook either renders with its declared semantics or fails with an exact target/capability diagnostic.
- Claude output validates with the official strict validator and a hook fire test proves stdin, matcher, timeout, block, and payload resolution behavior.
- Pi output is one installable aggregate package with exactly one registered thin extension, the embedded runtime, and a versioned descriptor; runtime tests prove blocking, input rewriting, timeout, cancellation, output limits, and path containment.
- Codex, Copilot, Cursor, and Grok outputs match current official plugin paths and hook schemas.
- Deterministic marketplace/catalog files point to all generated separate package roots.
- A cc-thingz-shaped multi-package fixture builds and checks all six targets, including hooks and Pi aggregation.
- `agbun check` remains read-only; native checks and optional smoke tests cannot mutate real user configuration.
- Go, TypeScript runtime, lint, vet, architecture, deterministic-build, and vendor-fixture checks pass.

## Development approach

- Complete tasks in order. Each task is independently reviewable and must leave focused tests green.
- Update normative `module.md` contracts before behavior-bearing code when implementation reveals a missing or incorrect contract.
- Use standard-library Go and the existing YAML dependency. Do not add a Go dependency for schema, template, or process behavior that can be implemented clearly with the standard library.
- Use Bun only for the Pi runtime development tests. Generated Pi packages must not require Bun; Pi loads their TypeScript through its supported extension loader.
- Do not perform a big-bang rewrite of `packageoutput`. Add narrow hook and aggregation seams, preserving existing skill/agent output tests.
- Do not weaken compiler validation to make a target pass. Add a precise capability rule or an explicit diagnostic.
- Do not add publishing, marketplace submission, credentials, network fetching, or live user installation to production code.
- Runtime development dependencies are installed from a committed lockfile in the Pi runtime directory; generated builds remain offline and contain no development dependencies.
- If a vendor contract cannot be verified, stop that target task, document the blocker, and leave its capability unsupported.
- Run `gitnexus detect-changes --scope unstaged` after each task and inspect unexpected upstream impact before proceeding.
- Keep the plan structure stable while RalphEx is executing. Mark checkboxes only; record unexpected scope as a blocker in the task result rather than adding hidden work.

## Testing strategy

- Go unit tests cover model validation, import, composition, renderers, paths, collisions, deterministic serialization, and diagnostics.
- Shared golden fixtures prove exact native package trees and catalog bytes.
- The Pi runtime uses Bun tests with a fake Pi extension API and fake process boundary. Test both success and failure behavior.
- Cross-language contract fixtures are parsed by Go and TypeScript tests to prevent descriptor drift.
- Vendor smoke tests use temporary roots and skip with an explicit reason when a CLI is absent. They are opt-in and separate from the normal fast test suite.
- Native validator declarations are tested without invoking real processes; artifact native-verification integration tests use fake executables.
- Full acceptance runs race-safe Go tests, TypeScript tests, formatting, lint, vet, architecture checks, deterministic two-root builds, and optional installed-CLI smokes.

## Validation commands

Run these from the repository root. Focused subsets are repeated inside tasks.

```bash
changed_go=$(git diff --name-only -- '*.go')
test -z "$changed_go" || gofmt -d $changed_go
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
(cd internal/target/pi/runtime && bun install --frozen-lockfile && bun run typecheck && bun test)
scripts/check-architecture
archfit check --config .archfit.yaml --require-tools --progress none
gitnexus detect-changes --scope unstaged
```

Optional vendor smoke command to add during this plan:

```bash
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
```

## Implementation steps

### Task 1: Align normative architecture and pin vendor contracts

Justification: Decisions D1–D14 conflict with current root, model, target, bundle, and Pi module invariants. The repository treats `module.md` as normative; code must not silently diverge from it.

Chosen approach: update the existing fractal module tree and add one concise vendor-contract note before production changes.

Rejected option: implement first and repair module docs later. That would temporarily authorize contradictory behavior and hide target-specific assumptions from RalphEx workers.

Files:

- `module.md` — allow a target-owned embedded runtime payload while keeping the compiler offline and self-contained; allow deterministic catalog generation but continue forbidding publication and installation.
- `docs/tech-stack.md` — add the Pi TypeScript runtime island, Bun test command, Pi loader compatibility, and no-runtime-dependency rule.
- `docs/vendor-package-contracts.md` — record verified paths, supported hook events, validation commands, source URLs, and the 2026-07-15 access date for all six targets.
- `.archfit.yaml` — enable TypeScript analysis; keep the runtime inside the existing `target` module boundary.
- `internal/compiler/model/module.md`, `internal/compiler/source/module.md`, `internal/compiler/source/bundle/module.md`, `internal/compiler/source/claudeplugin/module.md`, `internal/compiler/composition/module.md`, `internal/compiler/module.md` — specify typed hooks, file metadata, distribution render input, and propagation.
- `internal/target/module.md`, `internal/target/packageoutput/module.md`, and all six target `module.md` files — specify hook capability cells, package modes, target paths, runtime ownership, and validation boundaries.
- `internal/artifact/module.md`, `internal/artifact/nativeverify/module.md`, `internal/artifact/provenance/module.md`, `internal/artifact/compare/module.md`, `internal/artifact/write/module.md` — align executable and native-check behavior.

Preconditions: current architecture gate passes before edits; vendor source URLs are accessible or locally captured evidence is available.

Postconditions: module contracts agree on one model, target interface, Pi runtime boundary, and output ownership. No production code changes occur in this task.

Fitness gate: `scripts/check-architecture` must pass. TypeScript is analyzed as part of the existing `target` module; no new cross-layer edge is allowed.

Impact commands:

```bash
gitnexus impact RenderWithCodec --direction upstream --depth 3 --include-tests
gitnexus impact Compile --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
scripts/check-architecture
archfit check --config .archfit.yaml --require-tools --progress none
go test ./...
```

Manual checks:

- Confirm each target contract cites a primary official source, not a blog or inferred format.
- Confirm the root module still forbids network publishing, installation, and runtime dependence on the `agbun` executable.

- [x] Update the root, compiler, source, composition, target, artifact, and target-leaf module contracts to encode D1–D14 exactly.
- [x] Add `docs/vendor-package-contracts.md` with native paths, hook event/decision limits, catalog paths, and validator availability for all six targets.
- [x] Update `docs/tech-stack.md` and `.archfit.yaml` for the contained TypeScript runtime and Bun-only development checks.
- [x] Run the architecture and full Go tests; resolve every contract or dependency-direction failure before Task 2.

### Task 2: Add typed hook and file-content model values

Justification: D2–D6. `AssetContent.Files` and `FilePatch` currently lose executable intent, and hook semantics are stored in unvalidated generic fields.

Chosen approach: add small concrete model records while preserving `AssetKindHook` and existing asset content for payloads.

Rejected options:

- replace the entire asset model with a generic sum type — too broad for the concrete hook gap;
- keep hooks in frontmatter/body — cannot validate mutually exclusive command forms or semantic capabilities;
- attach modes only to `PlannedFile` — loses intent before target rendering.

Files:

- `internal/compiler/model/types.go` — add `FileContent`, typed hook enums/records, `Hook *HookDescriptor` on source and normalized assets, and executable-aware file patches.
- `internal/compiler/model/validation.go` and `normalized_validation.go` — validate hook-only fields, command form exclusivity, paths, timeouts, async restrictions, ordering, and deterministic uniqueness.
- `internal/compiler/model/json.go` — strict decode for new optional manifest/model fields where applicable.
- `internal/compiler/model/model_test.go` — table-driven success, invalid, boundary, and deterministic-order tests.
- `internal/compiler/model/module.md` — update only if implementation reveals a missing invariant; do not weaken the approved contract.

Preconditions: Task 1 contracts pass architecture checks.

Postconditions: the model can represent a command hook, literal/package-file arguments, payload executable intent, and precise invalid states without filesystem or target knowledge.

Fitness gate: model remains the innermost module and imports no sibling. `no_layer_back_edges` and `public_api_only` must pass.

Impact commands:

```bash
gitnexus impact AssetContent --direction upstream --depth 4 --include-tests
gitnexus impact SourceAsset --direction upstream --depth 4 --include-tests
gitnexus impact ValidateNormalizedPackage --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler/model
go test ./internal/compiler/... ./internal/target/... ./internal/artifact/...
scripts/check-architecture
```

Manual checks:

- Confirm no target names or vendor root variables appear in the normalized hook types.
- Confirm invalid shell/exec combinations fail with stable diagnostic codes.

- [x] Implement `FileContent` and migrate base/overlay file records to carry bytes plus executable intent.
- [x] Implement typed portable hook events, canonical tool categories, handler modes, argument kinds, failure policy, timeout, async, and order fields.
- [x] Add aggregate validation for hook-kind/descriptor consistency, package-file containment, duplicate arguments/paths where relevant, timeout bounds, and async blocking-event rejection.
- [x] Add model tests covering valid exec and explicit shell hooks, malformed descriptors, path escape, zero/negative timeout, incompatible flags, and deterministic ordering.
- [x] Run focused and cross-package tests; fix compile failures by adapting callers without adding rendering behavior yet.

### Task 3: Add explicit target render and distribution configuration

Justification: D7, D9, and D11. Package aggregation and target-wide catalogs need metadata that does not belong in one arbitrary normalized package.

Chosen approach: add a target-neutral `TargetRenderInput` plus explicit source distribution and package-mode configuration. `separate` is the compatibility default.

Rejected options:

- infer aggregation from package count — current multi-package separate output is valid and must remain valid;
- merge metadata from the first package — order-dependent and surprising;
- let each adapter reread `agentbundle.json` — violates adapter purity and module boundaries.

Files:

- `internal/compiler/model/types.go`, `json.go`, and validation files — add `DistributionMetadata`, `TargetPackageMode`, explicit aggregate package declaration, and `TargetRenderInput`.
- `internal/compiler/model/model_test.go` — strict decode and relational validation tests.
- `internal/compiler/compiler.go` and `compiler_test.go` — construct one render input per selected target from manifest, composition, and normalized packages.
- `internal/target/target.go` and `target_test.go` — change the adapter render boundary to consume the render input.
- `cmd/agbun/main_test.go` and manifest fixtures — preserve version-1 hook-free compatibility and add representative distribution examples.
- `docs/configuration.md` — document the exact JSON fields with separate and aggregate examples.

Preconditions: typed model from Task 2 is green.

Postconditions: adapters receive ordered packages, common distribution metadata, and an explicit separate/aggregate instruction without source filesystem access; model validation rejects aggregate mode for non-Pi targets.

Fitness gate: only the compiler orchestrator combines source manifest data with composed packages. Target leaves consume model values and remain pure.

Impact commands:

```bash
gitnexus impact Render --file internal/target/target.go --direction upstream --depth 4 --include-tests
gitnexus impact Compile --file internal/compiler/compiler.go --direction downstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler/model ./internal/compiler ./internal/target ./cmd/agbun
go test ./...
scripts/check-architecture
```

Manual checks:

- Confirm existing manifests without distribution fields still decode and render in separate mode.
- Confirm aggregate mode cannot exist without explicit aggregate identity and metadata.

- [x] Add strict source/distribution JSON fields and model validation, choosing optional version-1 fields unless strict compatibility proves a version bump is required.
- [x] Add `TargetRenderInput` and migrate the adapter/registry/compiler boundary from bare package slices.
- [x] Implement deterministic package ordering and explicit validation for aggregate identity, metadata, and dependency conflicts.
- [x] Add backward-compatibility, unknown-field, duplicate-key, missing-aggregate, and deterministic-render-input tests.
- [x] Update configuration docs and run compiler/target/full checks before Task 4.

### Task 4: Import canonical bundle hook directories and file modes

Justification: D4 and D5. cc-thingz hooks require scripts and support files; an exact descriptor file cannot own them safely.

Chosen approach: import a hook directory containing strict `hook.json` plus contained payload files. Retain descriptor-only exact JSON compatibility.

Rejected options:

- glob arbitrary executable files from `src/hooks` — package ownership becomes implicit;
- embed script content in JSON — poor authoring, no binary/support-file path, and mode loss;
- follow symlinks — breaks containment and reproducibility.

Files:

- `internal/compiler/source/bundle/bundle.go` — classify directory hooks, decode strict descriptors, walk payloads, capture source locations and executable intent, and compute inputs.
- `internal/compiler/source/bundle/bundle_test.go` — canonical, compatibility, mode, symlink, duplicate, path, and deterministic tests.
- `internal/compiler/source/frontmatter/` only if a shared strict decoder is genuinely reusable; do not force hook JSON through Markdown frontmatter.
- `testdata/` or importer-local fixtures — minimal hook packages with Bash/Python payloads and support files.
- `internal/compiler/source/bundle/module.md` — keep canonical layout synchronized.

Preconditions: Tasks 2–3 define hook and file model values.

Postconditions: a listed hook directory becomes exactly one typed `SourceAsset`, and every owned payload is represented once with deterministic paths and input hashes.

Fitness gate: source importer depends only on model/frontmatter helpers and filesystem boundaries; it must not import composition, target, or artifact code.

Impact commands:

```bash
gitnexus impact Inspect --file internal/compiler/source/bundle/bundle.go --direction upstream --depth 3 --include-tests
gitnexus impact readAsset --file internal/compiler/source/bundle/bundle.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler/source/bundle ./internal/compiler/source
go test ./internal/compiler/...
scripts/check-architecture
```

Manual checks:

- Inspect a fixture plan and confirm payload paths are asset-relative, not workspace-absolute.
- On POSIX, confirm an executable fixture retains intent; confirm interpreter-backed scripts also work when not executable.

- [x] Implement canonical `src/hooks/<id>/hook.json` import and descriptor-only exact JSON compatibility.
- [x] Import payload files with bytes, executable intent, sorted traversal, input hashes, source locations, containment, and symlink rejection.
- [x] Convert descriptor semantics into exact capability uses rather than only `asset.hook`.
- [x] Add success and failure fixtures for exec, shell compatibility, payload references, mode handling, unknown fields, duplicate keys, missing files, traversal, and symlinks.
- [x] Run source-focused and architecture checks before Task 5.

### Task 5: Normalize adopted Claude plugin hooks without unsafe command parsing

Justification: D3, D10, and D13. Existing Claude plugin ingestion recognizes command strings, but official packages use native hook files and may reference scripts.

Chosen approach: import the official Claude hook location and retain legacy shell commands explicitly. Resolve package files only for syntactically simple, unambiguous plugin-root references; otherwise preserve the command as shell mode and attach the appropriate semantic capability.

Rejected options:

- parse arbitrary shell into argv — unsafe and incorrect;
- drop unresolvable script references — silent semantic loss;
- copy the whole plugin tree as hook payload — hides ownership and creates collisions.

Files:

- `internal/compiler/source/claudeplugin/claudeplugin.go` and `helpers.go` — read `.claude-plugin/plugin.json` hook path/default and `hooks/hooks.json`, map events/matchers/timeouts, import simple referenced payloads, and preserve shell form.
- `internal/compiler/source/claudeplugin/claudeplugin_test.go` — official-layout fixtures, legacy compatibility, payload detection, unresolvable command diagnostics, and no-source-write tests.
- `internal/compiler/source/claudeplugin/module.md` — correct the native hook source contract and mapping.

Preconditions: canonical typed hook model and file import exist.

Postconditions: adopted Claude command hooks round-trip to Claude without reinterpretation and expose precise portability limits to other targets.

Fitness gate: importer remains target-format-aware only at the adoption boundary and produces target-neutral model values/capabilities.

Impact commands:

```bash
gitnexus impact InspectClaudePlugin --direction upstream --depth 3 --include-tests
gitnexus impact hooks --file internal/compiler/source/claudeplugin/claudeplugin.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler/source/claudeplugin ./internal/compiler/source
go test ./internal/compiler/...
scripts/check-architecture
```

Manual checks:

- Compare fixtures with the official OpenAI/Claude package layouts recorded in `docs/vendor-package-contracts.md`.
- Confirm arbitrary shell remains explicit shell mode and is never presented as safe exec mode.

- [x] Correct Claude plugin hook discovery to the official manifest/default hook file contract.
- [x] Map supported Claude events, matchers, timeout, async, and command forms into typed descriptors with exact capabilities.
- [x] Import only statically provable package-file references and preserve other commands as explicit shell compatibility.
- [x] Add round-trip and portability-diagnostic tests for official, legacy, simple-script, complex-shell, malformed, and source-ownership cases.
- [x] Run importer, compiler, and architecture checks before Task 6.

### Task 6: Preserve hook/file semantics through overlays and composition

Justification: D4, D6, and D13. A correct importer is insufficient if target overlays or composition drop mode, descriptor, or capability information.

Chosen approach: extend the existing copy-on-compose path and overlay validation; do not introduce a second hook-specific composition engine.

Rejected option: apply target hook translations during composition. Vendor event names and output paths belong to target adapters.

Files:

- `internal/compiler/composition/composition.go` — clone descriptors/files, apply executable-aware patches, preserve origins, and resolve target allow-lists/capabilities.
- `internal/compiler/composition/composition_test.go` — mode changes, hook descriptor preservation, exclusion/replacement, acknowledgment, and collision cases.
- `internal/compiler/model/validation.go` — relational checks found necessary during composition.
- bundle/Claude overlay decoders — accept executable metadata in file patches using one shared strict shape.

Preconditions: both source importers emit typed hooks.

Postconditions: every `NormalizedAsset` hook is complete, target-filtered, and capability-checked; overlays cannot create invalid handlers or escape asset roots.

Fitness gate: composition still imports neither source nor target packages. Existing `composition_no_source`, `composition_no_target`, and `composition_no_artifact` rules pass.

Impact commands:

```bash
gitnexus impact Compose --direction upstream --depth 4 --include-tests
gitnexus impact applyOverlay --file internal/compiler/composition/composition.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler/composition ./internal/compiler/model
go test ./internal/compiler/...
scripts/check-architecture
```

Manual checks:

- Confirm a target overlay can replace a payload and its executable intent but cannot rewrite a hook into an invalid descriptor.
- Confirm unsupported security behavior fails before rendering.

- [x] Migrate content/file cloning and overlay application to `FileContent` without aliasing byte slices.
- [x] Preserve typed descriptors, source locations, target allow-lists, deterministic order, and capability uses during composition.
- [x] Implement executable-aware overlay JSON and filesystem patch precedence with strict validation.
- [x] Add tests for target exclusion, replacement policy, exact acknowledgments, descriptor/file preservation, collisions, and invalid overlay combinations.
- [x] Run composition, compiler, race, and architecture checks before Task 7.

### Task 7: Add shared hook payload and renderer seams

Justification: D3, D4, D6, and D10. Target codecs need common collision-safe payload handling but must own vendor manifests and event semantics.

Chosen approach: extend `packageoutput` with target-owned hook callbacks and shared payload/file helpers. Keep vendor serialization in each target leaf.

Rejected options:

- one universal hooks JSON serializer — vendor schemas and decisions differ;
- duplicate payload copying in six adapters — high drift and collision risk;
- target adapter writes files directly — violates declarative target plans.

Files:

- `internal/target/packageoutput/codec.go` — add narrow hook render callback/contract and package-mode inputs needed by shared output.
- `internal/target/packageoutput/packageoutput.go` — render hook payloads, propagate executable flags, collect typed hook assets for codec serialization, and enforce deterministic collision rules.
- `internal/target/packageoutput/packageoutput_test.go` — mixed assets, hooks, executable/non-executable files, duplicate paths, stable package roots, unsupported semantics, and deterministic ordering.
- `internal/target/packageoutput/module.md` — document the division between shared file mechanics and vendor hook semantics.

Preconditions: composed typed hooks are available.

Postconditions: a target codec can render one native hook manifest and payload set without duplicating shared path/mode/collision logic.

Fitness gate: shared packageoutput remains under `internal/target`, imports only model/stdlib, and does not contain vendor event names.

Impact commands:

```bash
gitnexus impact RenderWithCodec --direction upstream --depth 4 --include-tests
gitnexus impact renderAsset --file internal/target/packageoutput/packageoutput.go --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/packageoutput ./internal/target/...
go test ./...
scripts/check-architecture
```

Manual checks:

- Inspect shared code for Claude/Codex/Cursor/Pi/Grok strings; there should be none.
- Confirm `PlannedFile.Executable` is set only from explicit `FileContent` intent.

- [x] Add a narrow target hook-render callback and immutable hook input view to the shared codec contract.
- [x] Render hook payload files with stable package roots, origin propagation, executable intent, and duplicate-path diagnostics.
- [x] Keep native hook manifest bytes and event mapping entirely target-owned while sorting shared hook inputs deterministically.
- [x] Add focused tests for mixed packages, executable propagation, payload containment, collisions, duplicate hook IDs, and deterministic plans.
- [x] Run shared renderer, all target, artifact, and architecture checks before Task 8.

### Task 8: Implement the Claude hook vertical slice

Justification: D10 and the staged-risk decision. Claude is the closest existing source/target contract and has an official strict validator.

Chosen approach: render native `hooks/hooks.json`, package payloads, and one validator declaration per plugin or marketplace root.

Rejected option: enable all targets in one change. That would make event and decision mismatches hard to isolate.

Files:

- `internal/target/claude/codec.go` and `claude.go` — native capability rules, event/tool mapping, command rendering, manifest hook declaration, root references, format revision, and native checks.
- `internal/target/claude/claude_test.go` — exact golden package trees and capability failures.
- `internal/target/claude/testdata/` — official-shaped plugin fixtures and expected JSON.
- `internal/target/claude/module.md` — final verified Claude contract.
- `internal/artifact/nativeverify/` tests if multiple working-directory checks reveal a generic defect.

Preconditions: shared hook renderer passes.

Postconditions: Claude package-profile hooks render losslessly; unsupported handler/event cells fail; hook-free output remains stable except the declared format revision.

Fitness gate: Claude adapter stays pure. Native validation is emitted as `NativeCheck`, never invoked during render.

Impact commands:

```bash
gitnexus impact Render --file internal/target/claude/claude.go --direction upstream --depth 4 --include-tests
gitnexus impact capabilities --file internal/target/claude/claude.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/claude ./internal/target/packageoutput ./internal/target
go test ./internal/compiler/... ./internal/artifact/...
command -v claude >/dev/null && claude plugin validate --strict internal/target/claude/testdata/plugin-golden
scripts/check-architecture
```

Manual checks:

- Compare generated event names, matcher regexes, timeout units, async fields, exit/decision semantics, and root variables with the pinned Claude docs.
- Confirm security hooks configured fail-closed are not rendered fail-open.

- [x] Declare exact Claude semantic capability states and increment the Claude format revision.
- [x] Render `.claude-plugin/plugin.json`, `hooks/hooks.json`, payload paths, package-root command arguments, and deterministic hook order.
- [x] Preserve adopted explicit shell commands for Claude while preferring exec/interpreter form for canonical hooks.
- [x] Emit safe official strict-validator checks for each generated Claude plugin/catalog scope.
- [x] Add golden, hook-free regression, unsupported-cell, collision, async, timeout, matcher, and command-root tests.
- [x] Run the focused tests and validate a generated fixture with the installed Claude CLI before Task 9.

### Task 9: Prove Claude end-to-end behavior and checkpoint the foundation

Justification: the model/import/composition/renderer chain must be proven before adding a second target. Green unit tests alone may still preserve the wrong stdin or decision protocol.

Chosen approach: add a hermetic fixture hook that records its input and returns allow/deny/rewrite outputs, plus an optional installed-CLI smoke test behind `vendor_smoke`.

Rejected option: depend on a live model session. Hook packaging can be tested without network or inference.

Files:

- `internal/compiler/testdata/hooks-claude/` — complete source manifest/package/hook fixture.
- `internal/compiler/compiler_test.go` or a focused integration test file — source-to-target plan and deterministic two-root tests.
- `internal/target/claude/*_smoke_test.go` — opt-in local validation/fire harness using temporary HOME/config where the CLI permits it.
- `docs/vendor-package-contracts.md` — record any verified correction found by the smoke.

Preconditions: Task 8 focused tests and official validator pass.

Postconditions: one source hook is proven through import, composition, native output, official validation, stdin decoding, matcher selection, timeout, and blocking result.

Fitness gate: smoke helpers remain test-only. Production compilation stays network-, process-, and environment-free.

Impact commands:

```bash
gitnexus impact Compile --file internal/compiler/compiler.go --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler ./internal/target/claude
go test -race ./internal/compiler ./internal/target/claude
go test -tags=vendor_smoke ./internal/target/claude
scripts/check-architecture
```

Manual checks:

- Review the generated tree and captured stdin/decision output.
- If the vendor CLI cannot fire a hook without a model/network call, retain official validation plus a protocol-level subprocess test and document that limitation rather than faking a pass.

- [x] Add a complete source-to-Claude fixture with interpreter-backed allow, deny, and input-rewrite hook cases.
- [x] Assert exact generated paths/bytes, executable intent, provenance origins, and byte equality across two absolute workspace roots.
- [x] Add a test-only subprocess protocol harness for stdin, stdout decisions, exit status, timeout, and output limits.
- [x] Add an opt-in installed-Claude validation/fire smoke when it can remain offline and isolated; otherwise document the exact unavailable boundary.
- [x] Run full Go, race, lint, vet, and architecture checks as the first implementation checkpoint.

### Task 10: Extract and harden the standalone Pi hook runtime

Justification: D8. Pi exposes lifecycle interception through TypeScript extensions rather than declarative hook manifests. The runtime must be independently testable but bundled with generated output.

Chosen approach: place a small TypeScript runtime under the Pi adapter, seeded from cc-thingz's existing runner behavior, with no runtime npm dependencies.

Rejected options:

- separately require users to install a Pi hook package — Pi does not recursively load dependency extensions and local packages may not install dependencies;
- generate the whole runtime from Go templates — difficult to test and version;
- use the Pi SDK — it embeds sessions and is not the package extension API.

Files:

- `internal/target/pi/runtime/package.json`, `internal/target/pi/runtime/bun.lock`, and `internal/target/pi/runtime/tsconfig.json` — exact pinned TypeScript development tooling, Bun tests, and strict typecheck only; runtime dependencies remain empty. Add a `typecheck` script so validation never uses `bunx`.
- `internal/target/pi/runtime/src/index.ts` — factory registering Pi lifecycle handlers.
- focused runtime modules for schema decode, event mapping, matcher evaluation, dispatch, process execution, and result translation; keep files cohesive rather than one large port.
- `internal/target/pi/runtime/test/` — fake Pi API/process tests.
- `internal/target/pi/runtime/testdata/` — shared versioned descriptor fixtures consumed by both TypeScript and Go tests.
- `.archfit.yaml` — adjust only if TypeScript analysis reveals a real module-boundary issue.

Preconditions: normalized descriptor schema is stable through the Claude checkpoint.

Postconditions: the runtime can load schema v1 and translate supported Pi events without package discovery or global filesystem scanning.

Fitness gate: runtime stays inside the `target` module and cannot import Go/compiler/source concerns. TypeScript architecture analysis passes.

Impact commands:

```bash
gitnexus impact Render --file internal/target/pi/pi.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
(cd internal/target/pi/runtime && bun install --frozen-lockfile && bun run typecheck && bun test)
scripts/check-architecture
go test ./internal/target/pi
```

Manual checks:

- Compare event use with installed Pi `docs/extensions.md`, including sequential `tool_call` preflight, `ctx.signal`, and idempotent `session_shutdown`.
- Confirm the runtime scans no `~/.pi/agent/git` or npm directories; config is injected by the generated adapter.

- [x] Create the dependency-free TypeScript runtime package, pin the exact TypeScript dev dependency in `package.json` and `bun.lock`, add the `typecheck` script, and implement schema-v1 decoding with explicit unknown-version failure.
- [x] Implement Pi event mappings for session, prompt, tool call/result, turn/agent end, and compaction events supported by the portable model.
- [x] Implement matcher evaluation, exec/shell dispatch, package-file resolution, bounded stdout/stderr, timeout, cancellation, and safe process termination.
- [x] Translate pre-tool allow/deny/input-rewrite and passive-event results without mutating unvalidated tool input.
- [x] Add tests for every supported event, ordering, fail-open/fail-closed behavior, malformed config/output, path traversal, timeout, cancellation, output limits, and idempotent shutdown.
- [x] Run Bun, TypeScript, Go Pi, and architecture checks before Task 11.

### Task 11: Render one aggregate Pi package with the bundled runtime

Justification: D8 and D9. Generated cc-thingz Pi packages need one loadable extension and one coherent package root, not one runner copy per logical package.

Chosen approach: the Pi adapter consumes explicit aggregate mode, merges allowed package content, embeds runtime source bytes, emits one thin adapter, and registers it in `package.json#pi.extensions`.

Rejected options:

- emit a runner into every logical package — duplicate handler registration;
- rely on npm runtime dependency loading — Pi only loads top-level package extension declarations;
- infer aggregate metadata from the first package — nondeterministic ownership.

Files:

- `internal/target/pi/runtime_embed.go` — `go:embed` only the reviewed runtime source files and expose deterministic embedded file entries inside the Pi package.
- `internal/target/pi/codec.go` and `pi.go` — aggregate merge, hook descriptor JSON, thin adapter, `pi.extensions`, skills, agents, dependencies, format revision, and capabilities.
- `internal/target/pi/pi_test.go` — package.json, aggregate conflicts, one-extension registration, embedded runtime bytes/hash, and deterministic output.
- `internal/target/pi/testdata/` — expected aggregate package tree and shared schema fixtures.
- `internal/target/packageoutput/` only for a generic aggregate seam; keep Pi metadata and runtime paths in the Pi leaf.

Preconditions: runtime tests pass and target render input supports explicit aggregate mode.

Postconditions: one Pi target root is directly installable and contains exactly one thin hook adapter plus the embedded runtime and schema-v1 hook configuration.

Fitness gate: Go embeds only files below `internal/target/pi`; no generated source-copy step or cross-module import is introduced.

Impact commands:

```bash
gitnexus impact Render --file internal/target/pi/pi.go --direction upstream --depth 4 --include-tests
gitnexus impact manifest --file internal/target/pi/codec.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/pi ./internal/target/packageoutput ./internal/target
(cd internal/target/pi/runtime && bun install --frozen-lockfile && bun run typecheck && bun test)
go test ./...
scripts/check-architecture
```

Manual checks:

- Inspect generated `package.json` against installed Pi `docs/packages.md`.
- Confirm only the thin adapter is listed in `pi.extensions`; helper runtime modules are imported, not independently discovered.

- [x] Embed the runtime source deterministically and emit it under one private generated runtime directory.
- [x] Implement explicit Pi aggregate package merging with identity, metadata, dependency, asset-name, and path conflict diagnostics.
- [x] Emit `hooks/hooks.v1.json`, one thin package-specific adapter, and exactly one `pi.extensions` registration.
- [x] Preserve skills and registered package agents, including the `pi-subagents` dependency only when agents are present.
- [x] Add Go/TypeScript shared contract fixtures and tests proving generated descriptors are accepted identically by both implementations.
- [x] Run focused, full, TypeScript, and architecture checks before Task 12.

### Task 12: Prove Pi install, load, and hook behavior

Justification: Pi packaging can look correct while failing extension discovery, TypeScript loading, or event translation.

Chosen approach: test package installation/listing with isolated Pi configuration, test real extension loading without a live model where possible, and use the runtime fake API for behavior that requires an active turn.

Rejected option: install into the developer's normal `~/.pi/agent` directory.

Files:

- `internal/target/pi/pi_smoke_test.go` — opt-in installed-Pi smoke that creates `t.TempDir()` and applies `t.Setenv("PI_CODING_AGENT_DIR", ...)` inside the test.
- `internal/compiler/testdata/hooks-pi/` — aggregate multi-package source fixture.
- `internal/compiler/compiler_test.go` — source-to-Pi plan, deterministic output, and check read-only tests.
- runtime tests — add only missing behavior discovered by real loader tests.
- `docs/vendor-package-contracts.md` — record exact supported smoke boundary.

Preconditions: aggregate package renders and Bun tests pass.

Postconditions: Pi can install/list the local aggregate package, load the generated TypeScript adapter, and execute the supported hook protocol without duplicate registration.

Fitness gate: installed-CLI operations are test-only, use temporary config/project paths, and restore no global state because none is touched.

Impact commands:

```bash
gitnexus impact Compile --file internal/compiler/compiler.go --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/compiler ./internal/target/pi
bun test internal/target/pi/runtime
go test -tags=vendor_smoke ./internal/target/pi
go test -race ./internal/compiler ./internal/target/pi
scripts/check-architecture
```

Manual checks:

- Confirm the smoke process leaves the normal Pi settings and package directories unchanged.
- Confirm Pi loader errors include generated adapter/runtime paths and schema diagnostics.

- [x] Add an aggregate multi-package Pi fixture with skills, agents, pre-tool hooks, passive hooks, and conflicting dependency negative cases.
- [x] Add isolated `pi install -l`/`pi list` package-discovery smoke coverage; the test itself must create `t.TempDir()`, set `PI_CODING_AGENT_DIR` with `t.Setenv`, and assert no real config path changed.
- [x] Add a real extension-loader import smoke and prove one registration; keep active-turn behavior in the deterministic fake-runtime tests when a model would otherwise be required.
- [x] Prove pre-tool deny and input rewrite, passive post-tool dispatch, timeout/cancellation, and schema mismatch behavior through the combined Go/TypeScript fixtures.
- [x] Run the full validation suite as the Pi checkpoint before enabling remaining targets.

### Task 13: Implement Codex-native hooks and plugin agent paths

Justification: D10. Current Codex package output does not yet render root `hooks.json`, and plugin-level agent discovery must follow the current official plugin contract rather than project `.codex/agents` assumptions.

Chosen approach: implement Codex as its own serializer using shared payload mechanics. Use root-relative script commands as shown by official OpenAI plugin fixtures.

Rejected option: copy Claude hook bytes. Codex supports overlapping events but has separate trust, concurrency, plugin manifest, and agent conventions.

Files:

- `internal/target/codex/codec.go` and `codex.go` — manifest component paths, plugin agent root, hook schema, semantic capabilities, command roots, and format revision.
- `internal/target/codex/codex_test.go` and `testdata/` — official-shaped golden fixtures and capability failures.
- `internal/target/codex/module.md` — verified native contract and the absence of a safe production native validator.
- optional test-only smoke file — local marketplace/add/list flow with temporary `CODEX_HOME` if the installed CLI supports it without network.

Preconditions: Claude and Pi checkpoints are green; vendor docs are rechecked.

Postconditions: separate Codex packages are installable plugin roots with root `hooks.json`, correct plugin agents, skills, and no silently weakened hook semantics.

Fitness gate: no Codex-specific names enter shared packageoutput or model code.

Impact commands:

```bash
gitnexus impact Render --file internal/target/codex/codex.go --direction upstream --depth 4 --include-tests
gitnexus impact agent --file internal/target/codex/codec.go --direction upstream --depth 3 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/codex ./internal/target/packageoutput ./internal/target
go test ./...
CODEX_HOME="$(mktemp -d)" go test -tags=vendor_smoke ./internal/target/codex
scripts/check-architecture
```

Manual checks:

- Compare paths and event/decision behavior against the pinned official Codex docs and `openai/plugins` fixtures.
- Confirm automatic native checks do not mutate Codex configuration; local install/list remains opt-in test code.

- [x] Correct Codex plugin manifest component and plugin-agent paths, preserving project-profile behavior separately.
- [x] Map only verified portable events, matchers, command forms, blocking/input-rewrite semantics, timeout, and failure policy to root `hooks.json`.
- [x] Declare unsupported/advisory capability cells exactly and increment the Codex format revision.
- [x] Add golden, hook-free regression, collision, unsupported-event, shell, trust-boundary, and deterministic multi-package tests.
- [x] Add an isolated optional local marketplace/install/list smoke when supported, otherwise document the missing official validator boundary.
- [x] Run focused/full/architecture checks before Task 14.

### Task 14: Implement GitHub Copilot CLI-native hooks and packages

Justification: D10. Copilot CLI supports plugins, agents, skills, and hook configurations but has target-specific failure and timeout behavior.

Chosen approach: render root `plugin.json` and `hooks.json`, use `${PLUGIN_ROOT}` where documented, and diagnose any failure-policy mismatch.

Rejected option: claim compatibility from Claude-form event names alone. Copilot timeouts and command failures have distinct fail-open/fail-closed behavior.

Files:

- `internal/target/copilot/codec.go` and `copilot.go` — native manifest fields, hooks path, root references, capability rules, and revision.
- `internal/target/copilot/copilot_test.go` and `testdata/` — official-shaped packages and failure-policy cases.
- `internal/target/copilot/module.md` — CLI scope and explicit cloud-agent limitation.
- optional `copilot_smoke_test.go` — direct local install/list with temporary HOME/config.

Preconditions: shared renderer is stable and Copilot docs are rechecked.

Postconditions: Copilot CLI package roots load their agents, skills, and hooks; cloud coding-agent behavior is not falsely claimed.

Fitness gate: test-only install uses a temporary HOME. Production native checks remain empty unless an official non-mutating validator is found.

Impact commands:

```bash
gitnexus impact Render --file internal/target/copilot/copilot.go --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/copilot ./internal/target/packageoutput ./internal/target
go test ./...
HOME="$(mktemp -d)" go test -tags=vendor_smoke ./internal/target/copilot
scripts/check-architecture
```

Manual checks:

- Verify target scope says Copilot CLI; installed user plugins are not promised for GitHub-hosted cloud coding agents.
- Verify timeout/failure diagnostics do not present fail-open behavior as equivalent to requested fail-closed security hooks.

- [x] Render root `plugin.json`, native agents/skills paths, root `hooks.json`, payload files, and documented root references.
- [x] Map verified PascalCase/Claude-compatible events while preserving Copilot-specific timeout and failure semantics through capability states.
- [x] Increment the Copilot format revision and add exact unsupported/advisory diagnostics.
- [x] Add golden, hook-free regression, fail-policy, timeout, matcher, collision, and deterministic package tests.
- [x] Add an isolated optional direct-install/list smoke and prove no normal Copilot configuration is touched.
- [x] Run focused/full/architecture checks before Task 15.

### Task 15: Implement Cursor-native hooks and plugin packages

Justification: D10. Cursor uses `.cursor-plugin/plugin.json` and camelCase event-specific hooks; it cannot be represented by only a Claude regex matcher translation.

Chosen approach: map portable event/tool categories to Cursor's native event families and reject semantics Cursor cannot preserve.

Rejected option: emit Claude `hooks/hooks.json` unchanged. Cursor reads some third-party hook forms but native plugin output should follow the documented Cursor contract.

Files:

- `internal/target/cursor/codec.go` and `cursor.go` — native manifest/components, event mapping, command roots, capabilities, and revision.
- `internal/target/cursor/cursor_test.go` and `testdata/` — fixture based on the official Cursor plugin template.
- `internal/target/cursor/module.md` — verified IDE/CLI packaging and smoke boundary.
- optional `cursor_smoke_test.go` — local plugin-dir or temporary local-plugin load when a noninteractive command exists.

Preconditions: Cursor docs/template are rechecked.

Postconditions: Cursor plugin roots contain native hook events and payloads, with explicit errors for unsupported blocking/input-rewrite semantics.

Fitness gate: no local plugin copy/symlink is performed by production code.

Impact commands:

```bash
gitnexus impact Render --file internal/target/cursor/cursor.go --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/cursor ./internal/target/packageoutput ./internal/target
go test ./...
HOME="$(mktemp -d)" go test -tags=vendor_smoke ./internal/target/cursor
scripts/check-architecture
```

Manual checks:

- Compare generated `hooks/hooks.json` with the official plugin template.
- Verify IDE, CLI, and cloud availability claims are scoped to behavior actually tested.

- [x] Render `.cursor-plugin/plugin.json`, native component paths, `hooks/hooks.json`, payloads, and stable package metadata.
- [x] Map portable session/command/file events to documented camelCase events and treat non-equivalent decisions as unsupported/advisory.
- [x] Increment the Cursor format revision and preserve project-profile output separately.
- [x] Add official-template golden, hook-free regression, event mapping, matcher, timeout, decision-gap, collision, and determinism tests.
- [x] Add an isolated optional local plugin load smoke when the installed Cursor CLI provides a noninteractive proof path.
- [x] Run focused/full/architecture checks before Task 16.

### Task 16: Implement Grok installable plugin compatibility and hooks

Justification: D10. Current Grok project output is `.grok/skills`; that is not an installable multi-asset plugin and cannot close the cc-thingz packaging gap.

Chosen approach: retain `.grok/skills` for project profile and render package profile as a Grok-validated Claude-compatible plugin. Use Grok-specific command environment/root behavior in the separately generated artifact.

Rejected options:

- label current `.grok/skills` output a plugin — false packaging claim;
- reuse Claude bytes blindly — Grok supplies different root variables and fail-open semantics;
- invent an undocumented Grok manifest — unnecessary because official docs promise Claude compatibility.

Files:

- `internal/target/grok/grok.go` and any target-owned codec needed for package profile — split project/profile rendering clearly.
- `internal/target/grok/grok_test.go` and `testdata/` — project regression plus package golden/hooks.
- `internal/target/grok/module.md` — exact compatibility claim, event subset, root variables, failure semantics, validator, and revision.

Preconditions: Claude serializer behavior is proven and Grok docs/CLI are current.

Postconditions: Grok package profile emits one valid installable plugin or explicit separate roots according to render input; project profile remains backward compatible.

Fitness gate: Grok may reuse a target-neutral/shared serialization helper but cannot call the Claude adapter as a hidden second compiler pass.

Impact commands:

```bash
gitnexus impact Render --file internal/target/grok/grok.go --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/grok ./internal/target/packageoutput ./internal/target
go test ./...
command -v grok >/dev/null && grok plugin validate internal/target/grok/testdata/plugin-golden
go test -tags=vendor_smoke ./internal/target/grok
scripts/check-architecture
```

Manual checks:

- Inspect the generated package with `grok plugin validate` and, in an isolated config, `grok plugin install --trust` only in opt-in smoke code.
- Confirm fail-closed requests are diagnosed because Grok documents non-explicit failures as fail-open.

- [x] Preserve `.grok/skills` project-profile behavior and add a distinct installable package-profile renderer.
- [x] Render Claude-compatible plugin paths with Grok-specific hook root, event, timeout, and explicit-deny semantics.
- [x] Declare exact capabilities, increment format revision, and reject unsupported agents/events/decisions rather than omitting them.
- [x] Add project regression, package golden, hook decision, failure-policy, collision, separate-package, and deterministic tests.
- [x] Emit official `grok plugin validate` native checks and add an isolated optional install/inspect smoke.
- [x] Run focused/full/architecture checks before Task 17.

### Task 17: Generate deterministic target marketplace catalogs

Justification: D7 and D11. Correct multi-package distribution requires target-wide catalogs that reference generated package roots; hand-maintained copies recreate drift.

Chosen approach: add one shared catalog data builder with target-owned serializers and paths. Generate only local metadata; publishing and user installation remain out of scope.

Rejected options:

- one universal marketplace JSON file — native schemas/paths differ;
- infer owner/repository/version from Git or environment — violates determinism;
- publish from `agbun build` — network and credential scope violation.

Files:

- `internal/target/marketplace/` with `module.md`, Go builder/validation, and tests — the justified third shared target leaf for deterministic ordered catalog entries.
- `internal/target/module.md` and `.archfit.yaml` if module declarations need synchronization.
- each package target codec — serialize its native catalog path/schema from common distribution/package metadata.
- `internal/compiler/model/` — only corrections needed for metadata validation found during implementation.
- `docs/configuration.md` — full distribution metadata and output examples.

Preconditions: all package target roots and separate mode are stable. The model
rejects aggregate mode for every catalog target; only Pi uses aggregate mode in
source manifest version 1.

Postconditions: separate package mode emits deterministic catalogs for Claude,
Codex, Copilot, Cursor, and Grok; Pi emits none; catalog sources point to real
package roots.

Fitness gate: the shared marketplace leaf contains no publication, filesystem, process, clock, Git, or network behavior. Target leaves own JSON schemas.

Impact commands:

```bash
gitnexus impact Render --file internal/target/target.go --direction upstream --depth 4 --include-tests
gitnexus impact RenderWithCodec --direction upstream --depth 4 --include-tests
gitnexus detect-changes --scope unstaged
```

Verification commands:

```bash
go test ./internal/target/marketplace ./internal/target/...
go test ./internal/compiler/... ./cmd/agbun
go test ./...
scripts/check-architecture
```

Manual checks:

- Compare every catalog path and required field with `docs/vendor-package-contracts.md`.
- Confirm generated relative `source` entries resolve inside the target output root.

- [x] Add the shared deterministic catalog-entry builder and strict common distribution metadata validation.
- [x] Render Claude `.claude-plugin/marketplace.json`, Codex `.agents/plugins/marketplace.json`, Copilot `.github/plugin/marketplace.json`, Cursor `.cursor-plugin/marketplace.json`, and Grok-compatible catalog output.
- [x] Handle flat single-package source `.` and multi-package package-ID roots explicitly; reject path/identity collisions.
- [x] Add target golden tests for required fields, ordering, missing metadata, single/multi-package roots, explicit Pi-only aggregate rejection, and reproducibility.
- [x] Update configuration/module docs and run all target, compiler, architecture, and official available validators before Task 18.

### Task 18: Add the cc-thingz-shaped acceptance fixture, release gates, and final documentation

Justification: all approved decisions and success criteria. Unit-level success does not prove one real migration shape across six target plans.

Chosen approach: add a minimal public fixture modeled on cc-thingz package/hook needs, not a copied private working tree, then run one source-to-output matrix and optional installed-CLI smokes.

Rejected options:

- make tests depend on `~/Workspace/cc-thingz` — non-hermetic and unavailable in CI;
- store generated fixture output without checking drift — stale golden risk;
- claim actual cc-thingz migration complete from Agent Bundler tests alone — consuming-repo work remains post-completion.

Files:

- `testdata/cc-thingz-hooks/` — multi-package bundle with skills, agents, target overlays, command/file hooks, support files, Pi aggregate metadata, and separate plugin catalogs.
- compiler/CLI integration tests — build/check all six targets, selectors, drift, determinism, and native-check declarations.
- `scripts/smoke-vendor-packages` or Go `vendor_smoke` tests — isolated installed CLI checks with clear skip/fail output and no real config mutation.
- `README.md`, `docs/configuration.md`, `docs/targets-and-cli.md`, `docs/architecture.md`, and relevant examples — authoring, output trees, capabilities, limitations, and commands.
- `.github/workflows/ci.yml` — Go/TypeScript/architecture fast gates; vendor validators only where installation is pinned and reliable.
- `.github/workflows/release.yml` and CLI tests — ensure released `agbun` embeds the exact tested Pi runtime and reports the injected version.
- this plan file — RalphEx will move it to `docs/plans/completed/` after completion.

Preconditions: Tasks 1–17 are green.

Postconditions: the complete matrix is deterministic and documented; all automatic checks pass; remaining consuming-repo migration steps are explicit.

Fitness gate: `scripts/check-architecture` and `archfit check --config .archfit.yaml --base origin/master --require-tools --progress none` pass with the new TypeScript island/shared marketplace leaf. No module cycle or stage back-edge exists.

Impact commands:

```bash
gitnexus detect-changes --scope all
gitnexus check
git diff --check
git diff --stat
```

Verification commands:

```bash
changed_go=$(git diff --name-only -- '*.go')
test -z "$changed_go" || gofmt -d $changed_go
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
(cd internal/target/pi/runtime && bun install --frozen-lockfile && bun run typecheck && bun test)
scripts/check-architecture
archfit check --config .archfit.yaml --require-tools --progress none
go test -tags=vendor_smoke ./internal/target/... ./internal/compiler/...
git diff --check
```

Manual checks:

- Review every generated target tree against its pinned primary contract.
- Confirm vendor-smoke skips name the missing CLI rather than reporting a false pass.
- Confirm no test or production command changed normal Claude, Codex, Pi, Copilot, Cursor, or Grok configuration.
- Confirm the consuming cc-thingz migration steps below remain external and are not falsely marked complete.

- [x] Add the hermetic cc-thingz-shaped fixture and all-six-target build/check/selectors/drift/determinism integration matrix.
- [x] Add safe opt-in vendor smoke infrastructure with temporary config roots, bounded subprocesses, exact CLI availability diagnostics, and cleanup assertions.
- [x] Add CI gates for Go, Pi runtime TypeScript, lint/vet, architecture, deterministic fixture output, and pinned safe validators.
- [x] Update README, configuration, target, architecture, and release documentation with exact authoring/output/install-validation examples and unsupported cells.
- [x] Verify release builds embed the tested runtime bytes, set `agbun` version correctly, and pass `version`/`--version` smoke checks.
- [x] Run every final validation command, resolve all failures, inspect GitNexus change impact, and record the scoped post-implementation architecture re-review target.

## Technical details

### Canonical hook authoring example

```json
{
  "event": "pre-tool",
  "matcher": {
    "tools": ["command"]
  },
  "handler": {
    "mode": "exec",
    "program": "bash",
    "arguments": [{ "literal": "-eu" }, { "packageFile": "hook.sh" }]
  },
  "timeoutMilliseconds": 10000,
  "asynchronous": false,
  "failurePolicy": "closed",
  "order": 100
}
```

The exact JSON field spelling becomes normative in Task 1 and must remain consistent across Go decode, docs, fixtures, and the Pi runtime schema. If implementation finds a better spelling, change the module contract and every fixture in the same task; do not support aliases speculatively.

### Initial portable event matrix

| Portable event    | Claude                               | Codex                                    | Pi                                            | Copilot CLI                          | Cursor                                              | Grok                       |
| ----------------- | ------------------------------------ | ---------------------------------------- | --------------------------------------------- | ------------------------------------ | --------------------------------------------------- | -------------------------- |
| session-start     | `SessionStart`                       | verified native equivalent               | `session_start`                               | verified PascalCase equivalent       | verified native session event                       | `SessionStart`             |
| session-end       | `SessionEnd`                         | verified native equivalent               | `session_shutdown`                            | verified PascalCase equivalent       | `sessionEnd`                                        | `SessionEnd`               |
| prompt-submit     | `UserPromptSubmit`                   | `UserPromptSubmit`                       | `before_agent_start`                          | verified PascalCase equivalent       | only if docs prove an equivalent                    | `UserPromptSubmit`         |
| pre-tool          | `PreToolUse`                         | `PreToolUse`                             | `tool_call`                                   | `PreToolUse`                         | event-family mapping such as `beforeShellExecution` | `PreToolUse`               |
| post-tool         | `PostToolUse`                        | `PostToolUse`                            | `tool_result`                                 | `PostToolUse`                        | event-family mapping such as `afterFileEdit`        | `PostToolUse`              |
| post-tool-failure | verified native event or unsupported | verified native event                    | `tool_result` error state                     | verified native event or unsupported | verified native event or unsupported                | `PostToolUseFailure`       |
| stop              | `Stop`                               | `Stop`                                   | `agent_end`/settled mapping chosen in Task 10 | `Stop`                               | verified end event                                  | `Stop`                     |
| notification      | `Notification`                       | unsupported unless current docs prove it | Pi UI/runtime event only if equivalent        | verified native event or unsupported | verified native event or unsupported                | `Notification`             |
| pre/post-compact  | Claude compact events                | `PreCompact`/`PostCompact`               | session compaction events                     | verified current support             | verified current support                            | `PreCompact`/`PostCompact` |

“Verified” cells are intentionally rechecked inside each target task. A row is not enabled merely because a similarly named event exists; payload, blocking, mutation, timeout, and failure semantics must also match.

### Pi generated package example

```text
dist/pi/
├── package.json
├── agents/
├── skills/
├── hooks/
│   ├── hooks.v1.json
│   └── payloads/
├── extensions/
│   ├── agentbundler-hooks.ts
│   └── _agentbundler-hooks/
│       └── runtime source modules
└── README.md
```

Conceptual package registration:

```json
{
  "pi": {
    "extensions": ["./extensions/agentbundler-hooks.ts"],
    "skills": ["./skills"],
    "subagents": {
      "agents": ["./agents"]
    }
  }
}
```

The `pi.subagents` block and `pi-subagents` dependency are emitted only when agents exist. The thin adapter passes its own package-relative descriptor/runtime URLs to the embedded runtime. The runtime does not discover packages globally.

### Failure-policy rule

Security-sensitive hooks must not be silently weakened:

- if a target can explicitly deny but fails open on timeout/crash, `failurePolicy: closed` is not native-equivalent unless the adapter can enforce it;
- if the target cannot rewrite input, a rewrite-dependent hook is unsupported;
- passive hooks may be asynchronous only when the target preserves delivery expectations documented by the source;
- target capability diagnostics must name hook ID, event, requested behavior, target, and the nearest supported alternative.

### Windows rule

- interpreter-backed payload files do not require executable intent and remain renderable;
- explicitly executable payloads retain `Executable: true` and continue to trigger the existing Windows unsupported-intent diagnostic;
- tests must not depend on the host checkout preserving POSIX mode when the descriptor already invokes an interpreter;
- no adapter may silently convert executable-only handlers to a different interpreter.

## Acceptance criteria

- All success criteria in this plan are demonstrated by tests or explicit documented external limits.
- No unchecked task item remains.
- Every modified normative `module.md` agrees with the implemented code and test behavior.
- Every target format revision changed when its generated package contract changed.
- Existing hook-free fixtures remain green or have an evidence-backed native-path correction recorded in the target module/vendor note.
- `go test ./...`, race tests, vet, golangci-lint, Bun runtime tests, TypeScript typecheck, architecture checks, and `git diff --check` pass.
- Claude and Grok official validators pass against generated fixtures when installed.
- Optional vendor smoke tests either pass or skip only for a named unavailable CLI/unsupported noninteractive boundary; they never mutate real configuration.
- The cc-thingz-shaped fixture emits Claude, Codex, Pi, Copilot, Cursor, and Grok artifacts with no silently omitted source hook.
- Pi output contains one extension registration and runtime copy for the aggregate artifact, and the Go/TypeScript schema contract is identical.
- Generated catalogs contain stable, valid relative package sources and no publishing/network behavior.
- GitNexus and archfit report no unintended dependency-direction regression.

## Safety notes

Hooks execute trusted package code. Treat hook source and generated packages as executable software. Validation prevents accidental ambiguity; it does not sandbox trusted commands.

High-risk areas:

- pre-tool decisions can block or mutate tool execution;
- shell compatibility mode can execute arbitrary shell syntax;
- child-process timeout must terminate descendants where the platform permits;
- environment inheritance can expose secrets;
- plugin install smoke tests can mutate user configuration if isolation is wrong;
- target failure semantics can weaken a requested security policy;
- package aggregation can hide collisions if diagnostics are incomplete.

Required safeguards:

- use `exec.CommandContext`/equivalent argument arrays for canonical exec handlers;
- make shell mode explicit and capability-visible;
- inherit only the environment required by the documented runtime contract, with a reviewed policy for secrets;
- use contained package-file paths and reject symlinks/path escapes;
- bound output, duration, and process cleanup;
- use temporary HOME/vendor config directories in every mutating smoke test;
- never run publication or network installation from production Agent Bundler code;
- preserve the current atomic output/write and read-only check guarantees.

No data migration or irreversible production operation is part of this plan. RalphEx should implement in a branch or isolated worktree. Failed tasks must leave the repository inspectable and must not attempt cleanup by deleting unrecognized user files.

## Post-completion

These are external follow-ups and intentionally have no RalphEx checkboxes:

- Run a scoped architecture review of `internal/compiler/model`, `internal/compiler/source`, `internal/compiler/composition`, `internal/target`, `internal/target/pi/runtime`, and `internal/artifact`. Re-check D1–D14, capability truthfulness, dependency direction, and runtime ownership.
- Release a new Agent Bundler version only after the complete automatic gate passes.
- In the cc-thingz repository, migrate `src/hooks/*/meta.yaml` plus scripts into canonical hook directories and declare target/package membership.
- Configure cc-thingz Pi output as one aggregate package with explicit identity/metadata.
- Keep cc-thingz-specific Pi features such as plan-mode UI behavior as ordinary Pi-native extensions; let them call portable hooks through the documented runtime contract only when a real integration is needed.
- Replace cc-thingz's old Pi hook-runner copy after generated package install/load/fire tests pass.
- Generate all target trees with the released `agbun`, run actual vendor installation checks, and compare behavior with the old compiler before deleting legacy source.
- Fix cc-thingz's `make check` so it does not run a build first and erase drift.
- Re-run the cc-thingz migration audit and confirm no hook, extension, agent overlay, package, or catalog is source-only without an explicit policy.

## Re-review

After implementation, run `architecture-review` on the hook data flow from bundle/Claude import through composition, target rendering, Pi runtime embedding, artifact write/check, and native verification. Acceptance signals are:

- typed semantics remain target-neutral;
- target-specific mappings stay in target leaves;
- the TypeScript runtime is cohesive inside the Pi adapter boundary;
- no new cycle, layer back-edge, distant volatile dependency, or hidden source/runtime coupling exists;
- capability diagnostics match real vendor behavior;
- all package and catalog claims are proven by fixtures or native validators.
