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
              ├── Raw data -> Object Storage
              │
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
    schedule: "0 */6 * * *"

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

  events:
    - DailySummary
    - BudgetWarning
    - BudgetExceeded
    - CostAnomaly
    - CollectionFailed
```

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
├── nhn/
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

This is the target schema, not a claim that every provider supplies every field today. The first normalized slice is account-level daily cost and contains only provider, billing account, source record ID, cost basis, amount, currency, and usage interval. AWS currently supplies `unblended_cost`; unavailable service, resource, usage, credit, and discount fields are omitted rather than fabricated.

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
   ├── Object Storage
   │
   ▼
Normalizer
   │
   ▼
ClickHouse
```

ClickHouse is the preferred analytical store for normalized cost records.

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
│   │   ├── nhn/
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
│   ├── nhn/
│   └── alibaba/
│
└── scripts/
```

Do not split every logical component into a separate repository or service prematurely.

---

## 11. Provider implementation order

Architecture must support all eight providers.

Recommended delivery order:

```text
P0
AWS
GCP
Azure

P1
NCP
NHN Cloud

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
