# Test Harness

## 1. Purpose

The test harness is a first-class part of the Multi-Cloud Cost Platform.

The platform currently supports seven CSP integrations, which create significant maintenance risk if correctness depends on manual tests against live accounts. The initial provider rollout covered AWS, GCP, and Azure; every additional supported provider uses the same harness.

The harness must allow developers and CI to verify:

```text
provider behavior
normalization behavior
budget/anomaly behavior
Kubernetes control-plane behavior
Vault/ESO secret flow
failure behavior
```

without requiring live cloud access for normal development.

---

## 2. Harness layers

Required layers:

```text
1. Provider Fixture Harness
2. Provider Contract Tests
3. Normalizer Golden Tests
4. Analyzer Tests
5. Integration Tests
6. Kubernetes E2E
7. Failure Injection
```

These layers serve different purposes and must not be collapsed into one large E2E suite.

---

## 3. Provider fixture harness

Each implemented CSP must have sanitized representative API/export responses. The initial fixture set covers AWS, GCP, and Azure; add fixtures for each subsequent CSP as it is implemented.

Suggested layout:

```text
testdata/
├── aws/
│   ├── normal.json
│   ├── empty.json
│   ├── pagination-page-1.json
│   ├── pagination-page-2.json
│   ├── credit.json
│   ├── discount.json
│   ├── malformed.json
│   ├── auth-error.json
│   └── rate-limit.json
├── azure/
├── gcp/
├── oci/
├── ibm/
├── ncp/
└── alibaba/
```

Never store:

- real credentials
- real tokens
- unnecessary customer data
- sensitive account metadata
- unredacted production payloads

The fixture transport should emulate provider behavior where practical.

