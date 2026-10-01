#!/usr/bin/env bash
set -euo pipefail

context="kind-${KIND_CLUSTER_NAME:-louder-e2e}"
namespace=cloud-cost
policy=kind-scheduled-budget
cronjob=kind-scheduled-budget-analyzer

kubectl --context "$context" apply -f config/kind/fixtures/budgetpolicy-scheduled.yaml
printf 'Waiting for Analyzer CronJob %s\n' "$cronjob"
for attempt in $(seq 1 30); do
  if kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" >/dev/null 2>&1; then
  kubectl --context "$context" logs deployment/louder-operator --namespace "$namespace" || true
  kubectl --context "$context" get events --namespace "$namespace" --sort-by=.lastTimestamp || true
  printf 'Analyzer CronJob %s was not created\n' "$cronjob" >&2
  exit 1
fi
printf 'Analyzer CronJob created; checking desired fields\n'

assert_equal() {
  local name="$1"
  local actual="$2"
  local expected="$3"
  if [[ "$actual" != "$expected" ]]; then
    printf '%s = %q, want %q\n' "$name" "$actual" "$expected" >&2
    exit 1
  fi
}

schedule=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.spec.schedule}')
concurrency=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.spec.concurrencyPolicy}')
timezone=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.spec.timeZone}')
service_account=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.spec.jobTemplate.spec.template.spec.serviceAccountName}')
command=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].command[0]}')
storage_secret=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].envFrom[0].secretRef.name}')
owner=$(kubectl --context "$context" get cronjob "$cronjob" --namespace "$namespace" -o jsonpath='{.metadata.ownerReferences[0].name}')
identity=system:serviceaccount:cloud-cost:louder-analyzer

assert_equal schedule "$schedule" "0 0 1 1 *"
assert_equal concurrency "$concurrency" "Forbid"
assert_equal timezone "$timezone" "Etc/UTC"
assert_equal service_account "$service_account" "louder-analyzer"
assert_equal command "$command" "/louder-analyzer"
assert_equal storage_secret "$storage_secret" "louder-clickhouse-credentials"
assert_equal owner "$owner" "$policy"

printf 'Analyzer CronJob fields passed; checking ServiceAccount permissions\n'
for permission in \
  "get budgetpolicies.finops.sre.local yes" \
  "patch budgetpolicies-status yes" \
  "list cloudaccounts.finops.sre.local yes" \
  "list notificationpolicies.finops.sre.local yes" \
  "get secrets yes" \
  "list secrets no"; do
  read -r verb resource expected <<<"$permission"
  if [[ "$resource" == budgetpolicies-status ]]; then
    actual=$(kubectl --context "$context" auth can-i "$verb" budgetpolicies.finops.sre.local --subresource=status --as="$identity" --namespace "$namespace" || true)
  else
    actual=$(kubectl --context "$context" auth can-i "$verb" "$resource" --as="$identity" --namespace "$namespace" || true)
  fi
  assert_equal "RBAC $verb $resource" "$actual" "$expected"
done

printf 'BudgetPolicy -> Analyzer CronJob schedule and runtime configuration passed\n'
