# Phase 1 LV17: ClickHouse Cost Storage

## Goal

Persist normalized AWS account-day cost records from Collector Jobs into ClickHouse, keeping cloud credentials separate from the shared storage credentials.

## Scope

- Add a ClickHouse table for the current normalized cost record and a versioned `ReplacingMergeTree` key based on provider, account, and source record ID.
- Add a ClickHouse Go storage adapter that writes a normalized batch and sanitizes connection/write failures.
- Add Collector paths that normalize all records before writing, preserve raw JSON Lines on stdout, and fail the Job when storage fails.
- Add an Operator option to include a separate namespace-local ClickHouse Secret in Collector Pods.
- Read connection settings from `CLICKHOUSE_ADDR`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USERNAME`, and `CLICKHOUSE_PASSWORD`.
- Add a disposable ClickHouse instance to the local kind storage E2E, verify persistence and replay deduplication, and document deployment/migration and `FINAL` query behavior.

## Non-goals

- Persisting raw provider payloads to object storage.
- AWS/GCP/Azure live calls in kind or CI.
- Analyzer queries, budget evaluation, or currency conversion.
- Production ClickHouse deployment, topology changes, or modification of the shared `innogrid-core-sre` cluster.
- Storing ClickHouse credentials in each provider's credential Secret or in a CloudAccount CR.

## Dependencies and prior documents

- `AGENTS.md`
- `SECURITY.md`
- `docs/architecture.md`
- `docs/provider-contract.md`
- `docs/harness.md`
- `docs/secrets.md`
- `docs/cluster-platform.md`
- `docs/task/README.md`
- `docs/task/completed/phase-1-lv16-aws-cost-normalizer.md`
- Official ClickHouse Go client and MergeTree documentation.

## Work items

1. Add focused failing tests for storage batch mapping, empty batches, and safe write failures; implement the smallest ClickHouse store behavior.
2. Add the SQL schema with a deterministic logical row key and a monotonically versioned replacement column.
3. Add Collector tests proving normalization happens before storage, all records are normalized before any write, and storage errors fail the run.
4. Add an optional shared storage Secret reference to Operator configuration and Collector CronJobs; retain fixture-only operation when it is unset.
5. Add a kind storage mode with synthetic ClickHouse credentials, apply the schema, execute an AWS fixture collection twice, and verify a single logical row with `FINAL`.
6. Document the secret keys, schema application, and the fact that deduplication is query-time/merge-time rather than synchronous uniqueness.

## Completion criteria

- A successful non-fixture AWS collection normalizes records, writes one ClickHouse batch, and retains raw JSON Lines stdout for diagnostics.
- A configured storage failure returns a sanitized error and makes the Collector Job fail.
- No ClickHouse credential is placed in CloudAccount CRs or the AWS credential Secret.
- A retry of the same fixture record is represented by one logical row when read with `FINAL`.
- The regular kind smoke and Vault/ESO flows remain usable without a ClickHouse Secret when storage integration is not enabled.
- Unit tests and kind storage E2E use synthetic records and credentials only.
- `make test`, `make vet`, `make fmt-check`, Docker build, kind storage E2E, and `git diff --check` pass.

## Design decisions and risks

- The Operator receives only the name of a shared ClickHouse Secret; the Secret lives in the watched CloudAccount namespace and is added to Collector Pods through `envFrom`.
- Collector SQL uses the ClickHouse native TCP protocol. The address, database, username, and password come from the shared Secret.
- Amounts remain strings in storage to preserve source precision and formatting. Analytical queries can cast them to an appropriate Decimal type after currency-aware query rules are designed.
- `ReplacingMergeTree(version)` uses a `UInt64` Unix-nanosecond ingestion version and `(provider, billing_account_id, source_record_id)` as its logical key. Background deduplication is asynchronous; readers requiring one current row must use `FINAL`.
- The storage adapter does not auto-create tables at runtime. Schema changes are applied explicitly from the checked-in SQL file so the runtime user does not need DDL permissions.
- Local kind E2E uses an ephemeral single-node ClickHouse server with synthetic runtime-generated credentials; it does not represent the production ClickHouse topology.

## Verification plan

- Record a genuine RED and GREEN test for each storage or Collector behavior.
- `GOCACHE=/tmp/louder-go-build go test ./internal/storage/clickhouse ./internal/collector ./internal/controller -count=1`
- `GOCACHE=/tmp/louder-go-build make test`
- `GOCACHE=/tmp/louder-go-build make vet`
- `GOCACHE=/tmp/louder-go-build make fmt-check`
- `KIND_CLUSTER_NAME=louder-e2e-lv17-storage GOCACHE=/tmp/louder-go-build make kind-e2e-storage`
- `bash -n scripts/kind/*.sh`
- `git diff --check`

## Verification results

- RED: `go test ./internal/collector -run TestRunWithStorageDoesNotEmitRawRecordsWhenStorageFails -count=1` failed because storage errors were ignored and `RunWithStorage` emitted no error.
- GREEN: `GOCACHE=/tmp/louder-go-build go test ./internal/collector ./internal/storage/clickhouse ./internal/controller ./cmd/collector ./cmd/operator -count=1` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make test` passed all packages.
- GREEN: `GOCACHE=/tmp/louder-go-build make vet` passed.
- GREEN: `GOCACHE=/tmp/louder-go-build make fmt-check` passed.
- GREEN: `bash -n scripts/kind/*.sh` passed.
- GREEN: `KIND_CLUSTER_NAME=louder-e2e-lv17-storage GOCACHE=/tmp/louder-go-build make kind-e2e-storage` passed. The E2E applied the migration, persisted the AWS fixture twice, and observed one row with `FINAL`; the disposable cluster was removed by the script.
- GREEN: `git diff --check` passed before final review.
