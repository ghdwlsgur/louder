# Phase 1 LV20: Evaluate Budgets from Stored Costs

## Goal

Connect the monthly budget evaluator to the ClickHouse reader through a storage interface so callers can evaluate a policy against persisted account costs.

## Scope

- Define a provider/account scope and cost-reader interface in the storage package.
- Have the ClickHouse Store implement that shared interface without changing its query behavior.
- Add an Analyzer service function that selects accounts by policy metadata, requests their current UTC month interval, then evaluates returned normalized records.
- Avoid storage reads when no CloudAccounts match the policy selector.
- Test the service through a fake public CostReader interface.
- Document the service boundary and remaining runtime work.

## Non-goals

- Kubernetes controller, CronJob, or scheduling.
- Teams notification transport, delivery deduplication, or retry.
- Changes to budget policy schema or status.
- Forecasts, anomaly detection, stale-data checks, or currency conversion.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv18-budget-threshold-evaluator.md`
- `docs/task/completed/phase-1-lv19-clickhouse-cost-reader.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Add a RED public Analyzer behavior test proving a matching policy requests the selected accounts and current UTC month from a CostReader, then returns threshold intents.
2. Add a RED test proving no matching accounts avoids the reader and returns no intents.
3. Add the common storage scope/reader interface and make the ClickHouse Store implement it.
4. Implement the minimum orchestration function and propagate read errors without returning partial intents.
5. Document that this service is callable but not yet scheduled or connected to Teams.

## Completion criteria

- Selector matching determines explicit provider/account pairs passed to the reader.
- Reader bounds are `[first day of current UTC month, evaluation time in UTC)`.
- The service evaluates only the rows returned by the reader and preserves exact decimal totals.
- No matching accounts causes no reader call and no notification intents.
- Read or policy errors return no partial intents.
- ClickHouse Store satisfies the shared storage interface at compile time.
- `make test`, `make vet`, `make fmt-check`, and `git diff --check` pass.

## Design decisions and risks

- The shared storage interface owns `AccountScope` to keep Analyzer independent of ClickHouse implementation types.
- The policy and account objects are provided by the caller; this service does not read Kubernetes objects itself.
- A matching policy with no stored rows evaluates as zero spend. Data freshness and collection failures remain separate future behaviors.
- No scheduler invokes the service in this task, so returned intents are not yet delivered or deduplicated.

## Verification plan

- Run each focused service behavior test immediately after adding it and record RED/GREEN results.
- `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer ./internal/storage/... -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `git diff --check`

## Verification results

- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -run TestEvaluateStoredBudgetReadsSelectedAccountsForCurrentUTCMonth -count=1` failed because the shared storage package and `EvaluateStoredBudget` API did not exist.
- GREEN: The focused service test passed after adding the shared `CostReader`/`AccountScope` contract and the Analyzer orchestration function.
- GREEN: The service tests verify exact matching account scopes, UTC month bounds, no-match short-circuiting, and no partial intents on reader errors.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer ./internal/storage/... -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `git diff --check` passed.
