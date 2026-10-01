# Phase 1 LV18: Monthly Budget Threshold Evaluator

## Goal

Evaluate normalized month-to-date cost against provider-neutral `BudgetPolicy` objects and return threshold notification intents without sending notifications.

## Scope

- Add a pure Analyzer API that accepts normalized cost records, CloudAccounts, a BudgetPolicy, and an evaluation time.
- Select CloudAccounts whose `spec.metadata` matches every `BudgetPolicy.spec.selector` entry.
- Sum selected-account records whose usage interval starts in the current UTC calendar month and whose currency matches the budget currency.
- Return one intent for each configured threshold reached, preserving exact decimal amount text.
- Reject malformed policy thresholds and currency mismatches rather than silently producing misleading results.
- Add incremental public-interface tests and document the current evaluator boundary.

## Non-goals

- ClickHouse query implementation or Kubernetes scheduling.
- Teams HTTP delivery, notification policies, alert deduplication, or delivery retry.
- Forecasting, anomaly detection, daily summaries, stale-data checks, or collection-failure alerts.
- Currency conversion or mixed-currency aggregation.
- Changes to CloudAccount or BudgetPolicy schemas.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv17-clickhouse-cost-storage.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Add one public behavior test proving a selected account's current-month normalized cost reaches a configured budget threshold; confirm RED before implementation.
2. Add one behavior test for selector exclusion and UTC month boundaries; implement only the minimum account and record filtering.
3. Add one behavior test for exact decimal totals and each reached threshold; implement exact decimal aggregation and threshold intents.
4. Add one behavior test for malformed thresholds and currency mismatch; return stable errors without partial intents.
5. Document the pure evaluator boundary and record verification results.

## Completion criteria

- Accounts are selected only when all selector metadata pairs match.
- Only normalized records in the current UTC calendar month and before the evaluation instant contribute to spend.
- Amounts are summed without floating-point conversion or precision loss.
- Every reached configured threshold yields a deterministic intent with the exact spend, budget, currency, and threshold percentage.
- Invalid thresholds or selected-account currency mismatches return an error and no partial result.
- Tests require no ClickHouse, Kubernetes, CSP, or Teams access.
- `make test`, `make vet`, `make fmt-check`, and `git diff --check` pass.

## Design decisions and risks

- Thresholds are percentages of the monthly budget. Reaching exactly the threshold counts as reached.
- A policy with no thresholds produces no intents. Thresholds must be positive, at most 100, and unique; returned intents are ordered ascending.
- Cost records are attributed to a CloudAccount by provider and billing account ID. Records not belonging to selected accounts are ignored.
- Only records with a matching currency are valid for selected accounts; mixed-currency totals are rejected because this project has no exchange-rate conversion.
- This pure evaluator does not remember previous results, so a future scheduler/delivery layer must define alert deduplication before enabling recurring notifications.
- `usage_start` defines the UTC month bucket for the current account-day records. Future interval-spanning records will need an explicit allocation rule.

## Verification plan

- Run each focused behavior test immediately after adding it and capture genuine RED/GREEN results.
- `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `git diff --check`

## Verification results

- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -run TestEvaluateBudgetReturnsReachedThreshold -count=1` failed because the evaluator returned no threshold intent for an account at 80% budget.
- GREEN: The same focused test passed after the evaluator selected CloudAccounts by metadata and returned a reached budget intent.
- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -run TestEvaluateBudgetRejectsInvalidThresholdWithoutPartialIntents -count=1` failed because a threshold above 100 was accepted.
- GREEN: The invalid-threshold behavior and currency mismatch behavior now return errors with no partial intents; selected-account and UTC month filtering tests pass.
- GREEN: Exact `99.99 + 0.01` aggregation returns `100.00`; thresholds are ordered ascending independent of policy order.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `git diff --check` passed.
