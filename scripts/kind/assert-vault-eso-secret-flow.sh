#!/usr/bin/env bash
set -euo pipefail

context="kind-${KIND_CLUSTER_NAME:-louder-e2e}"
namespace=cloud-cost
external_secret=aws-kind-e2e-credentials
cloud_account=kind-smoke-vault-eso

kubectl --context "$context" apply -f config/kind/secret-flow/cluster-secret-store.yaml
kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=120s \
  clustersecretstore/vault-kind

kubectl --context "$context" apply -f config/kind/fixtures/cloudaccount-vault-eso.yaml
initial_reason=
for attempt in $(seq 1 30); do
  initial_reason=$(kubectl --context "$context" get \
    -n "$namespace" \
    "cloudaccount/$cloud_account" \
    -o jsonpath='{.status.conditions[?(@.type=="CredentialsReady")].reason}')
  if [[ "$initial_reason" == SecretNotFound ]]; then
    break
  fi
  sleep 1
done
if [[ "$initial_reason" != SecretNotFound ]]; then
  printf 'Expected initial CredentialsReady reason SecretNotFound, got %q\n' "$initial_reason" >&2
  exit 1
fi

kubectl --context "$context" apply -f config/kind/secret-flow/external-secret.yaml
kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=120s \
  -n "$namespace" \
  "externalsecret/$external_secret"

access_key_id=$(kubectl --context "$context" get \
  -n "$namespace" \
  secret/aws-kind-e2e-credentials \
  -o jsonpath='{.data.AWS_ACCESS_KEY_ID}')
secret_access_key=$(kubectl --context "$context" get \
  -n "$namespace" \
  secret/aws-kind-e2e-credentials \
  -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}')

if [[ -z "$access_key_id" || -z "$secret_access_key" ]]; then
  printf 'ESO did not create both expected credential keys\n' >&2
  exit 1
fi
unset access_key_id secret_access_key

if ! kubectl --context "$context" wait \
  --for=condition=CredentialsReady=True \
  --timeout=60s \
  -n "$namespace" \
  "cloudaccount/$cloud_account"; then
  reason=$(kubectl --context "$context" get \
    -n "$namespace" \
    "cloudaccount/$cloud_account" \
    -o jsonpath='{.status.conditions[?(@.type=="CredentialsReady")].reason}')
  printf 'CloudAccount did not observe the newly created credential Secret; reason=%q\n' "$reason" >&2
  exit 1
fi

reason=$(kubectl --context "$context" get \
  -n "$namespace" \
  "cloudaccount/$cloud_account" \
  -o jsonpath='{.status.conditions[?(@.type=="CredentialsReady")].reason}')

if [[ "$reason" != SecretFound ]]; then
  printf 'Expected CredentialsReady reason SecretFound, got %q\n' "$reason" >&2
  exit 1
fi

printf 'Vault -> ESO -> Kubernetes Secret -> CloudAccount passed\n'
