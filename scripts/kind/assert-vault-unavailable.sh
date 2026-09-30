#!/usr/bin/env bash
set -euo pipefail

context=kind-louder-e2e
namespace=cloud-cost
external_secret=aws-kind-e2e-vault-outage

kubectl --context "$context" apply -f config/kind/secret-flow/external-secret-vault-outage.yaml
if kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=8s \
  --namespace "$namespace" \
  "externalsecret/$external_secret" >/dev/null 2>&1; then
  printf 'Vault is reachable; the outage assertion is expected to fail\n' >&2
  exit 1
fi

if ! kubectl --context "$context" wait \
  --for=condition=Ready=False \
  --timeout=30s \
  --namespace "$namespace" \
  "externalsecret/$external_secret" >/dev/null 2>&1; then
  printf 'ExternalSecret did not report Ready=False while Vault was expected to be unavailable\n' >&2
  exit 1
fi

reason=$(kubectl --context "$context" get \
  --namespace "$namespace" \
  "externalsecret/$external_secret" \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}')

if [[ "$reason" != SecretSyncedError ]]; then
  printf 'Expected SecretSyncedError during Vault outage, got %q\n' "$reason" >&2
  exit 1
fi

printf 'ExternalSecret reported SecretSyncedError while the Vault service had no ready endpoints\n'
