# Phase 1 Level 35: Alibaba Cloud Cost Adapter

## Goal

Add an offline-testable Alibaba Cloud adapter that collects daily account billing records for recurring cost-spike analysis.

## Scope

- Implement the Alibaba Cloud BSS OpenAPI `DescribeInstanceBill` daily billing query for the CloudAccount account ID.
- Request one billing date at a time with `Granularity=DAILY`; walk the UTC collection window across day and month boundaries and fully consume `NextToken` pages.
- Preserve source currency and exact decimal amounts. Use the provider's `PretaxAmount` (payable amount) as the adapter's `alibaba_pretax_cost` basis until repository semantics prove a compatible shared basis.
- Include refunds and adjustments as source records and create stable IDs from the account, billing date, currency, and source bill identifiers.
- Add fixture, provider contract, normalizer, registry/collector, sample CloudAccount, and credential documentation coverage following existing provider conventions.
- Keep tests independent of live Alibaba Cloud access.

## Non-goals

- Invoice reconciliation or claiming current-month data is final.
- Estimating costs from prices or allocating monthly amounts across days.
- Collecting detailed split-bill/resource allocations or enabling the Split Bill feature as part of this adapter.
- Adding Alibaba-specific policy or analysis logic to shared components.

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
- Alibaba Cloud `DescribeInstanceBill` API reference: https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-describeinstancebill
- Alibaba Cloud BSS API overview: https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-overview
- Alibaba Cloud bill export and subscription: https://www.alibabacloud.com/help/en/user-center/export-and-subscribe-bills/

## Work items

1. Verify the official request/response schema, authentication/signing requirements, paging behavior, permissions, and cost semantics before coding.
2. Add one public Provider behavior test and run it to confirm a genuine RED result.
3. Implement the smallest adapter slice that makes the test pass, then add behavior tests incrementally for date windows, month boundaries, pagination, decimals, refunds/adjustments, malformed responses, and provider error classes.
4. Add sanitized daily billing fixtures and run the shared provider contract tests, including pagination.
5. Add normalization and Collector registration/configuration, then document the RAM permission and credential Secret shape without secrets.
6. Record verification results, including checks not run and any live-account validation that remains unavailable.

## Completion criteria

- `DescribeInstanceBill` is queried for every requested daily date and every page is collected; any failed page fails the request rather than returning partial costs.
- Costs use exact decimal parsing, preserve original currency, and have stable source IDs across re-collection.
- Refund and adjustment `PretaxAmount` values are summed exactly as returned; coupon, resource-package, and invoice-discount fields are not silently recomputed or applied a second time. The field is pretax and must not be presented as a tax-inclusive invoice total.
- Provider fixtures, contract tests, normalization tests, and Collector registration/configuration exist and run without live CSP credentials.
- Documentation clearly states that bill data is delayed, current-month totals exclude unsettled PAYG, and attached-resource detail may be absent; the adapter is not described as invoice-final or fully inclusive of every attached resource.
- Verification results and any checks not run are recorded here.

## Design decisions, risks, and open questions

- `DescribeInstanceBill` officially supports `Granularity=DAILY` with `BillingDate` and `BillingCycle`; the API returns `PretaxAmount`, `Currency`, `Item`, `InstanceID`, and `NextToken`. `PretaxAmount` is documented as the payable amount, while the response also exposes discounts and coupon/resource-package deductions.
- The initial cost basis is `alibaba_pretax_cost`. Do not map it to `net_cost` or `actual_pre_tax_cost` unless a documented semantic equivalence is established. The adapter must not add discount fields back to the payable amount or subtract them twice.
- Alibaba documents a 24-hour billing-data delay and says current-month data excludes unsettled PAYG amounts. It documents an 18-month query history.
- Alibaba documents that instance-bill APIs do not provide specific costs for some attached resources (including domain names, buckets, and EIPs for CDN, OSS, and Internet Shared Bandwidth). Split Bill can provide attached-resource detail but must be enabled, has its own 48/72-hour update delay, and is outside this initial adapter slice. Cost-spike alerts may therefore undercount affected services; document this limitation for operators.
- Daily account bills can contain multiple currencies. Keep records distinct by currency and never sum unlike currencies in the adapter.
- Alibaba defines `PretaxAmount` as the payable amount before tax and separately exposes discount/coupon fields. This adapter preserves `PretaxAmount` and does not derive a tax-inclusive invoice total.
- Do not call Alibaba Cloud APIs during normal development or tests. Live account credential validation is not a completion dependency.

## Verification plan

- API facts checked against official references: BSS endpoint `business.aliyuncs.com`; RPC API version `2017-12-14`; action `DescribeInstanceBill`; RAM action `bssapi:DescribeInstanceBill`; daily requests require `BillingCycle`, `Granularity=DAILY`, and `BillingDate`; `MaxResults` is capped at 300 and `NextToken` paginates. Alibaba documents `PretaxAmount` as payable before tax, 24-hour billing data delay, 18-month history, unsettled current-month PAYG exclusion, and some attached-resource detail gaps.
- TDD RED/GREEN: the initial public `TestCollectCostsAggregatesDailyPayableAmountsWithExactDecimals` failed to compile because the provider source contract and constructor were not implemented. After adding the minimum Provider/source implementation, that test passed. The security regression test `TestBSSSourceDoesNotExposeSignedURLInTransportErrors` then failed because the wrapped `net/http` error exposed the signed URL, AccessKey ID, and signature; returning only the stable error class made it pass. Additional focused tests cover signature compatibility, pagination, month boundaries, errors, currencies, no partial results, malformed responses, and normalization; those follow-up cases did not each record a separate pre-implementation RED run.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/alibaba ./internal/provider/contracttest ./internal/normalize ./internal/collector -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed after allowing localhost binding for the existing Azure `httptest` suite; the sandbox-only attempt failed to bind a socket.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` — passed and generated the CloudAccount provider enum update.
- `GOCACHE=/tmp/louder-go-cache make build` — passed.
- `make kind-e2e` — not run because `docker info` returned permission denied for `/Users/jinhyeokhong/.docker/run/docker.sock`.
- Live Alibaba API validation — not run; no account credentials were available or needed for offline verification.
- `git diff --check` — passed.
