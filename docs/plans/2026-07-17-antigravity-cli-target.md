<!-- markdownlint-disable MD013 -->

# Add Antigravity CLI Target Support

## Overview

Add `antigravity` as a first-class Agent Bundler output target. The target will
compile portable skills and the verified portable agent subset into native
Antigravity CLI plugin roots, preserve explicitly declared Antigravity-only
files such as rules and MCP configuration, and declare the official local
validator for `agbun check --native`.

The first release is intentionally bounded by verified Antigravity CLI 1.1.3
behavior and the Conductor 0.3.0 example. It will not invent a marketplace
format, convert legacy `commands/`, translate portable hooks without a verified
semantic contract, install plugins, publish plugins, or modify user
configuration.

RalphEx 1.6.0 is installed locally. Execute this plan with:

```sh
ralphex --worktree docs/plans/2026-07-17-antigravity-cli-target.md
```

Worktree mode auto-commits the plan file when it has uncommitted changes. Review
that commit before execution if preserving local commit history matters.

Use a branch derived from the plan filename. Complete tasks in order. Every task
must leave its focused tests green before the next task starts. Keep the task
headings and checkbox structure stable while RalphEx is executing. If verified
vendor behavior contradicts this plan, stop the current task, record the exact
source and blocker, and revise the plan instead of silently expanding scope.

## Context and source evidence

### Repository baseline

- Repository: `alexei-led/agentbundler` at commit
  `cef32ee4ee6f98b3f0104b898ea532404ab74b41` when this plan was written.
- Current release: `v0.4.8`; the Antigravity target is a new feature and is
  intended for the next minor release, `v0.5.0`.
- Current GitHub description: `Compile portable coding-agent assets into
target-native packages.`
- Current GitHub topics: none.
- Current targets are closed over `claude`, `codex`, `pi`, `copilot`, `grok`,
  and `cursor` in `internal/compiler/model/types.go`, target parsers, the target
  registry, CLI help, acceptance tests, and documentation.
- `internal/target/packageoutput` already owns deterministic package roots,
  skills, agents, resources, native resources, collision checks, capability
  enforcement, and per-package manifests. Antigravity should extend this seam,
  not create another package renderer.
- `internal/compiler/composition/nativeAssetSupportedByTarget` currently
  recognizes only Pi native extensions. It is the required seam for preserving
  Antigravity-native `rules/`, `mcp_config.json`, `hooks.json`, and support
  directories.
- `.archfit.yaml` already models `internal/target/**` as the target module and
  forbids target dependencies on source, composition, or artifact modules. No
  new architecture module is needed.

### Verified Antigravity contract

Primary documentation was checked through Context7 library IDs
`/websites/antigravity_google_cli` and `/google-antigravity/antigravity-cli` on
2026-07-17:

- <https://antigravity.google/docs/cli/plugins>
- <https://antigravity.google/docs/cli/features>
- <https://antigravity.google/docs/cli/gcli-migration>
- <https://github.com/google-antigravity/antigravity-cli>

The verified native plugin root is:

```text
plugin.json              required package marker
skills/                  optional skill directories containing SKILL.md
agents/                  optional Markdown subagent definitions
rules/                   optional Antigravity rule Markdown
mcp_config.json          optional MCP server definitions
hooks.json               optional native hook definitions
commands/                accepted legacy input; converted to skills by agy
```

The published `plugin.json` schema requires `name`, allows optional
`description`, rejects additional properties, and documents the name pattern as
`^[a-zA-Z0-9-_]+$`. Although the local 1.1.3 validator is looser, Agent Bundler
must enforce the published schema rather than depend on permissive behavior.
The generated manifest must therefore contain only:

```json
{
  "name": "plugin-name",
  "description": "Optional description"
}
```

The installed local CLI is `/Users/alexei/.local/bin/agy`, version `1.1.3`.
Verified safe behavior:

- `agy plugin validate <path>` validates a local plugin and exits nonzero on a
  missing `plugin.json` or missing `name`.
- Both `agy plugin` and `agy plugins` aliases exist; generated checks will use
  the singular documented form.
- Validation discovers skills, agents, legacy commands, MCP servers, and hooks.
- Validation succeeds under isolated `HOME`, `XDG_CONFIG_HOME`, and
  `XDG_CACHE_HOME`; it creates only configuration directories.
- Mutating subcommands do not consistently implement `--help`; notably,
  `agy plugin uninstall --help` was parsed as an uninstall request. Tests and CI
  must never probe install, link, enable, disable, or uninstall in the normal
  user environment.

The official Antigravity CLI 1.1.3 Linux x64 release asset is:

- URL:
  `https://github.com/google-antigravity/antigravity-cli/releases/download/1.1.3/agy_cli_linux_x64.tar.gz`
- SHA-256:
  `7a7239a69b65d3cf3af7e75f27b2ff4e9cce696a7b9a9e5c37c695f1c74eec34`
- Archive member: `antigravity`; CI will install that member as
  `/usr/local/bin/agy`.

### Verified Conductor example

Conductor 0.3.0 was inspected at commit
`fb6212e8faee3f9ecb69f0ee19bd5b2a0765bb0a`:

- <https://github.com/gemini-cli-extensions/conductor/tree/fb6212e8faee3f9ecb69f0ee19bd5b2a0765bb0a>
- root `plugin.json` contains only `name` and `description`;
- `skills/` contains six `SKILL.md` assets;
- `rules/conductor_antigravity.md` is a native Antigravity rule;
- there are no agents, native hooks, MCP servers, or legacy commands;
- local `agy plugin validate` reports six processed skills and succeeds.

The repository example added by this plan will be Conductor-shaped, not a full
copy of upstream Conductor. It will contain one original demonstration skill and
one original Antigravity-only rule, cite the upstream repository and pinned
commit, and avoid importing upstream implementation text unnecessarily.

## Approved architecture decisions

### AG-D1 — Add an outbound target, not a new source kind

This release adds `TargetAntigravity` and renders existing `skills-repository`,
`bundle`, and `claude-plugin` inputs. Directly importing arbitrary Antigravity
plugin repositories is a separate feature because rules, MCP, and native hooks
need source-normalization policy. The Conductor-shaped example will use the
existing `bundle` source kind.

### AG-D2 — Use the shared package-output seam

The Antigravity adapter will use `packageoutput.RenderWithCodec`. It will own
only vendor-specific validation and serialization:

- root `plugin.json`;
- `agents/*.md` for the verified portable agent subset;
- exact passthrough of explicit Antigravity native resources;
- `agy plugin validate .` native checks.

Skills and portable resources remain owned by `packageoutput`.

### AG-D3 — Package profile and separate mode only

Antigravity output is an installable plugin, so the target requires
`composition[].profile: "package"` and `packageMode: "separate"`. One package
renders flat. Multiple packages render under package-ID roots and are installed
individually. No verified Antigravity marketplace/catalog format exists, so the
adapter will not emit a catalog.

