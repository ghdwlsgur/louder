# Phase 1 · LV5 — kind Operator Smoke Test

> Status: completed (2026-09-30)
> Scope: Deploy the Operator to a disposable local kind cluster and verify a real Kubernetes API reconciliation outcome.

## Goal

Provide one repeatable local command that creates a kind cluster, installs the CRDs and least-privilege Operator deployment, and verifies `CredentialsReady=False` with reason `SecretNotFound` for a CloudAccount whose credential Secret is absent.

## Dependencies

- `Dockerfile` and `make docker-build`
- `docs/architecture.md`
- `docs/cluster-platform.md`
- `docs/harness.md`
- `SECURITY.md`
- `skills/louder-tdd/SKILL.md`

## Scope and non-goals

- Included: kind cluster configuration, namespace-scoped ServiceAccount/Role/RoleBinding/Deployment, a synthetic CloudAccount fixture, an assertion script, and a `make kind-e2e` lifecycle target.
- Excluded: live CSP access, Collector execution, Vault/ESO integration, ClickHouse, Analyzer, Teams, and production deployment.
- This is a Kubernetes control-plane smoke test. It does not claim to satisfy the separate Vault -> ESO -> Secret flow required of the full platform E2E harness.

## TDD behavior

1. Write the public smoke-test assertion for a missing credential Secret and run it against a disposable kind cluster with the CRD installed but no Operator deployment. It must fail because the expected status condition is absent (RED).
2. Add only the CRD/RBAC/Deployment installation and cluster lifecycle needed to run the Operator; the same assertion must pass with the real API server (GREEN).
3. Repeat the test from the public Make target, which builds and loads the image and creates/deletes its disposable cluster.

## Work items

- [x] Add a kind cluster configuration and a CloudAccount fixture with no Secret material under `config/kind/`.
- [x] Add the smoke assertion and observe a genuine RED result against an Operator-free cluster.
- [x] Add namespace-scoped least-privilege RBAC: CloudAccount get/list/watch, status update/patch, and Secret get only.
- [x] Add a non-root, read-only-root-filesystem Operator Deployment with health probes, resource bounds, the local image, and `--watch-namespace=cloud-cost`.
- [x] Add a Make target that builds the image and invokes a script to load it, create the cluster, apply CRDs and runtime resources, run the assertion, and always delete the test cluster.
- [x] Add `make kind-e2e` and document prerequisites and scope.
- [x] Clarify that direct kubectl commands must specify the target context: `innogrid-core-sre` for platform work and the explicit kind context for disposable local tests.
- [x] Run the kind smoke test and record the result, then check formatting and task-document language.

## Acceptance criteria

- `make kind-e2e` never uses the production kube context and tears down only the named disposable kind cluster it created.
- The Operator runs as non-root with namespace-scoped RBAC and responds to Kubernetes health probes.
- The real API server accepts the generated CRD and CloudAccount, and the Operator sets `CredentialsReady=False`, reason `SecretNotFound`.
- No real or synthetic credential values are stored in the fixture.
- The plan clearly records that Vault/ESO remains necessary for full secret-flow E2E coverage.

## Verification

- Capture the assertion's missing-condition failure before deploying the Operator.
- Run `make kind-e2e` and confirm the status assertion passes and the disposable cluster is removed.
- Run `git diff --check` and scan all `docs/task` files for Hangul characters.

### Results

- RED: with only the CRD installed in a fresh kind cluster, `bash scripts/kind/assert-cloudaccount-missing-secret.sh` failed after 60 seconds with `timed out waiting for the condition on cloudaccounts/kind-smoke-missing-secret`.
- GREEN: after loading `louder-operator:local` and applying `config/kind/operator.yaml`, the same assertion passed and reported `CredentialsReady=False (SecretNotFound)`.
- `make kind-e2e` — passed using `kindest/node:v1.35.5`; it built/loaded the image, deployed the Operator, and passed the real API-server status assertion.
- The script deleted the `louder-e2e` cluster on exit; verify no kind clusters remain before closeout.
- `GOCACHE=/private/tmp/louder-go-build-cache make manifests` — passed.
- `GOCACHE=/private/tmp/louder-go-build-cache make test` and `GOCACHE=/private/tmp/louder-go-build-cache make vet` — passed.
- Final security review removed unused placeholder Secret data from the existing presence test; `GOCACHE=/private/tmp/louder-go-build-cache go test ./internal/controller -count=1` — passed.
- `make fmt-check`, `bash -n scripts/kind/*.sh`, skill metadata validation, and `git diff --check` — passed.
- The `docs/task` English-language scan — passed.
- Vault/ESO, provider calls, Collector execution, and full production-style secret-flow E2E — not run; they remain follow-up scope.

## Risks and follow-up

- Requires Docker, kind, kubectl, and permission to access the local Docker daemon.
- Namespace-scoped RBAC must match the Operator's actual cache and direct Secret-read behavior.
- Follow-up work is needed for a full Vault -> ESO -> Secret E2E flow and Collector execution.
