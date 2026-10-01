# Secrets and Credential Management

## 1. Principle

Vault is the source of truth for production secrets.

The platform follows:

```text
Vault
  ↓
External Secrets Operator
  ↓
Kubernetes Secret
  ↓
Collector / Notifier Pod
```

The Operator does not own Vault lifecycle.

The Operator does not need direct Vault credentials.

---

## 2. Cluster integration

Existing ClusterSecretStore:

```text
vault-backend
```

Production workloads should use `ExternalSecret`.

Example:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: aws-finops-prod
  namespace: cloud-cost
spec:
  refreshInterval: 1h

  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore

  target:
    name: aws-finops-prod

  data:
    - secretKey: AWS_ACCESS_KEY_ID
      remoteRef:
        key: kv/finops/aws/prod
        property: access_key_id

    - secretKey: AWS_SECRET_ACCESS_KEY
      remoteRef:
        key: kv/finops/aws/prod
        property: secret_access_key

    # Add AWS_SESSION_TOKEN when the selected access key is temporary.
```

The Collector passes this Secret through `envFrom`, so AWS credential keys must use the AWS SDK environment variable names. The AWS Cost Explorer adapter needs only the read-only `ce:GetCostAndUsage` action. It uses the SDK default credential chain and does not read Vault directly.

### Shared ClickHouse storage Secret

Collector storage credentials are held in a separate namespace-local Secret and are never added to a `CloudAccount` or a provider credential Secret. Configure the Operator with `--collector-storage-secret-name=<secret-name>`; the referenced Secret is added to each Collector Pod through `envFrom`. The fixture-mode kind configuration marks this reference optional so the ordinary offline smoke path works without ClickHouse.

The Secret must contain `CLICKHOUSE_ADDR`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USERNAME`, and `CLICKHOUSE_PASSWORD`. Set `CLICKHOUSE_SECURE=true` when the native TCP connection uses TLS. Production values should be synchronized from Vault through External Secrets Operator, for example:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: clickhouse-collector
  namespace: cloud-cost
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: clickhouse-collector
  data:
    - secretKey: CLICKHOUSE_ADDR
      remoteRef:
        key: kv/finops/clickhouse/collector
        property: address
    - secretKey: CLICKHOUSE_DATABASE
      remoteRef:
        key: kv/finops/clickhouse/collector
        property: database
    - secretKey: CLICKHOUSE_USERNAME
      remoteRef:
        key: kv/finops/clickhouse/collector
        property: username
    - secretKey: CLICKHOUSE_PASSWORD
      remoteRef:
        key: kv/finops/clickhouse/collector
        property: password
```

Apply `config/storage/clickhouse/cost_records.sql` through the deployment migration process before enabling ingestion. The runtime user needs insert permission on `cost_records`; it does not need DDL permission. For replay-safe reads, use `FINAL` because `ReplacingMergeTree` background deduplication is asynchronous.

---

## 3. CloudAccount references

`CloudAccount` references the generated Kubernetes Secret.

Example:

```yaml
apiVersion: finops.sre.local/v1alpha1
kind: CloudAccount
metadata:
  name: aws-prod
  namespace: cloud-cost
spec:
  provider: aws

  credentialRef:
    name: aws-finops-prod

  collection:
    enabled: true
    schedule: "0 0 * * *"
```

The CRD must not contain:

```text
accessKey
secretKey
clientSecret
privateKey
apiToken
webhookToken
password
```

The Operator should validate that the referenced Secret exists and expose readiness through status.

Suggested conditions:

```text
CredentialsReady=True
CredentialsReady=False / SecretNotFound
CredentialsReady=False / InvalidSecretShape
```

Do not place secret contents in status.

---

## 4. Vault path convention

Recommended high-level layout:

```text
kv/finops/
├── aws/
│   ├── prod
│   └── dev
├── azure/
├── gcp/
├── oci/
├── ibm/
├── ncp/
├── nhn/
├── alibaba/
└── teams/
```

The exact path structure may follow existing organizational Vault conventions.

Do not hard-code Vault paths into provider code.

Vault-path knowledge belongs in `ExternalSecret` manifests.

---

## 5. Teams secrets

Teams notification credentials must use the same secret path:

```text
Vault
  ↓
ExternalSecret
  ↓
Kubernetes Secret
  ↓