Project profile and aggregate mode must fail with existing explicit diagnostics;
they must not silently degrade to a skills-only project tree.

### AG-D4 — Keep the manifest strict and minimal

`plugin.json` will use the package identity as `name`, copy only an optional
string `description`, and reject package identities outside
`^[A-Za-z0-9_-]+$`. It will not emit `$schema`, version, author, repository,
skill paths, agent paths, hooks, MCP, or rule arrays. Antigravity discovers
components by filesystem convention.

Distribution metadata may still drive Agent Bundler provenance and other
targets, but Antigravity will not invent manifest fields for it. Documentation
must state that Antigravity has no generated catalog and no manifest version
field in this verified contract.

### AG-D5 — Support skills and a narrow agent subset

Skills remain normal `skills/<name>/SKILL.md` directories with support files and
frontmatter preserved by the shared renderer. This matches Conductor, including
its `metadata.version` skill frontmatter.

Portable agents render as `agents/<name>.md` only when frontmatter consists of
non-empty string `name` and `description`. Any additional field, including
Pi-specific `sandbox_mode`, `inheritSkills`, model selection, or tool policy,
must return `unsupported-agent-field`. This prevents Agent Bundler from claiming
unverified parity.

### AG-D6 — Preserve explicit Antigravity-native files without interpreting them

A bundle asset under `src/plugins/antigravity/<component>/` is target-native.
For the Antigravity target, composition will retain it without requiring
Pi-specific `piExtensions`; the codec will copy its complete contained,
symlink-free file tree at the plugin root. Typical entries are `rules/*.md`,
`mcp_config.json`, `hooks.json`, and shared scripts.

The source asset must explicitly declare `asset.native-resource` in
`.agentbundler/asset.json`. Existing path validation, target allow-lists,
collision detection, input hashing, executable-mode preservation, and native-gap
handling remain authoritative. The codec must reject an empty native-resource
asset and Pi extension declarations on an Antigravity resource.

### AG-D7 — Portable hooks remain unsupported in this release

A raw, explicitly target-native `hooks.json` can be preserved by AG-D6. A
portable `AssetKindHook` cannot be rendered because exact Antigravity event,
matcher, timeout, decision, async, ordering, and failure-policy semantics are not
fully documented. `asset.hook` and every portable `hook.*` capability will be
unsupported in the Antigravity capability table.

No command generation is needed. Antigravity compiles skills into slash
commands; `commands/` is legacy input handled by `agy`, not an Agent Bundler
output contract.

### AG-D8 — Validation is offline, isolated, and per plugin root

Every rendered package root declares:

```text
program: agy
arguments: plugin validate .
working directory: that package root
```

`agbun check --native` will execute the checks through the existing isolated
native-verification subsystem. CI will pin Antigravity CLI 1.1.3 by version and
SHA-256 and validate both acceptance-fixture package roots. Production code will
never install, link, enable, disable, uninstall, authenticate, or access the
network.

### AG-D9 — Release and GitHub metadata changes are explicit external follow-ups

Repository files will document the target and release contract. After the plan
is implemented, reviewed, merged to `master`, and CI is green, the owner will:

- update the GitHub description and repository topics;
- create annotated tag `v0.5.0` with target-specific release notes;
- push the tag and let `.github/workflows/release.yml` build binaries, checksums,
  the GitHub release, and the Homebrew dispatch.

RalphEx must not perform those remote mutations during task execution.

## Success criteria

- `antigravity` is accepted everywhere a target ID is decoded, validated,
  composed, recorded in provenance, selected by CLI, and resolved from the
  built-in target registry.
- A package-profile bundle renders deterministic Antigravity plugin roots with
  strict root `plugin.json`, skills, the supported agent subset, portable
  resources, and explicit target-native files.
- Unsupported project profile, aggregate mode, portable hooks, unsupported agent
  fields, invalid plugin names, malformed descriptions, empty native resources,
  wrong-target native resources, and output collisions fail before writing.
- One package renders flat; multiple packages render under stable package-ID
  roots and declare one native validation check per root without a fabricated
  marketplace.
- `agy plugin validate` accepts the adapter golden fixture, the
  Conductor-shaped example, and both generated acceptance-fixture roots.
- The all-target fixture builds and checks seven deterministic target trees and
  detects Antigravity drift and selectors.
- Unit, integration, race, vet, lint, architecture, native-validator, workflow,
  and documentation checks pass.
- User documentation names the exact supported and unsupported Antigravity
  semantics and keeps installation/publication outside Agent Bundler.
- GitHub description, topic list, release notes, and `v0.5.0` tag commands are
  ready as explicit post-completion actions.

## Development approach

- Use regular incremental development with focused tests in the same task as
  each behavior change.
- Make no broad target-registry refactor. Update the existing closed target
  switches surgically; a data-driven registry redesign is outside this feature.
- Keep vendor logic below `internal/target/antigravity`; do not import source,
  composition, artifact, CLI, or network packages from the target adapter.
- Use standard-library JSON, regexp, sorting, and filesystem abstractions. Add no
  runtime dependency.
- Reuse `packageoutput.UnsupportedAgentFieldError` so cross-target diagnostics
  remain consistent.
- Preserve deterministic ordering and source-origin evidence for every copied or
  generated file.
- Keep tests offline by default. Only the pinned CI validator download uses the
  network; validator execution runs with blocked proxies and temporary config.
- Run `gofmt` and focused tests after each code change. All focused tests must
  pass before the next task.
- Do not mark a checkbox complete if its verification command fails. Record the
  blocker instead.

## Testing strategy

- Model tests cover target identity and closed target validation.
- Source tests cover Antigravity target sidecars and native-resource import.
- Composition tests cover target-native retention, wrong-target exclusion,
  native-gap policy, capability use, and deterministic cloning.
- Adapter tests cover manifest schema, name/description rejection, skill and
  agent rendering, raw native-resource copying, collisions, package modes,
  capability declarations, multiple package roots, and native checks.
- Golden tests compare a complete plugin tree byte-for-byte.
- A `vendor_smoke` test invokes installed `agy plugin validate` only under an
  isolated home/config/cache environment and protects the normal `~/.gemini`
  path.
- Compiler/CLI tests cover build, check, selectors, drift, provenance adapter
  revision, archive naming, JSON results, and target help.
- The seven-target acceptance fixture proves cross-target determinism and that
  Antigravity-specific exclusions do not weaken other targets.
- The Conductor-shaped example test proves the minimal documented user flow.
- CI downloads the pinned Linux validator archive, verifies SHA-256, and
  validates generated plugin roots with network proxies blocked.

## Validation Commands

Run focused commands in their tasks, then run this complete gate from the
repository root:

