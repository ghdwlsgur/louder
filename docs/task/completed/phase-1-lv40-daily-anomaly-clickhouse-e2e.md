# Phase 1 Level 40: Daily Anomaly ClickHouse E2E

## Goal

Verify that stored daily AWS costs flow through the ClickHouse reader, seven-day anomaly evaluator, and `CostAnomaly` notifier payload in the local kind storage harness.

## Scope

- Add a ClickHouse integration test with one target day and seven baseline days for a synthetic AWS account.
- Use `EvaluateStoredDailyCostAnomalies` to query persisted data and `NotifyDailyAnomalyIntents` with a `FakeNotifier` to verify the observable alert.
- Run the test from `make kind-e2e-storage` and keep AWS budget and NCP monthly budget E2E checks intact.
- Use an injected fixed UTC clock and successful collection timestamp; do not depend on live providers or the current date.

## Non-goals

- Changing anomaly thresholds, evaluation behavior, or notification delivery semantics.
- Sending a real Teams webhook or calling AWS Cost Explorer.
- Adding another fixture provider or modifying the product runtime.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv37-daily-cost-spike-detection.md`
- `docs/task/completed/phase-1-lv39-ncp-monthly-collector-e2e.md`
- `internal/analyzer/daily_anomaly.go`
- `internal/analyzer/notify.go`
- `internal/analyzer/clickhouse_integration_test.go`
- `scripts/kind/e2e.sh`

## Work items

1. Add a storage-backed daily anomaly integration test and include it in the kind test command; run kind before writing the synthetic daily rows and observe RED.
2. Seed eight exact daily AWS records in ClickHouse from the test, set a matching successful collection time, and verify the expected anomaly intent.
3. Route the intent through a `CostAnomaly` NotificationPolicy to `FakeNotifier` and assert the date, today amount, seven-day average, and increase.
4. Update `docs/harness.md` and record verification results.

## Completion criteria

- The daily integration test fails before synthetic records are seeded and passes after seeding.
- The stored evaluator uses the latest complete day and seven baseline days from ClickHouse.
- `FakeNotifier` receives the expected `CostAnomaly` payload without live CSP or Teams access.
- Existing AWS budget and NCP monthly budget storage E2E checks continue to pass.
- Full Go tests, vet, formatting, manifest generation, build, kind storage E2E, and `git diff --check` pass.

## Design decisions and risks

- Fix the scenario to October 1–8, 2026 and evaluate at October 9, 2026 UTC so the daily test is deterministic.
- Use a unique synthetic provider/account and source IDs to avoid collisions with the AWS and NCP fixtures in the same ephemeral ClickHouse database.

## Verification plan

- RED/GREEN: `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `git diff --check`

## Results

- RED: `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage` reached `TestStoredDailyAnomalyProducesFakeNotifierIntents` and returned zero intents because ClickHouse did not yet contain the synthetic daily target and baseline rows.
- GREEN: the same kind target passed after seeding seven USD 10 baseline days and one USD 30 target day for a unique AWS account. The stored evaluator produced the October 8 alert, and `FakeNotifier` received the exact date, target, average, increase, and currency.
- The existing AWS budget, NCP monthly Collector/ClickHouse/budget path, and scheduled Analyzer CronJob checks passed in the same kind run.
- Focused tests: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run TestStoredDailyAnomalyProducesFakeNotifierIntents -count=1` — compiled and passed with the integration test skipped outside kind; the full assertion ran in kind.
- `bash -n scripts/kind/e2e.sh` and `git diff --check` — passed.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check`, `bash -n scripts/kind/e2e.sh`, and `git diff --check` — passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` and `GOCACHE=/tmp/louder-go-cache make build` — passed.
