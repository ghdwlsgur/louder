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
    - secretKey: access-key-id
      remoteRef:
        key: kv/finops/aws/prod
        property: access_key_id

    - secretKey: secret-access-key
      remoteRef:
        key: kv/finops/aws/prod
        property: secret_access_key
```

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
    schedule: "0 */6 * * *"
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
```

The notifier resolves only the Kubernetes Secret.

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
