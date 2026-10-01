# Phase 1 LV19: ClickHouse Cost Reader

## Goal

Expose normalized cost rows from ClickHouse for selected billing accounts and a bounded UTC usage interval so the Analyzer can evaluate stored data.

## Scope

- Add a reader API to the existing ClickHouse Store for provider/account scopes and a half-open usage interval.
- Query with `FINAL` so replayed records are returned as one logical row.
- Return the current normalized cost record fields without converting amount or currency.
- Return no rows without issuing a query when the requested account scope is empty.
- Sanitize database errors using the existing storage error contract.
- Add public Store tests and extend the local kind storage E2E to verify `FINAL` read semantics.

## Non-goals

- Analyzer scheduling or Kubernetes runtime.
- Teams notification delivery or deduplication state.
- SQL aggregation, currency conversion, forecasts, or anomaly analysis.
- Schema changes or CSP live access.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv17-clickhouse-cost-storage.md`
- `docs/task/completed/phase-1-lv18-budget-threshold-evaluator.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Add a RED Store behavior test proving an empty account scope returns an empty result without a database read.
2. Add a RED behavior test proving requested accounts and time bounds are passed through and normalized records are returned; implement the Store reader contract.
3. Add a RED behavior test proving reader errors are sanitized and invalid time ranges are rejected; implement validation and error handling.
4. Implement the native ClickHouse query using a half-open `usage_start` range and `FINAL`, with bound parameters for every account scope.
5. Extend kind storage verification to query a row after fixture replay and assert the normalized row fields.
6. Document the ClickHouse read contract and record verification results.

## Completion criteria

- Reader queries only requested provider/account pairs and `start <= usage_start < end`.
- The native query uses `FINAL` and returns only normalized schema fields.
- Amount strings, currencies, cost bases, IDs, and UTC timestamps are preserved.
- Empty account input returns an empty slice without calling ClickHouse.
- Invalid or empty time ranges fail explicitly; ClickHouse errors do not expose credentials or server details.
- The kind E2E proves the row is readable after replay and still resolves to one logical row.
- `make test`, `make vet`, `make fmt-check`, kind storage E2E, shell syntax checks, and `git diff --check` pass.

## Design decisions and risks

- Account scope is represented as explicit provider and billing account ID pairs; the reader does not infer accounts from policy metadata.
- Time bounds are UTC instants and are half-open to match the Collector's account-day intervals.
- The query applies `FINAL` because replacing-table background merges are asynchronous.
- This API reads normalized rows into memory; callers should bound account scopes and time intervals. Aggregation in ClickHouse can be considered if the data volume requires it.

## Verification plan

- Run each focused Store test immediately after adding it and capture genuine RED/GREEN results.
- `GOCACHE=/tmp/louder-go-build go test ./internal/storage/clickhouse ./internal/analyzer -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `KIND_CLUSTER_NAME=louder-e2e-lv19-reader GOCACHE=/tmp/louder-go-build make kind-e2e-storage`
- `bash -n scripts/kind/*.sh`
- `git diff --check`

## Verification results

- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/storage/clickhouse -run TestReadCostsReturnsEmptyForNoAccountsWithoutQuerying -count=1` initially failed to compile because the Store reader API did not exist; after adding the public contract, the behavioral test passed and confirmed no reader call for an empty account scope.
- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/storage/clickhouse -run TestReadCostsPassesUTCWindowAndAccountScopes -count=1` failed because the Store passed a non-UTC time location to its reader.
- RED: focused invalid-range and reader-error tests failed because ranges were not validated and backend errors were returned unsanitized.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/storage/clickhouse ./internal/analyzer -count=1` passed after UTC normalization, invalid-range validation, and error sanitization.
- GREEN: `KIND_CLUSTER_NAME=louder-e2e-lv19-reader GOCACHE=/tmp/louder-go-build make kind-e2e-storage` passed. The script replayed the fixture, checked exact normalized values through `FINAL`, and ran `TestNativeReaderReturnsNormalizedFinalRecord` against the ephemeral ClickHouse service through kubectl port-forward.
- GREEN: `bash -n scripts/kind/*.sh` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `git diff --check` passed.