```sh
test -z "$(git ls-files -z -- '*.go' | xargs -0 gofmt -l)"
git diff --check
go test ./...
go test -run '^$' -tags=vendor_smoke ./...
go test -tags=vendor_smoke ./internal/target/antigravity/...
go test -race ./...
go vet ./...
golangci-lint run
(
  cd internal/target/pi/runtime
  bun install --frozen-lockfile
  bun run typecheck
  bun test
)
scripts/check-acceptance-fixture
scripts/check-architecture
archfit check --config .archfit.yaml
actionlint -no-color .github/workflows/ci.yml .github/workflows/release.yml
gitleaks git --config .gitleaks.toml --redact --verbose
markdownlint-cli2 \
  examples/antigravity-conductor/README.md \
  docs/plans/2026-07-17-antigravity-cli-target.md
agy plugin validate internal/target/antigravity/testdata/plugin-golden
tmp_example="$(mktemp -d)"
trap 'rm -rf "$tmp_example"' EXIT
cp -R examples/antigravity-conductor/. "$tmp_example/"
go run ./cmd/agbun build --root "$tmp_example"
agy plugin validate "$tmp_example/generated/antigravity"
go run ./cmd/agbun check --root "$tmp_example" --native
rm -rf "$tmp_example"
trap - EXIT
gitnexus detect-changes --scope unstaged --repo agentbundler
```

If the GitNexus index is stale after implementation, run
`gitnexus analyze .` once, then rerun change detection. Do not treat a stale or
missing graph as passing architecture evidence.

## Implementation Steps

### Task 1: Pin the contract and add the target/native-resource model seam

Justification: AG-D1, AG-D4, AG-D6, and AG-D7. Target IDs are duplicated across
validation and source boundaries, while composition recognizes only Pi native
resources. The contract and safety seam must exist before a renderer can rely on
them.

Files:

- `docs/vendor-package-contracts.md` — add the dated Antigravity contract source,
  strict manifest, native roots, validator command, no-catalog boundary,
  supported asset subset, and explicit hook/agent limits.
- `internal/compiler/model/types.go` — add
  `TargetAntigravity TargetID = "antigravity"` alongside existing target IDs.
- `internal/compiler/model/validation.go` — accept `TargetAntigravity` in
  `validTargetID`; preserve every existing target and validation rule.
- `internal/compiler/model/model_test.go` — prove Antigravity manifests,
  compositions, overlays, acknowledgments, normalized packages, and build plans
  validate while unknown targets still fail.
- `internal/compiler/model/module.md` — include the new target in the normative
  target vocabulary without changing layer ownership.
- `internal/compiler/source/bundle/bundle.go` — accept `antigravity` in the
  bundle target parser.
- `internal/compiler/source/bundle/bundle_test.go` — import an explicit
  `src/plugins/antigravity/<component>` tree with target allow-list and
  `asset.native-resource`; reject malformed paths, symlinks, and unknown targets.
- `internal/compiler/source/bundle/module.md` — document Antigravity native
  resource paths as exact, explicit bundle assets.
- `internal/compiler/source/claudeplugin/helpers.go` — accept Antigravity in
  target sidecar parsing.
- `internal/compiler/source/claudeplugin/claudeplugin_test.go` — add focused
  Antigravity overlay/target-sidecar coverage without claiming direct
  Antigravity plugin import.
- `internal/compiler/source/skillrepo/skillrepo.go` — accept Antigravity in
  centralized target sidecars.
- `internal/compiler/source/skillrepo/skillrepo_test.go` — cover an Antigravity
  skill overlay and unknown-target rejection.
- `internal/artifact/provenance/provenance.go` — accept Antigravity when decoding
  or validating target values for build provenance.
- `internal/artifact/provenance/provenance_test.go` — prove the new target and
  adapter revision round-trip deterministically.
- `internal/compiler/composition/composition.go` — change
  `nativeAssetSupportedByTarget` to a target switch: retain current Pi behavior
  only when `piExtensions` is non-empty; retain an Antigravity native resource
  when its native-gap target is Antigravity and it has no Pi extension
  declaration; return false for every other target.
- `internal/compiler/composition/composition_test.go` — cover retained
  Antigravity files, wrong-target exclusion, explicit source-only/exclude/replace
  policies, duplicate references, Pi regression behavior, and deterministic
  content cloning.
- `internal/compiler/composition/module.md` — document that target-native
  capability recognition is explicit per target and is not inferred from file
  names.

Preconditions:

- The source tree is clean except for this plan file.
- The official contract and local CLI evidence in this plan remain current.
- No Antigravity adapter is registered yet; Task 1 must not add generated output.

Postconditions:

- `antigravity` is a valid model/source/provenance target.
- Explicit Antigravity native-resource assets survive composition only for that
  target.
- Existing six targets retain identical behavior and bytes.
- Focused model, source, composition, and provenance tests pass.

Fitness gate: existing `.archfit.yaml` rules remain unchanged. This task changes
model, source, composition, and artifact modules but introduces no prohibited
imports. `archfit check --config .archfit.yaml` must pass after the change.

Impact commands:

```sh
gitnexus impact validTargetID --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus impact nativeAssetSupportedByTarget --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus detect-changes --scope unstaged --repo agentbundler
```

Verification commands:

```sh
gofmt -w \
  internal/compiler/model/types.go \
  internal/compiler/model/validation.go \
  internal/compiler/model/model_test.go \
  internal/compiler/source/bundle/bundle.go \
  internal/compiler/source/bundle/bundle_test.go \
  internal/compiler/source/claudeplugin/helpers.go \
  internal/compiler/source/claudeplugin/claudeplugin_test.go \
  internal/compiler/source/skillrepo/skillrepo.go \
  internal/compiler/source/skillrepo/skillrepo_test.go \
  internal/compiler/composition/composition.go \
  internal/compiler/composition/composition_test.go \
  internal/artifact/provenance/provenance.go \
  internal/artifact/provenance/provenance_test.go
go test ./internal/compiler/model/...
go test ./internal/compiler/source/...
go test ./internal/compiler/composition/...
go test ./internal/artifact/provenance/...
go test ./internal/compiler/... ./internal/artifact/...
archfit check --config .archfit.yaml
git diff --check
```

Manual checks:

- Confirm the vendor contract cites official Antigravity docs and pinned
  Conductor evidence, not inferred manifest fields.
- Confirm no source importer claims it can adopt an arbitrary Antigravity plugin.
- Confirm native files remain explicit target-owned input and are not parsed as
  portable hooks, MCP, or rules.

- [x] Update `docs/vendor-package-contracts.md` first with AG-D1 through AG-D8,
      exact URLs, checked date, CLI version, manifest schema, native layout,
      validator, and unsupported semantics.
- [x] Add `model.TargetAntigravity` and update every closed target validator in
      `internal/compiler/model`; add valid and invalid model tests without
      loosening identifier or path validation.
- [x] Update the bundle, Claude-plugin sidecar, skill-repository sidecar, and
      provenance target parsers; add success and unknown-target regression tests
      in each changed package.
