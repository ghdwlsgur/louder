#!/usr/bin/env bash
set -euo pipefail

context=kind-louder-e2e
namespace=cloud-cost
account=kind-fixture-collector
cronjob="$account-collector"
job=kind-fixture-collector-run
synthetic_secret_value=synthetic-fixture-only

kubectl --context "$context" create secret generic kind-fixture-credentials \
  --namespace "$namespace" \
  --from-literal="fixture=$synthetic_secret_value" \
  --dry-run=client \
  -o yaml | kubectl --context "$context" apply -f -
kubectl --context "$context" apply -f config/kind/fixtures/cloudaccount-collector.yaml

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

logs=$(kubectl --context "$context" logs --namespace "$namespace" "job/$job")
if ! grep -Fq '"sourceRecordId":"fixture-aws-001"' <<<"$logs"; then
  printf 'Expected the AWS fixture record in Collector output, got: %s\n' "$logs" >&2
  exit 1
fi
if ! grep -Fq '"billingScope":"synthetic-kind-account"' <<<"$logs"; then
  printf 'Expected the synthetic account scope in Collector output, got: %s\n' "$logs" >&2
  exit 1
fi
if grep -Fq "$synthetic_secret_value" <<<"$logs"; then
  printf 'Collector output exposed a credential Secret value\n' >&2
  exit 1
fi

printf 'CloudAccount -> CronJob -> fixture Collector Job passed\n'
