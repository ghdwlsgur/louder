# Phase 1 LV9: Fixture Collector and CronJob Lifecycle

## Goal

Prove the Phase 1 collection execution boundary end to end: a CloudAccount with ready credentials and collection enabled is reconciled into a Collector CronJob, and a scheduled/manual Job runs a standalone fixture-based Collector process.

## Scope

- Add a standalone Collector command that reads an embedded sanitized provider-neutral fixture and emits validated collection records as JSON Lines to stdout.
- Support only the initial providers AWS, GCP, and Azure in the fixture runner; do not call live CSP APIs.
- Extend CloudAccount reconciliation to create and reconcile an owned CronJob only when the credential Secret exists and collection is enabled.
- Remove or suspend managed collection scheduling when credentials are unavailable or collection is disabled; honor the CloudAccount schedule.
- Pass the referenced Secret to the Collector pod through Kubernetes Secret environment references without reading or logging its values.
- Make fixture execution an explicit Operator opt-in, enabled only by the local kind deployment, so a production-default configuration cannot emit synthetic costs.
- Build the Operator and Collector binaries into the local image and verify the CronJob/Job path in disposable kind.
- Document the current fixture-only behavior and the remaining live-provider/storage integration gaps.

## Non-goals

- Live AWS, GCP, or Azure billing API adapters.
- ClickHouse writes, normalization, analysis, or Teams notifications.
- Additional CSPs beyond AWS, GCP, and Azure.
- Production deployment or changes to a shared Kubernetes cluster.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/cluster-platform.md`
- `docs/secrets.md`
- `SECURITY.md`
- `docs/task/README.md`

## Work items

1. Add a behavior test for a valid CloudAccount producing an owned CronJob with its schedule, provider/account arguments, and Secret reference.
2. Implement the minimal reconciliation behavior and add tests for disabled collection and credential absence.
3. Add a public Collector CLI behavior test for fixture validation and JSON Lines output; implement one behavior at a time in RED/GREEN cycles.
4. Build both binaries in the container image and add a synthetic fixture for AWS, GCP, and Azure.
5. Extend the local kind E2E to verify CronJob creation and a successful Collector Job execution.
6. Update harness and architecture documentation to state fixture runner limitations and current DoD progress.

## Completion criteria

- A valid CloudAccount gets a namespaced CronJob owned by that CloudAccount.
- CronJob changes follow the CloudAccount schedule and collection enabled state.
- The Collector gets only a Secret reference in its pod spec; tests prove no Secret values appear in args or environment literal values.
- The Collector rejects malformed fixtures and unsupported providers, and emits deterministic JSON Lines for valid fixtures.
- A kind Job runs the local Collector image to completion and its logs contain the expected synthetic record count without secret values.
- Unit, build, vet, formatting, and kind E2E checks pass.
- Task history records exact commands and results; the plan moves to `completed/` only after verification.

## Design decisions and risks

- The fixture runner is an execution harness, not a completed cloud billing provider. Successful fixture output does not imply fresh or provider-sourced billing data.
- CronJobs are gated on credential Secret existence and `collection.enabled`; fixture execution itself does not authenticate to a CSP. Fixture mode defaults off and is explicitly enabled only in the kind Operator manifest.
- Kubernetes Secret `envFrom` is used to pass credentials for future adapters. The fixture runner ignores credential values.
- A synthetic fixture is committed and must contain no real account identifiers or credentials.
- Secret appearance/disappearance is already watched; reconciliation must apply the same readiness gate consistently.

## Verification plan

- `GOCACHE=/tmp/louder-go-cache go test ./internal/controller -run TestReconcileCreatesCollectorCronJob -count=1` (RED then GREEN)
- `GOCACHE=/tmp/louder-go-cache go test ./internal/collector -count=1` (per-behavior RED/GREEN cycles)
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache go vet ./...`
- `GOCACHE=/tmp/louder-go-cache make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make kind-e2e` (extended to assert a completed fixture Collector Job)
- `bash -n scripts/kind/e2e.sh scripts/kind/assert-collector-cronjob.sh`
- `git diff --check`
- Validate task Markdown is English and inspect generated Kubernetes objects locally; no shared-cluster commands.

## Verification results

- RED: the new CronJob reconciliation test initially failed to compile because `CloudAccountReconciler` had no Collector image configuration. After implementation, it passed and verified the CronJob schedule, owner reference, image, and Secret reference.
- RED: the new unowned-CronJob test failed because reconciliation adopted a pre-existing CronJob with the matching name. Reconciliation now rejects resources not controlled by the current CloudAccount; the test passes.
- RED: malformed-fixture validation coverage initially failed to compile because the decoder boundary did not exist. Fixture decoding is now isolated, rejects malformed/trailing JSON and invalid records, and the regression test passes.
- GREEN: controller tests cover creation, idempotent reconciliation, disabled collection cleanup, missing-credential cleanup, and owner-reference protection.
- GREEN: Collector tests cover JSON Lines output, account scoping, malformed JSON, mismatched providers, initial AWS/GCP/Azure fixtures, and refusal to emit a synthetic fixture without explicit fixture selection.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go vet ./...` — passed.
- `GOCACHE=/tmp/louder-go-cache make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make build` — passed; Go emitted non-fatal warnings when its read-only module cache attempted to write stat-cache metadata.
- `GOCACHE=/tmp/louder-go-cache make kind-e2e` — passed. Docker built both executables into `louder-operator:local`; kind verified CloudAccount -> CronJob -> completed fixture Collector Job and checked that logs did not contain the synthetic Secret value. The script deleted its disposable cluster on exit.
- `bash -n scripts/kind/e2e.sh scripts/kind/assert-collector-cronjob.sh` — passed.
- `git diff --check` — passed.
- No live CSP, ClickHouse, Analyzer, Notifier, or Vault/ESO E2E work was performed as part of this task.