- [x] Generalize `nativeAssetSupportedByTarget` with explicit Pi and
      Antigravity branches; do not add a generic “all native resources pass”
      fallback.
- [x] Add composition tests for correct-target retention, wrong-target
      exclusion, native-gap actions, duplicate references, empty content,
      origins, executable modes, and unchanged Pi behavior.
- [x] Update the affected module documentation so the code and normative module
      contracts agree before adapter work begins.
- [x] Run the Task 1 verification and impact commands; all focused tests,
      Archfit, and diff checks must pass before Task 2.

### Task 2: Implement and register the Antigravity package adapter

Justification: AG-D2 through AG-D8. This task supplies the smallest native
vertical slice on top of the model seam: strict manifest, skills, narrow agents,
explicit native resources, capability failures, and safe validation.

Files:

- `internal/target/antigravity/antigravity.go` — define `Target`,
  `FormatRevision = 1`, leaf `Adapter`, package-only render dispatch, and one
  native check per rendered package root.
- `internal/target/antigravity/codec.go` — own strict manifest serialization,
  agent serialization/validation, native-resource copying, package validation,
  capability rules, sorted native paths, and diagnostics.
- `internal/target/antigravity/antigravity_test.go` — adapter identity,
  immutable capabilities, package/profile/mode behavior, strict manifest,
  multi-package roots, native checks, error cases, and deterministic output.
- `internal/target/antigravity/codec_test.go` — table-driven serializer tests for
  names, descriptions, skill/agent frontmatter, extra agent fields, native
  resources, executable files, empty resources, Pi declarations, and
  collisions.
- `internal/target/antigravity/testdata/plugin-golden/plugin.json` — exact
  minimal native manifest.
- `internal/target/antigravity/testdata/plugin-golden/skills/guide/SKILL.md` —
  representative skill and sidecar support file.
- `internal/target/antigravity/testdata/plugin-golden/agents/reviewer.md` —
  supported `name`/`description` agent subset.
- `internal/target/antigravity/testdata/plugin-golden/rules/conductor.md` —
  representative native rule passthrough.
- `internal/target/antigravity/testdata/plugin-golden/mcp_config.json` — benign
  empty MCP configuration proving exact root placement; do not start a server.
- `internal/target/antigravity/module.md` — target ownership, format revision,
  capabilities, paths, unsupported semantics, and native-check boundary.
- `internal/target/target.go` — import `antigravity` and register
  `fromLeaf(antigravity.New())` in `builtInRegistry`.
- `internal/target/target_test.go` — include Antigravity in resolver,
  format-revision, defensive capability-copy, and decision-capability matrices.
- `internal/target/packageoutput/packageoutput_test.go` — add the Antigravity
  codec to shared codec ownership and semantic-output tables while accounting
  for its intentionally absent version field and unsupported hooks.
- `internal/target/module.md` — list Antigravity as a target-owned package codec
  with no catalog.

Implementation details:

- `PackageCodec()` sets `TargetAntigravity`, `ManifestPath: "plugin.json"`,
  `AgentRoot: "agents"`, no hook renderer, no hook payload root, no catalog,
  and non-nil manifest/agent/native-resource/package validators.
- `manifest(pkg)` returns deterministic JSON with `name` and optional
  `description` only. A present non-string description is an error. Do not use
  `CopyMetadata` for unverified fields.
- `validatePackage(pkg)` rejects names outside `^[A-Za-z0-9_-]+$`, portable hook
  assets, malformed agent frontmatter, Antigravity native resources with
  `piExtensions`, and native resources without files. Shared profile, package
  mode, capability, and path validation remains in `packageoutput`.
- `markdownAgent(asset)` requires exact non-empty string `name` and
  `description`; any extra frontmatter key returns
  `packageoutput.UnsupportedAgentFieldError{Target: TargetAntigravity}`. Render
  with `packageoutput.Markdown` and suffix `.md`.
- `nativeResource(asset)` sorts `asset.Content.Files` by relative path and
  returns detached `NativeResourceFile` values at those exact plugin-root paths.
  Preserve bytes, origin, and executable mode. Reject nil/empty content and any
  Pi extension declaration.
- Capability rules mark `asset.skill`, `asset.agent`, `asset.resource`, and
  `asset.native-resource` native. Mark `asset.hook` plus every canonical
  `hook.*` key unsupported. Include every capability key used by repository
  fixtures so missing rules fail deterministically rather than depending on
  table order.
- `Adapter.Render` requires package profile through `RenderWithCodec`, leaves
  separate-mode validation to the shared renderer, and attaches native checks
  only after a successful plan.
- For one package, the native check leaves `WorkingDirectory` nil so the
  compiler uses the Antigravity target root. For multiple packages it uses each
  package ID. Every check runs `agy plugin validate .`; the compiler prefixes
  the target root during `check --native`. Never encode `"."` as a model
  `RelativePath`, because model validation rejects dot segments.

Preconditions:

- Task 1 is complete and green.
- Antigravity native resources survive composition only for their target.
- `agy --version` reports 1.1.3 in the local execution environment.

Postconditions:

- `target.Resolve(model.TargetAntigravity)` returns format revision 1.
- Valid package-profile input renders an installable Antigravity plugin tree.
- Invalid or unverified semantics fail before output.
- The golden tree validates with local `agy plugin validate`.

Fitness gate: the new package remains inside the existing `target` Archfit
module and imports only standard library, `internal/compiler/model`, and
`internal/target/packageoutput`. Existing `target_no_source`,
`target_no_composition`, and `target_no_artifact` rules must pass.

Impact commands:

```sh
gitnexus impact RenderWithCodec --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus impact builtInRegistry --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus detect-changes --scope unstaged --repo agentbundler
```

If GitNexus cannot resolve the unexported `builtInRegistry`, use:

```sh
gitnexus impact Resolve --file internal/target/target.go --direction upstream --depth 3 --include-tests --repo agentbundler
```

Verification commands:

```sh
gofmt -w internal/target/antigravity/*.go internal/target/target.go internal/target/target_test.go internal/target/packageoutput/packageoutput_test.go
go test ./internal/target/antigravity/...
go test ./internal/target/packageoutput/...
go test ./internal/target/...
agy plugin validate internal/target/antigravity/testdata/plugin-golden
scripts/check-architecture
archfit check --config .archfit.yaml
git diff --check
```

Manual checks:

- Inspect golden `plugin.json` byte-for-byte; it must have no version, component
  path, catalog, author, or schema fields.
- Inspect generated agent frontmatter and confirm unsupported fields produce a
  diagnostic naming the asset, field, and Antigravity target.
- Confirm raw `mcp_config.json` is copied but never parsed, executed, or tested
  against a live MCP process.
- Confirm no adapter code invokes installation or reads user configuration.

