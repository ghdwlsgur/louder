# Phase 1 Level 31: OCI Usage Cost Adapter

## Goal

Add an offline-testable Oracle Cloud Infrastructure (OCI) tenancy cost adapter that collects daily summarized costs through the OCI Usage API and follows the shared Provider contract.

## Scope

- Query one OCI tenancy identified by `CloudAccount.spec.accountId` using the Usage API `RequestSummarizedUsages` operation.
- Request `COST` data at `DAILY` granularity over the existing half-open UTC collection window and preserve currency.
- Follow the OCI `opc-next-page` response token until the full logical collection window is read.
- Sign OCI REST requests with API signing key fields injected from the referenced Kubernetes Secret; keep Vault as the source of truth through ESO.
- Preserve decimal cost precision when decoding numeric JSON and create stable tenancy/day/currency record IDs.
- Register OCI in the live Collector and add API response fixtures, provider contract cases, normalization coverage, sample resource, and credential/setup documentation.
- Keep all tests offline; do not use live OCI credentials or call OCI endpoints.

## Non-goals

- OCI cost report CSV export through Oracle-owned Object Storage, automatic compartment inventory, resource-level persistence, or currency conversion.
- Instance principal/workload identity authentication in this first slice; use the existing secret-backed credential flow.
- Cost forecasts, budgets, or OCI resource inventory beyond the shared platform features already implemented.
- Changing AWS, GCP, or Azure cost semantics.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/secrets.md`
- `docs/cluster-platform.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv13-provider-contract-foundation.md`
- `docs/task/completed/phase-1-lv30-azure-cost-query-adapter.md`
- OCI Request Summarized Usages API: https://docs.oracle.com/en-us/iaas/tools/go/latest/usageapi/index.html
- OCI Usage API CLI request parameters, timestamp precision, and pagination: https://docs.oracle.com/en-us/iaas/tools/oci-cli/latest/oci_cli_docs/cmdref/usage-api/usage-summary/request-summarized-usages.html
- OCI REST request-signing specification: https://docs.oracle.com/en-us/iaas/Content/API/Concepts/signingrequests.htm
- OCI Cost Analysis API request example: https://docs.oracle.com/en-us/iaas/Content/Billing/Concepts/costanalysisoverview.htm
- OCI Cost Analysis IAM policy: https://docs.oracle.com/en-us/iaas/Content/Billing/Concepts/costanalysisoverview.htm
- OCI Go SDK configuration model (reference): https://pkg.go.dev/github.com/oracle/oci-go-sdk/v65/common

## Work items

1. Verify the OCI Usage API request, response fields, API-key signing configuration, pagination, and cost semantics against Oracle's current official documentation.
2. Write one public Provider behavior test for daily tenant costs and run it to record RED.
3. Implement tenancy OCID and UTC-window validation, OCI API-key credential shape validation, signed daily `COST` requests, pagination, numeric precision preservation, and stable provider error classes.
4. Add representative OCI API response fixtures and provider contract cases for normal, empty, authentication, permissions, rate limit, timeout, malformed, duplicate, and partial-page responses.
5. Add currency-aware stable source IDs, normalizer coverage, CloudAccount sample, Collector registration, and Secret/IAM/setup documentation.
6. Run repository tests, vet, formatting, manifests, build, and local kind smoke if Docker is available; make no live OCI requests.

## Completion criteria

- A CloudAccount with a valid tenancy OCID collects all returned daily cost rows in the requested half-open UTC window.
- The request uses OCI Usage API daily COST query semantics and includes required date and tenancy fields.
- OCI page tokens are followed without returning partial data if a later page fails; token loops are rejected.
- Numeric cost fields preserve decimal precision through normalization; currency is preserved and is part of stable daily IDs.
- Duplicate tenancy/day/currency rows fail with a stable provider error rather than colliding in ClickHouse.
- OCI API signing credentials are read only from Secret-injected environment values; errors do not reveal key content. The initial adapter accepts unencrypted PKCS#1 or PKCS#8 RSA PEM private keys.
- Provider fixtures, shared contract scenarios, normalization coverage, and relevant failure cases are offline.
- Documentation records the tenancy-level scope, required `read usage-report` IAM policy, credentials, and the verified cost/credit semantics.

## Design decisions, risks, and open questions

- The tenancy OCID is the account scope because OCI Usage API requests require `tenantId` and the initial adapter returns tenancy-level usage. Compartment scoping is excluded to keep the first slice aligned with existing account-level records.
- Use `RequestSummarizedUsages` with `queryType: COST`, `granularity: DAILY`, `isAggregateByTime: false`, and an empty `groupBy`. Reject duplicate tenant/day/currency rows because the request has no grouping dimensions and the platform source ID must remain unique. Send UTC start exactly and the exclusive UTC end minus one microsecond; OCI documents timestamp input to microsecond precision, so do not send Go nanosecond timestamps.
- Avoid generated OCI SDK response models for `computedAmount`, whose generated representation is floating point. Decode the REST JSON number directly and sign requests with standard-library RSA SHA-256. The SDK dependency could not be downloaded in this offline workspace, so the narrow REST client avoids adding an unavailable module solely for transport/signing.
- Page tokens are returned in `opc-next-page`; send each token as the next request's `page` parameter and cap/detect repeated tokens to prevent infinite loops.
- Oracle Cost Analysis requires the `read usage-report` IAM policy. Production credentials must flow Vault -> ESO -> Kubernetes Secret -> Collector; do not add direct Vault access.
- OCI exposes `COST` and credit query types separately, and the API's `computedAmount` semantics are not established as equivalent to AWS/GCP `net_cost` or Azure `actual_pre_tax_cost`. Assign the explicit OCI-only basis `oci_cost`; do not silently combine it with other providers in budget evaluation. Document unresolved credit/tax treatment as an accepted gap until Oracle semantics or representative account data establish it.
- No live account access is available for this task. API behavior must be established with official documentation and synthetic response fixtures; date-boundary behavior cannot be confirmed against a real OCI tenancy in this slice.

## Verification plan

- Record RED then GREEN per externally observable behavior.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/oci ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `make kind-e2e` when Docker is available.
- `git diff --check`
- Record checks not run and confirm no live OCI request was made.

## Verification results

- TDD RED/GREEN: the first executable public behavior test failed with `UnsupportedBillingScope` because the initial tenancy OCID pattern was wrong; duplicate daily/currency rows, malformed signing metadata, and unsupported Collector fixture provider each had their own failing test before the corresponding behavior was fixed. A compile-only test setup failure and an `httptest` socket-bind failure were treated as setup issues, not RED evidence.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/oci ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed, including the existing Azure HTTP tests when run with loopback access.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` — passed; generated CloudAccount CRD includes `oci`.
- `GOCACHE=/tmp/louder-go-cache make build` — passed.
- `make kind-e2e` — passed; disposable `louder-e2e` cluster was cleaned up by the harness.
- `git diff --check` — passed.
- No live OCI API request was made. Cost/credit/tax equivalence and date boundaries remain unconfirmed against an OCI tenancy; OCI records therefore use the isolated `oci_cost` basis.
