# Architecture

## 1. Scope

This document defines the architecture of the SRE Multi-Cloud Cost Platform.

The long-term platform may cover:

```text
Cost
Inventory
Activity
Governance
Security
```

The current phase implements only:

```text
Cost collection
Normalization
Budget / anomaly analysis
Teams notification
```

Resource inventory, cloud audit/event ingestion, governance, and remediation are future phases.

---

## 2. System overview

```text
                         Git
                          │
                        ArgoCD
                          │
                          ▼
                 Kubernetes Cluster
                          │
              ┌───────────┴───────────┐
              │                       │
              ▼                       ▼
        CloudCost CRDs          ExternalSecret
              │                       │
              │                       ▼
              │                     Vault
              │
              ▼
       CloudCost Operator
              │
              │ reconcile
              ▼
     Collector Jobs / CronJobs
              │
              ▼
        Provider Adapters
              │
              ▼
          Normalizer
              │
              │ normalized cost records
              ▼
          ClickHouse
              │
              ▼
          Cost Analyzer
              │
              ▼
        Teams Notifier
```

---

## 3. Control-plane boundary

The Operator is the Kubernetes control plane for the platform.

It should:

- watch platform CRDs
- validate references and desired state
- create/update/delete managed Kubernetes resources
- create Collector Jobs/CronJobs
- expose useful conditions and status
- reconcile drift
- expose metrics

It should not:

- perform long-running CSP billing extraction in reconcile
- execute large data transformations
- store billing datasets in CR status
- call Teams as part of normal reconciliation
- become a monolithic application server

A deleted managed CronJob should be recreated when its owning `CloudAccount` still requires it.

Use owner references where appropriate.

---

## 4. Initial CRDs

Keep the Phase 1 API intentionally small.

Initial CRDs:

```text
CloudAccount
BudgetPolicy
NotificationPolicy
```

Do not create CRDs for analytical rows such as:

```text
CostRecord
DailyCost
BillingLineItem
ProviderUsageRecord
ResourceCost
```

Those belong in ClickHouse or object storage.

### 4.1 CloudAccount

Purpose:

- declare provider
- identify billing scope
- reference credentials
- declare collection schedule
- expose collection status

Example:

```yaml
apiVersion: finops.sre.local/v1alpha1
kind: CloudAccount
metadata:
  name: aws-prod
  namespace: cloud-cost
spec:
  provider: aws

  accountId: "123456789012"

  credentialRef:
    name: aws-finops-prod

  collection:
    enabled: true
    schedule: "0 0 * * *"

  metadata:
    team: sre
    environment: prod
    costCenter: platform
```

Suggested status:

```yaml
status:
  state: Ready

  lastCollectionTime: "2026-09-30T00:00:00Z"
  lastSuccessfulCollectionTime: "2026-09-30T00:00:00Z"

  conditions:
    - type: CredentialsReady
      status: "True"

    - type: CollectorReady
      status: "True"

    - type: DataFresh
      status: "True"
```

### 4.2 BudgetPolicy

BudgetPolicy should target provider-neutral organizational dimensions.

Example:

```yaml
apiVersion: finops.sre.local/v1alpha1
kind: BudgetPolicy
metadata:
  name: sre-monthly
spec:
  selector:
    team: sre

  amount:
    value: 10000000
    currency: KRW

  thresholds:
    - 80
    - 90
    - 100

  schedule: "0 * * * *"

  forecast:
    enabled: true
```

### 4.3 NotificationPolicy

Example:

```yaml
apiVersion: finops.sre.local/v1alpha1
kind: NotificationPolicy
metadata:
  name: central-teams
spec:
  type: teams

  credentialRef:
    name: teams-finops-webhook

  selector:
    team: sre

  events:
    - BudgetThreshold
```

`spec.events` is required and lists the event types the policy subscribes to. A policy is selected only when its event list includes the emitted event and at least one relevant `CloudAccount.spec.metadata` entry matches every key in `spec.selector`. An empty selector matches any relevant account. The Analyzer emits `BudgetThreshold`, `BudgetForecast`, and `CostAnomaly`; the other enumerated event types are reserved for later Phase 1 event producers. The one-shot Analyzer resolves the referenced same-namespace Secret at run time. A non-empty `BudgetPolicy.spec.schedule` creates an owned Analyzer CronJob; an empty value disables scheduled evaluation.

---

## 5. Provider abstraction

Provider-specific code must sit behind a stable interface.

Recommended shape:

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

Implementation layout:

```text
internal/provider/
├── aws/
├── azure/
├── gcp/
├── oci/
├── ibm/
├── ncp/
└── alibaba/
```

Shared code should not repeatedly branch on provider names.

Bad:

```go
if provider == "aws" {
    ...
} else if provider == "gcp" {
    ...
}
```

Prefer registration of provider implementations behind a common interface.