- [ ] Create `internal/target/antigravity` with a leaf adapter at format revision
      1 and the exact package-only dispatch described above.
- [ ] Implement the strict two-field manifest serializer and table-driven tests
      for valid names, invalid names, absent/valid/invalid descriptions, and
      deterministic bytes.
- [ ] Implement the narrow Markdown agent serializer and tests for required
      fields, extra fields, multiline/non-string values, body preservation, and
      `.md` path generation.
- [ ] Implement sorted exact native-resource passthrough and tests for rules,
      MCP JSON, nested shared scripts, executable mode, empty trees,
      Pi-extension rejection, traversal rejection, and path collisions.
- [ ] Define the complete capability table and prove portable hooks and every
      unverified hook cell fail without acknowledgments or silent omission.
- [ ] Attach one `agy plugin validate .` native check per flat or package-ID
      root; test working directories, arguments, source location, and stable
      order.
- [ ] Add a complete golden fixture and byte-compare test covering manifest,
      skill, agent, portable resource, native rule, and benign MCP file.
- [ ] Register the adapter and extend shared target/package-output tests without
      changing existing adapter revisions or output bytes.
- [ ] Run the Task 2 verification and impact commands, including real local
      `agy` validation; all checks must pass before Task 3.

### Task 3: Integrate compiler, CLI, archives, acceptance fixture, and Conductor example

Justification: AG-D1 through AG-D8. Adapter unit tests are insufficient; the
public compiler and CLI must select, build, check, package, record, and explain
the target through the same deterministic flow as existing targets.

Files:

- `internal/compiler/compiler_test.go` — include Antigravity in adapter revision,
  selection, package profile, and error/consolidation tables.
- `internal/compiler/cc_thingz_acceptance_test.go` — change the expected matrix
  from six to seven targets; assert Antigravity paths, package roots, native
  checks, selectors, drift, check immutability, determinism, and provenance.
- `internal/compiler/antigravity_conductor_example_test.go` — build the example
  in a temporary copy, assert exact generated paths/manifest/rule, run check,
  and assert the declared native-check shape without requiring `agy` in default
  unit tests.
- `cmd/agbun/main.go` — list `antigravity` in `help targets` and any public target
  examples; keep parsing delegated to the model/compiler.
- `cmd/agbun/main_test.go` — cover help text, `--target antigravity`, JSON output,
  check/drift exits, package selection, and `--native` tool-unavailable
  reporting through existing test seams.
- `internal/artifact/archive/archive_test.go` — assert Antigravity package output
  receives the default deterministic `.tar.gz` archive behavior and does not
  inherit Pi-specific `.tgz` assumptions.
- `.gitignore` — ignore only
  `examples/antigravity-conductor/generated/`, matching the example's dedicated
  output without hiding other example source files.
- `testdata/cc-thingz-hooks/agentbundle.json` — add `antigravity` and a package
  composition with `profile: "package"`, `packageMode: "separate"`.
- `testdata/cc-thingz-hooks/source/packages/core-tools.json` — make the portable
  agent eligible for Antigravity; keep portable command hooks in exact target
  allow-lists that exclude Antigravity; add the native-resource asset as an
  object with `"targets": ["antigravity"]` so no other adapter receives it.
- `testdata/cc-thingz-hooks/source/packages/workflow-tools.json` — explicitly
  exclude Antigravity from portable hooks while preserving skills/resources.
- `testdata/cc-thingz-hooks/source/src/plugins/antigravity/conductor-ux/.agentbundler/asset.json`
  — declare `asset.native-resource`.
- `testdata/cc-thingz-hooks/source/src/plugins/antigravity/conductor-ux/rules/conductor_antigravity.md`
  — benign target-native rule proving root passthrough.
- `scripts/check-acceptance-fixture` — report seven target trees and keep the
  two-copy deterministic check.
- `examples/antigravity-conductor/README.md` — explain the Conductor-shaped
  source, build, local validation, optional installation, generated layout,
  limitations, upstream credit, and pinned source commit.
- `examples/antigravity-conductor/agentbundle.json` — one Antigravity package,
  package profile, separate mode, and dedicated `generated` output.
- `examples/antigravity-conductor/source/packages/conductor-example.json` — one
  skill plus one Antigravity-native rule asset whose explicit target allow-list
  is exactly `["antigravity"]`.
- `examples/antigravity-conductor/source/src/skills/conductor-setup/SKILL.md` —
  original small setup workflow using normal skill frontmatter.
- `examples/antigravity-conductor/source/src/plugins/antigravity/conductor-ux/.agentbundler/asset.json`
  — explicit native capability declaration.
- `examples/antigravity-conductor/source/src/plugins/antigravity/conductor-ux/rules/conductor_antigravity.md`
  — original small `trigger: model_decision` rule demonstrating Antigravity UX
  adaptation without copying Conductor implementation text.

Implementation details:

- Keep acceptance target order lexical; `antigravity` precedes `claude`.
- Expected Antigravity acceptance paths include
  `core-tools/plugin.json`, `core-tools/skills/review/SKILL.md`,
  `core-tools/agents/reviewer.md`,
  `core-tools/rules/conductor_antigravity.md`, and the corresponding
  `workflow-tools/plugin.json`/skills. There is no target-wide catalog.
- Expect two Antigravity native checks, one per package root. Existing Claude and
  Grok native-check expectations remain unchanged.
- Mutate one Antigravity generated file in drift coverage and require
  `DRIFT_CHANGED`; restore it and prove `check` is write-free.
- Select `--target antigravity --package core-tools` and prove the historical
  single-package flat root contains `plugin.json` rather than a nested
  `core-tools/plugin.json`.
- The Conductor example must not commit generated output. Its integration test
  copies the example to `t.TempDir`, builds, checks, compares a second plan, and
  inspects generated bytes.
- Document optional installation as an external user command only:
  `agy plugin install "$(pwd)/generated/antigravity"`. Tests must not execute it.

Preconditions:

- Tasks 1 and 2 are complete and green.
- The adapter resolves through the built-in registry and its golden fixture
  validates locally.

Postconditions:

- `agbun build`, `check`, `package`, selectors, provenance, and help handle
  Antigravity.
- The hermetic acceptance fixture covers seven targets.
- The repository contains one runnable, tested Conductor-shaped example.
- Existing target outputs remain deterministic and semantically unchanged.

Fitness gate: compiler remains the only orchestrator joining source,
composition, target, and artifact stages. The example adds no runtime module.
`scripts/check-architecture` and Archfit must pass without config exceptions.

Impact commands:

```sh
gitnexus impact Compile --file internal/compiler/compiler.go --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus impact targetsHelp --file cmd/agbun/main.go --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus detect-changes --scope unstaged --repo agentbundler
```

Verification commands:

