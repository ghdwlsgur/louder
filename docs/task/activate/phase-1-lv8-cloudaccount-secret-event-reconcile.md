# Phase 1 · LV8 — Reconcile CloudAccount When Credential Secret Changes

> Status: pending (2026-09-30)
> Scope: Reconcile a CloudAccount when its referenced credential Secret is created, updated, or deleted.

## Goal

Ensure CloudAccount credential readiness follows the referenced Kubernetes Secret lifecycle. An account that first reports `SecretNotFound` must become `CredentialsReady=True` when ESO later creates its Secret, without requiring a CloudAccount edit or Operator restart.

## Dependencies

- `internal/controller/cloudaccount_controller.go`
- `internal/controller/cloudaccount_controller_test.go`
- `config/kind/operator.yaml`
- `scripts/kind/assert-vault-eso-secret-flow.sh`
- `docs/architecture.md`
- `docs/secrets.md`
- `docs/harness.md`
- `docs/cluster-platform.md`
- `SECURITY.md`
- `skills/louder-tdd/SKILL.md`
- Phase 1 LV6 Vault and ESO kind E2E
- Phase 1 LV7 Vault outage failure injection

## Scope and non-goals

- Included: namespace-scoped Secret watch, mapping a changed Secret to CloudAccounts that reference its name, the minimal RBAC verbs needed for a Secret informer, and E2E coverage where the CloudAccount precedes its ESO-generated Secret.
- Excluded: CronJob/Collector implementation, credential-shape validation, Secret data reads beyond current readiness behavior, and cross-namespace Secret references.
- The Operator continues to access only Secrets in its configured watch namespace.

## Design decisions

- Keep `CredentialsReady` reconciliation as the single source of condition behavior; a Secret event only enqueues referring CloudAccounts.
- Watch Secret events as `PartialObjectMetadata` so credential values do not enter the Operator cache. List CloudAccounts only in the Secret's namespace when mapping an event, then match `spec.credentialRef.name`; do not enqueue unrelated accounts.
- Grant the Operator Role namespace-scoped `list` and `watch` on Secrets in addition to its existing `get` permission, because controller-runtime needs those verbs for the informer.
- Change the Vault/ESO E2E ordering so the CloudAccount exists and reports `SecretNotFound` before ESO creates its target Secret.

## TDD behavior

1. Change the existing kind secret-flow assertion to create CloudAccount before ExternalSecret/target Secret creation. Run `make kind-e2e-secrets` against current code; the expected `CredentialsReady=True` transition must time out while status remains `SecretNotFound` (RED).
2. Add Secret event mapping and the namespace-scoped RBAC verbs. Rerun the same E2E and require the account to become `CredentialsReady=True` / `SecretFound` after ESO creates the Secret (GREEN).
3. Add focused mapping coverage proving only CloudAccounts referencing the changed Secret are enqueued; rerun the focused test and relevant repository checks.

## Work items

- [x] Reorder the secret-flow assertion so CloudAccount is created while the referenced Secret is absent; capture a genuine RED before controller changes.
- [x] Watch Secret lifecycle events and enqueue only CloudAccounts in the same namespace that reference the changed Secret.
- [x] Add minimal namespaced Secret list/watch RBAC to the kind Operator deployment.
- [x] Add a focused test for Secret-to-CloudAccount request mapping, including an unrelated Secret/account case.
- [x] Run `make kind-e2e-secrets` and confirm ESO-created Secret transitions the existing account to ready without a spec edit.
- [x] Update harness documentation and this verification log.
- [x] Run `make kind-e2e`, Go tests, vet, manifests, formatting, shell syntax, English task-doc scan, and diff checks.
- [ ] Record PR link, checks, and merge commit before moving this task to `completed`.

## Acceptance criteria

- A CloudAccount created before its credential Secret reports `CredentialsReady=False` / `SecretNotFound` initially.
- When ESO later creates the referenced Secret, the existing CloudAccount becomes `CredentialsReady=True` / `SecretFound` without any CloudAccount mutation or process restart.
- Secret updates and deletion also enqueue only CloudAccounts that reference that Secret in the same namespace.
- Operator RBAC remains namespace-scoped and grants only the Secret verbs required for the informer.
- The kind tests still never use real CSP credentials or print synthetic Secret values.

## Verification

- Capture the failing pre-fix kind status transition as RED.
- Verify focused mapping behavior with a controller unit test.
- Run both kind E2E targets and all repository quality checks listed above.
- Confirm each kind script removes only its disposable cluster.

## Verification log

- RED: the first valid CloudAccount-first run showed `CredentialsReady` remained `SecretNotFound` after ESO reported Ready and materialized the target Secret. The test setup's earlier path-missing probe initially masked this by leaving an ExternalSecret behind; the E2E now deletes that probe before seeding Vault.
- GREEN: `make kind-e2e-secrets` — passed. CloudAccount first reached `SecretNotFound`; after ESO created the Secret, the metadata-only Secret watch enqueued it and it reached `CredentialsReady=True` / `SecretFound` without a CloudAccount update. Existing Vault outage and recovery checks also passed.
- `make kind-e2e` — passed; the Operator retained missing-Secret behavior.
- `GOCACHE=/tmp/louder-go-build make test` — passed, including `TestRequestsForCredentialSecretOnlyEnqueuesReferencingAccounts`.
- `GOCACHE=/tmp/louder-go-build make vet` — passed.
- `GOCACHE=/tmp/louder-go-build make manifests` — passed.
- `make fmt-check`, `bash -n scripts/kind/*.sh`, the task-doc English scan, and `git diff --check` — passed.
- The Secret informer uses metadata-only objects and the Role grants only namespaced `get`, `list`, and `watch`; credential values are not cached for event mapping.

## Risks and open questions

- A Secret metadata informer requires list/watch rights but does not cache Secret values. Keep the operator's namespace scope unchanged and do not log or expose Secret data.
- A Secret event can enqueue multiple CloudAccounts if they intentionally share a credential Secret; each is expected to reconcile independently.