The AWS live adapter queries Cost Explorer `GetCostAndUsage` for daily account totals using `NetUnblendedCost`. The GCP live adapter queries the Standard BigQuery Billing Export and aggregates `cost + credits` by UTC day and currency. The Azure live adapter queries subscription-scope ActualCost daily totals using `PreTaxCost`, recorded as `actual_pre_tax_cost`; its source IDs include currency to preserve distinct same-day currency rows. The OCI live adapter queries tenancy-level `COST` at daily granularity, preserves JSON decimal precision, and follows `opc-next-page`; it uses the isolated `oci_cost` basis because equivalence with other providers and credit/tax treatment have not been established. The IBM Cloud live adapter streams the account FOCUS 1.2 CSV export for each overlapping billing month; IBM documents CSV mode as returning the full report, so there is no provider cursor to follow. It sums daily `BilledCost` by currency and uses an isolated `ibm_billed_cost` basis. Alibaba Cloud queries BSS `DescribeInstanceBill` daily with `PretaxAmount`, follows `NextToken`, and uses the isolated `alibaba_pretax_cost` basis. Alibaba documents a 24-hour billing-data delay, unsettled current-month PAYG exclusions, and attached-resource detail gaps in this API; Split Bill is not currently collected. NCP queries monthly `getDemandCostList`, groups `thisMonthAmountIncludingVat` by month and currency, and stores the billed amount including VAT under `ncp_monthly_invoice_cost`; the API does not promise when a current-month amount is available. NCP records are included in monthly budgets but not daily anomaly detection. NCP daily usage APIs expose quantities without daily actual cost values, so NCP remains outside daily anomaly detection. NHN Cloud is excluded from the supported-provider scope. The IBM adapter rejects non-daily charge periods because public docs do not define how to allocate them. Budget evaluation rejects account selections whose collected records contain mixed cost bases. Unit tests use synthetic API results; kind E2E stays offline and does not require CSP credentials. Daily live runs request the previous eight complete UTC days so the Analyzer can compare the target day with seven baseline days. NCP's month-period source IDs remain stable so ClickHouse can replace revised monthly values. CloudAccount schedules remain user-controlled; examples recommend one run per day. AWS refreshes data at least every 24 hours, but some upstream data can arrive later, so the lookback is bounded and does not guarantee final values. BigQuery export is asynchronous and queries can incur BigQuery charges. Azure Cost Management data may be delayed; Azure Cost Management doesn't include credits before invoice finalization. See [AWS refresh behavior](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-what-is.html), [AWS API pricing](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/), [Google export setup and cost](https://cloud.google.com/billing/docs/how-to/export-data-bigquery-setup), [Azure Cost Management Query API](https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage?view=rest-cost-management-2023-03-01), [NCP monthly billing API](https://api.ncloud-docs.com/docs/platform-costandusage-getdemandcostlist), and [Alibaba `DescribeInstanceBill`](https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-describeinstancebill).

A developer should be able to run something conceptually like:

```bash
cost-collector \
  --provider=ncp \
  --account-id=2760000 \
  --fixture=embedded:ncp
```

or equivalent test-only wiring.

---

## 4. Provider contract tests

Every implemented provider must pass the same behavioral contract. Each provider's suite may add granularity-specific cases; NCP verifies monthly aggregation while the daily providers verify day-level aggregation.

Required cases:

```text
successful collection
empty result
pagination
authentication failure
permission denied
rate limiting
timeout
malformed response
credit
discount
currency handling
duplicate source records
partial response
```

Reusable Go helper:

```go
contracttest.Run(t, contracttest.Suite{
    Credentials: []contracttest.CredentialScenario{
        {Name: "valid credentials", Provider: newAWSFixtureProvider()},
    },
    Collections: []contracttest.CollectionScenario{
        {
            Name: "complete logical result",
            Provider: newAWSFixtureProvider(),
            Request: request,
            WantRecords: expectedRecords,
        },
    },
})
```

The shared helper runs every credential and collection scenario through the public `Provider` interface, compares exact returned records, and checks stable error classes. Both scenario types can provide a canceled or deadline context to exercise timeout behavior. Adapter packages provide their fixture or fake-upstream scenarios, including the complete expected result for a paginated logical window.

A provider is not complete because its happy path works. Adapter packages remain responsible for cases covering pagination, authentication, permissions, rate limits, timeouts, malformed responses, discounts, credits, currencies, duplicate records, and partial responses. The helper checks expected results and error classes but does not invent provider-specific fixture behavior.

---

## 5. Normalizer golden tests

Normalization should be deterministic.

Pattern:

```text
provider input fixture
       ↓
provider adapter
       ↓
normalizer
       ↓
expected golden output
```

Example layout:

```text
testdata/ncp/
├── raw-normal.json
└── expected-normalized.json
```

Tests compare normalized output against the expected golden file.

The golden cases normalize an AWS account-day `NetUnblendedCost` record and an NCP account-month invoice record. They verify exact amount-string, currency, source ID, and UTC interval preservation. They do not imply that service/resource mapping or currency conversion exists.

Golden tests are especially useful for:

- provider schema changes
- service-name mappings
- currency mappings
- credit/discount behavior
- tags/labels
- billing account identifiers

A normalization change must update tests intentionally.

---

## 6. Analyzer tests

Analyzer tests must operate entirely on normalized data. The monthly budget evaluator tests selector matching, UTC month filtering, exact amount aggregation, threshold ordering, and safe errors for invalid policy input or currency mismatch. Forecast tests project month-to-date daily spend at a constant calendar-day pace, check month length and budget boundaries, and exclude NCP monthly invoice totals. Daily anomaly tests verify the seven-day baseline, relative and absolute thresholds, account isolation, currency and cost-basis errors, stale-source rejection, and the eight-day storage window. The storage-backed services are tested with a fake `CostReader` for account scoping, UTC month bounds, no-match behavior, and read failures. `EvaluateAndNotifyBudget` uses `FakeNotifier` to verify notification payloads, no-send behavior below a threshold, and stopping after a delivery error. `SelectNotificationPolicies` tests event subscription, Teams type, CloudAccount metadata selectors, and deterministic policy ordering. `RunBudgetPolicy` additionally tests month-scoped threshold and forecast receipts, `BudgetForecast` and `CostAnomaly` payload routing, duplicate suppression, and failure-before-recording behavior.

Do not call live provider APIs.

Required scenarios:

```text
normal daily cost
relative-only increase
absolute-only increase
true anomaly
80% budget
90% budget
100% budget
forecast exceed
no forecast exceed
stale source data
collector failed
```

Example anomaly expectation:

```text
today > avg_7d * 1.5
AND
today - avg_7d > configured_absolute_threshold
```

Tests should verify notification intent, not Teams delivery. Forecasts use current UTC month-to-date daily spend divided by the current UTC day number and multiplied by the number of days in that month. This simple pace estimate excludes NCP monthly invoice records and may be affected by provider data delays. A `BudgetForecast` notification is sent only when projected spend is strictly above the budget and is recorded once per UTC month after successful delivery. The kind storage integration seeds synthetic daily rows into ClickHouse, invokes the full `RunBudgetPolicy` path with Kubernetes resources from a fake client, and verifies notification status receipts and duplicate suppression for daily anomalies and forecasts.

---

## 7. Notifier coverage

Notifier is abstracted so Analyzer tests can verify notification intent without sending real Teams messages. `internal/notifier` provides a `FakeNotifier` and tests the Teams Workflows HTTP request through an in-process transport; tests do not call a real tenant. The budget policy dispatch service tests NotificationPolicy selection, injected destination resolution, and fanout. `RunBudgetPolicy` tests namespace-scoped Kubernetes reads, referenced Secret handling, month rollover, and threshold deduplication with a fake client and notifier factory.

Required implementations:

```text
TeamsNotifier
FakeNotifier
StdoutNotifier
```

The default test path uses `FakeNotifier`.

Example conceptual payload:

```json
{
  "severity": "warning",
  "type": "BudgetThreshold",
  "provider": "aws",
  "currentCost": 8420000,
  "budget": 10000000
}
```

Unit and analyzer tests must not send real Teams messages. They use `FakeNotifier` to assert notification intent and payload.

A Teams integration test may target a disposable fake HTTP endpoint or dedicated test flow.

---

## 8. Integration harness

Integration tests verify multiple application layers together without requiring a full Kubernetes cluster.

Suggested targets:

```text
fixture provider
   ↓
adapter
   ↓
normalizer
   ↓
test ClickHouse
   ↓
analyzer
   ↓
FakeNotifier
```

Recommended assertions:

- correct record count
- idempotent re-ingestion
- correct aggregation
- expected policy result
- expected notification payload
- safe failure on malformed records

---

## 9. Kubernetes E2E

Use a disposable Kubernetes cluster such as `kind`.

The local `make kind-e2e` smoke test deploys the Operator into a disposable kind cluster and verifies CloudAccount reconciliation against the real Kubernetes API server. It also creates a fixture-backed Collector CronJob, launches a Job from it, and checks the synthetic JSON Lines output. The fixture runner does not call a CSP, authenticate with the mounted credentials, normalize data, or write to ClickHouse; a successful fixture Job is only proof that the Kubernetes execution path works. `make kind-e2e-secrets` separately starts a disposable Vault dev server and External Secrets Operator, then verifies Vault -> ESO -> Kubernetes Secret -> CloudAccount credential readiness, missing-path failure, Vault-unavailable failure, and recovery. It uses runtime-generated synthetic values and requires Docker, kind, kubectl, Helm, and OpenSSL; it never calls a CSP. Restarting the dev server clears its in-memory data, so the recovery check reseeds synthetic data and issues a fresh limited ESO token. The full Kubernetes E2E suite must still cover the production secret path; the local test is not a replacement for that coverage.

Target flow:

```text
create kind cluster
       │
       ▼
install CRDs
       │
       ▼
install Operator
       │
       ▼
install test Vault
       │
       ▼
install ESO
       │
       ▼
seed Vault
       │
       ▼
create CloudAccount
       │
       ▼
create ExternalSecret and materialize credential Secret
       │
       ▼
reconcile CredentialsReady from the Secret event
       │
       ▼
Operator reconcile
       │
       ▼
Collector CronJob / Job
       │
       ▼
embedded fixture Collector
       │
       ▼
assert JSON Lines output
```

The Collector has daily adapters for AWS, GCP, Azure, OCI, IBM Cloud, and Alibaba Cloud, plus a monthly invoice adapter for NCP. Unit and provider contract tests use synthetic API responses; the local kind flow stays offline and exercises embedded fixtures without CSP credentials. `make kind-e2e-storage` additionally starts an ephemeral ClickHouse instance, applies the checked-in schema, runs the AWS fixture twice and the NCP monthly fixture through Operator-created Collector Jobs, and checks AWS replay deduplication. It verifies the NCP invoice basis, amount, currency, and month interval in Collector output, reads the stored NCP fixture through the native ClickHouse reader, and evaluates it against a monthly budget with `FakeNotifier`. A ClickHouse integration test also reads a synthetic AWS target day and seven-day baseline, then verifies the `CostAnomaly` payload through `FakeNotifier`. This verifies the cost-to-notification path without CSP access or a real Teams tenant.

The kind E2E scripts default to cluster name `louder-e2e`. Set `KIND_CLUSTER_NAME` to run against a separate disposable cluster, for example `KIND_CLUSTER_NAME=louder-e2e-local make kind-e2e`. A cluster that already has the selected name is never replaced.

The local targets cover distinct slices:

| Target | Verified behavior |
|---|---|
| `make kind-e2e` | CloudAccount missing-Secret status, CronJob creation, a completed fixture Collector Job, and `CollectionReady=True` status reporting |
| `make kind-e2e-secrets` | Vault -> ESO -> Secret -> CloudAccount readiness and Vault failure/recovery; its CloudAccount keeps collection disabled |
| `make kind-e2e-storage` | AWS replay deduplication, NCP monthly collection/read/budget alert, stored daily anomaly and budget forecast evaluation with notification receipts, and the BudgetPolicy-owned Analyzer CronJob with its scoped ServiceAccount |

These targets do not verify a production billing API or a single combined Vault/ESO/Collector/ClickHouse flow. The storage E2E uses runtime-generated synthetic ClickHouse credentials and an ephemeral database; it does not exercise production Vault provisioning.

The E2E suite must exercise the actual production-style secret path:

```text
Vault -> ESO -> Kubernetes Secret -> workload
```

Do not replace this path with a manually created Secret when the test is meant to verify secret integration.

The CloudAccount is created before its ESO-managed credential Secret. The Operator must initially report `SecretNotFound` and then reconcile it to `SecretFound` when the Secret event arrives; creating the CloudAccount after the Secret would not verify this lifecycle transition.

---

## 10. Failure injection

The harness should make failures easy to reproduce.

Required scenarios include:

```text
Vault unavailable
ExternalSecret not Ready
Kubernetes Secret missing
CSP API 401
CSP API 403
CSP API 429
CSP timeout
provider malformed payload
ClickHouse unavailable
duplicate billing window
Teams endpoint 5xx
stale provider data
```

Desired developer UX:

```bash
make test-scenario SCENARIO=aws-rate-limit
make test-scenario SCENARIO=vault-secret-missing
make test-scenario SCENARIO=clickhouse-down
make test-scenario SCENARIO=teams-500
```

Failure behavior should be observable through:

```text
CR status
logs
metrics
test assertions
```

---

## 11. Expected status behavior

Examples:

### Missing Secret

```yaml
status:
  conditions:
    - type: CredentialsReady
      status: "False"
      reason: SecretNotFound

    - type: CollectorReady
      status: "False"
      reason: SecretNotFound
```

`CollectorReady` describes whether the desired Collector CronJob is available. It does not indicate that a Job is running or that billing data is fresh.

### Collection enabled

```yaml
status:
  conditions:
    - type: CredentialsReady
      status: "True"
      reason: SecretFound

    - type: CollectorReady
      status: "True"
      reason: CronJobReady
```

### Collection disabled

```yaml
status:
  conditions:
    - type: CollectorReady
      status: "False"
      reason: CollectionDisabled
```

### Collector CronJob reconciliation failure

```yaml
status:
  conditions:
    - type: CollectorReady
      status: "False"
      reason: CronJobReconcileFailed
      message: Unable to reconcile the managed Collector CronJob.
```

The condition message is deliberately stable and does not expose Kubernetes API errors or credential data. The underlying error is available in Operator logs.

### CSP rate limit

```yaml
status:
  conditions:
    - type: CollectionReady
      status: "False"
      reason: ProviderRateLimited
```

### Collector Job failure

Kubernetes Job outcomes use a stable condition and do not copy pod termination details into CloudAccount status:

```yaml
status:
  lastCollectionTime: "2026-09-30T00:00:00Z"
  lastSuccessfulCollectionTime: "2026-09-29T00:00:00Z"
  conditions:
    - type: CollectionReady
      status: "False"
      reason: CollectorJobFailed
```

### Stale billing data

```yaml
status:
  conditions:
    - type: CollectionReady
      status: "True"

    - type: DataFresh
      status: "False"
      reason: ProviderDataStale
```

Collection success and data freshness must not be conflated.

---

## 12. CI quality gates

Recommended CI sequence:

```text
1. format / lint
2. unit tests
3. provider contract tests
4. normalizer golden tests
5. analyzer tests
6. build binaries
7. build container images
8. create kind cluster
9. deploy test Vault
10. deploy ESO
11. deploy CRDs + Operator
12. run E2E
13. run selected failure scenarios
14. publish images
15. update GitOps artifact only when explicitly intended
```

A provider change must not bypass contract tests.

A normalization change must not bypass golden tests.

A CRD change must consider API compatibility and future versioning.

---

## 13. Local developer commands

Recommended targets:

```text
make test
make test-contract
make test-golden
make test-analyzer
make test-integration
make e2e
make test-scenario SCENARIO=<name>
```

Provider-specific helpers are acceptable:

```text
make test-provider PROVIDER=aws
make test-provider PROVIDER=ncp
```

Avoid requiring developers to remember long command sequences.

---

## 14. Definition of Done for Phase 1 harness

```text
[ ] Provider fixture framework exists
[ ] Common provider contract suite exists
[ ] AWS contract tests pass
[ ] GCP contract tests pass
[ ] Azure contract tests pass
[ ] Golden normalization tests exist
[ ] Analyzer tests exist
[ ] FakeNotifier exists
[ ] ClickHouse integration test exists
[ ] kind E2E exists
[ ] Vault test instance is used in E2E
[ ] ESO is used in E2E
[ ] ExternalSecret -> K8s Secret flow is asserted
[ ] Operator reconciliation is asserted
[x] Collector CronJob creation and fixture Job execution are asserted by `make kind-e2e`
[ ] idempotent re-ingestion is asserted
[ ] at least one provider failure is injected
[ ] ClickHouse failure is injected
[ ] Teams failure is injected
[ ] stale-data condition is tested
```

Daily provider criteria include AWS, GCP, Azure, OCI, IBM Cloud, and Alibaba Cloud; NCP has a monthly invoice contract and remains excluded from daily anomaly detection. Every implemented provider must pass its contract tests before it is considered complete.

---

## 15. Harness principle

The most important rule:

> A new CSP should be testable before it is trusted in production.

The desired implementation pattern is:

```text
new provider
   ↓
fixtures
   ↓
contract suite
   ↓
golden normalization
   ↓
integration
   ↓
E2E
```

If a provider can only be tested against a live production account, the harness is incomplete.