```sh
gofmt -w \
  internal/compiler/compiler_test.go \
  internal/compiler/cc_thingz_acceptance_test.go \
  internal/compiler/antigravity_conductor_example_test.go \
  cmd/agbun/main.go \
  cmd/agbun/main_test.go \
  internal/artifact/archive/archive_test.go
go test ./internal/compiler/...
go test ./cmd/agbun/...
go test ./internal/artifact/archive/...
scripts/check-acceptance-fixture
tmp_example="$(mktemp -d)"
trap 'rm -rf "$tmp_example"' EXIT
cp -R examples/antigravity-conductor/. "$tmp_example/"
go run ./cmd/agbun build --root "$tmp_example"
go run ./cmd/agbun check --root "$tmp_example"
agy plugin validate "$tmp_example/generated/antigravity"
rm -rf "$tmp_example"
trap - EXIT
scripts/check-architecture
archfit check --config .archfit.yaml
git diff --check
```

Manual checks:

- Read the example as a new user and confirm every command uses the correct root.
- Confirm the example credits upstream Conductor and does not imply that Agent
  Bundler imports Conductor repositories directly.
- Compare snapshots for all pre-existing targets and investigate any changed
  byte rather than updating expectations blindly.
- Confirm no generated directory from temporary examples is tracked.

- [ ] Update compiler, CLI, archive, and target-list tests so Antigravity is
      selected and recorded through every public path with exact failure exits.
- [ ] Extend the cc-thingz fixture to seven targets, explicitly exclude portable
      hooks from Antigravity, and add a benign Antigravity-only native rule.
- [ ] Extend acceptance assertions for lexical target order, generated paths,
      two native checks, selectors, drift, deterministic plans, provenance, and
      unchanged existing targets.
- [ ] Update `scripts/check-acceptance-fixture` from six to seven targets without
      weakening its two-copy byte comparison.
- [ ] Add the complete Conductor-shaped example source and README with original
      minimal content, upstream/pinned references, exact build/validate/install
      commands, unsupported-feature notes, and a narrow `.gitignore` entry for
      its generated output.
- [ ] Add an example integration test that builds and checks in temporary roots,
      inspects exact bytes and native checks, and never installs or contacts a
      model service.
- [ ] Validate the generated example with local `agy plugin validate` and verify
      no generated example files were added to Git.
- [ ] Run the Task 3 verification and impact commands; all focused tests,
      acceptance checks, and architecture gates must pass before Task 4.

### Task 4: Add isolated vendor smoke coverage and pinned CI validation

Justification: AG-D8 and repository risk R-CI. A first-class target needs
version-pinned evidence from the real vendor validator, but vendor CLI execution
must remain offline, bounded, and isolated from developer state.

Files:

- `internal/target/antigravity/antigravity_smoke_test.go` — `vendor_smoke` test
  that requires `agy`, protects the normal `~/.gemini` tree, validates the
  golden plugin under temporary home/config/cache roots, blocks proxies, bounds
  output/time, and asserts component discovery without invoking mutating
  commands.
- `internal/testutil/vendorsmoke/` tests only if a missing shared helper is
  needed; prefer existing `RequireExecutable`, `UserHome`, `ProtectPaths`,
  `Environment`, and `Run` unchanged.
- `.github/workflows/ci.yml` — add pinned `ANTIGRAVITY_CLI_VERSION: 1.1.3` and
  `ANTIGRAVITY_CLI_SHA256`; rename the quality fixture step from six to seven;
  download/check/extract/install the official validator in `safe-validators`;
  validate both generated Antigravity package roots with blocked proxies and
  isolated config.
- `.github/workflows/release.yml` — no behavioral change expected because its
  release contract already runs `go test ./...` and
  `scripts/check-acceptance-fixture`; update wording only if it names six
  targets.
- `docs/release.md` — record pinned Antigravity validator version/digest,
  isolation boundary, seven-tree release inspection, and the no-install policy.

Implementation details:

- The smoke test build tag remains exactly `//go:build vendor_smoke`.
- Resolve the golden path to an absolute path before invoking `agy`.
- Protect the real `~/.gemini` directory before and after the process with the
  shared digest harness.
- Use a 30-second timeout and the shared 32 KiB bounded output contract.
- Set `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `TMPDIR`, `HTTP_PROXY`,
  `HTTPS_PROXY`, and `NO_PROXY` through the shared isolated environment.
- Invoke only `agy plugin validate <absolute-golden-root>`. Assert exit success
  and stable evidence such as `skills` and `agents`; strip or tolerate ANSI
  decoration rather than matching color bytes.
- In CI, download the exact Linux x64 tarball, verify the pinned SHA-256 with
  `sha256sum --check --strict`, extract member `antigravity`, and install it as
  `/usr/local/bin/agy`. Run `agy --version` and require `1.1.3` before use.
- Validate:
  `testdata/cc-thingz-hooks/generated/antigravity/core-tools` and
  `testdata/cc-thingz-hooks/generated/antigravity/workflow-tools`.
- Keep proxies blocked for validation and retain temporary `HOME` and
  `XDG_CONFIG_HOME`. Do not call install, import, list, link, enable, disable, or
  uninstall.
- Keep the release workflow network-independent beyond its existing release
  operations. Pinned validator installation belongs in CI safe validation, not
  the produced Agent Bundler binary.

Preconditions:

- Task 3 is complete and the generated acceptance roots validate locally.
- Official release 1.1.3 and its digest still exist at the pinned URL.

Postconditions:

- Default tests remain independent of `agy`.
- Tagged smoke tests compile everywhere and run safely when `agy` exists.
- CI verifies real Antigravity output with pinned vendor bytes.
- Release documentation names the exact evidence and limitations.

Fitness gate: workflow and test changes add no architecture dependencies.
`actionlint`, smoke-test compilation, default tests, Archfit, and the acceptance
fixture must pass. No `.archfit.yaml` exception is expected.

Impact commands:

```sh
gitnexus impact RunNativeChecks --file internal/artifact/nativeverify/nativeverify.go --direction upstream --depth 3 --include-tests --repo agentbundler
gitnexus detect-changes --scope unstaged --repo agentbundler
```

Verification commands:

```sh
gofmt -w internal/target/antigravity/antigravity_smoke_test.go
go test ./internal/target/antigravity/...
go test -run '^$' -tags=vendor_smoke ./...
go test -tags=vendor_smoke ./internal/target/antigravity/...
actionlint -no-color .github/workflows/ci.yml .github/workflows/release.yml
go run ./cmd/agbun build --root testdata/cc-thingz-hooks
validator_root="$(mktemp -d)"
trap 'rm -rf "$validator_root" testdata/cc-thingz-hooks/generated' EXIT
mkdir -p \
  "$validator_root/home" \
  "$validator_root/config" \
  "$validator_root/cache"
