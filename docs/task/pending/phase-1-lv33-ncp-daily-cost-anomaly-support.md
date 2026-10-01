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

- Official NCP docs describe `getContractUsageListByDaily` as daily contract usage, with usage quantities rather than a cost amount in its response model.
- Official NCP docs describe `getContractDemandCostList`, `getProductDemandCostList`, and `getDemandCostList` as monthly billing-cost queries. These expose cost/discount fields but do not establish daily actual-cost values.
- Public list-price APIs do not establish the exact effective price after negotiated pricing, discounts, credits, and billing adjustments. Joining those rates to usage would not be a verified substitute for billed costs.
- The existing normalized record carries `UsageStart` and `UsageEnd`; daily records fit the anomaly analysis interval.
- Open product decision: may the adapter calculate and label daily amounts as estimates using NCP daily usage and published prices, or must it wait for exact daily billed amounts?
- NCP's monthly cost endpoints must not be mapped to days. The daily usage endpoint's documented model contains usage quantities but no cost amount.
- User decision: use actual daily costs for recurring anomaly detection; estimated daily costs are not acceptable.
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
