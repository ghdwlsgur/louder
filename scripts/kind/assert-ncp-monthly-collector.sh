#!/usr/bin/env bash
set -euo pipefail

context="kind-${KIND_CLUSTER_NAME:-louder-e2e}"
namespace=cloud-cost
account=kind-ncp-monthly-collector
cronjob="$account-collector"
job=kind-ncp-monthly-collector-run

kubectl --context "$context" apply -f config/kind/fixtures/cloudaccount-ncp-monthly-collector.yaml

for attempt in $(seq 1 30); do
  if kubectl --context "$context" get --namespace "$namespace" "cronjob/$cronjob" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
kubectl --context "$context" get --namespace "$namespace" "cronjob/$cronjob" >/dev/null

kubectl --context "$context" create job "$job" \
  --namespace "$namespace" \
  --from="cronjob/$cronjob"
kubectl --context "$context" wait \
  --for=condition=Complete \
  --timeout=120s \
  --namespace "$namespace" \
  "job/$job"
kubectl --context "$context" wait \
  --for=condition=CollectionReady=True \
  --timeout=60s \
  --namespace "$namespace" \
  "cloudaccount/$account"

logs=$(kubectl --context "$context" logs --namespace "$namespace" "job/$job")
for expected in \
  '"sourceRecordId":"ncp-monthly-2760000-2026-10-KRW"' \
  '"billingScope":"2760000"' \
  '"costBasis":"ncp_monthly_invoice_cost"' \
  '"amount":"12345.67"' \
  '"currency":"KRW"' \
  '"usageStart":"2026-10-01T00:00:00Z"' \
  '"usageEnd":"2026-11-01T00:00:00Z"'; do
  if ! grep -Fq "$expected" <<<"$logs"; then
    printf 'Expected NCP monthly fixture field %s in Collector output, got: %s\n' "$expected" "$logs" >&2
    exit 1
  fi
done

printf 'NCP monthly CloudAccount -> CronJob -> fixture Collector Job passed\n'
