#!/usr/bin/env bash
set -euo pipefail

context="kind-${KIND_CLUSTER_NAME:-louder-e2e}"
namespace=cloud-cost
name=kind-smoke-missing-secret

kubectl --context "$context" apply -f config/kind/fixtures/cloudaccount-missing-secret.yaml
kubectl --context "$context" wait \
  --for=condition=CredentialsReady=False \
  --timeout=60s \
  -n "$namespace" \
  "cloudaccount/$name"
kubectl --context "$context" wait \
  --for=condition=CollectorReady=False \
  --timeout=60s \
  -n "$namespace" \
  "cloudaccount/$name"

reason=$(kubectl --context "$context" get \
  -n "$namespace" \
  "cloudaccount/$name" \
  -o jsonpath='{.status.conditions[?(@.type=="CredentialsReady")].reason}')

if [[ "$reason" != SecretNotFound ]]; then
  printf 'Expected CredentialsReady reason SecretNotFound, got %q\n' "$reason" >&2
  exit 1
fi

collector_reason=$(kubectl --context "$context" get \
  -n "$namespace" \
  "cloudaccount/$name" \
  -o jsonpath='{.status.conditions[?(@.type=="CollectorReady")].reason}')

if [[ "$collector_reason" != SecretNotFound ]]; then
  printf 'Expected CollectorReady reason SecretNotFound, got %q\n' "$collector_reason" >&2
  exit 1
fi

printf 'CloudAccount reported CredentialsReady=False and CollectorReady=False (SecretNotFound)\n'
