# Provider Contract

## 1. Purpose

This document defines the implementation contract for cloud billing providers.

Supported providers:

```text
AWS
Azure
GCP
OCI
IBM Cloud
NCP
Alibaba Cloud
```

The rest of the platform should not need to understand provider-specific billing APIs.

---

## 2. Provider boundary

Recommended interface:

```go
type Provider interface {
    ValidateCredentials(ctx context.Context) error

    CollectCosts(
        ctx context.Context,
        req CollectRequest,
    ) ([]RawCostRecord, error)

    Metadata(ctx context.Context) ProviderMetadata
}
```

The exact Go API may evolve, but the responsibility boundary must remain.

The shared implementation currently provides `internal/provider.Registry` for resolving registered factories and `internal/provider.ProviderError` for stable error classes. Registration happens during process setup; provider packages should register under the lowercase names used by `CloudAccount.spec.provider`. Keep provider-specific configuration inside the factory closure or adapter package rather than adding it to the shared registry API.

Provider implementations are responsible for:

- provider authentication
- provider billing API/export access
- pagination
- provider throttling semantics
- provider-specific raw response interpretation
- provider-specific source identifiers

The AWS adapter uses Cost Explorer `GetCostAndUsage` with `DAILY` granularity and `NetUnblendedCost`, filtered to one linked account. The GCP adapter reads the Standard usage cost export from BigQuery, sums `cost` and nested `credits`, and groups totals by UTC usage date and currency. The Azure adapter queries subscription-scoped Cost Management `ActualCost` at daily granularity and sums `PreTaxCost`, which is represented as `actual_pre_tax_cost` because Azure credits aren't included before invoice finalization. Azure supports subscription UUIDs only in this initial slice. Azure source record IDs include currency so same-day rows in different currencies remain distinct. The OCI adapter queries the tenancy-scoped Usage API with `COST` and `DAILY`, signs REST calls with an RSA API key, follows `opc-next-page`, and decodes JSON amounts without floating-point conversion. OCI rows use the distinct `oci_cost` basis because available documentation does not establish equivalence with another provider basis; its treatment of credits and tax remains an accepted gap. OCI source record IDs include currency. The IBM Cloud adapter reads the account FOCUS 1.2 export for each billing month overlapping the UTC collection window, streams CSV rows, sums `BilledCost` by daily charge period and billing currency, and uses `ibm_billed_cost` because IBM billed-cost semantics are not established as equivalent to another provider basis. It rejects overlapping charge periods that are not a single UTC day instead of allocating those amounts arbitrarily. The collector uses a UTC date interval with an inclusive start and exclusive end; each live run requests the previous eight complete UTC days so daily anomaly analysis can compare the latest complete date with seven baseline dates. CloudAccount schedules remain user-controlled, and examples recommend one run per day. Stable account-day source IDs let ClickHouse replace revised values on later runs. AWS refreshes Cost Explorer data at least once every 24 hours, but upstream data can arrive later; the seven-day lookback does not guarantee final billing values. With the primary billing view, AWS currently charges $0.01 per paginated API request. A once-daily schedule therefore costs about $0.01 per account per day at one page per run, compared with about $0.04 at a six-hour schedule; additional pages increase the charge ([refresh behavior](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-what-is.html), [API pricing](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/)).

For NCP, `CloudAccount.spec.accountId` is the member number for an ordinary account. The adapter calls `getDemandCostList` for months overlapping the collection window and groups `thisMonthAmountIncludingVat` by `demandMonth` and payment currency. This field is the current month's billed amount including VAT; the adapter does not use `totalDemandAmount`, which can include outstanding amounts from other months. The normalized record interval spans the calendar month and uses `ncp_monthly_invoice_cost`. Daily anomaly evaluation ignores full-month records. The public API reference does not promise when an in-progress month's amount is available, so Louder stores only the monthly amount actually returned. Store `NCP_ACCESS_KEY` and `NCP_SECRET_KEY` in the referenced Secret. Create a Sub Account with API access enabled and grant the `NCP_COST_EXPLORER_VIEWER` system policy. Do not enable NCP daily anomaly detection.

For GCP, `CloudAccount.spec.providerConfig` must identify the query project, export dataset, and standard table through `projectId`, `datasetId`, and `tableId`. The billing account ID is the `CloudAccount.spec.accountId`. Enable Standard usage cost export before collection and grant the workload service account BigQuery job creation on the query project and read access to the export dataset. Export rows arrive asynchronously; the seven-day recollection range does not guarantee final billing data. Query processing may incur charges based on the amount of data scanned.

