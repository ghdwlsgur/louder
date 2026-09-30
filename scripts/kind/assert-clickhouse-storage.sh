#!/usr/bin/env bash
set -euo pipefail

context="kind-${KIND_CLUSTER_NAME:-louder-e2e}"
query="SELECT count() FROM cost_records FINAL WHERE provider = 'aws' AND billing_account_id = 'synthetic-kind-account' AND source_record_id = 'fixture-aws-001'"
count=$(kubectl --context "$context" exec --namespace cloud-cost deployment/clickhouse -- \
  sh -c 'clickhouse-client --user="$CLICKHOUSE_USER" --password="$CLICKHOUSE_PASSWORD" --database="$CLICKHOUSE_DB" --format=TabSeparatedRaw --query="$1"' sh "$query")
if [[ "$count" != 1 ]]; then
  printf 'Expected one logical ClickHouse row after replay, got %q\n' "$count" >&2
  exit 1
fi

printf 'ClickHouse fixture persistence and replay deduplication passed\n'
