#!/usr/bin/env bash
set -euo pipefail

context=kind-louder-e2e
namespace=cloud-cost
external_secret=aws-kind-e2e-credentials
cloud_account=kind-smoke-vault-eso

kubectl --context "$context" apply -f config/kind/secret-flow/cluster-secret-store.yaml
kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=120s \
  clustersecretstore/vault-kind

kubectl --context "$context" apply -f config/kind/secret-flow/external-secret.yaml
kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=120s \
  -n "$namespace" \
  "externalsecret/$external_secret"

access_key_id=$(kubectl --context "$context" get \
  -n "$namespace" \
  secret/aws-kind-e2e-credentials \
  -o jsonpath='{.data.access-key-id}')
secret_access_key=$(kubectl --context "$context" get \
  -n "$namespace" \
  secret/aws-kind-e2e-credentials \
  -o jsonpath='{.data.secret-access-key}')

if [[ -z "$access_key_id" || -z "$secret_access_key" ]]; then
  printf 'ESO did not create both expected credential keys\n' >&2
  exit 1
fi
unset access_key_id secret_access_key

kubectl --context "$context" apply -f config/kind/fixtures/cloudaccount-vault-eso.yaml
kubectl --context "$context" wait \
  --for=condition=CredentialsReady=True \
  --timeout=60s \
  -n "$namespace" \
  "cloudaccount/$cloud_account"

reason=$(kubectl --context "$context" get \
  -n "$namespace" \
  "cloudaccount/$cloud_account" \
  -o jsonpath='{.status.conditions[?(@.type=="CredentialsReady")].reason}')

if [[ "$reason" != SecretFound ]]; then
  printf 'Expected CredentialsReady reason SecretFound, got %q\n' "$reason" >&2
  exit 1
fi

printf 'Vault -> ESO -> Kubernetes Secret -> CloudAccount passed\n'
