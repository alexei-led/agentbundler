<!-- markdownlint-disable MD013 -->

# Repository-root compatibility

Package profiles normally live only under `output/<target>`. Some vendor CLIs
still discover a repository by fixed files at its root. Repository-root
compatibility is an opt-in distribution feature that generates those files
without symlinks or copied package trees.

## Configure it

Add `compatibility.rootManifests` to `agentbundle.json`:

```json
{
  "output": "dist",
  "compatibility": {
    "rootManifests": ["claude", "codex", "copilot", "cursor", "pi"]
  }
}
```

Every listed target must also be selected. Claude, Codex, Copilot, Cursor, and
Grok require package profile, separate mode, and `distribution` metadata so the
target renderer produces its native marketplace. Pi requires package profile
and an existing repository `package.json`. An empty list, duplicate target,
project profile, missing catalog metadata, unknown target, or Antigravity entry
fails validation.

The feature is disabled when `compatibility` is absent. It never mutates every
Agent Bundler project implicitly.

## Generated files and routing

Agent Bundler derives root wrappers from the generated target-native manifest;
that target manifest is the single source of wrapper metadata. It rewrites only
local relative sources. Remote strings and non-empty remote source objects are
preserved.

| Target | Repository-root file | Local route |
| ------- | -------------------- | ----------- |
| Claude | `.claude-plugin/marketplace.json` | `./dist/claude/<package>` |
| Codex | `.agents/plugins/marketplace.json` | local source object path `./dist/codex/<package>` |
| Copilot | `.github/plugin/marketplace.json` | `./dist/copilot/<package>` |
| Cursor | `.cursor-plugin/marketplace.json` | `./dist/cursor/<package>` |
| Grok | `.claude-plugin/marketplace.json` | `./dist/grok/<package>` |
| Pi | merged root `package.json#pi` and `.npmrc` | `./dist/pi/...` plus root runtime dependencies |

`dist` above is the configured `output`; another normalized output path is used
verbatim. Local sources must resolve to a generated target package manifest.
Absolute paths, backslashes, volume separators, traversal (including contained
`a/../b` traversal), dangling targets, duplicate IDs, duplicate local roots,
non-regular destinations, and symlinked path components fail with structured
diagnostics.

Agent Bundler records only its ownership data in
`.agentbundler/compatibility.json`. Fully generated marketplace and Codex agent
files are owned as complete files. Prior ownership accepts only the four fixed
marketplace markers and `.codex/agents/*.toml` entries that exactly match a
canonical generated Codex profile. Pi ownership entries must be present in the
current canonical generated Pi manifest. Unknown, forged, escaping, symlinked,
or otherwise unprovable ownership fails with
`compatibility-ownership-invalid`; it is never scheduled for deletion.
Removing a target from `rootManifests`, or removing the compatibility block,
removes only validated files named by that state. Untracked legacy root files
and unrelated package fields are never deleted as stale output.

## Codex project agents

Codex plugins do not contain custom agent profiles. The marketplace installs
skills and hooks, but it does not install the generated
`dist/codex/.codex/agents/*.toml` project profiles.

When Codex compatibility is enabled, Agent Bundler also writes the same
canonical profile bytes to root `.codex/agents/*.toml`. Codex discovers these
profiles when the repository is trusted. They are compatibility artifacts, not
part of the marketplace package installation. A repository-root install used
outside a project still gets only the plugin capabilities.

Codex also supports `agents.<name>.config_file` paths in `.codex/config.toml`,
but Agent Bundler does not merge TOML because that file can contain unrelated
project security and model configuration. Copying the generated profile bytes
is the smaller owned contract.

## Pi root merge and installs

Pi has no marketplace file. Agent Bundler merges generated Pi registration into
the existing development `package.json`:

- generated `pi.extensions`, `pi.skills`, and `pi.subagents.agents` paths are
  rebased under `./dist/pi`;
- for separate multi-package output, the package ID remains in the route;
- a generated `./node_modules/...` extension stays root-relative;
- generated runtime dependencies are added to root `dependencies`;
- unrelated top-level fields, dependency entries, `pi` fields, and author-owned
  array entries remain;
- an unequal existing dependency version is a collision, not an overwrite.

