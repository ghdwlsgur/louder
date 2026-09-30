# Phase 1 LV16: AWS Cost Record Normalizer

## Goal

Introduce the first provider-neutral normalized cost record and normalize AWS daily account totals while preserving the source amount, currency, date interval, account scope, cost basis, and source identifier.

## Scope

- Add an explicit `CostBasis` to raw provider records and map AWS Cost Explorer `UnblendedCost` to `unblended_cost`.
- Add a normalized cost record with provider, billing account, source record ID, cost basis, amount, currency, and usage interval.
- Implement a pure normalizer that validates required fields and preserves amount precision and source identity.
- Add AWS-focused normalization tests and update the provider, architecture, and harness documentation to state the minimal first schema.

## Non-goals

- Service, SKU, resource, tag, or usage quantity breakdowns.
- Credits, discounts, list/effective/billed cost reconciliation, exchange-rate lookup, or KRW conversion.
- ClickHouse schema, persistence, ingestion, or deduplication policy.
- Normalization for providers whose billing metric semantics have not been mapped.
- Changes to Analyzer, Notifier, Kubernetes APIs, or live CSP test requirements.

## Dependencies and prior documents

- `AGENTS.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv14-aws-cost-explorer-collector.md`
- `skills/louder-tdd/SKILL.md`

## Work items

1. Add a RED behavior test for normalizing a valid AWS account-day record and preserving exact source values.
2. Add one RED validation behavior at a time for missing basis/identity, malformed amount/currency, and invalid usage interval.
3. Add the minimum raw cost-basis field and AWS `UnblendedCost` mapping needed for those tests.
4. Implement the provider-neutral normalized record and pure normalization function.
5. Update architecture, provider contract, and harness docs to describe the first supported normalized slice and its limits.

## Completion criteria

- AWS `UnblendedCost` is represented explicitly as `unblended_cost` in both raw and normalized records.
- The normalizer preserves amount text exactly, including trailing decimal zeros and negative values.
- Source record ID, provider, account scope, currency, and UTC usage interval survive normalization unchanged.
- Invalid records return a stable validation error and do not produce a normalized record.
- Existing collector fixtures and provider contract tests remain passing.
- No cloud credentials or live CSP calls are needed for tests.
- `make test`, `make vet`, `make fmt-check`, and `git diff --check` pass.

## Design decisions and risks

- Start with a deliberately small normalized schema that can represent the AWS data already collected. Do not invent absent service/resource attributes or treat a missing metric as zero.
- Keep monetary values as decimal strings in this layer; do not use floating-point conversion or perform currency conversion.
- Preserve the raw provider record as the source-of-truth input. Normalization produces an analytical shape and does not mutate the raw record.
- Other providers must define the semantics of their cost metric before their raw records can use this normalizer.

## Verification plan

- Run each focused normalization test before implementation and record genuine assertion failures as RED.
- `GOCACHE=/tmp/louder-go-build go test ./internal/normalize ./internal/provider/aws -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `git diff --check`
- Review changed Markdown for English-only task documentation and check that it matches the minimal schema decision.

## Verification results

- RED/GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/normalize -run TestNormalizePreservesAWSUnblendedDailyCost -count=1` first failed because normalization returned an empty record, then passed after mapping the raw account-day fields and converting the time interval to UTC.
- RED/GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/provider/aws -run TestCollectCostsQueriesUnblendedDailyAccountTotalsAndAllPages -count=1` failed until the AWS adapter populated `CostBasisUnblended`.
- RED/GREEN: focused tests for missing cost basis, malformed amount, invalid currency, and empty usage interval each failed before their validation was added and passed afterward.
- The normalized AWS JSON golden test passes and verifies exact serialization, including the decimal string and UTC timestamps.
- `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- `GOCACHE=/tmp/louder-go-build make vet` passed.
- `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- `git diff --check` passed.
- kind E2E was not run for this pure normalization-library change; no Kubernetes manifests or runtime wiring changed.