Adding a provider should mostly require:

```text
adapter
+ fixtures
+ normalization mapping
+ contract tests
```

If adding a provider requires significant edits to Operator, Analyzer, Notifier, or storage packages, revisit the abstraction.

---

## 6. Common cost schema

Use an internal provider-neutral cost model.

FOCUS may be used where available, but the platform must not assume every provider exposes native FOCUS data.

Minimum recommended fields:

```text
provider
billing_account_id
account_name

service
sku
resource_id
resource_name
region

team
project
environment
owner
cost_center

usage_start
usage_end

usage_quantity
usage_unit

list_cost
effective_cost
billed_cost

currency

credit
discount

tags

source_record_id
collected_at
```

This is the target schema, not a claim that every provider supplies every field today. Normalized records contain provider, billing account, source record ID, cost basis, amount, currency, and usage interval. AWS `NetUnblendedCost` and GCP Standard Billing Export (`cost + credits`) supply daily `net_cost`; Azure `ActualCost` (`PreTaxCost`) supplies daily `actual_pre_tax_cost`, which excludes taxes and does not include credits before invoice finalization. NCP supplies monthly invoice amounts including VAT under the isolated `ncp_monthly_invoice_cost` basis. Its monthly interval is represented by UTC month start and next-month start. Budget evaluation includes each monthly record once; daily anomaly evaluation skips full-month intervals. Budget evaluation rejects selected records with mixed cost bases instead of silently adding incomparable amounts. Unavailable service and resource dimensions are omitted rather than fabricated.

Preserve original currency and normalized reporting currency separately.

Recommended:

```text
original_cost
original_currency

exchange_rate
exchange_rate_date

normalized_cost_krw
```

Do not overwrite or lose the original billed amount.

---

## 7. Data persistence

Billing datasets must remain outside Kubernetes etcd.

Preferred flow:

```text
Provider
   │
   ▼
Raw billing data
   │
   │ normalized cost records
   ▼
Normalizer
   │
   ▼
ClickHouse
```

ClickHouse is the preferred analytical store for normalized cost records.

The current Collector writes normalized daily and monthly cost records to `cost_records`; each record's usage interval identifies its billing period. Daily live Collectors re-query the previous eight complete UTC days once per scheduled run so revised daily totals can replace earlier values and the Analyzer has a full seven-day baseline. NCP collects monthly invoice amounts through `getDemandCostList` and stores each row over its calendar-month interval. CloudAccount schedules remain user-controlled; the examples use one run per day to limit provider API calls. AWS Cost Explorer refreshes at least every 24 hours, but upstream billing data can arrive later, so the bounded daily window is not a finality guarantee. Cost Explorer API calls are charged per paginated request. The schema keeps provider amount text unchanged and uses `(provider, billing_account_id, source_record_id)` as the logical key. Source IDs remain stable for the same account and billing period. `ReplacingMergeTree(version)` performs replacement during background merges, so readers requiring one logical row must query with `FINAL` until query patterns are formalized. The ClickHouse Store reader accepts explicit provider/account pairs and a half-open UTC interval, applies `FINAL`, and returns normalized rows for the Analyzer. The Collector emits the original raw records as JSON Lines only after the ClickHouse batch succeeds. Object-storage archival is not implemented in this phase.

The exact object-storage backend may evolve.

### Idempotency

Billing data may be delayed or revised.

Re-fetching the same interval must not create uncontrolled duplicates.

Use a deterministic ingestion key or stable provider record identifier.

The system should explicitly distinguish:

```text
collector failed
```

from:

```text
collector succeeded but provider data is stale
```

These are different operational conditions.

---

## 8. Analyzer

Phase 1 analysis includes:

- daily cost summary
- month-to-date cost
- budget thresholds
- simple forecast
- cost increase detection
- collection failures
- data freshness failures

Cost anomaly rules should consider both relative and absolute changes.

Example:

```text
today > avg_7d * 1.5
AND
today - avg_7d > configured_absolute_threshold
```

The Analyzer consumes normalized cost data only.

