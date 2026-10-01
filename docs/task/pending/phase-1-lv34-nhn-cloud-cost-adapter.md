# Phase 1 Level 34: NHN Cloud Cost Adapter

## Goal

Add an offline-testable NHN Cloud cost adapter only when an officially supported billing source provides costs at the granularity required by Louder's cost anomaly analysis.

## Scope

- Target: commercial NHN Cloud, as confirmed by the user.
- Verify official billing API/export availability, cost granularity, cost basis, currency, authentication, pagination, and data refresh behavior.
- Preserve the platform's actual daily cost requirement for cost increase/anomaly detection; do not turn monthly totals or published-price calculations into daily actual costs.
- If an exact daily actual-cost source is available for the target account, implement the adapter with fixtures, contract tests, normalization coverage, Collector registration, CloudAccount configuration, and credential documentation.
- If no such source is available, record the limitation and keep the adapter pending until an official daily cost feed or export is provided.

## Non-goals

- Estimating daily cost from usage quantities and published prices.
- Dividing monthly costs across days or presenting month-to-date deltas as actual daily cost.
- Using undocumented console endpoints or automating manual console downloads.
- Implementing another provider's billing semantics as part of this task.

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
- NHN Cloud public API guide index: https://docs.nhncloud.com/en/
- NHN Cloud commercial Framework API guide: https://docs.nhncloud.com/en/nhncloud/en/public-api/framework-api/
- NHN Cloud commercial Partner Management API guide: https://docs.nhncloud.com/en/nhncloud/en/public-api/partner-api/
- NHN Cloud cost management quickstart: https://docs.nhncloud.com/en/quickstarts/en/cost-management/
- NHN Cloud payment detail FAQ: https://cloud-private.oc.nhncloud.com/cloud/hc/article/2653/
- NHN Cloud for Public Institutions partner API guide: https://docs.gov-nhncloud.com/ko/nhncloud/ko/public-api/partner-api-gov/
- NHN Cloud for Public Institutions framework API guide: https://docs.gov-nhncloud.com/ko/nhncloud/ko/public-api/framework-api-gov/
- NHN Cloud public pricing and billing overview: https://www.nhncloud.com/kr/pricing

## Work items

1. Verify that commercial NHN Cloud has a supported unattended API/export returning actual daily cost amounts.
2. Confirm the source's cost semantics, scope, response periods, currency, credentials, pagination, and refresh delay.
3. Write one public Provider behavior test and observe RED before each implementation slice.
4. Implement only the approved exact daily behavior with stable IDs, exact decimal parsing, complete-result semantics, and stable provider error classes.
5. Add sanitized fixtures, contract tests, normalizer coverage, Collector registration, CloudAccount sample, and Vault/ESO setup documentation.
6. Run focused and repository tests, vet, formatting, manifests, build, and kind smoke when Docker is available; make no live NHN Cloud request.

## Completion criteria

- The adapter consumes an officially supported NHN Cloud source that returns actual daily costs for the confirmed commercial account.
- No monthly-to-daily allocation, price-derived estimate, or undocumented endpoint is used.
- Credentials remain in Kubernetes Secrets sourced from Vault through ESO.
- Provider fixtures, contract scenarios, normalization coverage, and failure tests run offline.
- Documentation states the account scope, source API, exact cost basis, daily granularity, permissions, credential keys, and refresh limitations.
- Verification results and any checks not run are recorded here.

## Design decisions, risks, and open questions

- The NHN Cloud for Public Institutions partner API guide requires a partner or delegated partner user and describes billing paths containing `/payments/{month}`. It returns monthly payment and usage summaries; the documented path does not establish actual daily cost records.
- The guide is for NHN Cloud for Public Institutions and must not be assumed to apply to commercial NHN Cloud accounts.
- The NHN Cloud for Public Institutions framework guide documents billing metadata and product prices, while its partner payment API returns a monthly amount. Combining metering with published rates would calculate a price-derived amount, not consume an actual daily billed-cost record.
- The partner API's `/payments/{month}` route requires a `yyyy-MM` value and the `Partner.Payment.Get` permission. Its example contains monthly charge, total amount, tax, and currency fields. The adjacent project usage endpoints likewise require a month.
- The public institution framework guide documents billing meters and `DAILY_MAX` as a metering aggregation type. A daily meter value is usage data, not a daily cost amount; it does not satisfy the platform's actual daily cost requirement and is outside the confirmed commercial account scope.
- NHN's commercial billing FAQ describes the invoice settlement cycle as monthly. This does not negate the partner-only daily usage-price endpoint described below, but it means daily source amounts still need to be distinguished from final invoice totals.
- User goal: recurring cost monitoring and early cost-spike alerts. Exact daily costs are required; estimate-based data is not acceptable.
- Confirmed target: commercial NHN Cloud.
- The commercial Public API catalog exposes the Framework API and Partner Management API as separate API families. The general-member Framework API documents public unit-price and service-metadata endpoints, but no actual-usage-cost query endpoint.
- The official commercial Partner Management API documents `GET /v1/billing/partners/{partnerId}/daily-usage-prices`, with a required `date` (`yyyy-MM-dd`) and organization or project scope. Its response includes `deltaBasicPrice` (daily pay-as-you-go price) and `deltaContractPrice` (daily agreement price), alongside daily usage and resource identifiers. This is a promising daily cost source, not a price-table estimate.
- The same Partner Management API guide explicitly says that only partners or partner-authorized users can call it and that general users cannot. It requires `Partner.Daily.Usage.List` and verifies a partner agreement for the queried date. Therefore this endpoint does not establish an accessible source for the user's confirmed ordinary commercial account.
- The commercial console quickstart documents organization and project usage views, budget setup, and threshold email alerts. It says threshold notifications may arrive at 10:00 the next day. The public guide does not document a general-user API or automated export for retrieving those actual daily amounts; console-only visibility is not sufficient for a recurring Louder collector.
- NHN's payment FAQ directs general users to the OWNER's invoice email or the project/organization Service Status console view for detailed charges, and sends unresolved questions to Customer Center. It documents neither an automated daily export nor a public API for general users.
- NHN's public billing FAQ describes charges as monthly, but that does not negate the partner-only daily usage-price API. The distinction is access scope: daily amounts are documented for partners, while a general-user daily cost retrieval interface was not found in the official commercial API catalog.
- Remaining source question: NHN support or the account owner must confirm whether this ordinary account can be granted access to the partner daily usage-price API, or identify an officially supported general-user daily actual-cost API/export. Any candidate also needs verification of currency, credit/tax/discount treatment, and data freshness before mapping to Louder's common cost basis.
- Questions for NHN Customer Center before implementation: (1) Is there a supported daily actual-usage-cost API/export for ordinary commercial accounts? (2) Can a resource-owning ordinary account call the documented partner daily endpoint, or is a reseller/partner agreement mandatory? (3) Are `deltaBasicPrice` and `deltaContractPrice` alternatives or additive, and do they include discounts, credits, and taxes? (4) What currency, lookback limit, pagination behavior, and data-finalization delay apply?
- Keep this plan pending until a daily actual-cost source is established. If the only supported billing endpoint is monthly, do not implement it as a daily provider adapter.

## Verification plan

- Verify API references and response schemas against official NHN Cloud documentation; record account-specific references supplied by the user or NHN support before implementation.
- Record a failing public behavior test before each code slice (RED), then a passing result (GREEN).
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/nhn ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `make kind-e2e` when Docker is available.
- `git diff --check`
