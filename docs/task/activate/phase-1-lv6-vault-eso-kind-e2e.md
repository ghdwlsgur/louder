# Phase 1 · LV6 — Vault and ESO kind E2E

> Status: active (2026-09-30)
> Scope: Extend local kind verification to exercise Vault -> External Secrets Operator -> Kubernetes Secret -> CloudAccount status.

## Goal

Add a repeatable local test that stores runtime-generated synthetic values in a disposable Vault dev server, lets ESO create the referenced Kubernetes Secret, and verifies the real Operator reports `CredentialsReady=True`.

## Dependencies

- `make kind-e2e` and the `config/kind/` layout
- `docs/secrets.md`
- `docs/harness.md`
- `docs/cluster-platform.md`
- `SECURITY.md`
- `skills/louder-tdd/SKILL.md`
- Official ESO documentation for chart installation and Vault KV v2 authentication
- Official HashiCorp documentation for ephemeral Vault dev mode

## Scope and non-goals

- Included: pinned local Vault and ESO versions, a test-only Vault workload, a Vault KV v2 `ClusterSecretStore`, an `ExternalSecret`, a CloudAccount fixture, and a separate `make kind-e2e-secrets` target.
- Excluded: production Vault deployment, Vault Kubernetes authentication, real cloud credentials, live CSP calls, Collector/ClickHouse/Analyzer/Teams execution, and failure-injection scenarios.
- The test-only Vault root token and fake billing values must be generated at runtime and must never be committed or printed.

## Design decisions

- Use External Secrets Operator Helm chart `2.11.0` and Vault image `hashicorp/vault:2.1.1`, verified against their upstream release/install information on 2026-09-30.
- Run Vault only in dev mode inside the disposable cluster; the official HashiCorp guidance says dev mode is insecure and must not be used in production.
- Keep Vault and bootstrap tokens in a separate `vault-system` namespace. Give ESO a short-lived Vault token with read-only access to the single test KV path. Do not grant the Louder Operator access to that namespace.
- Keep the existing fast `make kind-e2e` smoke test and add a separate secrets-flow target.
- Apply the CloudAccount only after ESO reports Ready and its target Secret has the expected keys, so this test verifies the production-style materialization path without changing controller polling or Secret-watch behavior.

## TDD behavior

1. Write the secret-flow assertion and run it against a disposable cluster where Vault has no value at the referenced KV path. It must fail because ESO cannot mark the ExternalSecret Ready (RED).
2. Seed the runtime-generated KV entry and rerun the same assertion. ESO must create the Kubernetes Secret and the Operator must report `CredentialsReady=True` (GREEN).
3. Run the full flow through `make kind-e2e-secrets` and ensure the disposable cluster is removed.

## Work items

- [x] Add kind-only Vault and ESO resources under `config/kind/` without embedding token or secret values.
- [x] Add SecretStore/ExternalSecret and CloudAccount fixtures for the test path.
- [x] Add a focused assertion for ExternalSecret readiness, target Secret key presence, and Operator credential readiness; observe RED with the Vault value absent.
- [x] Add runtime token generation, restricted Vault policy/token setup, and runtime KV seeding to the kind test script.
- [x] Add the `make kind-e2e-secrets` target while retaining `make kind-e2e` as the fast control-plane smoke test.
- [x] Document the full path, local prerequisites, and test-only Vault limitation.
- [x] Run the red/green assertion, full secret-flow target, Go tests, manifest checks, shell syntax, and docs-language checks.
- [ ] Record PR link, CI/merge outcome, and final verification here before moving this file to `completed`.

## Acceptance criteria

- The test follows Vault -> ESO -> Kubernetes Secret -> CloudAccount reference, and no manually populated credential Secret substitutes for ESO output.
- Committed manifests contain no Vault token or credential values; the script does not print runtime-generated values.
- ESO's Vault token is short-lived and limited to read access for the test KV entry; the Operator cannot read the `vault-system` token Secret.
- The real Kubernetes API shows `ExternalSecret Ready=True`, the ESO-owned Secret has both expected data keys, and CloudAccount reports `CredentialsReady=True` / `SecretFound`.
- `make kind-e2e-secrets` refuses to replace an existing `louder-e2e` cluster and deletes only the disposable cluster it creates.
- The existing `make kind-e2e` smoke test remains available independently.

## Verification

- Capture the missing-Vault-value RED result before seeding the test KV entry.
- Run `make kind-e2e-secrets` against the pinned kind node version.
- Run the existing unit, vet, generated-manifest, shell-syntax, task-language, and diff checks.
- Verify no kind cluster remains after the script exits.

## References

- [ESO installation guide](https://external-secrets.io/latest/introduction/getting-started/)
- [ESO HashiCorp Vault provider](https://external-secrets.io/latest/provider/hashicorp-vault/)
- [ESO releases](https://github.com/external-secrets/external-secrets/releases)
- [HashiCorp Vault dev server mode](https://developer.hashicorp.com/vault/docs/concepts/dev-server)
- [HashiCorp Vault Docker image](https://hub.docker.com/r/hashicorp/vault)

## Verification log

- `make kind-e2e-secrets` — passed. The script observed RED (`SecretSyncedError` while the Vault path was absent), seeded synthetic values, then observed GREEN (`ExternalSecret Ready=True`, both target keys present, `CredentialsReady=True` / `SecretFound`). The disposable kind cluster was removed by the exit trap.
- `make kind-e2e` — passed; the existing missing-Secret smoke behavior remains intact.
- `GOCACHE=/tmp/louder-go-build make test` — passed.
- `GOCACHE=/tmp/louder-go-build make vet` — passed.
- `GOCACHE=/tmp/louder-go-build make manifests` — passed.
- `make fmt-check`, `bash -n scripts/kind/*.sh`, and `git diff --check` — passed.
- An initial Vault rollout failed because kubelet could not verify the image's named `vault` user as non-root. Setting the image UID/GID explicitly (`100:1000`) resolved it. Failure diagnostics intentionally omit Vault container logs so runtime token material cannot be printed.
- Pull request and merge are pending: `gh auth status` reports the configured GitHub token is invalid. The branch will be pushed; PR creation/merge require GitHub CLI authentication to be restored.
