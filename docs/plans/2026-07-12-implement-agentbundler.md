# Implement Agentbundler

## Overview

Agentbundler is a standalone Go compiler for coding-agent packages. It turns an explicit source repository into deterministic, target-native package trees without becoming a package manager, installer, registry, or universal runtime. Without this module, each repository must maintain target-specific compiler logic and cannot prove that generated output is current.

- Design tree: `./` — 21 modules in 4 waves, ordered bottom-up by module height
- All design modules are in scope; the repository currently contains design documents only
- Each task implements exactly one module; its complete specification is that module's `module.md`

## Development Approach

- **Testing approach**: risk-based. Add focused tests for non-trivial logic, boundaries, module contracts, composition, and regressions; do not write tests merely to satisfy a category or count. Test-first development is optional and is useful only when it clarifies behavior or guards a bug.
- **Tech stack**: defined in `docs/tech-stack.md` — read it before any task; every task uses it, with no per-task deviations. New stack decisions made during implementation are recorded there immediately
- Each task's spec is exactly two files — the module's `module.md` and `docs/tech-stack.md`; needing any other file to implement the module is a design defect: stop and report it (⚠️), do not improvise
- Implement the module's code inside its own folder; never reach into another module's folder or internals — consumers code against the counterpart contracts restated in their own `module.md`
- Tasks in the same wave are independent: a parallel executor may run them concurrently in isolated worktrees; a sequential executor completes them in task order — both are correct
- Never start a parent module's task before all its submodules' tasks are complete
- **CRITICAL: tests must prove behavior, not implementation trivia.** Use the module's Test Specification as a risk checklist, not as a mandatory test count.
- **CRITICAL: all relevant tests must pass before the task is complete** — no exceptions
- **CRITICAL: if the design proves wrong or incomplete during implementation, update the module.md first** (it is validated on write), mark the finding with ⚠️ in this plan, then implement to the updated design — never silently diverge from the documents

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with `➕` prefix
- Document issues and blockers with `⚠️` prefix
- Update this plan if implementation deviates from the design tree — and update the affected module.md, which is the normative record

## Implementation Steps

### Task 1 [Wave 0]: Implement `cmd/agentbundler/`

- [ ] Read `cmd/agentbundler/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `cmd/agentbundler/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

⚠️ Blocked: `cmd/agentbundler/module.md` does not define concrete flag grammar/defaults, manifest search semantics, JSON/human rendering schemas, or an importable compiler Go API.

### Task 2 [Wave 0]: Implement `internal/artifact/compare/`

- [ ] Read `internal/artifact/compare/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/artifact/compare/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

⚠️ Blocked: `internal/artifact/compare/module.md` does not define an interoperable Go API or type-ownership seam for the restated `BuildPlan`.

### Task 3 [Wave 0]: Implement `internal/artifact/nativeverify/`

- [ ] Read `internal/artifact/nativeverify/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/artifact/nativeverify/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

⚠️ Blocked: `internal/artifact/nativeverify/module.md` omits its Go-callable API, diagnostic categorization, and bounded output/truncation semantics.

### Task 4 [Wave 0]: Implement `internal/artifact/provenance/`

- [ ] Read `internal/artifact/provenance/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/artifact/provenance/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

⚠️ Blocked: `internal/artifact/provenance/module.md` does not supply the required provenance inputs or define the JSON schema, hash algorithm/scope/order, output-root representation, or Go API behavior.

### Task 5 [Wave 0]: Implement `internal/artifact/write/`

- [ ] Read `internal/artifact/write/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/artifact/write/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 6 [Wave 0]: Implement `internal/compiler/composition/`

- [ ] Read `internal/compiler/composition/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/compiler/composition/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 7 [Wave 0]: Implement `internal/compiler/model/`

- [ ] Read `internal/compiler/model/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/compiler/model/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 8 [Wave 0]: Implement `internal/compiler/source/bundle/`

- [ ] Read `internal/compiler/source/bundle/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/compiler/source/bundle/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 9 [Wave 0]: Implement `internal/compiler/source/claudeplugin/`

- [ ] Read `internal/compiler/source/claudeplugin/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/compiler/source/claudeplugin/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 10 [Wave 0]: Implement `internal/compiler/source/skillrepo/`

- [ ] Read `internal/compiler/source/skillrepo/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/compiler/source/skillrepo/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 11 [Wave 0]: Implement `internal/target/claude/`

- [ ] Read `internal/target/claude/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/target/claude/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 12 [Wave 0]: Implement `internal/target/codex/`

- [ ] Read `internal/target/codex/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/target/codex/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 13 [Wave 0]: Implement `internal/target/copilot/`

- [ ] Read `internal/target/copilot/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/target/copilot/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 14 [Wave 0]: Implement `internal/target/cursor/`

- [ ] Read `internal/target/cursor/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/target/cursor/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 15 [Wave 0]: Implement `internal/target/grok/`

- [ ] Read `internal/target/grok/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/target/grok/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 16 [Wave 0]: Implement `internal/target/pi/`

- [ ] Read `internal/target/pi/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module per its Functional Responsibilities, Public Contract, and Constraints and Invariants inside `internal/target/pi/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 17 [Wave 1]: Implement `internal/artifact/`

- [ ] Read `internal/artifact/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module's own code and wire its submodules per its Internal Design inside `internal/artifact/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 18 [Wave 1]: Implement `internal/compiler/source/`

- [ ] Read `internal/compiler/source/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module's own code and wire its submodules per its Internal Design inside `internal/compiler/source/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 19 [Wave 1]: Implement `internal/target/`

- [ ] Read `internal/target/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module's own code and wire its submodules per its Internal Design inside `internal/target/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 20 [Wave 2]: Implement `internal/compiler/`

- [ ] Read `internal/compiler/module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the module's own code and wire its submodules per its Internal Design inside `internal/compiler/`
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 21 [Wave 3]: Implement `./` (root module)

- [ ] Read `module.md` in full — it is the complete and only spec for this task
- [ ] Assess test value from this module's Test Specification; add focused tests only for material logic, boundaries, contracts, composition, or regression risk
- [ ] Implement the root module's own code and wire its submodules per its Internal Design
- [ ] Run relevant verification, including `go test` for any tests added or affected

### Task 22: Verify acceptance criteria

- [ ] Run `go test ./...` — all implemented package tests must pass
- [ ] Run the fractal module validator in tree mode over `.` — the design tree must remain defect-free
- [ ] Run `gofmt` and `go vet ./...` — all issues fixed
- [ ] Verify every `⚠️` noted during implementation is resolved or explicitly accepted by the user

### Task 23: [Final] Update documentation

- [ ] Update `README.md` if the implementation affects its current product description
- [ ] Verify each implemented `module.md` still matches what was built; use `/modularity:fractal-align` for a rigorous alignment pass

## Post-Completion

Run `/modularity:fractal-align` to verify code ↔ design alignment rigorously. Perform manual testing, deployment, and external-system updates as applicable.
