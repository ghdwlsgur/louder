# Phase 1 LV12: Collector Ready Condition

## Goal

Expose whether a CloudAccount's configured Collector CronJob is reconciled and eligible to run through the `CollectorReady` condition.

## Scope

- Set `CollectorReady=True` with reason `CronJobReady` when credentials exist, collection is enabled, and the managed CronJob is reconciled.
- Set `CollectorReady=False` with reason `SecretNotFound` when the referenced credential Secret is absent.
- Set `CollectorReady=False` with reason `CollectionDisabled` when collection is disabled and the managed CronJob is removed.
- Set `CollectorReady=False` with reason `CronJobReconcileFailed` when CronJob reconciliation fails, using a safe status message.
- Keep `CollectionReady` as the result of the last terminal Job; do not conflate desired Collector readiness with execution outcome or data freshness.
- Add focused reconciliation tests and update the harness status documentation.

## Non-goals

- Changes to CRD schema, status fields, Job outcome handling, or provider behavior.
- Reporting pod health, Job progress, or provider data freshness through `CollectorReady`.
- Changing collection scheduling or Secret readiness semantics.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv11-collector-job-status.md`

## Work items

1. Add a behavior test for `CollectorReady=True/CronJobReady` after successful CronJob reconciliation and confirm RED.
2. Implement status updates for ready, missing-Secret, and disabled-collection states.
3. Add a behavior test for safe `CollectorReady=False/CronJobReconcileFailed` on an ownership conflict and implement it.
4. Update `docs/harness.md` examples and record verification results.

## Completion criteria

- Collector readiness accurately reflects the managed CronJob desired state.
- Missing credentials, disabled collection, and reconciliation failure each report stable false reasons.
- No Kubernetes API error details, Secret values, or Job/Pod messages are copied into the condition message.
- `CollectionReady` and its timestamps remain unchanged by CollectorReady-only state transitions.
- Unit tests, `go vet`, formatting, build, kind E2E, and `git diff --check` pass.

## Design decisions and risks

- `CollectorReady=True` means the desired CronJob exists and matches the CloudAccount. It does not mean a Job is currently running or that CSP data is fresh.
- `CollectionReady` remains the independent last-terminal-Job result introduced in LV11.
- Status messages use fixed text; the underlying reconcile error remains available in Operator logs.
- The existing generic condition list and CRD schema already support this condition.

## Verification plan

- `GOCACHE=/tmp/louder-go-cache go test ./internal/controller -run TestReconcileReportsCollectorReady -count=1` (RED then GREEN)
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache go vet ./...`
- `GOCACHE=/tmp/louder-go-cache make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make kind-e2e`
- `git diff --check`
- Inspect task Markdown for English-only content.

## Verification results

- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/controller -run TestReconcileReportsCollectorReady -count=1` failed before implementation because `CollectorReady` was absent.
- GREEN: the focused CollectorReady and state-transition tests pass.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go vet ./...` — passed.
- `GOCACHE=/tmp/louder-go-cache make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make build` — passed (required access to the local Go module cache).
- `GOCACHE=/tmp/louder-go-cache make kind-e2e` — passed, including assertions for missing-Secret `CredentialsReady=False` and `CollectorReady=False`, then the fixture Collector Job flow (required access to Docker).
- `git diff --check` — passed.
- Task plan content is English.