The Analyzer provides storage-backed monthly budget, simple month-end forecast, and daily cost anomaly evaluation in `internal/analyzer`. Monthly tracking sums stored daily records and includes monthly-period records such as NCP invoice totals once, from the start of the current UTC month through the current time. When `BudgetPolicy.spec.forecast.enabled` is true, the forecast divides month-to-date daily spend by the current UTC day number and multiplies by the number of days in the current UTC month. It emits `BudgetForecast` only when the projection is strictly above the budget, and excludes NCP monthly invoice records because a monthly total is not a daily pace observation. This simple estimate is subject to provider data delays. Daily anomaly detection compares the previous complete UTC day with the preceding seven complete UTC days for each provider/account independently. A notification requires both a 1.5x increase and an absolute increase above `BudgetPolicy.spec.dailyAnomaly.absoluteIncreaseThreshold`, expressed in the policy currency. The daily detector currently applies to AWS, Azure, GCP, OCI, IBM Cloud, and Alibaba Cloud records. NCP monthly records are excluded from daily anomaly detection; NHN Cloud is outside the supported-provider scope. A successful collection timestamp must cover the evaluated day before missing rows can be treated as zero. `EvaluateAndNotifyBudgetPolicies` routes monthly threshold intents through matching `NotificationPolicy` objects. `RunBudgetPolicy` also evaluates forecasts and daily anomalies, resolves the referenced same-namespace Teams Secrets, and records successful delivery receipts. `cmd/analyzer` loads one namespaced BudgetPolicy, CloudAccounts, NotificationPolicies, and referenced webhook Secrets, then reads ClickHouse through the existing cost reader. BudgetPolicy status records successfully delivered threshold percentages and forecast months by UTC month and the last notified daily anomaly UTC date. `BudgetForecast` and `CostAnomaly` subscriptions, account selectors, and Teams Secret resolution govern corresponding delivery. The BudgetPolicy controller creates an Analyzer CronJob when a schedule is set and prevents overlapping runs; notification delivery is at-least-once if a status update fails after sending.

It must not contain CSP API logic.

---

## 9. Notification architecture

Teams is the Phase 1 human notification channel.

Provider-specific payloads must not leak into the Teams UX.

Normalize notification payloads.

Conceptual example:

```text
Severity: Warning
Type: CostAnomaly

Provider: AWS
Account: prod-ai
Team: ai-platform

Today: ₩2,180,320
7-day average: ₩814,210
Increase: +167.8%

Top contributor:
EC2

Budget:
₩20,000,000

Forecast:
₩27,840,000
```

Notifier interface:

```go
type Notifier interface {
    Send(ctx context.Context, notification Notification) error
}
```

Recommended implementations:

```text
TeamsNotifier
FakeNotifier
StdoutNotifier
```

Tests must never require a real Teams channel.

The first implementation lives in `internal/notifier`: a fake notifier supports local Analyzer tests, and `TeamsWebhookNotifier` sends Adaptive Cards through a Teams Workflows callback URL. It accepts the callback URL from trusted Secret configuration, requires HTTPS, disables redirects, and returns a stable error without including the URL or remote response body. Analyzer policy fanout accepts resolved Notifier instances through an injected resolver; the one-shot runtime resolves webhook Secrets only for selected policies, and the Operator manages a scheduled Analyzer CronJob per opted-in BudgetPolicy.

---

## 10. Repository structure

Prefer a monorepo initially.

```text
cloud-finops/
├── AGENTS.md
│
├── cmd/
│   ├── operator/
│   ├── collector/
│   ├── analyzer/
│   └── notifier/
│
├── api/
│   └── v1alpha1/
│
├── controllers/
│
├── internal/
│   ├── provider/
│   │   ├── aws/
│   │   ├── azure/
│   │   ├── gcp/
│   │   ├── oci/
│   │   ├── ibm/
│   │   ├── ncp/
│   │   └── alibaba/
│   ├── normalize/
│   ├── analyzer/
│   ├── notifier/
│   │   └── teams/
│   └── storage/
│       └── clickhouse/
│
├── config/
│   ├── crd/
│   ├── rbac/
│   ├── manager/
│   ├── monitoring/
│   └── samples/
│
├── deploy/
│   ├── helm/
│   └── argocd/
│
├── docs/
│   ├── architecture.md
│   ├── cluster-platform.md
│   ├── secrets.md
│   ├── harness.md
│   └── provider-contract.md
│
├── test/
│   ├── contract/
│   ├── integration/
│   └── e2e/
│
├── testdata/
│   ├── aws/
│   ├── azure/
│   ├── gcp/
│   ├── oci/
│   ├── ibm/
│   ├── ncp/
│   └── alibaba/
│
└── scripts/
```

Do not split every logical component into a separate repository or service prematurely.

---

## 11. Provider implementation order

Architecture must support all seven currently supported providers.

Recommended delivery order:

```text
P0
AWS
GCP
Azure

P1
NCP

P2
OCI
IBM Cloud
Alibaba Cloud
```

P0 exists to prove the provider contract and common schema.

Before adding P1/P2 providers, verify that implementation mostly consists of:

```text
new adapter
new fixtures
new mappings
new tests
```

---

## 12. Future expansion

Do not implement future resource tracking in Phase 1 unless explicitly requested.

However, avoid blocking it.

The provider domain may later expand conceptually toward:

```go
type Provider interface {
    ValidateCredentials(...)
    CollectCosts(...)
    CollectResources(...)
    CollectEvents(...)
}
```

Future data domains must remain separate:

```text
Cost
Inventory
Activity
Governance
Security
```

Do not overload `CloudAccount` status with inventory or resource lifecycle datasets.