For Azure, `CloudAccount.spec.accountId` is the subscription UUID. The adapter requests non-amortized `ActualCost` using `PreTaxCost`, which excludes tax and does not include credits before invoice finalization; it is not an invoice reconciliation. The workload service principal needs the Cost Management Reader role on the subscription and the Azure Resource Manager API access described in the Microsoft documentation.

For IBM Cloud, `CloudAccount.spec.accountId` is the IBM Cloud account ID. The adapter requests the FOCUS 1.2 monthly report using CSV mode, which returns all report rows, and streams it to daily billed totals. The IBM service principal needs Billing Administrator access or Account Owner access to the account. The adapter rejects non-daily charge periods until their allocation semantics can be confirmed.

For OCI, `CloudAccount.spec.accountId` is the tenancy OCID. The adapter requests daily tenancy totals using Usage API `queryType: COST`. The OCI principal needs `read usage-report` on the tenancy. The adapter keeps OCI cost records isolated under `oci_cost` until Oracle cost/credit/tax semantics have been validated against representative billing data.

For Alibaba Cloud, `CloudAccount.spec.accountId` is the Alibaba Cloud account UID. The adapter queries BSS OpenAPI `DescribeInstanceBill` with `Granularity=DAILY`, one `BillingDate` at a time, and follows `NextToken`. It sums `PretaxAmount` by UTC billing date and currency under the distinct `alibaba_pretax_cost` basis. Alibaba documents a 24-hour data delay, excludes unsettled current-month pay-as-you-go amounts, and notes that some attached-resource costs require Split Bill; these records are not final invoice reconciliation data.

Provider implementations are not responsible for:

- Teams formatting
- budget policy evaluation
- ClickHouse query policy
- Kubernetes reconciliation
- organization-wide business rules

---

## 3. Collection request

A provider-neutral collection request should include enough scope to make re-fetching deterministic.

Conceptual model:

```go
type CollectRequest struct {
    AccountID   string
    StartTime   time.Time
    EndTime     time.Time
    CollectionID string
    ProviderConfig map[string]string
}
```

`CollectRequest.ProviderConfig` carries non-secret provider settings such as GCP BigQuery project, dataset, and table identifiers. Credentials remain in the referenced Kubernetes Secret.

If required, prefer an opaque provider configuration block validated by the provider implementation rather than leaking provider fields through shared business logic.

---

## 4. Raw record

`RawCostRecord` should represent provider-originated cost data before common normalization.

It should preserve enough information to:

- trace source data
- retry normalization
- debug mappings
- detect duplicates

Avoid destroying provider-originated identifiers too early.

A raw record should normally carry:

```text
provider
source_record_id
billing_scope
service
sku
usage interval
amount
currency
credits
discounts
tags/labels
raw dimensions needed for normalization
```

`RawCostRecord.CostBasis` carries the provider adapter's normalized cost semantic. AWS `NetUnblendedCost` and GCP `cost + credits` map to `net_cost`; Azure `ActualCost` / `PreTaxCost` maps to `actual_pre_tax_cost`. Budget evaluation rejects a selected set of records containing more than one non-empty cost basis, so unlike values are never silently summed. Adapters must not populate fields with guessed values.

Existing ClickHouse rows written by older Collector versions keep their original `unblended_cost` basis. Before upgrading a populated installation, replay the affected billing period or start analysis at a clean period; the normal seven-day recollection window cannot rewrite older rows.

The first `internal/normalize` output keeps provider, billing account, source record ID, cost basis, amount, currency, and usage interval. Amounts remain decimal strings in the source currency. Service/resource dimensions and currency conversion are outside this initial slice.

---

## 5. Error taxonomy

Provider implementations should map upstream failures into stable error classes.

Recommended classes:

```text
AuthenticationFailed
PermissionDenied
RateLimited
Timeout
ProviderUnavailable
InvalidResponse
InvalidCredentialShape
UnsupportedBillingScope
```

Do not force callers to parse provider-specific error strings.

Where useful, errors may wrap upstream details for logs while still exposing a stable class.

Never include secrets in errors.

The shared Go error classes are `ErrorAuthenticationFailed`, `ErrorPermissionDenied`, `ErrorRateLimited`, `ErrorTimeout`, `ErrorProviderUnavailable`, `ErrorInvalidResponse`, `ErrorInvalidCredentialShape`, and `ErrorUnsupportedBillingScope`. Wrap upstream errors in `ProviderError` and use `IsErrorClass` to inspect the stable class without parsing message text.

---

## 6. Pagination

All provider implementations must handle pagination internally.

Shared callers should not need provider-specific pagination tokens.

The contract must guarantee:

```text
CollectCosts(request)
```

returns the complete result set for the requested logical page/window or fails explicitly.

Contract tests must include pagination.

---

## 7. Rate limiting

Providers must detect provider-native rate-limit responses.

Expected behavior:

