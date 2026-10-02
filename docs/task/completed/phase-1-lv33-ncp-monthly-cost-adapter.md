# Phase 1 Level 33: NCP Monthly Cost Adapter

## Goal

Collect Naver Cloud Platform (NCP) monthly actual cost totals through the documented Cost and Usage billing API and include them in monthly budget evaluation. NCP does not provide Louder with a documented daily actual-cost API, so NCP records must never enter daily cost anomaly detection.

## Scope

- Implement a commercial NCP adapter using `getDemandCostList` and `thisMonthAmountIncludingVat` as the account's monthly invoice cost.
- Aggregate provider cost rows by billing month and payment currency without floating-point conversion.
- Represent a monthly billing period with its UTC month-start and next-month-start interval; keep the existing normalized interval and ClickHouse schema.
- Make monthly budget evaluation include monthly records and daily anomaly analysis skip them.
- Register the adapter, add sanitized fixtures, contract and normalizer tests, sample CloudAccount configuration, and credential documentation.
- Keep tests offline and preserve Vault -> ESO -> Kubernetes Secret credential flow.

## Non-goals

- Daily NCP costs, NCP daily anomaly detection, or a browser automation/scraping path.
- Deriving costs from daily usage, published prices, or month-to-date deltas.
- NCP organizational/partner billing scope in the initial adapter.
- Changing other providers' cost basis or granularity.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/secrets.md`
- `docs/cluster-platform.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv32-ibm-cloud-focus-cost-adapter.md`
- NCP Cost and Usage API overview: https://api.ncloud-docs.com/docs/platform-costandusage
- NCP monthly contract cost API: https://api.ncloud-docs.com/docs/platform-costandusage-getcontractdemandcostlist
- NCP monthly account billing API: https://api.ncloud-docs.com/docs/platform-costandusage-getdemandcostlist
- NCP monthly service cost API: https://api.ncloud-docs.com/docs/platform-costandusage-getproductdemandcostlist
- NCP Cost Explorer Cost Analysis: https://guide.ncloud-docs.com/docs/costexplorer-costanalysis

## Work items

1. Verify the selected endpoint, account-level amount semantics, currency, monthly update behavior, pagination, and ordinary commercial account permissions.
2. Add the public adapter behavior test and run it to observe a genuine RED before implementing each behavior.
3. Implement NCP Signature v2 authentication, monthly request windows, pagination, decimal-safe aggregation, stable month/currency record IDs, and stable provider error classes.
4. Represent NCP's monthly billing period using the existing normalized usage interval and cost-basis fields; keep existing providers' records unchanged.
5. Verify ClickHouse writes and reads the existing monthly interval without a schema change, and make monthly rows count once in budgets while daily anomaly analysis skips them.
6. Add fixtures, provider contract tests, normalization tests, sample CloudAccount, Collector registration, and NCP credential documentation.
7. Update architecture, provider contract, harness, and secrets documentation to describe monthly-only NCP support and the daily anomaly gap.
8. Run focused tests, full tests, vet, format checks, manifest generation, build, kind E2E when Docker is available, and `git diff --check`.

## Completion criteria

- The adapter collects documented monthly actual-cost amounts for an ordinary NCP account without live CSP access in tests.
- Monthly records have deterministic month/currency identities and exact decimal amounts.
- Monthly NCP records contribute once to monthly budget totals and never cause daily anomaly evaluation to count or reject them.
- Existing daily provider records and anomaly behavior remain unchanged.
- Secret material is limited to Secret-backed Access Key and Secret Key values and is never logged or committed.
- Documentation clearly states monthly granularity, cost basis, update caveats, permissions, and that daily NCP anomaly detection is unsupported.
- Verification commands and results are recorded below.

## Design decisions, risks, and open questions

### User decision (2026-10-02)

- Support NCP monthly costs only.
- Keep NCP excluded from daily cost anomaly detection.
- Do not treat browser automation or undocumented Cost Explorer requests as an API integration.

### Official source review (2026-10-02)

- The Cost Explorer Cost Analysis page describes monthly cost periods, Excel downloads, and refreshes based on the prior day's data. It does not document an automated export endpoint.
- The public Cost and Usage API lists separate monthly billing-cost APIs and a daily usage API. The daily usage API is not a cost source.
- `getDemandCostList` queries monthly billing summaries by `startMonth` and `endMonth` (up to three months), paginates up to 1,000 rows, and exposes `demandMonth`, `thisMonthAmountIncludingVat`, and `payCurrency`.
- The adapter uses `thisMonthAmountIncludingVat`, which the official response schema defines as this month's billing amount including VAT. It does not use `totalDemandAmount`, which is a total billing amount and can include prior outstanding charges.
- The official API documents monthly records but does not promise when an in-progress month's amount becomes available. Collection preserves the API value as returned; it does not calculate daily costs or infer an unavailable current-month amount.
- Cost Explorer's Cost Analysis console refreshes from prior-day data, but its public guide documents monthly views and manual Excel download, not an automated export endpoint.

## Verification plan

- Focused TDD cycles: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/ncp ./internal/provider/contracttest ./internal/normalize ./internal/analyzer ./internal/storage/clickhouse ./internal/collector ./cmd/collector -count=1`
- Full suite: `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `make kind-e2e` when Docker is available.
- `git diff --check`

## Results

- TDD Red/Green: adapter aggregation, member-account validation, month-window limits, signed API pagination, daily anomaly exclusion, fixture support, and CRD provider registration each had a failing behavioral test before implementation and passed after the corresponding change.
- Focused verification: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/ncp ./internal/provider/contracttest ./internal/normalize ./internal/analyzer ./internal/storage/clickhouse ./internal/collector ./cmd/collector ./api/v1alpha1 -count=1` — passed.
- Full suite: `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed. The first sandboxed attempt could not bind the existing Azure `httptest` listener; rerunning with approved local test-port access passed.
- Static and generated-file checks: `GOCACHE=/tmp/louder-go-cache make vet`, `make fmt-check`, `GOCACHE=/tmp/louder-go-cache make manifests`, and `GOCACHE=/tmp/louder-go-cache make build` — passed.
- `git diff --check` — passed.
- `GOCACHE=/tmp/louder-go-cache make kind-e2e-storage` — passed, including Operator/fixture Collector flow, ClickHouse replay deduplication, monthly NCP native-reader round-trip, and Analyzer stored-budget verification. The initial run exposed a stale AWS fixture assertion (`unblended_cost` vs. current `net_cost`); the expectation was corrected and the complete E2E passed on rerun.
