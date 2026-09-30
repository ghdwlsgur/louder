# Phase 1 · LV1 — Harness Provider Scope and Notifier Coverage

> Status: completed (2026-09-30)
> Implementation: complete · Verification: document review complete · Deployment: not applicable

## Goal

Clarify the initial provider rollout and the place of notifier coverage in the test harness.

## Completion criteria

- [x] Name AWS, GCP, and Azure as the initial providers for contract tests.
- [x] State that later CSP implementations must pass the same contract.
- [x] Include notifier tests within analyzer and integration tests rather than as a separate harness layer.

## Changes

- Updated provider fixture, contract, and completion sections in `docs/harness.md` to identify the initial providers.
- Clarified that notifier coverage belongs to analyzer and integration test layers.

## Verification

- Searched Markdown references with `rg -n -i 'harness|test harness' --glob '*.md' .`.
- Reviewed `git diff -- docs/harness.md` and confirmed the edits were limited to the requested criteria.
- Tests: not applicable to this documentation-only change.

## Reference

- Follow-on work: [Phase 1 LV2 — Ariadne-Based Boilerplate and Task History](phase-1-lv2-ariadne-based-boilerplate-and-task-history.md).
