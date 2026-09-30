# Phase 1 LV13: Provider Contract Foundation

## Goal

Add a small provider registration seam and reusable contract test support so initial CSP adapters can be added consistently without selecting a billing API source in this task.

## Scope

- Add a registry that resolves provider names to provider constructors behind the existing `Provider` interface.
- Define stable provider error classes for the taxonomy already described in `docs/provider-contract.md` without changing the `Provider` method signatures.
- Add a reusable contract test harness for adapter packages to exercise credential validation and collection scenarios with fixture-backed or fake upstreams.
- Verify the shared harness itself with an in-repository test provider.
- Document the current foundation and the adapter authoring pattern.

## Non-goals

- Live AWS, GCP, or Azure API calls.
- Choosing Cost Explorer, CUR/Athena, or another billing export source.
- Adding provider-specific credential fields or changing the `RawCostRecord` schema.
- ClickHouse ingestion, normalization, analysis, or Teams notification.
- Registering incomplete provider implementations in the production Collector.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv9-fixture-collector-cronjob.md`
- `docs/task/completed/phase-1-lv12-collector-ready-condition.md`

## Work items

1. Write a public-behavior test proving a provider registered under a canonical name can be constructed and used through the `Provider` interface; confirm RED.
2. Implement the minimal registry with clear errors for invalid registrations, duplicate names, and unknown names.
3. Write a contract-harness test proving a passed collection scenario checks credential validation and exact collected records; confirm RED before implementing the harness.
4. Add provider error classes and reusable contract scenarios for successful collection, empty results, complete multi-page output, and classified failures, without requiring live CSP access.
5. Update `docs/provider-contract.md` and `docs/harness.md` with the code-supported registry and test harness usage, while retaining provider-specific behaviors as adapter completion requirements.

## Completion criteria

- The registry returns a provider that implements the existing interface and rejects duplicate, blank, nil, and unknown registrations deterministically.
- Provider failures can be inspected by stable class while retaining an underlying cause for logs and diagnostics.
- Adapter packages can run the same reusable contract suite against their fixture or fake-upstream implementation.
- The shared contract test verifies credential validation, exact record preservation, empty result behavior, full logical-window collection, and stable error classes.
- Credential and collection scenarios can provide a deadline/canceled context for adapter timeout tests.
- Tests do not require cloud credentials, live CSP APIs, or network access.
- `RawCostRecord` and `Provider` method signatures remain unchanged.
- Unit tests, vet, formatting, build, and documentation checks pass.

## Design decisions and risks

- Provider constructors are registered as closures so each future adapter can capture its own test or runtime configuration without exposing provider-specific fields in shared business logic.
- This foundation does not prove a real adapter's pagination or retry implementation. Each adapter must invoke the contract suite with scenarios backed by its own fake upstream.
- The task deliberately leaves the AWS billing source and credential representation undecided.
- Provider errors expose a stable class separately from their wrapped cause; callers should avoid placing wrapped details in user-visible status.

## Verification plan

- Focused provider registry and contract test runs, recording RED then GREEN for each behavior.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache go vet ./...`
- `GOCACHE=/tmp/louder-go-cache make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `git diff --check`
- Inspect task Markdown for English-only content.

## Verification results

- RED/GREEN: `TestRegistryConstructsProviderByName` first failed because the registry API did not exist; it passed after the registry implementation.
- RED/GREEN: the provider error test first failed because the stable error type and class inspector did not exist; it passed after implementation.
- RED/GREEN: the shared contract suite test first failed because the suite API did not exist; it passed after implementation.
- RED/GREEN: deadline scenarios first failed because the contract suite scenarios could not supply a context; they passed after context support was added for both credential validation and collection.
- RED/GREEN: registry validation for non-lowercase provider names failed before enforcing canonical lowercase identifiers, then passed after the registry change.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go vet ./...` — passed.
- `GOCACHE=/tmp/louder-go-cache make fmt-check` — passed after formatting the new Go files.
- `GOCACHE=/tmp/louder-go-cache make build` — passed (required access to the local Go module cache).
- `git diff --check` — passed.
- Task plan content is English.

## Open decisions carried forward

- Select the AWS billing data source and provider credential representation before starting a live adapter.
- Expand `RawCostRecord` only after agreeing on the shared normalization schema for credits, discounts, and additional dimensions.
