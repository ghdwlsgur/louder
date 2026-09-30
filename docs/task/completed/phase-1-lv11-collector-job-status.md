# Phase 1 LV11: Collector Job Status Reporting

## Goal

Report the latest terminal Collector Job result and attempt timestamps on its owning CloudAccount, so collection outcomes are observable through the Kubernetes API.

## Scope

- Add a stable CloudAccount annotation to generated Collector Jobs for event-to-account mapping.
- Watch Job metadata events and reconcile the referenced CloudAccount.
- Inspect Jobs in the same namespace and accept only Jobs controlled by the CloudAccount's managed CronJob.
- Set `CollectionReady=True` with `CollectionSucceeded` for the latest successful Job.
- Set `CollectionReady=False` with `CollectorJobFailed` for the latest failed Job, without copying Pod messages or secret data into status.
- Update `lastCollectionTime` for terminal attempts and `lastSuccessfulCollectionTime` only after success.
- Preserve status from the latest terminal attempt when older Job events arrive.
- Extend unit tests and local kind E2E to verify status updates after a completed fixture Job.
- Add namespace-scoped Job RBAC and document the distinction between collection execution status and data freshness.

## Non-goals

- Live CSP collection or provider API error classification.
- Provider data freshness detection, normalization, persistence, analysis, or notifications.
- Storing Job logs, Pod termination messages, billing records, or credential values in CloudAccount status.
- Reporting intermediate Job progress or retry attempts before Kubernetes marks a Job terminal.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/harness.md`
- `docs/provider-contract.md`
- `docs/secrets.md`
- `SECURITY.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv9-fixture-collector-cronjob.md`

## Work items

1. Add one reconciliation behavior test proving a completed owned Job sets `CollectionReady=True` and both timestamps; confirm RED before implementation.
2. Add a failure behavior test proving a failed owned Job sets `CollectionReady=False`, updates only the attempt time, and does not expose Pod messages.
3. Implement latest-terminal-Job selection with namespace and owner checks; keep older Job events from overwriting newer status.
4. Add metadata-only Job watches and namespace-scoped Job permissions.
5. Update kind E2E to wait for and inspect `CollectionReady=True` after the fixture Job succeeds.
6. Update harness status examples and record verification in this plan.

## Completion criteria

- The latest successful managed Job sets `CollectionReady=True`, records its completion time in both collection timestamp fields, and uses a stable reason code.
- The latest failed managed Job sets `CollectionReady=False`, records the attempt time, and preserves the previous successful collection time.
- Job events from another namespace or Jobs not controlled by the CloudAccount's managed CronJob do not alter CloudAccount status.
- Older terminal Job events cannot replace status derived from a newer attempt.
- Status messages contain no Pod termination text, Secret contents, or fixture record payloads.
- `make kind-e2e` confirms the CloudAccount reports successful collection after the fixture Job completes.
- Unit tests, `go vet`, formatting, build, kind E2E, shell syntax, and `git diff --check` pass.

## Design decisions and risks

- `CollectionReady` describes the last terminal execution result. A successful Job does not prove provider data freshness or completeness.
- Only terminal Job conditions are reported. Active Jobs and in-progress retries leave the last terminal result visible.
- Job-to-account mapping uses a generated annotation, while reconciliation verifies the Job's controller owner is the managed CronJob. This avoids trusting arbitrary labels as ownership.
- Status uses stable classifications and timestamps, not Job or Pod error text, to avoid credential/data leakage.
- Existing `CloudAccountStatus` already contains timestamp fields and generic conditions; no CRD schema change is expected.

## Verification plan

- `GOCACHE=/tmp/louder-go-cache go test ./internal/controller -run TestReconcileReportsCollectorJob -count=1` (RED then GREEN)
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache go vet ./...`
- `GOCACHE=/tmp/louder-go-cache make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make kind-e2e`
- `bash -n scripts/kind/e2e.sh scripts/kind/assert-collector-cronjob.sh`
- `git diff --check`
- Inspect task Markdown for English-only content.

## Verification results

- RED: `TestReconcileReportsCollectorJobSuccess` initially failed because `CollectionReady` was absent after a successful Job. The reconciler now discovers the latest terminal Job owned by the CloudAccount's managed CronJob and writes the success condition and timestamps.
- GREEN: controller tests verify successful and failed outcomes, failure-time handling, preservation of the last successful time, safe status messages, stale event ordering, namespace mapping, and rejection of Jobs owned by another CronJob.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/controller -run 'TestReconcile(CreatesCollectorCronJob|ReportsCollectorJob|IgnoresCollectorJob)' -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go vet ./...` — passed.
- `GOCACHE=/tmp/louder-go-cache make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make build` — passed; Go emitted non-fatal warnings while its read-only module cache attempted to write stat-cache metadata.
- `GOCACHE=/tmp/louder-go-cache make kind-e2e` — passed. The actual kind Job completed, then the CloudAccount reached `CollectionReady=True` with reason `CollectionSucceeded`; the disposable cluster was removed.
- `bash -n scripts/kind/e2e.sh scripts/kind/assert-collector-cronjob.sh` — passed.
- `git diff --check` — passed.
- README image-only request completed and tracked separately in `docs/task/completed/phase-1-lv10-readme-image-only.md`.
