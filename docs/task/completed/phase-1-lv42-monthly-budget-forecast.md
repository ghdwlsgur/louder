# Phase 1, Level 42: Monthly Budget Forecast

## Goal

Implement the existing `BudgetPolicy.spec.forecast.enabled` option so the scheduled Analyzer can estimate month-end spend and notify when the estimate exceeds the configured budget.

## Decisions

- Project month-to-date daily costs at a constant calendar-day pace: `MTD daily spend / current UTC day-of-month * number of days in the current UTC month`.
- Emit a distinct `BudgetForecast` event only when projected month-end spend is strictly greater than the budget.
- Send at most one forecast alert per BudgetPolicy per UTC month, recording the month only after successful delivery.
- Exclude NCP monthly invoice records from pace projection because they are monthly totals rather than daily observations. They remain part of actual monthly budget tracking.
- Treat this as a simple estimate over currently stored data; provider data delay can affect the result.

## Scope

- Add exact-rational forecast evaluation over normalized daily cost records.
- Add forecast notification routing, payload, monthly status receipt, and notification event schema support.
- Keep normal monthly threshold notifications and daily anomaly behavior intact.
- Add unit tests for projection boundaries, UTC month lengths, NCP exclusion, delivery failure, and deduplication.
- Update architecture, harness, secrets, and notification examples to describe the new event.

## Plan

1. Add a `RunBudgetPolicy` runtime test for an enabled forecast exceeding budget; run it and confirm no forecast notification is currently produced.
2. Implement exact forecast intent evaluation and add evaluator tests for below-budget, above-budget, leap-year February, and NCP monthly-record handling.
3. Add `BudgetForecast` NotificationPolicy event routing, an at-most-once-per-month successful-delivery receipt, and status/API schema coverage.
4. Add runtime tests for disabled forecasts, no-match routing, delivery failure without a receipt, and repeat-run suppression.
5. Run formatting, focused and full Go tests, vet, manifest validation, build, and the kind storage E2E.
6. Move this task document to `completed` only after verification succeeds; commit, push the branch, open a PR, and merge it under the repository workflow.

## Acceptance criteria

- Forecast-disabled policies produce no forecast notification.
- A forecast strictly above budget routes a `BudgetForecast` notification with MTD spend, projected spend, budget, currency, and month.
- A forecast equal to or below budget does not notify.
- NCP monthly invoice amounts are not extrapolated as daily spend.
- A successful forecast notification is deduplicated for that UTC month; a failed delivery records no receipt.
- Existing thresholds and daily anomaly paths continue to pass.

## Verification record

- RED: `TestRunBudgetPolicySendsMonthlyBudgetForecast` failed before forecast evaluation existed because the runtime emitted no notification.
- GREEN: forecast evaluator, routing, status receipt, and repeated-run suppression tests passed.
- `make manifests` regenerated the BudgetPolicy status and NotificationPolicy event CRD schemas.
- `go test ./... -count=1` passed.
- `make vet` passed.
- `make fmt-check` passed.
- `make build` passed.
- `make kind-e2e-storage` passed, including the ClickHouse-backed forecast runtime test.
