# Test Support

**Path**: `internal/testutil/` — test-only helpers and vendor smoke utilities
**Parent**: repository root
**Submodules**: `vendorsmoke`

## Purpose

This module contains test support only. It is not part of the production compiler, generated output, or runtime dependency graph.

## Functional Responsibilities

- Provide bounded vendor smoke-test setup and cleanup helpers.
- Protect test paths and isolate optional external CLI checks.

## Subdomain Classification

**Generic.** Test harness mechanics do not define product behavior, though fixtures validate core and target contracts.

## Public Contract

No production API. Helpers are consumed only by tests and test fixtures.

## Constraints and Invariants

- Production packages must not import this module.
- Smoke checks are opt-in and run against temporary roots and isolated vendor configuration.
- Test helpers do not publish, mutate user configuration, or alter source-owned files.
