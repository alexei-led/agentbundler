# Decision Hook Protocol Adapter

**Path**: `internal/target/hookdecision/` — shared portable decision-hook protocol wrapper
**Parent**: `internal/target`
**Submodules**: none (leaf)

## Purpose

This module translates the canonical Agent Bundler decision-hook process protocol into the verified vendor response protocols used by supported target leaves.

## Functional Responsibilities

- Detect decision capability use in a normalized hook.
- Wrap a POSIX command with canonical input and target protocol identity.
- Translate allow, deny, and input-rewrite decisions without changing their meaning.

## Subdomain Classification

**Core.** Security-relevant hook decision semantics are product behavior and target contracts are volatile.

## Public Contract

```text
uses-decision-capability(capability-uses) -> Boolean
wrap-posix(command, protocol, identity) -> shell command
```

The wrapper is a target-render helper. It does not execute the command; artifact/native verification and the vendor runtime own process execution.

## Integrations

- **Counterpart**: target vendor leaves
  - **Direction**: Claude, Codex, Copilot, Cursor, and Grok leaves request protocol-specific wrapping.
  - **Shared knowledge**: normalized decision capability keys and the canonical hook input contract.
- **Counterpart**: `internal/compiler/model`
  - **Direction**: reads capability-use values only.

## Constraints and Invariants

- Protocol names are closed and target-owned; unsupported decision mappings fail in the leaf.
- The wrapper never reads files, invokes processes, accesses network or environment state, or writes output.
- Shell quoting preserves command argument boundaries.
- Decision semantics are never silently dropped or weakened.

## Test Specification

- Each supported protocol receives canonical pre-tool input.
- Allow, deny, rewrite, malformed output, non-zero exit, and unsupported protocol cases are covered.
- Quoting remains safe for spaces, quotes, and shell metacharacters.
