# Phase 1 Level 27: Scheduled Budget Analyzer

## Goal

Run the one-shot Analyzer from a BudgetPolicy-owned CronJob and suppress repeat threshold notifications within the same UTC calendar month.

## Scope

- Add an optional BudgetPolicy schedule; an empty value disables recurring evaluation.
- Persist the last notification month and delivered threshold percentages in BudgetPolicy status.
- Update the one-shot runtime to send only thresholds not yet recorded for the current UTC month, then update status after all destinations accept delivery.
- Add a BudgetPolicy reconciler that creates, updates, and deletes an owned Analyzer CronJob.
- Add a dedicated namespace-scoped Analyzer ServiceAccount and least-privilege Role for reading policies/accounts/secrets and patching BudgetPolicy status.
- Include Analyzer image and ClickHouse Secret configuration in Operator wiring.
- Regenerate the BudgetPolicy CRD and add fake-client, controller, and schema tests.
- Extend the local kind storage E2E to verify the scheduled CronJob shape without invoking a real Teams endpoint.
- Update architecture, secrets, harness, and task documentation.

## Non-goals

- Daily summaries, anomaly notifications, delivery retries beyond Kubernetes Job backoff, or a notification outbox.
- Recording per-destination delivery receipts.
- Cloud-provider collection changes.
- Production deployment to the shared `sre-core` cluster.

## Dependencies and prior documents

- `docs/architecture.md`
- `docs/cluster-platform.md`
- `docs/secrets.md`
- `docs/harness.md`
- `SECURITY.md`
- `docs/task/completed/phase-1-lv26-analyzer-one-shot-runtime.md`

## Design decisions

- `spec.schedule` is opt-in. A missing schedule creates no Analyzer CronJob.
- One CronJob is owned by each scheduled BudgetPolicy, uses `concurrencyPolicy: Forbid`, and invokes `/louder-analyzer` for that policy.
- The Analyzer records successfully delivered threshold percentages by UTC month in BudgetPolicy status. A new month starts with an empty threshold set.
- A threshold is recorded only after all matching destinations finish successfully. The delivery guarantee is at-least-once: if delivery succeeds but the subsequent status update fails, that threshold can be sent again.
- Deduplication is per BudgetPolicy threshold, not per destination. Adding a destination after a threshold has been delivered does not replay that threshold in the same month.
- The Analyzer ServiceAccount is separate from the Operator account and has namespace-scoped reads plus BudgetPolicy status updates only.
- CronJobs use the configured shared ClickHouse Secret through `envFrom`; webhook Secrets remain resolved by the Analyzer at run time.

## Work items

1. Add a run-twice test proving an already delivered threshold is suppressed; confirm RED.
2. Implement monthly status deduplication and verify GREEN.
3. Add tests one behavior at a time for month rollover, multiple thresholds, delivery failure, and status update behavior.
4. Add a controller test first for creating a scheduled Analyzer CronJob; implement and verify it, then cover disable/update/idempotence and ownership conflicts.
5. Add the API field, status copy behavior, generated CRD schema checks, RBAC, and Operator registration.
6. Extend kind storage E2E to assert the CronJob command, schedule, ServiceAccount, Secret reference, and `Forbid` concurrency policy.
7. Update documentation and run repository verification.

## Completion criteria

- Repeated successful runs in one UTC month send each reached threshold only once.
- Thresholds can alert again after the UTC month changes.
- Failed notification delivery does not mark a threshold delivered.
- Scheduled BudgetPolicies own a reconciled Analyzer CronJob; unscheduled policies have none.
- Analyzer runtime credentials are supplied by Secret references, and the Analyzer ServiceAccount has no access outside its namespace.
- Generated CRD validation includes the schedule field.
- Unit, kind, build, schema, and static checks pass; exact results are recorded below.

## Risks and open questions

- Status is updated after delivery to avoid silently losing alerts, so a status-write failure can cause a duplicate on retry.
- Per-threshold deduplication is shared across all matching destinations; it does not replay to a newly added destination during the same month.
- The schedule is interpreted by Kubernetes CronJob in UTC to match budget month calculations.

## Verification

- Red: the monthly repeat-delivery test failed before implementation because the threshold was sent twice in the same month.
- Red: the CronJob reconciler test failed before implementation because the scheduled BudgetPolicy reconciler did not exist.
- Green: `GOCACHE=/tmp/louder-go-cache go test ./internal/analyzer -run 'TestRunBudgetPolicy(SuppressesThresholdsAfterSuccessfulDeliveryThisMonth|DoesNotRecordThresholdWhenDeliveryFails)$' -count=1` passed.
- Green: `GOCACHE=/tmp/louder-go-cache go test ./internal/controller -run 'TestBudgetPolicyReconcile(CreatesAnalyzerCronJob|DeletesCronJobWhenScheduleIsRemoved|UpdatesCronJobWhenScheduleChanges)$' -count=1` passed.
- Green: generated CRD schema and BudgetPolicy deep-copy tests passed; `GOCACHE=/tmp/louder-go-cache make manifests` regenerated the CRD.
- `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage` passed, including collector persistence/replay deduplication, scheduled Analyzer CronJob shape, dedicated ServiceAccount permissions, and ClickHouse integration checks.
- `GOCACHE=/tmp/louder-go-cache make test` passed.
- `GOCACHE=/tmp/louder-go-cache make vet` passed.
- `make fmt-check` passed.
- `GOCACHE=/tmp/louder-go-cache make build` passed for the Operator, Collector, and Analyzer.
- `git diff --check` passed.
- Delivery note: notification delivery and the subsequent BudgetPolicy status update are not atomic; a status update failure can result in a repeat notification on retry.
