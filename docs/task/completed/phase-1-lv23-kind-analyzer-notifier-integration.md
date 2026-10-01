# Phase 1 LV23: Kind Cost-to-Notification Integration

## Goal

Verify the complete offline path from a persisted ClickHouse fixture record through monthly budget evaluation to FakeNotifier notification intents.

## Scope

- Add an opt-in Analyzer integration test that reads the existing synthetic AWS fixture row from ClickHouse.
- Run `EvaluateAndNotifyBudget` with a synthetic BudgetPolicy, CloudAccount, and FakeNotifier.
- Assert the UTC month query produces exact 80% and 100% threshold notification payloads.
- Extend `make kind-e2e-storage` to run the integration test against its temporary ClickHouse instance through the existing local port-forward.
- Document the verified integration path and its boundaries.

## Non-goals

- Sending to a real Teams tenant.
- Analyzer scheduling, Kubernetes policy resolution, or notification deduplication.
- Production ClickHouse, Vault, or CSP access.
- Changes to the Collector, storage schema, or Analyzer calculations.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv19-clickhouse-cost-reader.md`
- `docs/task/completed/phase-1-lv20-stored-budget-evaluation.md`
- `docs/task/completed/phase-1-lv21-teams-workflow-notifier.md`
- `docs/task/completed/phase-1-lv22-budget-notification-routing.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Add a RED opt-in integration test proving stored AWS cost data generates the expected FakeNotifier threshold payloads.
2. Run the test without integration configuration and verify it skips without requiring a live service.
3. Extend kind storage E2E to run the test with its ephemeral ClickHouse connection settings.
4. Verify the integration test passes after two fixture collection attempts and uses `FINAL` semantics through the production reader.
5. Update harness documentation and record exact verification results.

## Completion criteria

- The test is skipped by ordinary `go test` unless `CLICKHOUSE_INTEGRATION=1` is set.
- The test queries ClickHouse through `clickhouse.OpenFromEnv` and calls the public Analyzer service.
- The fixture's exact `12.34 USD` value over a `10 USD` budget produces deterministic 80% and 100% FakeNotifier notifications.
- The integration path uses only synthetic data and no real Teams endpoint.
- The kind storage E2E completes and removes its disposable cluster.
- `make test`, `make vet`, `make fmt-check`, shell syntax checks, and `git diff --check` pass.

## Design decisions and risks

- Use a fixed January 2026 evaluation interval matching the embedded AWS fixture so the test remains deterministic regardless of wall-clock date.
- Run integration tests from the host through kubectl port-forward; the application connection still uses the same ClickHouse native protocol as the Collector.
- This verifies the existing service composition but does not provide a production Analyzer scheduler or persisted notification deduplication.

## Verification plan

- Run the focused test first without the opt-in environment and verify it skips.
- Run the complete kind target using a unique cluster name and record the native ClickHouse and Analyzer integration results.
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `bash -n scripts/kind/*.sh`
- `git diff --check`

## Verification results

- `GOCACHE=/tmp/louder-go-build go test ./internal/analyzer -run TestStoredBudgetProducesFakeNotifierIntents -count=1 -v` passed with the test skipped when integration configuration was absent.
- `KIND_CLUSTER_NAME=louder-e2e-lv23-notifier GOCACHE=/tmp/louder-go-build make kind-e2e-storage` passed. After two fixture collection attempts, the native ClickHouse reader returned the single logical row and the Analyzer integration test verified exact 80% and 100% notifications through FakeNotifier.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `bash -n scripts/kind/*.sh` passed.
- GREEN: `git diff --check` passed.
- No real Teams endpoint or CSP API was called; the kind target removes its disposable cluster on completion.
