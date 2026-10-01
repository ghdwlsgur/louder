# Phase 1 Level 29: GCP BigQuery Billing Export

## Goal

Add an offline-testable GCP billing adapter backed by the standard BigQuery Billing Export, and pass its non-secret provider configuration from `CloudAccount` through the Operator and Collector.

## Scope

- Add opaque string provider configuration for BigQuery project, dataset, and export table identifiers.
- Forward that configuration from the `CloudAccount` spec to Collector execution.
- Query daily account totals from the standard BigQuery billing export with parameterized billing scope and UTC date bounds.
- Include credits in GCP net cost and align AWS collection on `NetUnblendedCost` so the Analyzer can compare a common `net_cost` basis.
- Preserve source currency and stable account-day record IDs.
- Add adapter unit, shared provider contract, fixture, normalization, and controller coverage using fakes and synthetic fixtures only.
- Document required GCP Secret keys, read permissions, export setup, and BigQuery query costs.

## Non-goals

- Live GCP or AWS API calls, cloud resource provisioning, or production deployment.
- GCP resource/service-level cost dimensions or currency conversion.
- Supporting detailed usage cost export in this slice.
- Changes to budgets, notification behavior, or the seven-day collection window.
- Changes to other provider adapters beyond AWS's cost metric and shared cost-basis label.

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
- `docs/task/completed/phase-1-lv14-aws-cost-explorer-collector.md`
- `docs/task/completed/phase-1-lv28-aws-cost-revision-lookback.md`
- Google Cloud standard Billing Export schema and query guidance: https://cloud.google.com/billing/docs/how-to/export-data-bigquery-tables/standard-usage
- Google Cloud Billing Export setup and cost guidance: https://cloud.google.com/billing/docs/how-to/export-data-bigquery-setup
- AWS Cost Explorer `GetCostAndUsage` metrics: https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsage.html

## Work items

1. Add a single GCP adapter behavior test for daily net-cost aggregation and run it to confirm RED.
2. Implement query execution behind an injectable boundary, validate configured BigQuery identifiers, and map rows into stable raw records; rerun focused tests for GREEN.
3. Add and verify provider configuration forwarding from CRD through CronJob args and Collector CLI.
4. Change AWS request metric to `NetUnblendedCost`, update its fixture expectations, and establish the shared `net_cost` label with focused tests.
5. Register GCP and AWS provider factories without requiring AWS credentials to start GCP collection.
6. Update provider/secret/setup documentation and the GCP example manifest. Do not include credentials in configuration or fixtures.
7. Run focused tests, repository test/static/build checks, and the offline kind smoke path where practical; make no live CSP requests.

## Completion criteria

- A GCP `CloudAccount` can provide BigQuery project, dataset, and standard export table identifiers without placing secrets in its spec.
- Collector passes these values to the selected provider and resolves GCP credentials only from the Kubernetes Secret environment.
- GCP adapter queries the requested half-open UTC interval, filters by billing account, aggregates `cost + credits`, preserves currency, and emits stable daily records with `net_cost` basis.
- BigQuery table identifiers cannot inject arbitrary SQL; value filters use query parameters.
- AWS asks Cost Explorer for `NetUnblendedCost` and emits the same `net_cost` basis.
- Tests use fake BigQuery results and synthetic AWS SDK responses; no live credentials are required.
- Documentation describes export delay, first export/setup requirements, minimum permissions, and that BigQuery query usage can incur charges.

## Design decisions, risks, and open questions

- Use Google Standard usage cost export. The export schema provides `cost` and nested `credits`; net amount is their sum. The adapter groups by UTC usage date and currency.
- Treat `CloudAccount.spec.providerConfig` as non-secret opaque string configuration. GCP validates the required keys `projectId`, `datasetId`, and `tableId`; credentials remain in the referenced Kubernetes Secret as `GOOGLE_CREDENTIALS_JSON`.
- Use an injected query interface so unit tests do not need ADC, a live project, or BigQuery API calls.
- Use integer micros when aggregating to avoid floating point arithmetic. Convert to canonical decimal strings at the adapter boundary.
- BigQuery export is asynchronous and may not contain recent complete data. A successful query is not proof of billing freshness.
- BigQuery queries may incur query-processing charges; users must configure an appropriate billing project and control scanned data.
- Switching AWS from unblended to net unblended changes the semantic value analyzed for AWS and requires confirming the SDK enum supports the metric. Accounts without applicable discounts may report net values equal to unblended values.
- Existing ClickHouse data collected before the metric change keeps the old `unblended_cost` basis. A populated installation needs a replay of the affected analysis period or a clean-period cutover because the normal seven-day recollection range cannot rewrite older rows.

## Verification plan

- Record RED and GREEN for each public behavior before proceeding to the next behavior.
- `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/gcp ./internal/provider/aws ./internal/collector ./internal/controller ./cmd/collector -count=1`
- `GOCACHE=/tmp/louder-go-cache make test`
- `GOCACHE=/tmp/louder-go-cache make vet`
- `make fmt-check`
- `GOCACHE=/tmp/louder-go-cache make build`
- Run the relevant offline kind smoke target if Docker and kind are available; no direct cluster context is used.
- `git diff --check`
- Record any unavailable checks and confirm that no live CSP request was made.

## Verification results

- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/gcp -run TestCollectCostsMapsBigQueryDailyNetTotals -count=1` failed because the GCP adapter, query boundary, and shared net cost basis did not exist.
- GREEN: the same focused GCP test passed after implementing daily net-cost mapping.
- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/aws -count=1` failed because the adapter still requested and decoded `UnblendedCost` after the test response changed to `NetUnblendedCost`.
- GREEN: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/aws ./internal/provider/gcp -count=1` passed after changing the AWS metric and common basis.
- RED: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/gcp -run TestBuildQuerySumsCostAndCreditsWithBoundedBillingScope -count=1` failed because the parameterized SQL builder was not implemented.
- GREEN: `GOCACHE=/tmp/louder-go-cache go test ./internal/provider/gcp ./internal/normalize ./internal/collector -count=1` passed with shared contract scenarios, GCP fixture replay, GCP normalization, net aggregation, parameter bounds, credentials shape, unsafe identifiers, and permission error classification.
- GREEN: `GOCACHE=/tmp/louder-go-cache go test ./... -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-cache make vet` passed.
- GREEN: `make fmt-check` passed.
- GREEN: `GOCACHE=/tmp/louder-go-cache make manifests` passed and regenerated the CloudAccount CRD schema for `providerConfig`.
- GREEN: `GOCACHE=/tmp/louder-go-cache make build` exited successfully for Operator, Collector, and Analyzer. Go printed module cache write warnings under the shared GOPATH; the build artifacts were produced.
- BLOCKED: kind smoke/E2E could not run because Docker CLI cannot connect to `/Users/jinhyeokhong/.docker/run/docker.sock`; no cluster was created.
- GREEN: `git diff --check` passed.
- No live AWS or GCP request was made.
