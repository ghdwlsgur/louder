# Phase 1, Level 41: Daily Anomaly Runtime with ClickHouse

## Goal

Verify that `RunBudgetPolicy` can read daily costs from ClickHouse, dispatch the anomaly through a fake Teams notifier, persist the anomaly date in `BudgetPolicy` status, and suppress a duplicate notification on a repeated run.

## Scope

- Use the existing kind storage E2E environment for a real ClickHouse instance.
- Use the controller-runtime fake client for Kubernetes resources and status updates.
- Use synthetic AWS cost rows and a fake notifier; do not call live cloud or Teams APIs.
- Keep production behavior unchanged unless the integration test exposes a runtime defect required for this behavior.

## Plan

1. Add an integration test that invokes `RunBudgetPolicy` twice against ClickHouse and checks the notification and persisted `LastNotifiedAnomalyDate`.
2. Run the focused kind integration test before adding the cost rows to confirm it fails because no anomaly is available.
3. Seed seven baseline days and one target day, rerun the test, and make the smallest test or runtime correction needed for it to pass.
4. Include the runtime test in `scripts/kind/e2e.sh` and document the runtime status receipt coverage in `docs/harness.md`.
5. Run formatting, focused tests, the full Go test suite, vet, build, and the kind storage E2E.
6. Move this task document to `completed` only after verification succeeds; commit, push the branch, open a PR, and merge it under the repository workflow.

## Acceptance criteria

- The first runtime pass emits one `CostAnomaly` notification based on persisted ClickHouse rows.
- The second pass emits no duplicate notification for the same UTC date.
- `BudgetPolicy.Status.LastNotifiedAnomalyDate` records the delivered date.
- Focused integration and repository verification pass without live CSP or Teams credentials.

## Verification record

- RED: kind storage E2E failed at the new runtime assertion with no notifications before synthetic rows were seeded.
- GREEN: kind storage E2E passed after seeding seven `$10` baseline days and one `$30` target day; two `RunBudgetPolicy` calls produced one notification and persisted `LastNotifiedAnomalyDate=2026-10-08`.
- `make test` passed.
- `make vet` passed.
- `make fmt-check` passed.
- `make build` passed.
- `make kind-e2e-storage` passed, including the ClickHouse runtime integration test.