The ownership state lists every generated dependency and Pi array entry. On the
next build, Agent Bundler removes the previous generated entries before merging
the new manifest. Disabling Pi compatibility removes those entries while
preserving unrelated development fields. `package.json` is serialized as
stable two-space JSON, so key order and formatting become deterministic.

Pi compatibility also requires root `.npmrc` to contain
`legacy-peer-deps=true`. npm otherwise auto-installs broad development
`peerDependencies` during Pi's production Git install, which can pull a second
`@earendil-works/pi-coding-agent` or `@earendil-works/pi-ai` host and create a
host/runtime mismatch. `peerDependenciesMeta` does not prevent that install.
Agent Bundler preserves unrelated `.npmrc` bytes and keys. It preserves an
existing `true` setting as author-owned, rejects `false` or duplicate active
settings, and marks a setting it adds with an adjacent Agent Bundler comment.
Cleanup requires that exact marker/setting pair, removes only that pair, and
removes `.npmrc` only when no author content remains. A forged ownership record
cannot turn an unmarked author setting into generated cleanup state.

For a Git install, Pi clones the repository and runs `npm install --omit=dev`
when root `package.json` exists. Root runtime dependencies such as
`pi-subagents` are therefore installed at root, and an extension path such as
`./node_modules/pi-subagents/src/extension/index.ts` resolves without committed
`dist/pi/node_modules`. The generated `.npmrc` keeps npm from auto-installing
root development peers; normal dependencies of `pi-subagents`, including its
nested Pi TUI/typebox runtime when required, still install. Other generated
resources continue to resolve under `dist/pi`.

A local-path install does not install root dependencies. Run the repository's
package-manager install first. Agent Bundler does not assume
`dist/pi/node_modules` is committed and does not act as a bootstrap installer.
Package dependencies imported by author-owned native extensions must likewise
be declared in the generated Pi package metadata so they can be merged into the
root.

## Grok and Claude limitation

Grok automatically reads Claude Code marketplaces. It has no distinct
repository-root Grok marketplace manifest; its native marketplace sources live
in user configuration, while native project plugins use `.grok/plugins`.
Claude and Grok therefore collide at `.claude-plugin/marketplace.json`, and one
file cannot route identical package IDs to both `dist/claude` and `dist/grok`.

Agent Bundler rejects enabling both root routes with
`compatibility-marker-collision`. Choose one policy:

- enable Claude compatibility and distribute Grok through its target archive or
  an explicit local install; or
- enable Grok compatibility alone, which writes the shared Claude-compatible
  marker pointing to `dist/grok`.

The native `dist/grok` target remains unchanged in either case. Agent Bundler
does not claim a Grok-native root marketplace marker.

## Build, check, and package

`agbun build` first replaces the configured generated output, then writes the
prepared root compatibility files. If target output fails, root files are not
updated. Root writes reject symlinks and non-regular files and use same-directory
temporary replacement. Unchanged root files are not rewritten.

Compatibility builds must select every configured compatibility target and all
packages. `--target` is valid only when it includes every
`compatibility.rootManifests` target; `--package` is rejected. This avoids
rewriting root routes from a partial marketplace or deleting routes that were
not rendered.

`agbun check` is read-only. It compares target output and root compatibility
bytes, reports exact `COMPATIBILITY_DRIFT_MISSING`,
`COMPATIBILITY_DRIFT_CHANGED`, or `COMPATIBILITY_DRIFT_EXTRA` diagnostics, and
exits `2` for drift. `--json` returns the same diagnostic codes in the normal
machine-readable envelope.

`agbun package` requires both target output and root compatibility to be current,
but archives only `output/<target>`. Root wrappers, the ownership state, the
development `package.json`, `.npmrc`, and root Codex profiles are not included
in target archives.

## Migrate a legacy root manifest

1. Keep the legacy file in place.
2. Add its target to `compatibility.rootManifests`.
3. Run `agbun check --json`; the missing ownership state or changed routes are
   reported without writes.
4. Review the planned target paths, then run `agbun build` to adopt and record
   ownership.
5. Run `agbun check` again.
6. Verify with an isolated vendor CLI or repository clone before release.

Do not add symlinks or copied package roots. The compatibility files should
remain small routes into generated target output.

## Verification

```sh
agbun build
agbun check --json
agbun package --out release

go test ./internal/compatibility ./internal/compiler ./internal/artifact/archive
go test ./...
scripts/check-acceptance-fixture
```
