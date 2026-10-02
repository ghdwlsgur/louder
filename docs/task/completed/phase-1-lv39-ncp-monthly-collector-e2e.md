# Phase 1 Level 39: NCP Monthly Collector-to-Budget E2E

## Goal

Exercise NCP's monthly embedded fixture through the local kind Collector, ClickHouse storage, monthly budget reader, and `FakeNotifier` without live CSP credentials.

## Scope

- Add a kind fixture CloudAccount for NCP that uses the existing fixture mode and a synthetic member account.
- Run the NCP Collector Job and verify the emitted record has the monthly invoice basis, amount, currency, and interval.
- Confirm ClickHouse stores the record and the storage-backed monthly budget path evaluates it once and produces the expected threshold intents.
- Keep the current AWS fixture E2E intact.

## Non-goals

- Calling NCP APIs or adding credentials to kind.
- Changing NCP billing behavior, budget semantics, or ClickHouse schema.
- Testing a real Teams webhook; use `FakeNotifier` as the existing harness does.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/harness.md`
- `docs/provider-contract.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv33-ncp-monthly-cost-adapter.md`
- `docs/task/completed/phase-1-lv23-kind-analyzer-notifier-integration.md`
- `config/kind/fixtures/cloudaccount-collector.yaml`
- `scripts/kind/e2e.sh`
- `internal/analyzer/clickhouse_integration_test.go`

## Work items

1. Add an integration test proving a monthly NCP fixture record in ClickHouse contributes to a monthly budget and emits the expected notification intents.
2. Run the kind storage harness with that test before adding the NCP Collector Job and observe a genuine RED because the stored NCP fixture row is absent.
3. Add a kind NCP fixture CloudAccount and assertion script that runs the embedded NCP Collector Job and verifies its monthly JSONL output.
4. Run the NCP Collector Job before the ClickHouse/Analyzer checks and verify GREEN end to end.
5. Update `docs/harness.md` to describe this coverage and record all verification results.

## Completion criteria

- `make kind-e2e-storage` collects the NCP fixture through an Operator-created Collector Job without CSP credentials.
- The NCP row is stored and read through the ClickHouse reader using its monthly interval.
- The monthly BudgetPolicy evaluation includes the NCP amount once and `FakeNotifier` receives the expected threshold notification.
- Existing AWS fixture persistence, replay deduplication, and notification checks continue to pass.
- The full Go suite, vet, format, manifests, build, and `git diff --check` pass.

## Design decisions and risks

- The fixture is fixed to October 2026 to align with existing deterministic NCP month tests; the Analyzer integration test supplies a matching fixed UTC clock instead of relying on wall-clock time.
- Reuse the existing NCP fixture adapter path. No special NCP kind runtime or provider credentials are required in fixture mode.

## Verification plan

- Focused test before implementation (RED): `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage`
- Focused test after implementation (GREEN): `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `git diff --check`

## Results

- RED: `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage` reached the new stored-budget test and produced zero intents because no NCP fixture row had been collected into ClickHouse.
- GREEN: `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage` passed after adding the NCP CloudAccount fixture Job. The test verified the monthly Collector JSONL fields, ClickHouse persistence/read path, and 80%/100% monthly budget notifications through `FakeNotifier`.
- Focused tests: `GOCACHE=/tmp/louder-go-cache go test ./internal/collector ./internal/analyzer ./internal/storage/clickhouse -count=1` — passed.
- Full suite: `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check`, `bash -n scripts/kind/assert-ncp-monthly-collector.sh scripts/kind/e2e.sh`, and `git diff --check` — passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` and `GOCACHE=/tmp/louder-go-cache make build` — passed.
