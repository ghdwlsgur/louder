# Security Policy

## 1. Purpose

Louder is a Kubernetes-native multi-cloud cost and governance platform.

It integrates with cloud billing APIs, Kubernetes, Vault, External Secrets Operator, ClickHouse, Microsoft Teams, and potentially other infrastructure systems.

Security reports are treated as operationally sensitive.

Do not disclose vulnerabilities, credentials, internal infrastructure details, or exploit instructions publicly before maintainers have had a reasonable opportunity to investigate and remediate them.

---

## 2. Supported Versions

Until Louder reaches a stable release policy, security fixes are provided for:

- the latest released version
- the current default development branch when no release exists yet

Older releases may not receive backported security fixes.

Once a formal release lifecycle exists, this section should be replaced with an explicit supported-version matrix.

---

## 3. Reporting a Vulnerability

Do **not** report security vulnerabilities through:

- public GitHub issues
- public discussions
- pull request comments
- public chat channels
- social media

Use GitHub's **Private Vulnerability Reporting** feature when it is enabled for this repository.

If private vulnerability reporting is not available, contact the repository maintainers through the private security contact method published in the repository organization profile or project metadata.

A useful report should include:

- affected component
- affected version or commit
- vulnerability description
- security impact
- reproduction steps
- prerequisites
- relevant logs or traces
- suggested remediation, if known

Do not include real production credentials, access tokens, customer data, or unnecessary sensitive infrastructure information in a report.

Use synthetic or redacted examples whenever possible.

---

## 4. Scope

Security-sensitive areas include, but are not limited to:

### Kubernetes control plane

- Operator reconciliation
- CRD validation
- RBAC
- admission webhooks
- leader election
- Job and CronJob creation
- namespace boundaries
- owner references
- privilege escalation paths

### Secret management

- Vault integration
- External Secrets Operator
- Kubernetes Secrets
- credential references
- secret rotation
- accidental secret exposure
- logs containing credentials
- metrics containing credentials

The production secret path is:

```text
Vault
  ↓
External Secrets Operator
  ↓
Kubernetes Secret
  ↓
Workload
```

Louder components should consume Kubernetes Secrets and must not require direct Vault access unless the architecture is explicitly changed.

### Cloud credentials and APIs

- AWS
- Azure
- GCP
- OCI
- IBM Cloud
- Naver Cloud Platform
- NHN Cloud
- Alibaba Cloud

Cloud permissions should be read-only and least-privilege for the current cost-collection phase.

Unexpected resource mutation capability should be treated as a security defect.

### Cost and billing data

Billing data may contain:

- cloud account identifiers
- resource identifiers
- internal project names
- tags
- owner metadata
- regional information
- spending patterns

Treat this data as potentially sensitive operational metadata.

### Notification integrations

- Microsoft Teams credentials
- webhook or workflow endpoints
- alert payloads
- accidental leakage of account or cost information

### Storage

- ClickHouse
- object storage
- backup/export paths
- ingestion pipelines
- cross-tenant data access

---

## 5. Out of Scope

The following are generally not considered security vulnerabilities unless they create a real confidentiality, integrity, or availability impact:

- cosmetic UI issues
- documentation typos
- unsupported provider behavior clearly documented as unsupported
- missing hardening recommendations without an exploitable condition
- denial of service requiring unrealistic local administrative access
- vulnerabilities that require modification of trusted build artifacts after verification
- findings based only on automated scanners without a demonstrated impact

Maintainers may still choose to fix out-of-scope findings.

---

## 6. Secret Handling Rules

Never commit plaintext secrets to:

- source code
- Git history
- Helm values
- Kubernetes CR specifications
- ConfigMaps
- fixtures
- test data
- documentation
- example manifests

Never log:

- passwords
- access keys
- secret keys
- API tokens
- bearer tokens
- client secrets
- private keys
- full Authorization headers
- Teams webhook secrets

Never expose secret values through:

- CR status
- Kubernetes Events
- Prometheus metrics
- tracing attributes
- error messages returned to users

Errors should use safe classifications such as:

```text
SecretNotFound
AuthenticationFailed
PermissionDenied
InvalidCredentialShape
ProviderRateLimited
ProviderUnavailable
```

---

## 7. Least Privilege

Every Louder component should operate with the minimum permissions it needs.

### Kubernetes

Prefer:

- namespace-scoped access where possible
- narrowly scoped Roles and RoleBindings
- dedicated ServiceAccounts
- explicit resource and verb lists

Avoid:

- broad wildcard permissions
- `cluster-admin`
- unnecessary ClusterRole usage
- access to Secrets outside the required namespace

### Cloud providers

