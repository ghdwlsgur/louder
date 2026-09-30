#!/usr/bin/env bash
set -euo pipefail

cluster_name=louder-e2e
context="kind-$cluster_name"
image=louder-operator:local

for tool in docker kind kubectl; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$tool" >&2
    exit 1
  fi
done

if ! docker info >/dev/null 2>&1; then
  printf 'Docker daemon is unavailable\n' >&2
  exit 1
fi

if kind get clusters | grep -Fxq "$cluster_name"; then
  printf 'Refusing to replace existing kind cluster %s\n' "$cluster_name" >&2
  exit 1
fi

kind create cluster \
  --name "$cluster_name" \
  --config config/kind/cluster.yaml \
  --wait 120s

cleanup() {
  kind delete cluster --name "$cluster_name" --quiet
}
trap cleanup EXIT

kind load docker-image "$image" --name "$cluster_name"
kubectl --context "$context" apply -f config/crd/bases
kubectl --context "$context" wait \
  --for=condition=Established \
  --timeout=60s \
  crd/cloudaccounts.finops.sre.local
kubectl --context "$context" apply -f config/kind/operator.yaml
kubectl --context "$context" rollout status \
  --timeout=120s \
  -n cloud-cost \
  deployment/louder-operator

bash scripts/kind/assert-cloudaccount-missing-secret.sh
