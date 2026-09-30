# Phase 1 LV14: AWS Cost Explorer Collector

## Goal

Collect one daily AWS account cost total through Cost Explorer `GetCostAndUsage`, using `UnblendedCost`, and make the live AWS path runnable from the existing Collector CronJob without affecting fixture-mode tests.

## Scope

- Add an AWS Provider adapter using AWS SDK for Go v2 and the SDK default credential chain.
- Query `DAILY` `UnblendedCost` for the CloudAccount billing scope with `LINKED_ACCOUNT` filtering and handle every response page.
- Map each daily total to the existing `RawCostRecord` without expanding its schema; derive a stable source ID from the account scope and UTC day.
- Make the live Collector choose the previous complete UTC day when no fixture is selected; retain fixture mode unchanged.
- Pass AWS credential environment variables from the existing Kubernetes Secret reference and document the exact Secret key names and least-privilege IAM action.
- Test adapter behavior using SDK client doubles or a local fake endpoint; tests must not call AWS.

## Non-goals

- CUR/Data Exports, S3, Athena, resource-level or service-level breakdowns.
- Amortized, net, blended, or billed cost metrics; the selected metric is `UnblendedCost` only.
- AWS Organizations management-account discovery or cross-account role assumption.
- Changes to the common `RawCostRecord` schema, normalization, ClickHouse ingestion, or analysis.
- Real AWS credentials or live AWS API tests in unit tests, CI, or kind.
- Changing Vault as the source of production secrets or adding direct Vault integration.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/secrets.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv13-provider-contract-foundation.md`

## Work items

1. Add a behavior test for the live Collector's default previous-complete-UTC-day window and confirm RED.
2. Add an AWS Cost Explorer adapter test proving the requested metric, daily granularity, account filter, exclusive end date, and pagination behavior; implement one behavior at a time in RED/GREEN cycles.
3. Map AWS SDK credential and service failures to stable provider error classes without logging credentials or copying raw errors into user-facing status.
4. Wire the Collector's non-fixture mode through the Provider registry to the AWS adapter and emit JSON Lines; retain the explicit fixture path.
5. Update Vault/ESO Secret key examples for AWS SDK environment-variable names, document `ce:GetCostAndUsage`, the selected cost semantics, collection window, and known freshness limits.
6. Verify the existing kind fixture flow still works; do not configure kind to call AWS.

## Completion criteria

- AWS collection requests use `DAILY`, `UnblendedCost`, the requested CloudAccount account scope, and an inclusive start/exclusive end date range.
- All Cost Explorer pages are read; malformed or missing metric responses fail explicitly instead of becoming zero-cost records.
- Output records preserve amount and currency strings, use UTC daily boundaries, and have deterministic source IDs.
- Live collection defaults to the previous completed UTC day and fixture mode remains explicit and offline.
- AWS SDK credentials are loaded from the standard chain, with Vault/ESO-provided environment variables documented and no secret values logged.
- Stable provider error classes cover invalid credentials, authentication, permission, throttling, timeout, and provider/service failures.
- Tests use synthetic data and never require live CSP access.
- Unit tests, vet, formatting, build, kind E2E, and `git diff --check` pass.

## Design decisions and risks

- The initial implementation uses account-level daily totals without service/resource grouping to keep the existing common record model unchanged.
- A scheduled run re-collects only the previous complete UTC day. AWS cost data can refresh later, so this first version may need a later lookback/idempotent-ingestion task to capture revisions.
- The Cost Explorer API uses an exclusive end date; date validation and conversion must prevent off-by-one-day collection.
- The adapter filters on `LINKED_ACCOUNT` equal to `CloudAccount.spec.accountId` so an organization credential cannot accidentally collect all member accounts.
- The initial credential path uses the AWS SDK default credential chain. Production still follows Vault -> ESO -> Kubernetes Secret -> Collector; the Operator receives only the Secret reference.
- AWS collection has per-request Cost Explorer API charges; page count should be observable in tests and must not be hidden by retries.
- This task does not establish invoice-level reconciliation: `UnblendedCost` is the selected operational cost view, not the final invoice amount.

## Verification plan

- Focused adapter and Collector window tests, recording RED then GREEN for each behavior.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache go vet ./...`
- `GOCACHE=/tmp/louder-go-cache make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make kind-e2e`
- `git diff --check`
- Inspect task Markdown for English-only content.

## Verification results

- RED: `GOCACHE=/tmp/louder-go-build go test ./internal/provider/aws -run TestCostExplorerProviderContract -count=1` failed because empty static credentials were classified as `ProviderUnavailable` instead of `InvalidCredentialShape`.
- GREEN: the same focused test passed after mapping AWS SDK `StaticCredentialsEmptyError` to `InvalidCredentialShape`.
- `GOCACHE=/tmp/louder-go-build make test` passed all Go tests.
- `GOCACHE=/tmp/louder-go-build make vet` passed.
- `GOCACHE=/tmp/louder-go-build make fmt-check` passed after formatting `cmd/collector/main.go`.
- `GOCACHE=/tmp/louder-go-build make build` passed for the operator and collector. Go printed non-fatal module-cache stat-cache write warnings because the shared module cache is outside the writable workspace.
- `GOCACHE=/tmp/louder-go-build make kind-e2e` built the Docker image, then stopped because the script found an existing `louder-e2e` cluster and intentionally refuses to replace it. The existing cluster was left untouched.
- `git diff --check` passed.

## English-language design summary

The first AWS collection path reads daily linked-account totals with `UnblendedCost` from Cost Explorer. The live collector requests the previous complete UTC day, uses the AWS SDK default credential chain, and emits the existing raw record format. Vault and ESO remain the production secret path. Tests use synthetic SDK responses and never call AWS. The initial slice excludes service or resource groups, CUR/Data Exports, cross-account role assumption, and live CSP checks. Cost Explorer data may refresh after collection; backfill and invoice reconciliation remain future work.
