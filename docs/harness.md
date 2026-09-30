# Test Harness

## 1. Purpose

The test harness is a first-class part of the Multi-Cloud Cost Platform.

The platform targets eight CSP integrations, which create significant maintenance risk if correctness depends on manual tests against live accounts. The initial provider rollout covers AWS, GCP, and Azure; the remaining providers should use the same harness as they are added.

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
├── nhn/
└── alibaba/
```

Never store:

- real credentials
- real tokens
- unnecessary customer data
- sensitive account metadata
- unredacted production payloads

The fixture transport should emulate provider behavior where practical.

The AWS live adapter currently queries Cost Explorer `GetCostAndUsage` for daily account totals using `UnblendedCost`. Unit tests use an SDK client double and synthetic responses; kind E2E stays offline and does not require AWS credentials. The live collector defaults to the previous complete UTC day. Cost Explorer data can be refreshed later, so this first pass does not backfill revised days.

A developer should be able to run something conceptually like:

```bash
cost-collector \
  --provider=ncp \
  --fixture=testdata/ncp/normal.json
```

or equivalent test-only wiring.

---

## 4. Provider contract tests

Every implemented provider must pass the same behavioral contract. AWS, GCP, and Azure are the initial providers; subsequent CSPs must pass the same contract before they are considered complete.

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

The initial golden case normalizes an AWS account-day `UnblendedCost` record and verifies exact amount-string, currency, source ID, and UTC interval preservation. It does not imply that service/resource mapping or currency conversion exists.

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

Analyzer tests must operate entirely on normalized data. They verify notification intent through `FakeNotifier`; notifier behavior is covered within analyzer and integration tests rather than as a separate harness layer.

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

Tests should verify notification intent, not Teams delivery.

---

## 7. Notifier coverage

Notifier must be abstracted so analyzer and integration tests can verify notification intent without sending real Teams messages. Notifier coverage is part of those test layers, not a separate harness layer.

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

The Collector has an AWS Cost Explorer adapter for daily account-level `UnblendedCost` totals. Its unit and provider contract tests use a synthetic SDK client; the local kind flow stays offline and runs embedded fixtures for AWS, GCP, and Azure. The current kind flow proves scheduling, credential Secret reference wiring, image startup, and output encoding only. Normalization, ClickHouse ingestion, Analyzer, and Notifier are not yet exercised by this local flow.

The kind E2E scripts default to cluster name `louder-e2e`. Set `KIND_CLUSTER_NAME` to run against a separate disposable cluster, for example `KIND_CLUSTER_NAME=louder-e2e-local make kind-e2e`. A cluster that already has the selected name is never replaced.

The local targets cover distinct slices:

| Target | Verified behavior |
|---|---|
| `make kind-e2e` | CloudAccount missing-Secret status, CronJob creation, a completed fixture Collector Job, and `CollectionReady=True` status reporting |
| `make kind-e2e-secrets` | Vault -> ESO -> Secret -> CloudAccount readiness and Vault failure/recovery; its CloudAccount keeps collection disabled |

Neither target yet verifies a production billing API or a single combined Vault/ESO/Collector/ClickHouse flow.

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

The initial provider completion criteria cover AWS, GCP, and Azure. As additional CSPs are implemented, each must pass the same provider contract before it is considered complete.

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
