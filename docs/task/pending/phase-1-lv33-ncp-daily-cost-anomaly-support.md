# Phase 1 Level 33: NCP Daily Cost Anomaly Support

## Goal

Provide daily NAVER Cloud Platform (NCP) cost signals for recurring cost anomaly detection so sudden increases can be surfaced before monthly billing closes.

## Scope

- Verify NCP daily usage and monthly cost API fields from official documentation and, if available, representative sanitized account data.
- Find an officially supported, automatically callable source that returns actual daily NCP cost amounts.
- Implement the adapter only if the source provides actual costs at daily granularity and can be reconciled to NCP billing semantics.
- If the only available sources remain daily usage quantities or monthly billing totals, document NCP as unsupported pending a provider-supported daily cost source.
- Add the selected behavior's fixtures, provider contract tests, normalization coverage, Collector registration, CloudAccount configuration, and credential documentation.
- Keep normal tests offline and never infer daily amounts by multiplying usage by list prices or allocating monthly totals across days.

## Non-goals

- Calculating estimated costs from daily usage and published prices.
- Allocating monthly charges uniformly or proportionally across days without provider-issued daily cost values.
- Changing other providers' cost semantics without a separate requirement.
- Making live NCP API calls in routine tests.

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
- NCP daily contract usage API: https://api.ncloud-docs.com/docs/en/platform-costandusage-getcontractusagelistbydaily
- NCP monthly contract cost API: https://api.ncloud-docs.com/docs/en/platform-costandusage-getcontractdemandcostlist
- NCP monthly service cost API: https://api.ncloud-docs.com/docs/en/platform-costandusage-getproductdemandcostlist
- NCP monthly billing cost API: https://api.ncloud-docs.com/docs/en/platform-costandusage-getdemandcostlist
- NCP daily usage response model: https://api.ncloud-docs.com/docs/common-vapidatatype-contractusagebydaily
- NCP list-price lookup API: https://api.ncloud-docs.com/docs/en/platform-listprice-getpricelist
- NCP price model: https://api.ncloud-docs.com/docs/en/common-vapidatatype-Price
- NCP Cost Explorer Cost Analysis guide: https://guide.ncloud-docs.com/docs/en/costexplorer-costanalysis
- NCP Cost Explorer permissions: https://guide.ncloud-docs.com/docs/en/costexplorer-subaccount

## Work items

1. Search for a provider-documented API or export that exposes actual daily cost amounts and supports unattended collection.
2. Verify the source's daily cost semantics, authentication, pagination, discounts, currency, and date boundaries.
3. If an exact daily-cost source is found, implement its behavior against the existing daily provider contract.
4. Write one public Provider behavior test and observe RED before each implementation slice.
5. Implement the approved adapter behavior with exact decimal parsing, complete pagination, stable source IDs, and stable provider error classes.
6. Add fixtures, provider contract cases, normalizer coverage, CloudAccount sample, Collector registration, and Vault/ESO setup documentation.
7. Run focused and repository tests, vet, formatting, manifests, build, and kind smoke when Docker is available; make no live NCP request.

## Completion criteria

- The selected NCP source provides actual billed cost amounts at daily granularity through a documented, unattended access method.
- No list-price estimate, usage-only proxy, monthly-to-daily delta, or arbitrary allocation is represented as actual daily cost.
- Provider fixtures, contract scenarios, normalization coverage, and failure tests run offline.
- Documentation states the data source, exact cost basis, granularity, required NCP permissions, secret keys, and any remaining upstream limitations.
- Verification results and any checks not run are recorded here.

## Design decisions, risks, and open questions

### Official source recheck (2026-10-02)

