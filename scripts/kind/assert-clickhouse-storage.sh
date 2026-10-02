#!/usr/bin/env bash
set -euo pipefail

context="kind-${KIND_CLUSTER_NAME:-louder-e2e}"
query="SELECT provider, billing_account_id, source_record_id, cost_basis, amount, currency FROM cost_records FINAL WHERE provider = 'aws' AND billing_account_id = 'synthetic-kind-account' AND source_record_id = 'fixture-aws-001'"
rows=$(kubectl --context "$context" exec --namespace cloud-cost deployment/clickhouse -- \
  sh -c 'clickhouse-client --user="$CLICKHOUSE_USER" --password="$CLICKHOUSE_PASSWORD" --database="$CLICKHOUSE_DB" --format=TabSeparatedRaw --query="$1"' sh "$query")
expected=$'aws\tsynthetic-kind-account\tfixture-aws-001\tnet_cost\t12.34\tUSD'
if [[ "$rows" != "$expected" ]]; then
  printf 'Expected one normalized ClickHouse row after replay, got %q\n' "$rows" >&2
  exit 1
fi

printf 'ClickHouse fixture persistence and replay deduplication passed\n'
