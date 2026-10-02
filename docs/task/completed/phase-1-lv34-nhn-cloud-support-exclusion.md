# Phase 1 Level 34: NHN Cloud Support Exclusion

## Goal

Record the decision to exclude NHN Cloud from Louder's supported-provider scope. The investigated billing APIs did not establish a suitable unattended source for the confirmed ordinary commercial account.

## Scope

- Remove NHN Cloud from the active supported-provider list and current architecture, test, and credential examples.
- Preserve the prior API investigation as historical context.
- Do not implement an NHN adapter or accept NHN as a current CloudAccount provider.

## Non-goals

- Re-investigating NHN APIs or requesting account-specific access confirmation.
- Implementing monthly or daily NHN billing collection.
- Changing provider code or CRD enums.

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

## Outcome

- User decision (2026-10-02): exclude NHN Cloud from the supported providers.
- No adapter was implemented. The commercial partner daily-cost endpoint is restricted to partner-authorized users, and the investigation found no documented general-user billing API/export.
- This task is closed as a scope decision, not as a completed provider adapter.

## Completion criteria

- NHN Cloud is absent from current supported-provider lists, architecture/package layouts, and credential/test examples.
- This plan is in `docs/task/completed/` and clearly states that no adapter was implemented.
- Historical completed plans may retain references to the earlier pending investigation.

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
- At the time of the initial investigation, the adapter remained pending because an accessible actual daily-cost source had not been established.

### Final decision (2026-10-02)

- The user chose to exclude NHN Cloud from the supported-provider scope. Further API investigation is outside the current plan.

## Verification

- Confirm no supported-provider list or current provider/credential layout includes NHN; explicit exclusion notes and historical task records may retain the name.
- `git diff --check`
