# Phase 1 · LV7 — Vault Unavailable Failure Injection in kind

> Status: completed (2026-09-30)
> Scope: Extend local secret-flow E2E to verify ESO reports a safe failure while Vault is unavailable and recovers after Vault returns.

## Goal

Verify the Vault-to-ESO integration's outage behavior against the real Kubernetes API: a new ExternalSecret cannot sync while the Vault Pod is stopped, ESO exposes the failure without secret values, and the same ExternalSecret recovers after Vault is restored.

## Dependencies

- `make kind-e2e-secrets`
- `scripts/kind/e2e.sh`
- `config/kind/secret-flow/`
- `docs/harness.md`
- `docs/secrets.md`
- `docs/cluster-platform.md`
- `SECURITY.md`
- `skills/louder-tdd/SKILL.md`
- Phase 1 LV6 Vault and ESO kind E2E

## Scope and non-goals

- Included: a synthetic ExternalSecret outage fixture, a focused assertion, Vault stop/recovery orchestration in the disposable kind script, and harness/task documentation.
- Excluded: changes to the production controller, Vault production configuration, real credentials, live CSP access, and broader ESO failure matrices.
- The existing path-missing RED/GREEN and CloudAccount credential assertions remain required.

## Design decisions

- Reuse the existing disposable Vault and ESO installation; do not create a second cluster or introduce a new runtime dependency.
- Keep the test ExternalSecret pointed at the already-seeded synthetic Vault path so it can recover without adding credential values or reseeding.
- Create the outage ExternalSecret while Vault is online and first observe the outage assertion fail. Delete that probe object, stop the Vault Deployment, independently verify that the Vault Service has no ready endpoints, then run the same assertion and expect `Ready=False` with ESO's `SecretSyncedError`.
- Restore Vault, reseed its in-memory KV data, reissue the limited ESO token, and require the same ExternalSecret to become `Ready=True` with the expected target Secret keys.
- Do not print Secret data or Vault logs.

## TDD behavior

1. Add an outage assertion that expects an ESO sync error and a not-ready ExternalSecret. Run it while Vault is healthy; it must fail because ESO successfully syncs (RED).
2. Stop the Vault Deployment, confirm the Vault Service has no ready endpoints, and rerun the same assertion. It must pass because ESO reports `SecretSyncedError` (GREEN).
3. Restore Vault, reseed its ephemeral KV entry and limited token, then verify that ExternalSecret readiness and target Secret materialization recover.

## Work items

- [x] Add a fixture for a second ExternalSecret using only the existing synthetic Vault KV entry.
- [x] Add a focused assertion for Vault-unavailable failure that checks condition reason without reading status messages or Secret values.
- [x] Observe RED against the healthy Vault, then GREEN while the Vault Deployment has zero replicas.
- [x] Restore Vault, reseed the in-memory test value, reissue ESO's limited token, and assert ExternalSecret Ready plus target Secret keys.
- [x] Extend `make kind-e2e-secrets` orchestration without weakening its cleanup or credential-output protections.
- [x] Update `docs/harness.md` and record the task's verification results.
- [x] Run focused failure/recovery checks, both kind targets, Go checks, manifest generation, shell syntax, English task-doc scan, and diff checks.
- [x] Record PR link, check results, and merge commit before moving this plan to `completed`.

## Acceptance criteria

- The failure assertion fails while Vault is healthy and passes only when the Vault service is unavailable.
- While Vault is stopped and the Service has no ready endpoints, the new ExternalSecret reports `Ready=False` / `SecretSyncedError`.
- After Vault returns, ESO reports `Ready=True` and creates the target Secret with both expected keys.
- The test uses only runtime-generated synthetic data, never prints secret material, does not use a production Kubernetes context, and deletes the disposable cluster on exit.
- Existing path-missing RED/GREEN and `make kind-e2e` smoke coverage continue to pass.

## Verification

- Run the same outage assertion before and after stopping Vault and retain both outcomes.
- Run the full `make kind-e2e-secrets` flow, including Vault recovery.
- Run `make kind-e2e`, `make test`, `make vet`, `make manifests`, `make fmt-check`, `bash -n scripts/kind/*.sh`, the task-language scan, and `git diff --check`.
- Confirm no disposable kind cluster remains after each E2E run.

## Risks and open questions

- ESO's error message is provider-controller output and did not contain a stable connection phrase in the first run. The test therefore asserts the stable `SecretSyncedError` reason and independently confirms that the Vault Service has no ready endpoints; it never reads or prints the status message.
- Vault dev mode stores KV data and issued tokens in memory. Restarting its Pod clears both, so the recovery stage must seed a new synthetic value and issue a fresh, path-scoped ESO token before expecting ExternalSecret recovery.
- An outage probe must not expose secret values through status, shell tracing, or diagnostic logs.

## Verification log

- `make kind-e2e-secrets` — passed. The Vault-path-missing RED/GREEN remained intact. The Vault-outage assertion first failed while Vault was healthy, then passed with `SecretSyncedError` after the Vault Pod was stopped and the Service had no ready endpoints. After restarting Vault, the script reseeded synthetic KV values, rotated the path-scoped ESO token, and observed the same ExternalSecret become Ready with both keys.
- The first outage recovery attempt showed that Vault dev mode resets its KV data and tokens when the Pod restarts. The final recovery flow explicitly reseeds and reissues the token before checking ESO recovery.
- The ESO condition message did not have a stable connection phrase, so the test does not inspect it. It uses the stable condition reason and independently verifies zero Vault Service endpoints.
- `make kind-e2e` — passed; CloudAccount reported `CredentialsReady=False (SecretNotFound)`.
- `GOCACHE=/tmp/louder-go-build make test` — passed.
- `GOCACHE=/tmp/louder-go-build make vet` — passed.
- `GOCACHE=/tmp/louder-go-build make manifests` — passed.
- `make fmt-check`, `bash -n scripts/kind/*.sh`, task-doc English scan, and `git diff --check` — passed.
- `make kind-e2e-secrets` removed the disposable `louder-e2e` cluster through its EXIT cleanup after both successful and failed runs.
- Implementation PR: [#3 Test Vault outage and ESO recovery in kind](https://github.com/ghdwlsgur/louder/pull/3) — merged into `main` at `bd21fa62a90ec027ee39dff03007fd5d93abcf40` on 2026-09-30.
- GitHub reported no PR checks for this branch; all listed local verification completed before merge. The PR used the regular merge method without an administrative bypass.
