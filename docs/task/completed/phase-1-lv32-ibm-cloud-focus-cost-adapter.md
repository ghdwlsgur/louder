# Phase 1 Level 32: IBM Cloud FOCUS Cost Adapter

## Goal

Add an offline-testable IBM Cloud account cost adapter that consumes IBM's FOCUS 1.2 usage export and follows the shared Provider contract.

## Scope

- Query each billing month overlapping the Collector's half-open UTC window through `GET /v4/accounts/{account_id}/focus/{month}`.
- Use FOCUS CSV mode, which IBM documents as returning all rows, and stream the response instead of buffering a whole month's report.
- Exchange the IBM API key from the referenced Kubernetes Secret for an IAM bearer token; keep Vault as the production source of truth through ESO.
- Map `BilledCost`, `BillingCurrency`, `ChargePeriodStart`, and `ChargePeriodEnd` into UTC daily account totals. Preserve decimal values and currencies; use stable account/day/currency source IDs.
- Accept only charge intervals that can be represented as a single UTC day. Reject unsupported intervals rather than allocating monthly or partial-period charges to days without provider evidence.
- Register IBM Cloud in the Collector and CloudAccount CRD; add sanitized FOCUS fixtures, provider contract cases, normalization coverage, a sample CloudAccount, and credential/IAM documentation.
- Keep all tests offline; do not use live IBM Cloud credentials or call IBM endpoints.

## Non-goals

- IBM classic infrastructure bills that must be queried through SoftLayer APIs.
- Cost conversion, resource-level storage, report snapshots in COS, and future budget/anomaly features.
- Reinterpreting IBM FOCUS `BilledCost` as another provider's cost basis without evidence.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/secrets.md`
- `docs/cluster-platform.md`
- `docs/harness.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv31-oci-usage-cost-adapter.md`
- IBM Usage Reports FOCUS export and CSV/JSON behavior: https://cloud.ibm.com/docs/account?locale=en&topic=account-export-usage-focus
- IBM billing usage access requirements: https://cloud.ibm.com/docs/account?topic=account-viewingusage
- IBM FOCUS report field mappings: https://cloud.ibm.com/docs/account?topic=account-understand-reports
- IBM IAM API-key token exchange: https://cloud.ibm.com/docs/iam?interface=ui&topic=iam-iamtoken_from_apikey
- IBM IAM token API: https://cloud.ibm.com/docs/apis/iam-identity-token-api

## Work items

1. Verify the IAM token exchange, FOCUS CSV headers and numeric types, monthly endpoint shape, account access policy, and charge period semantics from official IBM documentation.
2. Write one public Provider behavior test for exact daily FOCUS costs and run it to record RED.
3. Implement account/month window validation, API-key shape validation, IAM token exchange, bounded HTTP requests, monthly CSV streaming, exact decimal parsing, daily aggregation, and stable error classes.
4. Add fixtures and offline cases for normal, empty, multiple currencies, repeated line items in one daily aggregate, authentication, permissions, rate limits, timeout, malformed CSV, unsupported non-daily intervals, and later-month failure.
5. Add normalizer coverage, CloudAccount sample and enum, Collector registration, and Vault/ESO plus IBM IAM setup documentation.
6. Run focused and repository Go tests, vet, formatting, manifests, build, and kind smoke if Docker is available; make no live IBM request.

## Completion criteria

- An IBM Cloud account request collects every FOCUS billing month intersecting the requested half-open UTC interval and returns only representable daily rows inside that interval.
- The adapter exchanges `IBM_CLOUD_API_KEY` for a short-lived bearer token without logging or returning credential values.
- `BilledCost` remains decimal-exact, `BillingCurrency` is preserved, and daily output IDs include account, date, and currency.
- Non-daily charge intervals and any incomplete month result fail explicitly; no monthly amount is silently assigned to a day.
- Provider fixtures, contract scenarios, normalization coverage, and failure tests are offline; multiple FOCUS line items for one day/currency aggregate into exactly one stable daily output record.
- Documentation records Billing Administrator/Account Owner access, the IAM flow, the FOCUS endpoint, daily-interval limitation, and `ibm_billed_cost` semantics.

## Design decisions, risks, and open questions

- Prefer the FOCUS 1.2 endpoint over IBM-native monthly summary fields because FOCUS exposes `BilledCost`, `BillingCurrency`, `ChargePeriodStart`, and `ChargePeriodEnd`. Its exported rows are still queried by month, so the adapter must fetch each overlapping month and filter periods locally.
- Use `format=csv` because IBM documents that CSV returns all results while JSON is paginated. Stream CSV rows to bound memory and avoid guessing undocumented JSON cursor behavior.
- Use the distinct cost basis `ibm_billed_cost`; it includes IBM's billed-cost semantics and must not be combined with `net_cost` or `actual_pre_tax_cost` until equivalence is established.
- IBM's public docs identify the FOCUS charge-period fields but do not establish that every line is a one-day interval. Reject non-daily periods and document this accepted limitation; live account data is needed to confirm a representative export's granularity.
- Read the API key only from Secret-injected `IBM_CLOUD_API_KEY`. Keep tokens in process memory only and do not cache them beyond one Collector invocation.
- Live IBM account access is not available for this task. Use synthetic FOCUS reports and error responses.

## Verification plan

- Record RED then GREEN per externally observable behavior.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/ibm ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1`
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make manifests`
- `GOCACHE=/tmp/louder-go-cache make build`
- `make kind-e2e` when Docker is available.
- `git diff --check`
- Record exact results, any checks not run, and confirm no live IBM request was made.

## Verification results

- TDD RED/GREEN: the daily exact-decimal behavior initially failed because the Provider had no implementation; a monthly charge interval overlapping the requested day initially returned no rows instead of failing; the HTTP integration initially reported the REST source as unimplemented; source credential errors were initially reclassified as `ProviderUnavailable`; and fixture mode initially rejected IBM. Each behavior had a failing test before its corresponding implementation was completed.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/ibm ./internal/provider/contracttest ./internal/normalize ./internal/collector ./internal/controller ./cmd/collector -count=1` — passed.
- `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` — passed, including existing Azure loopback tests.
- `GOCACHE=/tmp/louder-go-cache make vet` — passed.
- `make fmt-check` — passed.
- `GOCACHE=/tmp/louder-go-cache make manifests` — passed; generated CloudAccount CRD includes `ibm`.
- `GOCACHE=/tmp/louder-go-cache make build` — passed.
- `make kind-e2e` — passed; disposable `louder-e2e` cluster was cleaned up by the harness.
- `git diff --check` — passed.
- No live IBM API request was made. The adapter supports only rows whose charge period is exactly one UTC day; this limitation and the need for representative account data are documented.
