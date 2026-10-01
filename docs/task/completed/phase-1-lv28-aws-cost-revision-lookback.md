# Phase 1 Level 28: AWS Cost Revision Lookback

## Goal

Recollect a bounded window of recent complete UTC days so AWS Cost Explorer revisions can replace previously stored daily totals, while reducing the documented collection cadence from every six hours to once per day.

## Scope

- Change the live Collector's Cost Explorer request window to the previous seven complete UTC days, with a half-open UTC date range.
- Apply the same collection window to both live JSONL and ClickHouse storage execution paths.
- Preserve deterministic AWS account-day source record IDs so ClickHouse `ReplacingMergeTree(version)` can select revised values on subsequent reads with `FINAL`.
- Change CloudAccount examples and harness guidance to recommend one daily collection instead of every six hours.
- Update provider, architecture, and harness documentation with the lookback, retry/revision behavior, and per-request Cost Explorer API pricing implications.
- Add focused Collector window tests using a provider fake; do not contact AWS in tests.

## Non-goals

- Configurable per-account lookback windows or new CRD fields.
- Backfilling historical billing periods beyond the recent seven complete UTC days.
- Changing AWS metric, Cost Explorer filters, provider mappings, or ClickHouse schema.
- Changing the schedule on any deployed CloudAccount or shared Kubernetes cluster.
- Adding retries, alerting, or a freshness condition to CloudAccount status.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/secrets.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv14-aws-cost-explorer-collector.md`
- `docs/task/completed/phase-1-lv17-clickhouse-cost-storage.md`
- `docs/task/completed/phase-1-lv27-scheduled-budget-analyzer.md`
- [AWS Cost Explorer refresh behavior](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-what-is.html)
- [AWS Cost Explorer API pricing](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/)

## Work items

1. Add one public Collector behavior test proving the requested window contains the seven previous complete UTC days; run it and record RED.
2. Implement one shared window helper and use it from both registry execution paths; rerun the focused test for GREEN.
3. Add a focused test proving the storage-enabled registry path uses the same window, then implement the shared behavior.
4. Update the CloudAccount schedule examples to `0 0 * * *` and document that actual schedules remain user-controlled.
5. Document the half-open lookback window, stable source IDs/ClickHouse replacement behavior, and per-request API pricing without claiming guaranteed AWS revision latency.
6. Run unit, static, formatting, build, and repository checks; do not make live AWS calls.

## Completion criteria

- At a fixed UTC instant, live Collector requests start at midnight UTC seven calendar days before today's UTC date and end at midnight UTC today.
- Both live execution paths send that same half-open window to the provider.
- AWS daily source IDs remain stable across overlapping lookback requests, and ClickHouse retains the latest ingested version for each logical account-day record.
- CloudAccount examples recommend one run per day; controller behavior continues to honor each account's configured schedule.
- Documentation states the AWS data-refresh caveat and the $0.01 per paginated API request pricing, with no promise that seven days guarantees final billing data.
- Tests remain offline and use synthetic data.

## Design decisions and risks

- The fixed seven-day lookback is a simple initial revision window, not a guarantee that all AWS billing revisions arrive within seven days. AWS says Cost Explorer refreshes at least every 24 hours and that upstream data can be updated later.
- Collection remains controlled by `CloudAccount.spec.collection.schedule`; changing documentation examples does not mutate deployed resources.
- A daily run with a seven-day range is expected to reduce API invocation frequency relative to a six-hour cadence while revisiting recent totals. Cost remains dependent on API pagination and the number of accounts.
- ClickHouse replacement is asynchronous. Analyzer reads use `FINAL`, so a completed write is visible as the latest logical record through the existing reader path.
- Overlapping daily totals may temporarily change budget analysis as upstream data is revised. Existing monthly threshold deduplication prevents repeat delivery of thresholds already recorded in the month.

## Verification plan

- Record RED then GREEN for each focused Collector behavior.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/collector ./internal/provider/aws ./internal/storage/clickhouse -count=1`
- `GOCACHE=/tmp/louder-go-cache make test`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage`
- `git diff --check`

## Verification results

- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/collector -run TestRunWithRegistryCollectsSevenPreviousCompleteUTCDays -count=1` failed because the live request still covered only the previous UTC day. The assertion was integrated into the existing registry behavior test.
- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/collector -run TestRunWithRegistryAndStorageCollectsPreviousSevenCompleteUTCDays -count=1` failed because the storage-enabled request still covered one day.
- GREEN: `GOCACHE=/tmp/louder-go-cache go test ./internal/collector -count=1` passed after both paths used the seven-day UTC window.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/collector ./internal/provider/aws ./internal/storage/clickhouse -count=1` passed.
- `GOCACHE=/tmp/louder-go-cache make test` passed.
- `GOCACHE=/tmp/louder-go-cache make vet` passed.
- `make fmt-check` passed.
- `GOCACHE=/tmp/louder-go-cache make build` passed for the Operator, Collector, and Analyzer.
- `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage` passed, including ClickHouse fixture replay deduplication and scheduled Analyzer checks.
- `git diff --check` passed.
- No live AWS Cost Explorer request was made.