HOME="$validator_root/home" \
XDG_CONFIG_HOME="$validator_root/config" \
XDG_CACHE_HOME="$validator_root/cache" \
HTTP_PROXY=http://127.0.0.1:9 \
HTTPS_PROXY=http://127.0.0.1:9 \
NO_PROXY='' \
agy plugin validate testdata/cc-thingz-hooks/generated/antigravity/core-tools
HOME="$validator_root/home" \
XDG_CONFIG_HOME="$validator_root/config" \
XDG_CACHE_HOME="$validator_root/cache" \
HTTP_PROXY=http://127.0.0.1:9 \
HTTPS_PROXY=http://127.0.0.1:9 \
NO_PROXY='' \
agy plugin validate testdata/cc-thingz-hooks/generated/antigravity/workflow-tools
rm -rf "$validator_root" testdata/cc-thingz-hooks/generated
trap - EXIT
scripts/check-acceptance-fixture
scripts/check-architecture
archfit check --config .archfit.yaml
git diff --check
```

Manual checks:

- Review the workflow URL, version, digest, archive member, and installed binary
  name against the official release page.
- Confirm the workflow uses no floating `latest` asset or installer pipe.
- Confirm smoke tests protect the real configuration path and do not leave
  temporary generated output in the repository.
- Confirm release workflow behavior remains unchanged except any necessary
  target-count wording.

- [ ] Add the isolated `vendor_smoke` validator test using only shared bounded
      subprocess helpers and `agy plugin validate`.
- [ ] Add tests or assertions proving normal `~/.gemini` state is unchanged and
      validator output is handled without depending on ANSI bytes.
- [ ] Pin Antigravity CLI 1.1.3 and the verified Linux x64 digest in CI; download,
      checksum, extract, install as `agy`, and verify its version.
- [ ] Update CI target-count wording and validate both generated Antigravity
      package roots with temporary config and blocked proxies.
- [ ] Update `docs/release.md` with pinned evidence, isolation, seven-target
      inspection, and the explicit no-install/no-model boundary.
- [ ] Run smoke compilation, the real local smoke, Actionlint, acceptance,
      architecture, and diff checks; all must pass before Task 5.

### Task 5: Final documentation, repository metadata handoff, and release verification

Justification: AG-D9 and all success criteria. Users need exact target behavior,
configuration, example, validation, and installation boundaries. The repository
owner also needs reproducible GitHub metadata and release commands, but those
remote mutations occur only after implementation approval and merge.

Files:

- `README.md` — add Antigravity CLI to the supported-target summary and scope;
  link the Conductor-shaped example; change “six” to “seven”; retain the
  compiler-not-installer boundary.
- `docs/README.md` — index the Antigravity example and target contract.
- `docs/architecture.md` — include the seventh adapter and native-resource
  passthrough/native-validation flow; keep module boundaries unchanged.
- `docs/configuration.md` — list `antigravity` as a target; document package
  profile/separate mode, strict package names, supported agent frontmatter,
  `src/plugins/antigravity/<component>` native-resource declarations, and an
  example rule/MCP tree.
- `docs/customization.md` — show Antigravity target sidecars and warn that raw
  target-native files are explicit gaps, not portable semantics.
- `docs/guide.md` — include Antigravity in the end-to-end build/check flow.
- `docs/quickstart.md` — add Antigravity to target arrays and generated-tree
  examples without making installation part of `agbun`.
- `docs/targets-and-cli.md` — add package paths, no catalog, capability cells,
  event limitations, native validation, archive behavior, exact optional
  `agy plugin install` example, seven-target wording, and official links.
- `docs/troubleshooting.md` — add invalid plugin name, unsupported agent field,
  portable-hook rejection, missing `agy`, native-validator failure, and blocked
  install-state guidance.
- `docs/vendor-package-contracts.md` — reconcile the initial contract entry with
  implemented behavior and pinned CI evidence; do not broaden claims.
- `docs/release.md` — finalize `v0.5.0` release notes/checklist guidance and
  seven-target acceptance language.
- `skills/agentbundler/SKILL.md` — include Antigravity in agent-facing target
  routing, configuration examples, capability limits, and verification commands.
- `docs/plans/2026-07-17-antigravity-cli-target.md` — record any approved scope
  correction and final command results; RalphEx will move the completed plan to
  `docs/plans/completed/`.

Documentation requirements:

- Use “Antigravity CLI plugin” and executable `agy` consistently.
- Show only verified manifest fields and paths.
- State that skills become slash commands through Antigravity discovery; Agent
  Bundler does not emit legacy `commands/`.
- Distinguish portable agents from raw target-native resources.
- State that portable hooks are unsupported while explicit native `hooks.json`
  can be preserved without semantic validation.
- Explain that raw MCP configuration and scripts are trusted code/configuration;
  `agy plugin validate` is validation, not a sandbox.
- Explain flat one-package output versus multi-package package-ID roots and the
  absence of a marketplace.
- Keep `agy plugin install` optional and external. Never recommend
  `--dangerously-skip-permissions`.
- Link official docs, official CLI releases, and pinned Conductor source.

Preconditions:

- Tasks 1 through 4 are complete and all focused checks pass.
- There are no unresolved behavior, security, or validator blockers.
- Existing target output differences have been explained or removed.

Postconditions:

- All project documentation and agent-facing instructions match the code.
- The complete validation gate passes from a clean working tree plus intended
  changes.
- GitNexus change detection and scoped architecture re-review show no unexpected
  cross-module impact.
- GitHub description/topics and `v0.5.0` commands are ready for the owner after
  merge.

Fitness gate: final `scripts/check-architecture` and
`archfit check --config .archfit.yaml` must pass. Re-run GitNexus analysis if
stale, then inspect all changed processes/modules. No architecture waiver is
allowed for this target.

Impact commands:

```sh
gitnexus detect-changes --scope unstaged --repo agentbundler
```

Verification commands:

Run every command under `## Validation Commands`, then:

```sh
git status --short
git diff --stat
git diff --check
gitnexus detect-changes --scope unstaged --repo agentbundler
```

Manual checks:

- Review every Antigravity claim against `docs/vendor-package-contracts.md` and
  the official sources.
- Review generated `plugin.json`, skill, agent, rule, MCP, README, provenance,
  and archive paths for both flat and multi-package cases.
- Confirm no user home, plugin install state, credentials, model service, remote
  repository, GitHub metadata, or release tag was mutated during RalphEx tasks.
- Run a scoped architecture review over `internal/compiler/model`,
  `internal/compiler/source`, `internal/compiler/composition`,
  `internal/target/antigravity`, `internal/target/packageoutput`,
  `internal/target`, `internal/compiler`, `internal/artifact`, and `cmd/agbun`.
- Confirm the intended semantic-version bump is minor (`v0.5.0`) rather than a
  patch because this adds a public target.

- [ ] Update all listed human-facing and agent-facing documentation with exact
      Antigravity paths, capabilities, limits, validation, example, security,
      and external installation guidance.
- [ ] Remove every stale “six targets” reference in active documentation,
      scripts, tests, and workflow labels; do not rewrite historical completed
      plans except this active plan’s completion move.
