# Phase 1 Level 30: Azure Cost Management Query Adapter

## Goal

Add an offline-testable Azure subscription cost adapter using the Cost Management Query API and route its credentials through the existing Collector contract.

## Scope

- Query Azure subscription-scope `ActualCost` at daily granularity with the `PreTaxCost` sum and preserve currency.
- Use the existing half-open UTC collection interval and stable subscription-day record IDs.
- Authenticate through a Microsoft Entra service principal supplied as `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `AZURE_CLIENT_SECRET` in the referenced Kubernetes Secret.
- Register the Azure provider in the Collector without requiring AWS or GCP credentials.
- Add provider fixture, shared contract scenarios, normalization coverage, and focused REST request/response tests using an HTTP test server.
- Document scope, permissions, credential keys, cost semantics, data freshness, and setup example.

## Non-goals

- Billing account, billing profile, management group, resource-group, or Azure China/Government scope support; this slice accepts one subscription ID.
- Amortized cost, Cost Details exports, resource/service breakdowns, tax modeling, or currency conversion.
- Live Azure credentials, requests, or deployment.
- Changes to AWS or GCP behavior.

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
- `docs/task/completed/phase-1-lv16-aws-cost-normalizer.md`
- `docs/task/completed/phase-1-lv29-gcp-bigquery-billing-export.md`
- Microsoft Cost Management Query REST API: https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage?view=rest-cost-management-2023-03-01
- Microsoft Cost Management API permissions: https://learn.microsoft.com/en-us/azure/cost-management-billing/automate/cost-management-api-permissions
- Azure Cost Management subscription access: https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/assign-access-acm-data
- Azure cost definitions: https://learn.microsoft.com/en-us/azure/cost-management-billing/manage/review-subscription-billing
- Azure Cost Management data freshness and credits: https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/understand-cost-mgt-data

## Work items

1. Write one REST request construction test and run it to record RED.
2. Implement request scope, custom date range, daily `PreTaxCost` aggregation, response column mapping, pagination, and stable provider error classes behind an injectable HTTP/query boundary.
3. Add service-principal credential-shape validation from Secret environment values and Collector registration.
4. Add provider contract scenarios, fixture replay, normalizer test, CloudAccount sample, and security/setup documentation.
5. Run repository tests, vet, formatting, build, manifests, and local kind smoke if Docker is available; do not make live CSP requests.

## Completion criteria

- An Azure CloudAccount using a subscription UUID can collect its previous seven complete UTC days through the Azure Cost Management Query API.
- Query values are sent as JSON fields; the subscription identifier is validated before it is placed in the URL.
- Daily `PreTaxCost`, `UsageDate`, and `Currency` columns are decoded independent of response column order; next links are followed until all pages are collected.
- Records preserve the original currency and use stable subscription-day IDs while fitting the shared normalized record schema.
- Authentication reads service-principal values only from Secret-injected environment variables; errors do not expose secrets.
- Unit, shared provider contract, fixture, and normalization tests are offline.
- Documentation identifies subscription-scope-only support and the ActualCost/PreTaxCost semantics.

## Design decisions, risks, and open questions

- Use the Microsoft Cost Management Query API with API version `2023-03-01`, `type: ActualCost`, `granularity: Daily`, and `PreTaxCost` sum. Microsoft documents `Usage` as equivalent to `ActualCost` for compatibility with older exports; the request will name `ActualCost` explicitly.
- Use `CloudAccount.spec.accountId` as a canonical Azure subscription UUID, which keeps this first Azure scope deterministic and avoids interpolating arbitrary ARM paths.
- `PreTaxCost` is the chosen actual, non-amortized, pre-tax metric and maps to the explicit `actual_pre_tax_cost` basis. Microsoft documents that Cost Management estimates exclude credits before invoice finalization.
- Budget evaluation rejects a selected record set when it contains more than one non-empty cost basis. This prevents silent aggregation of Azure actual pre-tax cost with AWS/GCP net cost until a cross-provider comparable basis is deliberately designed.
- Use Microsoft Entra client-secret credentials from Vault -> ESO -> Kubernetes Secret -> Collector environment. The adapter performs no direct Vault calls.
- Query API responses use numeric JSON values and a column-described row matrix. Decode by column name and retain decimal text without binary floating-point aggregation.
- The Query API exposes a `nextLink`; follow it with the original request body and validate that pagination URLs remain on the configured Azure Resource Manager host before attaching a bearer token.
- Existing ClickHouse records from other providers may have different cost-basis migration history; this task does not rewrite stored records.

## Verification plan

- Record RED then GREEN per externally observable behavior. The mixed-basis analyzer test failed before the basis constant and rejection behavior were implemented; the Azure collector contract tests failed while the adapter still emitted `net_cost`, then passed after it emitted `actual_pre_tax_cost`.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/azure ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `GOCACHE=/tmp/louder-go-cache make kind-e2e` when Docker is available.
- `git diff --check`
- Record unavailable checks and confirm no live Azure, AWS, or GCP request was made.

## Verification results

- TDD RED: the first request-construction test failed because the Azure provider/query API did not yet exist.
- TDD RED/GREEN: the mixed-basis analyzer test first failed to compile because the explicit basis and error were absent; after implementation it passed and returns `ErrMixedCostBasis` without partial intents.
- TDD RED/GREEN: Azure provider contract assertions failed while collection still emitted `net_cost`; after mapping `PreTaxCost` to `actual_pre_tax_cost`, the provider tests passed.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` passed outside the restricted sandbox. The Azure REST tests use localhost HTTP servers.
- `GOCACHE=/tmp/louder-go-cache make vet` passed.
- `make fmt-check` passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` passed.
- `GOCACHE=/tmp/louder-go-cache make build` passed.
- `git diff --check` passed.
- `make kind-e2e` passed. It built the local image, created the temporary `louder-e2e` kind cluster, verified the Operator/CloudAccount/CronJob/fixture Collector Job path, and cleaned up the cluster on exit. `kind get clusters` confirmed there were no remaining clusters. This did not call Azure.
- No live Azure calls have been made.
- User approved the separate `actual_pre_tax_cost` basis and mixed-basis budget rejection.
