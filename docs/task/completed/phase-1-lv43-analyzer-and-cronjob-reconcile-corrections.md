# Phase 1, Level 43: Analyzer and CronJob Reconciliation Corrections

## Goal

Fix four reported regressions in daily anomaly notification receipts, independent Analyzer event processing, daily-provider freshness checks, and CronJob drift reconciliation.

## Scope

1. Track daily anomaly delivery receipts by UTC date, provider/account, and NotificationPolicy destination so a later anomaly for another account or destination is not suppressed.
2. Process budget thresholds, forecasts, and daily anomalies independently; a missing subscriber or delivery error for one event must not prevent evaluation and delivery of other event types.
3. Exclude providers without daily cost data, currently NCP, before checking collection freshness for daily anomaly analysis.
4. Compare managed CronJob fields while ignoring API-server defaulted fields for both Analyzer and Collector CronJobs. Preserve existing reconciliation of labels, ownership, and explicit Louder settings.

## Plan

1. Add focused runtime tests reproducing same-date per-account anomaly loss, threshold-routing errors blocking anomaly delivery, and stale NCP accounts blocking AWS anomaly evaluation. Run each test before production changes and capture the failure.
2. Add focused reconciler tests that add API-server default values to existing CronJobs and verify a no-op reconcile leaves their resource versions unchanged.
3. Implement per-destination anomaly receipts and independent event processing, including persistence of successful per-destination receipts when another destination fails.
4. Filter daily-capable accounts before freshness checks, using one centralized provider capability declaration.
5. Replace whole-object spec equality with default-tolerant comparison of desired managed fields in both CronJob reconcilers; remove ineffective scheme defaulting if unused.
6. Run focused package tests, full tests, vet, format, build, CRD generation/schema tests if API status changes, and the relevant kind E2E when the local cluster is available.
7. Record outcomes and move this plan to `completed` only after all required verification succeeds; then commit, push, open a PR, and merge through the normal repository flow.

## Acceptance criteria

- A new same-date anomaly for another account and/or NotificationPolicy destination is delivered even after an earlier receipt for that date.
- A `BudgetThreshold` routing or delivery error does not suppress a subscribed `CostAnomaly` event; errors are still reported to the caller.
- Stale NCP collection status does not prevent current daily-capable accounts from anomaly evaluation.
- API-server defaults alone do not trigger CronJob updates, while explicit managed-field changes still do.

## Verification record

- RED/GREEN: same-date account/destination notification regression, missing BudgetThreshold subscriber, stale NCP freshness, and defaulted CronJob Spec tests reproduced the reported behavior and pass after the fixes.
- `GOCACHE=/tmp/louder-go-build go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-build go vet ./...` — passed.
- `GOCACHE=/tmp/louder-go-build make fmt-check` — passed.
- `git diff --check` — passed.
- `GOCACHE=/tmp/louder-go-build make manifests` — passed; generated BudgetPolicy CRD now exposes the daily anomaly receipt list.
- `GOCACHE=/tmp/louder-go-build make build` — passed for Operator, Collector, and Analyzer.
- Kind storage E2E was not run because the current execution environment cannot access the Docker socket; `docker info` returned permission denied.