TeamsNotifier
```

Example `NotificationPolicy`:

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

The one-shot Analyzer resolves the referenced Kubernetes Secret only after the event and account selectors match. `events` is required and explicitly subscribes this destination to notifications; the current budget Analyzer event is `BudgetThreshold`. An optional selector matches the policy when at least one relevant CloudAccount has all configured metadata values. The runtime reads `TEAMS_WEBHOOK_URL` from the Secret in the same namespace and does not include its value in errors or logs.

Run the Analyzer once for a BudgetPolicy with:

```bash
./bin/louder-analyzer --namespace=cloud-cost --budget-policy=sre-monthly
```

After building the image, a Kubernetes Job can invoke `/louder-analyzer --namespace=cloud-cost --budget-policy=sre-monthly`. Setting `BudgetPolicy.spec.schedule` lets the Operator manage this Job through a namespaced CronJob and a dedicated ServiceAccount.

The process also needs `CLICKHOUSE_ADDR`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USERNAME`, `CLICKHOUSE_PASSWORD`, and optional `CLICKHOUSE_SECURE` environment variables. The managed CronJob provides these values from the namespace-local ClickHouse Secret with `envFrom`. Its dedicated ServiceAccount has namespace-scoped access to one BudgetPolicy and its status, CloudAccounts, NotificationPolicies, and referenced Secrets. Successful threshold percentages are recorded in BudgetPolicy status by UTC month to suppress repeats. If delivery succeeds but the status update fails, Kubernetes may retry the Job and send a duplicate; delivery is at-least-once.

Store the complete Teams Workflows callback URL as `TEAMS_WEBHOOK_URL` in the referenced Secret. The callback URL contains its authentication material and must be sourced through Vault and External Secrets Operator; never put it in a `NotificationPolicy`, ConfigMap, fixture, or log. The notifier accepts HTTPS URLs only and does not follow redirects. Workflows are associated with their owners, so production setup must assign and maintain an owner who will remain responsible for the workflow.

---

## 6. Least privilege

Use the minimum cloud permissions required for billing collection.

Principles:

- read-only billing access where possible
- separate credentials by cloud account/environment where practical
- do not reuse broad administrator credentials
- do not grant resource mutation permissions for Phase 1
- keep future inventory/event permissions separate from current billing permissions

The future resource-tracking phase may need additional read permissions.

Do not grant them preemptively.

---

## 7. Logging and errors

Never log:

- secret values
- access tokens
- full Authorization headers
- private keys
- client secrets
- webhook secrets

Provider API errors must be sanitized before being stored in CR status or logs if the upstream error may contain credential material.

Expose safe error classes such as:

```text
SecretNotFound
AuthenticationFailed
PermissionDenied
ProviderRateLimited
ProviderUnavailable
InvalidCredentialShape
```

---

## 8. Local development

Ordinary unit and provider contract tests should not depend on Vault.

Use fixtures/fakes at those layers.

Vault is required in integration/E2E tests that specifically verify the production secret path.

Keep a clear distinction:

```text
unit / contract test
  -> fake credentials / fake provider

Kubernetes E2E
  -> test Vault -> ESO -> K8s Secret -> workload
```

---

## 9. E2E Vault

The E2E environment may run a disposable Vault test instance.

Target flow:

```text
kind
 │
 ├── Vault test instance
 ├── ESO
 ├── ExternalSecret
 ├── K8s Secret
 ├── Operator
 └── Collector
```

Seed only synthetic test credentials.

Never seed real production credentials into CI.

Example test paths:

```text
kv/finops/aws/test
kv/finops/gcp/test
kv/finops/teams/test
```

---

## 10. Secret rotation

The design must tolerate secret refresh by ESO.

Collectors should load credentials at execution time rather than assuming credentials remain static forever.

For long-running components, define reload behavior explicitly.

Avoid requiring Operator restart for routine credential rotation.

---

## 11. Security checklist

```text
[ ] Secret plaintext absent from Git
[ ] Secret plaintext absent from CR specs
[ ] Secret plaintext absent from logs
[ ] Secret plaintext absent from metrics
[ ] Secret plaintext absent from fixtures
[ ] ExternalSecret uses vault-backend
[ ] CloudAccount references only K8s Secret name
[ ] Operator does not integrate directly with Vault
[ ] CSP permission scope is read-only/minimal
[ ] CI uses synthetic credentials
[ ] Rotation does not require manual redeploy where avoidable
```