- `getContractUsageListByDaily` accepts a daily date range (up to three months) and returns contract usage records. Its documented usage model includes quantity, unit, metering type, and contract identifiers, but no billed amount.
- `getDemandCostList`, `getContractDemandCostList`, and `getProductDemandCostList` query billing amounts by month. Their amount, discount, and tax fields do not provide daily actual-cost granularity.
- Cost Analysis is documented as refreshing the prior day's data each morning and supports console analysis and spreadsheet download. The reviewed public API catalog documents neither an unattended Cost Explorer cost-data endpoint nor a scheduled export endpoint suitable for the collector.
- NCP's native budget notifications use previous-day cost data, which confirms an NCP-managed alerting path but does not expose a documented data feed for Louder to collect.
- References rechecked: [daily contract usage API](https://api.ncloud-docs.com/docs/en/platform-costandusage-getcontractusagelistbydaily), [daily usage response model](https://api.ncloud-docs.com/docs/common-vapidatatype-contractusagebydaily), [monthly billing cost API](https://api.ncloud-docs.com/docs/en/platform-costandusage-getdemandcostlist), [Cost Analysis guide](https://guide.ncloud-docs.com/docs/en/costexplorer-costanalysis), and [Cost Explorer budgets](https://guide.ncloud-docs.com/docs/en/costexplorer-budget).
- Decision: the adapter's source acceptance criteria are not met by the available evidence. Keep this plan pending until NCP documents an unattended daily actual-cost API/export or the account owner supplies an official integration contract that can be verified.

### Official documentation review (2026-10-02)

- The Cost and Usage API overview lists `getContractUsageListByDaily` alongside separate contract, product, and demand cost APIs. The overview describes access through `billingapi.apigw.ntruss.com/billing/v1` and NCP Signature v2 authentication using an Access Key, Secret Key, timestamp, and HMAC-SHA256.
- `getContractUsageListByDaily` accepts `useStartDay` and `useEndDay` (up to three months) and supports page sizes up to 1,000. Its sample record contains `account`, `useDate`, `contract`, `contractProduct`, and `usage`; the usage fields are quantities and units. Contract product identifiers such as `priceNo` and `promiseNo` do not provide a billed amount.
- `getContractDemandCostList` and `getProductDemandCostList` explicitly query monthly billing costs using `startMonth` and `endMonth` (up to three months). Their `demandMonth`, `useAmount`/`demandAmount`, discounts, and currency fields are useful monthly billing data, not daily billed cost records.
- Cost Analysis refreshes data each morning using the previous day's data, but the documented view period is monthly and its detailed-cost export is an Excel download from the console. Cost Insight is documented with monthly graphs. These guides do not describe a machine-callable daily actual-cost endpoint or scheduled export.
- Cost Explorer permission documentation lists `View/getCostData` and `View/getCostDataDetail` as console actions. A separate Resource Manager page lists `Download Budget Cost Data` as a Budget action history entry; neither page supplies an API URI, request schema, response schema, or automation contract for collecting daily cost data.
- Cost Explorer's native budget notifications use cost data and are delivered the following day. This is an available NCP-managed alert path, but the public guide does not document a cost-data feed for Louder to ingest.
- Sources reviewed: [Cost and Usage API overview](https://api.ncloud-docs.com/docs/platform-costandusage), [daily contract usage API](https://api.ncloud-docs.com/docs/en/platform-costandusage-getcontractusagelistbydaily), [daily contract usage response model](https://api.ncloud-docs.com/docs/common-vapidatatype-contractusagebydaily), [monthly contract cost API](https://api.ncloud-docs.com/docs/en/platform-costandusage-getcontractdemandcostlist), [monthly service cost API](https://api.ncloud-docs.com/docs/en/platform-costandusage-getproductdemandcostlist), [Cost Analysis guide](https://guide.ncloud-docs.com/docs/en/costexplorer-costanalysis), [Cost Explorer permissions](https://guide.ncloud-docs.com/docs/en/costexplorer-subaccount), [Cost Explorer resource actions](https://guide.ncloud-docs.com/docs/costexplorer-resource), and [Cost Explorer overview and notifications](https://guide.ncloud-docs.com/docs/en/costexplorer-overview).
- Conclusion: the reviewed public docs still do not establish a supported unattended source of actual NCP daily cost amounts. Do not infer those amounts from daily usage, list prices, monthly amounts, or month-to-date changes. Keep this task pending for an official daily cost API/export contract or an official NCP support response identifying one.

- Official NCP docs describe `getContractUsageListByDaily` as daily contract usage, with usage quantities rather than a cost amount in its response model.
- Official NCP docs describe `getContractDemandCostList`, `getProductDemandCostList`, and `getDemandCostList` as monthly billing-cost queries. These expose cost/discount fields but do not establish daily actual-cost values.
- Public list-price APIs do not establish the exact effective price after negotiated pricing, discounts, credits, and billing adjustments. Joining those rates to usage would not be a verified substitute for billed costs.
- The existing normalized record carries `UsageStart` and `UsageEnd`; daily records fit the anomaly analysis interval.
- The user requires actual daily costs for recurring anomaly detection; estimated daily amounts are not acceptable.
- NCP's monthly cost endpoints must not be mapped to days. The daily usage endpoint's documented model contains usage quantities but no cost amount.
- The daily usage response includes contract product identifiers such as `priceNo` and `promiseNo`, but calculating from published prices would still produce estimates because NCP has free, flat, tiered, and package pricing as well as discounts and credits. Estimation is explicitly out of scope.
- NCP Cost Explorer's console Cost Analysis is updated each morning from the previous day's usage and allows Excel export, but its public guide describes monthly cost views and the reviewed public developer API references do not document a Cost Explorer data API. A console-only/manual export does not satisfy unattended periodic collection.
- Do not implement or register an NCP provider until an officially supported daily actual-cost feed is identified. If the user has a Cost Explorer integration endpoint or billing export not covered by public docs, request its official API reference or sanitized output sample.

## Verification plan

- Verify official daily actual-cost feed semantics and access support before implementation.
- Record a failing public behavior test before each code slice (RED), then a passing result (GREEN).
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/ncp ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `make kind-e2e` when Docker is available.
- `git diff --check`

## Research verification results (2026-10-02)

- Reviewed the official NCP Cost and Usage API overview, daily contract usage API/schema, monthly cost APIs, Cost Analysis, Cost Insight, Cost Explorer permissions, resource-action, and budget notification guides linked above.
- Confirmed that the daily usage schema has no billed amount and the documented cost APIs are monthly.
- Confirmed that the reviewed Cost Explorer documentation describes daily refresh of prior-day data, monthly views, console spreadsheet download, and native next-day budget notifications, but no public unattended daily-cost collection API/export contract.
- `git diff --check`: passed.
- Provider tests, repository tests, vet, formatting, manifests, build, and kind E2E: not run because no provider code was changed and the required daily actual-cost source is still unavailable.
- Status: pending upstream API/export evidence; no NCP adapter has been implemented or registered.
