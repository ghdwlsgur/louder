# Phase 1 Level 37: Daily Cost Spike Detection

## Goal

Add provider-neutral daily cost spike detection so a sharp increase can notify through Teams, while preserving monthly budget tracking.

## Current behavior and selected approach

- The existing `BudgetPolicy` analyzer reads stored daily cost records from the start of the current UTC month and sums them for monthly thresholds. That is the monthly tracking path; no second monthly record stream is needed for this task.
- The new daily detector will analyze normalized daily records from all implemented adapters: AWS, Azure, GCP, OCI, IBM Cloud, and Alibaba Cloud.
- NCP and NHN remain outside daily anomaly coverage until an official source provides daily actual cost values. Do not estimate daily costs from usage or monthly totals. Their existing task plans remain pending.
- Evaluate the previous complete UTC day against the seven complete UTC days before it. A candidate is an anomaly when its cost exceeds 1.5 times the baseline average and the increase exceeds the configured absolute threshold.
- Evaluate each provider/account scope independently. Cost bases from different providers or accounts must not be combined to calculate a daily baseline, and records in a currency other than the configured policy currency fail safely.
- The daily absolute threshold uses the BudgetPolicy currency and must be configured explicitly. `NotificationPolicy` subscribers must opt into the existing `CostAnomaly` event.
- A detector run requires a successful collection timestamp at or after the end of the day being evaluated; after that gate, dates with no cost rows count as zero spend.

## Scope

- Extend `BudgetPolicy` with optional daily anomaly configuration and per-date notification receipt status.
- Add a public, offline-testable anomaly evaluator over normalized records.
- Add a storage-backed daily evaluator using `storage.CostReader` and an eight-day query window (target day plus seven baseline days).
- Extend the existing Analyzer runtime to emit `CostAnomaly` intents through matching Teams `NotificationPolicy` objects and deduplicate a completed UTC date.
- Extend the collector re-fetch window from seven to eight complete UTC days so the target day and baseline are both present on each successful collection run.
- Add CRD, deepcopy, sample, architecture, provider contract, and harness documentation updates as needed.
- Preserve existing monthly budget aggregation and notification behavior.

## Non-goals

- Implementing NCP or NHN adapters without a verified daily actual-cost source.
- Fetching a second set of provider-native monthly totals or reconciling them to invoices.
- Currency conversion or summing records with different cost bases.
- Configurable baseline length or relative multiplier; the initial rule follows the existing architecture example (seven days and 1.5x).
- Live CSP or real Teams calls in tests.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/secrets.md`
- `docs/task/README.md`
- `api/v1alpha1/budgetpolicy_types.go`
- `api/v1alpha1/cloudaccount_types.go`
- `internal/analyzer/budget.go`
- `internal/analyzer/runtime.go`
- `internal/analyzer/notify.go`
- `internal/collector/window.go`
- `internal/storage/cost_reader.go`

## Work items

1. Add one focused public evaluator test proving a 1.5x-and-absolute increase creates a `CostAnomaly` intent; run it to observe RED.
2. Implement the minimum exact-decimal daily evaluator and verify GREEN, including below-threshold, missing-baseline, mixed-currency, mixed-cost-basis, and UTC-boundary cases.
3. Add optional BudgetPolicy anomaly configuration and validation; test invalid and disabled configurations before implementation.
4. Add the storage-backed evaluation query, collection freshness gate, and eight-day collector window; test requested dates and stale/missing collection status.
5. Route anomaly intents only to matching `CostAnomaly` notification policies and their matching account scopes, verify notification payloads and no-send behavior, and record the notified UTC date after successful delivery.
6. Run focused and repository verification, update generated CRDs, and record exact results.

## Completion criteria

- The daily rule analyzes one complete UTC day against the prior seven complete UTC days and applies both relative and configured absolute thresholds.
- A missing successful collection covering the analysis day does not produce a zero-cost assumption or anomaly notification.
- Provider/account scopes are evaluated independently; mixed cost bases fail safely, and currencies must match the configured policy currency.
- `CostAnomaly` notifications honor event subscriptions, account selectors, Teams Secret resolution, and per-date deduplication.
- The eight-day collector lookback covers the target date and seven baseline dates.
- Existing monthly budget tests and behavior remain unchanged.
- No NCP or NHN daily costs are fabricated or inferred.

## Verification plan

- `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./internal/collector ./internal/controller ./api/v1alpha1 -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `git diff --check`

## Verification results

- RED: the public daily evaluator test failed before the implementation returned the expected anomaly intent.
- RED: invalid daily threshold validation failed before policy validation was added.
- RED: the collector window test failed while the request covered seven days instead of the required eight.
- RED: runtime deduplication test did not compile until its status receipt field was added; it then passed after notification routing and status persistence were implemented.
- RED: per-destination selector test showed both Teams destinations receiving both account anomalies; after routing intents by provider/account metadata, the focused test passed.
- GREEN: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer ./internal/collector ./internal/controller ./api/v1alpha1 -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` passed. It required execution with local loopback socket access because the existing Azure adapter `httptest` test binds IPv6 localhost.
- GREEN: `GOCACHE=/tmp/louder-go-cache make vet` passed.
- GREEN: `make fmt-check` passed.
- GREEN: `GOCACHE=/tmp/louder-go-cache make manifests` passed and generated the BudgetPolicy CRD schema.
- GREEN: `GOCACHE=/tmp/louder-go-cache make build` passed; Go printed non-fatal module-stat-cache write warnings because the shared module cache is outside the writable workspace.
- GREEN: `git diff --check` passed.
- No live CSP or Teams calls were made. kind E2E was not run because this feature changes Analyzer logic and did not modify Kubernetes workload behavior.
