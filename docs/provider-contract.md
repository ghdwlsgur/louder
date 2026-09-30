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
NHN Cloud
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

The initial AWS adapter uses Cost Explorer `GetCostAndUsage` with `DAILY` granularity and `UnblendedCost`, filtered to one linked account. It maps one account total per day into the existing raw record shape and does not provide service or resource breakdowns. The collector uses a UTC date interval with an inclusive start and exclusive end; the default live run requests the previous complete UTC day.

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
}
```

Additional provider-specific options should be minimized.

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

`RawCostRecord.CostBasis` carries the provider adapter's normalized cost semantic. The initial AWS adapter maps Cost Explorer `UnblendedCost` to `unblended_cost`. Adapters must not populate fields with guessed values.

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
├── nhn/
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