- [ ] Run Markdownlint on the new example README and this plan. Review edits to
      existing Markdown with `git diff --check`; do not turn historical MD013
      cleanup in unrelated text into hidden feature scope.
- [ ] Run the complete validation gate: format, diff, all Go tests, smoke
      compilation, race, vet, GolangCI-Lint, Pi runtime, seven-target fixture,
      architecture, Archfit, Actionlint, Gitleaks, scoped Markdownlint, local
      `agy` golden/example validation, and native check.
- [ ] Run GitNexus change detection, inspect every affected process/module, and
      resolve unexpected target-to-source/composition/artifact coupling.
- [ ] Perform the scoped architecture re-review and record whether AG-D1 through
      AG-D9 and every success criterion are satisfied.
- [ ] Verify `git status` contains only intended source, test, fixture,
      workflow, documentation, example, and completed-plan changes; remove all
      temporary generated output.
- [ ] Record final verification evidence in the RalphEx task result and leave
      remote GitHub metadata, merge, tagging, release, and Homebrew dispatch for
      Post-Completion.

## Acceptance criteria

- The target ID, source parsers, composition, provenance, target registry, CLI,
  compiler, archive path, tests, and documentation all recognize
  `antigravity`.
- `plugin.json` exactly follows the verified schema and rejects invalid package
  names/descriptions before writing.
- Skills, supported agents, resources, and explicit Antigravity-native files
  render deterministically; unsupported semantics fail visibly.
- Portable hooks are never rendered or silently converted for Antigravity.
- The Conductor-shaped example builds, checks, and validates from its documented
  commands.
- The seven-target acceptance fixture passes twice with byte-identical plans and
  files and detects Antigravity drift.
- `agbun check --native` declares and executes one isolated
  `agy plugin validate .` check per plugin root.
- Pinned CI validator bytes match the official 1.1.3 Linux x64 digest and validate
  both acceptance roots without network access or user-state mutation.
- Every command in the complete validation gate succeeds.
- Active docs contain no stale six-target claims and no fabricated Antigravity
  fields, hook parity, marketplace, or installation behavior.
- Architecture checks show the new target remains inside the target module and
  creates no cycles or forbidden dependencies.
- The repository is ready for reviewed merge and a `v0.5.0` minor release.

## Safety notes

- Plugin assets, native hooks, MCP servers, and scripts are trusted input.
  Validation does not sandbox them. Default tests must never execute copied
  plugin scripts or start MCP servers.
- `agbun build` replaces the configured output tree. All examples and tests must
  use dedicated or temporary `generated` directories.
- `agy` mutating subcommands parse help flags unsafely in 1.1.3. Automated tests
  may invoke only `agy --version` and `agy plugin validate`.
- Native verification must use temporary home/config/cache paths and bounded
  subprocess output/time. A missing validator is a verification failure, not a
  skip, in production `check --native`.
- CI may download only the pinned official release asset and must verify its
  SHA-256 before execution.
- No data migration or irreversible production operation belongs to this plan.
- RalphEx should execute in an isolated worktree. Failed tasks must leave the
  repository inspectable and must not delete unrecognized user files.
- Remote GitHub metadata, merging, tags, releases, and Homebrew dispatch are
  intentionally outside RalphEx checkboxes.

## Re-review

After Task 5, run a scoped `architecture-review` on the model/source/composition
to target/packageoutput/compiler/artifact/CLI flow. Verify:

- Antigravity vendor volatility remains confined to
  `internal/target/antigravity` and pinned contract docs;
- composition contains only explicit target-native recognition, not vendor file
  parsing;
- no target imports source, composition, or artifact modules;
- native verification remains generic and isolated;
- unsupported hooks and agent fields fail before output;
- acceptance and CI fitness checks prevent the target from drifting.

## Post-Completion

These actions mutate remote state or depend on merged `master`; they are not
RalphEx tasks and contain no checkboxes.

### Pull request and merge

- Review the final diff and architecture-review result.
- Open a PR whose title is `feat: add Antigravity CLI target`.
- In the PR body, summarize the strict manifest, supported assets, raw native
  passthrough, unsupported portable hooks, pinned validator, Conductor example,
  and seven-target fixture.
- Require normal CI and `Pinned safe vendor validators` to pass.
- Merge to `master` without bypassing branch protections.

### GitHub description and repository topics

After merge, update repository discovery metadata:

```sh
gh repo edit alexei-led/agentbundler \
  --description 'Compile portable coding-agent skills, agents, hooks, and resources into native packages for Claude Code, Codex, Pi, Copilot CLI, Cursor, Grok Build, and Antigravity CLI.' \
  --add-topic agent-skills \
  --add-topic agentic-ai \
  --add-topic ai-agents \
  --add-topic antigravity-cli \
  --add-topic claude-code \
  --add-topic coding-agents \
  --add-topic codex-cli \
  --add-topic cursor \
  --add-topic developer-tools \
  --add-topic github-copilot \
  --add-topic go \
  --add-topic grok \
  --add-topic mcp \
  --add-topic pi-coding-agent \
  --add-topic plugin-system

gh repo view alexei-led/agentbundler \
  --json description,repositoryTopics,url
```

Do not set a homepage unless a stable project site exists.

### Version tag and release

The latest tag at planning time is `v0.4.8`. After the merged commit is on
`master`, create an annotated next-minor tag:

```sh
git switch master
git pull --ff-only origin master
git status --short
scripts/check-acceptance-fixture
git tag -a v0.5.0 -m 'Release v0.5.0

- Add first-class Antigravity CLI plugin output.
- Render strict plugin manifests, skills, supported agents, and explicit native resources.
- Validate generated plugins with pinned Antigravity CLI 1.1.3.
- Add a tested Conductor-shaped example and seven-target acceptance coverage.'
git push origin v0.5.0
```

Then monitor and verify the existing release workflow:

```sh
gh run list --workflow release.yml --limit 5
gh run watch --exit-status "$(gh run list --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
gh release view v0.5.0 --json tagName,isDraft,isPrerelease,assets,url
```

Confirm that the workflow:

- validates the semantic tag on `master`;
- runs all tests and the seven-target fixture;
- builds all configured OS/architecture binaries;
- publishes checksums and release notes from the annotated tag;
- dispatches the Homebrew formula update using the existing protected token.

### External plugin smoke

After release, optionally test a generated example in a disposable Antigravity
profile. This is a manual integration check, not a product test:

```sh
tmp_root="$(mktemp -d)"
cp -R examples/antigravity-conductor/. "$tmp_root/"
go run ./cmd/agbun build --root "$tmp_root"
agy plugin validate "$tmp_root/generated/antigravity"
```

Only if the owner explicitly approves a mutating disposable-profile test, set a
temporary `HOME` and run `agy plugin install` there. Never run install/uninstall
against the normal user profile as part of release automation.