- return stable `RateLimited` classification
- expose retry metadata when safely available
- avoid tight retry loops
- respect provider retry-after semantics where possible
- allow the job/controller layer to apply retry/backoff policy

Do not hide repeated 429/throttle failures as empty cost data.

---

## 8. Timeouts

Provider calls must use context-aware timeouts.

A hung CSP API must not block a collector indefinitely.

Timeout behavior must be testable.

Do not use unbounded HTTP calls.

---

## 9. Currency

Providers may bill in:

```text
KRW
USD
CNY
or other currencies
```

Provider adapters must preserve the original billed currency.

They must not convert everything to KRW internally.

Currency conversion belongs to normalization/reporting logic.

Required preservation:

```text
original amount
original currency
billing date/period
```

---

## 10. Credits and discounts

Credits and discounts must not be silently discarded.

Where a provider exposes:

```text
list cost
discount
credit
net/effective cost
```

preserve these dimensions when possible.

The common schema should be able to explain the relationship between:

```text
list_cost
credit
discount
effective_cost
billed_cost
```

Do not fabricate a field when the provider does not expose it.

Use null/unknown semantics where needed.

---

## 11. Duplicate handling

Provider APIs may:

- return overlapping windows
- revise historical data
- repeat records
- expose aggregate records without stable row IDs

The provider should surface the strongest stable source identifier available.

When no stable provider ID exists, normalization/storage must derive a deterministic ingestion key.

Re-collecting a time window must not blindly duplicate rows.

---

## 12. Data freshness

A successful API request does not guarantee fresh billing data.

Provider metadata should make freshness measurable when possible.

The platform should support status such as:

```text
CollectionReady=True
DataFresh=False
```

Do not infer freshness solely from collector execution time.

---

## 13. Provider metadata

`Metadata()` should expose stable implementation capabilities.

Conceptual example:

```go
type ProviderMetadata struct {
    Name                 string
    SupportsFOCUS        bool
    SupportsResourceID   bool
    SupportsCredits      bool
    SupportsDiscounts    bool
    TypicalDataDelay     time.Duration
}
```

Do not hard-code these capabilities throughout unrelated packages.

---

## 14. Implementation layout

Recommended:

```text
internal/provider/
├── provider.go
├── registry.go
├── aws/
│   ├── provider.go
│   ├── client.go
│   └── mapper.go
├── azure/
├── gcp/
├── oci/
├── ibm/
├── ncp/
└── alibaba/
```

Keep provider HTTP/SDK details inside the provider package.

---

## 15. Registration

Prefer centralized provider registration.

Conceptual example:

```go
registry.Register("aws", aws.New)
registry.Register("gcp", gcp.New)
registry.Register("ncp", ncp.New)
```

Shared code should resolve a provider through the registry.

Do not spread provider-name switch statements across the codebase.

---

## 16. Contract tests

Every provider must pass the same test suite.

Required cases:

```text
normal collection
empty collection
pagination
authentication failure
permission denied
rate limit
timeout
malformed response
credit
discount
currency
duplicate source record
partial response
```

See:

```text
docs/harness.md
```

A provider implementation is incomplete without fixtures and contract tests.

---

## 17. Normalization boundary

Provider code may translate SDK-specific structures into `RawCostRecord`.

The common Normalizer owns conversion from provider-neutral raw records into the platform cost schema.

Do not mix Teams/business policy logic into provider mappers.

Recommended flow:

```text
Provider SDK/API
     ↓
Provider Adapter
     ↓
RawCostRecord
     ↓
Normalizer
     ↓
Common CostRecord
```

---

## 18. New-provider acceptance checklist

Before a new CSP is considered implemented:

```text
[ ] Provider registered behind common interface
[ ] Credential validation implemented
[ ] Collection implemented
[ ] Pagination handled
[ ] Timeout behavior implemented
[ ] Rate limit mapped
[ ] Authentication/permission errors mapped
[ ] Original currency preserved
[ ] Credits handled where available
[ ] Discounts handled where available
[ ] Stable source ID or deterministic key strategy defined
[ ] Fixtures added
[ ] Contract suite passes
[ ] Golden normalization files added
[ ] Integration test passes
[ ] No provider-specific branching added to Analyzer
[ ] No provider-specific branching added to Notifier
[ ] No provider-specific billing logic added to Operator
```

---

## 19. Future expansion

Future resource inventory and audit/event collection should use separate capability contracts.

Do not force unrelated future operations into `CollectCosts`.

Possible future interfaces:

```go
type CostProvider interface {
    CollectCosts(...)
}

type InventoryProvider interface {
    CollectResources(...)
}

type ActivityProvider interface {
    CollectEvents(...)
}
```

Composition is preferable to one oversized interface if capabilities diverge significantly across CSPs.
