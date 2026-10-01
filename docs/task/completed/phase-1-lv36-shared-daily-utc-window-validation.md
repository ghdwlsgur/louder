# Phase 1 Level 36: Shared Daily UTC Window Validation

## Goal

Remove duplicated daily UTC collection-window validation from the Azure, GCP, OCI, IBM Cloud, and Alibaba Cloud adapters while preserving their current externally observable behavior.

## Scope

- Add one shared helper in `internal/provider` that converts both bounds to UTC and accepts only a non-empty interval whose bounds are UTC midnight.
- Use the helper in the five adapters listed above.
- Keep provider-specific account-scope validation and error classes in each adapter.
- Preserve existing daily record grouping, cost basis, pagination, and source IDs.
- Keep NCP and NHN plans pending; this task does not implement either adapter because a documented unattended daily actual-cost source has not been established.

## Non-goals

- Changing collection windows, refresh lookback, billing semantics, or source APIs.
- Refactoring AWS Cost Explorer date handling, which has a separate provider-specific conversion path.
- Changing record aggregation or provider error classification.
- Adding CSP dependencies or live API calls.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/secrets.md`
- `SECURITY.md`
- `docs/task/README.md`
- `internal/provider/azure/provider.go`
- `internal/provider/gcp/provider.go`
- `internal/provider/oci/provider.go`
- `internal/provider/ibm/provider.go`
- `internal/provider/alibaba/provider.go`

## Work items

1. Run and record the existing provider tests for the five adapters as characterization coverage before refactoring.
2. Add a shared UTC daily-window helper and focused tests for valid UTC dates, equivalent offset-aware midnight bounds, empty/reversed windows, and sub-day bounds.
3. Replace only the duplicated request-boundary checks in the five adapters with the shared helper.
4. Run the five focused provider package tests after each adapter migration and the shared provider tests.
5. Run repository tests, vet, formatting, build, and manifest checks; record any unavailable checks.

## Completion criteria

- The shared helper returns normalized UTC bounds for valid daily windows and rejects empty, reversed, or sub-day windows.
- All five adapters retain their prior public behavior, including stable error classes and returned records.
- No provider-specific cost or request semantics change.
- Focused and repository verification results are recorded below.

## Refactor approach

This is behavior-preserving cleanup rather than a new provider behavior. Existing adapter tests are the initial green characterization baseline; focused helper tests will define the common boundary contract. No production changes are made before the baseline is captured.

## Verification plan

- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/azure ./internal/provider/gcp ./internal/provider/oci ./internal/provider/ibm ./internal/provider/alibaba -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/... -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `git diff --check`

## Verification results

- Baseline `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/azure ./internal/provider/gcp ./internal/provider/oci ./internal/provider/ibm ./internal/provider/alibaba -count=1`: passed after rerunning with local listener permission; the sandbox initially prevented Azure's `httptest` listener from binding.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/... -count=1`: passed, including the shared helper tests.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`: passed.
- `GOCACHE=/tmp/louder-go-cache make vet`: passed.
- `make fmt-check`: passed.
- `GOCACHE=/tmp/louder-go-cache make build`: passed.
- `GOCACHE=/tmp/louder-go-cache make manifests`: passed; no unrelated generated-file changes were produced.
- `git diff --check`: passed.
- Behavior note: no live cloud APIs were called. NCP and NHN remain pending under their existing plans because neither currently meets the verified daily actual-cost source requirement.