Phase 1 credentials should allow only the billing and usage reads necessary for cost collection.

Do not grant:

- resource creation
- resource deletion
- IAM administration
- network mutation
- storage mutation

unless a future explicitly approved feature requires it.

---

## 8. Container Security

Production containers should:

- run as non-root where practical
- use explicit `securityContext`
- drop unnecessary Linux capabilities
- avoid privileged mode
- avoid host namespaces
- avoid hostPath unless explicitly justified
- use a read-only root filesystem where practical
- define resource requests and limits
- minimize runtime packages and debugging tools

The absence of Pod Security Admission enforcement in a target cluster must not be treated as permission to weaken workload security.

---

## 9. Network Security

Do not introduce broad network privileges unnecessarily.

When NetworkPolicy is used:

- prefer standard Kubernetes `NetworkPolicy` where sufficient
- preserve only required DNS and service communication
- explicitly document required external CSP API egress
- explicitly document Teams egress
- explicitly document ClickHouse access

Avoid adding namespace-wide default-deny behavior as an unrelated side effect of another change.

---

## 10. Public Repository Hygiene

Louder is intended to be safe for public development.

Never commit internal infrastructure details that are not required for the open-source project.

Examples of information that should not be published unless intentionally public:

- private cluster names
- internal domains
- internal IP addresses
- private registry hostnames
- private GitLab URLs
- internal Vault paths
- employee identifiers
- production cloud account IDs
- customer names
- private network topology
- internal incident details

Use placeholders in public examples:

```text
<cluster-context>
<internal-registry>
<vault-cluster-secret-store>
<cloud-account-id>
<teams-secret-name>
```

Deployment-specific private configuration should live in a separate private repository where practical.

---

## 11. Test Data

Fixtures must use sanitized or synthetic data.

Do not copy production responses into `testdata/` without review and redaction.

Before committing fixtures, check for:

- access keys
- tokens
- account IDs
- email addresses
- internal hostnames
- private IP addresses
- customer identifiers
- resource names that reveal sensitive projects
- URLs containing credentials or signed parameters

Security-sensitive test scenarios should prefer fake providers and synthetic credentials.

---

## 12. Dependency and Supply-Chain Security

Dependencies should be reviewed before introduction.

Prefer:

- actively maintained libraries
- minimal dependency surface
- pinned or reproducible builds
- verified container images
- internal mirroring where required by deployment policy

Do not add a dependency solely to avoid implementing a small, well-understood function.

When a dependency is added, consider:

- maintenance activity
- license compatibility
- transitive dependency cost
- known security history
- whether it executes code during build or initialization

Container images used in production should be published or mirrored to the approved internal registry for the target environment.

---

## 13. Security-Sensitive Changes

Changes affecting any of the following require extra review and explicit verification:

- authentication
- authorization
- RBAC
- Secrets
- Vault / ESO integration
- webhook validation
- cloud credentials
- provider SDK authentication
- TLS
- certificate handling
- network policy
- admission webhooks
- container privileges
- GitHub Actions or CI credentials
- release/signing pipelines

Such changes should include tests that cover both expected success and expected failure.

---

## 14. Vulnerability Handling

When a valid vulnerability is reported, maintainers should:

1. acknowledge the report privately
2. reproduce and assess impact
3. identify affected versions
4. prepare a fix privately where feasible
5. add a regression test
6. review for related variants
7. release or merge the remediation
8. disclose the issue publicly only after remediation is available, when appropriate

Do not publish exploit details before a fix is reasonably available if doing so would increase risk to users.

---

## 15. Security Fix Quality Bar

A security fix is not complete only because the immediate exploit path is blocked.

Where applicable, also:

- add a regression test
- remove leaked credentials from history or rotate them
- verify adjacent code paths
- review authorization boundaries
- update documentation
- update operational detection
- assess whether already-deployed installations require remediation

If a secret may have been exposed, assume it is compromised and rotate it.

---

## 16. Responsible Disclosure

We appreciate good-faith security research.

Please:

- avoid accessing data that does not belong to you
- avoid destructive testing
- avoid persistence in systems
- avoid unnecessary data exfiltration
- stop once sufficient evidence exists to demonstrate the issue
- report findings privately

Maintainers should make a reasonable effort to communicate status and coordinate disclosure with reporters.

---

## 17. Security Documentation

Security-related implementation work must also follow:

- `AGENTS.md`
- `docs/architecture.md`
- `docs/cluster-platform.md`
- `docs/secrets.md`
- `docs/harness.md`
- `docs/provider-contract.md`

When these documents conflict, the more specific security requirement should take precedence unless the repository owner explicitly decides otherwise.
