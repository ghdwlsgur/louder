# Phase 1, Level 44: Cost Data Freshness and Analysis Readiness

## Goal

Make collection execution, the billing interval requested from a provider, stored billing-period coverage, and source-data freshness distinct signals. Ensure daily and monthly analysis only evaluates accounts whose required data is ready, without treating a successful Kubernetes Job as proof that provider billing data is current.

## Scope

- Preserve `CollectorReady` as CronJob availability and `LastSuccessfulCollectionTime` as the completion time of the latest successful Collector Job.
- Determine how to record the actual half-open collection interval used by a Collector run.
- Expose stored billing-period coverage and ingestion time separately where the source data provides them.
- Make daily and monthly analysis readiness provider- and period-aware, including NCP's monthly-only data.
- Ensure a stale or incomplete account does not silently appear as zero spend or block unrelated ready accounts.
- Document the selected freshness policy and observable status semantics.

## Non-goals

- Do not change provider billing API requests or collection schedules.
- Do not add live CSP access to tests.
- Do not claim a provider source is fresh when its API does not expose enough evidence.
- Do not implement future inventory, audit, or governance features.

## Dependencies and prior documents

- `docs/architecture.md`
- `docs/provider-contract.md`, especially Data Freshness and provider-specific collection windows
- `docs/harness.md`, especially CloudAccount status and stale billing data behavior
- `api/v1alpha1/cloudaccount_types.go`
- `internal/controller/cloudaccount_controller.go`
- `internal/collector/collector.go`
- `internal/storage/clickhouse/connection.go`
- `internal/analyzer/daily_anomaly.go`
- `internal/analyzer/budget.go`

## Work items

1. Add a storage-backed collection-run record with the exact provider request interval, completion time, row count, latest returned usage-period end, and storage ingestion time.
2. Add behavioral tests proving Job completion time alone cannot make stale billing data fresh, while unrelated ready accounts remain analyzable.
3. Expose per-account freshness state to Analyzer and report `Unknown` when the provider response cannot prove source freshness.
4. Apply the state to daily and monthly analysis only where source-period semantics support it; preserve best-effort handling for `Unknown`.
5. Update storage schema, CRD schemas/status, harness documentation, and kind storage E2E coverage for persisted run metadata.

## Acceptance criteria

- A completed Collector Job is observable independently from the request interval it executed.
- Billing usage-period coverage and database ingestion time remain distinct values.
- An account with a successful recent Job but stale or incomplete billing data is not reported as data-ready.
- Daily and monthly analysis use only the periods and provider capabilities relevant to that analysis.
- One stale account cannot prevent analysis of other accounts that have ready data.
- Tests cover successful, stale, missing-data, and mixed-account cases without live CSP access.

## Design decisions

- Use `Fresh`, `Stale`, and `Unknown` as distinct freshness states. A successful Kubernetes Job never implies `Fresh`.
- Apply documented provider delays as expected bounds, not guarantees. If a provider has no documented bound or a response cannot prove coverage, report `Unknown` rather than inventing a freshness claim.
- Exclude only accounts proven `Stale` from the affected analysis; continue evaluating other accounts. Preserve best-effort evaluation for `Unknown` accounts and keep their freshness state visible.
- Keep Job completion time, the request's half-open collection window, billing usage-period end, and storage ingestion time as separate timestamps.

The user asked to proceed with this recommendation after it was presented.

## Verification plan

- Run each new behavioral test before implementation and capture a genuine RED result.
- Run the focused Analyzer, Collector, Storage, and Controller packages after each vertical slice.
- Run `go test ./... -count=1`, `go vet ./...`, `make fmt-check`, `make build`, and `make manifests`.
- Run the disposable local kind storage E2E with its explicit kind context when Docker socket access is available.
- Record exact commands and any unavailable checks below.

## Verification record

- Design policy selected and implemented as recorded above.
- RED observed for `TestAssessDataFreshness...`, `TestExcludeStaleAccountsKeepsFreshAndUnknownAccounts`, `TestEvaluateStoredDailyCostAnomaliesDoesNotUseJobTimeAsDataFreshness`, and `TestRunBudgetPolicyExcludesStaleCostDataAndUpdatesAccountReadiness` before their corresponding implementation changes.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./... -count=1` passed when run with local loopback access for the Azure `httptest` package.
- GREEN: `GOCACHE=/tmp/louder-go-build go vet ./...` passed.
- GREEN: `make fmt-check`, `bash -n scripts/kind/assert-clickhouse-storage.sh`, `GOCACHE=/tmp/louder-go-build make manifests`, and `git diff --check` passed.
- GREEN: all three binaries built successfully to `/tmp` with `GOCACHE=/tmp/louder-go-build`.
- The kind storage E2E script now asserts that fixture replay writes one collection-run row with a latest usage period. Execution is blocked because Docker cannot connect to `/Users/jinhyeokhong/.docker/run/docker.sock` (`permission denied`).
- `make build` initially could not write Go module cache metadata outside the workspace. Re-running all three `go build` commands with outputs under `/tmp` and approved cache access passed.
- No known test failures remain. Kind E2E is the only planned verification not executed.
