#!/usr/bin/env bash
set -euo pipefail

context=kind-louder-e2e
namespace=cloud-cost

if red_output=$(bash scripts/kind/assert-vault-unavailable.sh 2>&1); then
  printf 'Expected the Vault outage assertion to fail while Vault is healthy\n' >&2
  exit 1
fi
if [[ "$red_output" != *'Vault is reachable; the outage assertion is expected to fail'* ]]; then
  printf 'Outage assertion did not fail because Vault was healthy\n%s\n' "$red_output" >&2
  exit 1
fi
printf 'RED: outage assertion fails while Vault is healthy\n'
kubectl --context "$context" delete \
  --namespace "$namespace" \
  externalsecret/aws-kind-e2e-vault-outage \
  --ignore-not-found
kubectl --context "$context" scale deployment/vault \
  --namespace vault-system \
  --replicas=0
kubectl --context "$context" wait \
  --for=delete \
  --timeout=60s \
  --namespace vault-system \
  pods \
  -l app.kubernetes.io/name=vault

for ((attempt = 0; attempt < 30; attempt++)); do
  vault_endpoints=$(kubectl --context "$context" get endpoints vault \
    --namespace vault-system \
    -o jsonpath='{.subsets[*].addresses[*].ip}')
  if [[ -z "$vault_endpoints" ]]; then
    break
  fi
  sleep 1
done
if [[ -n "$vault_endpoints" ]]; then
  printf 'Vault service still has ready endpoints after scale-down\n' >&2
  exit 1
fi
bash scripts/kind/assert-vault-unavailable.sh

kubectl --context "$context" scale deployment/vault \
  --namespace vault-system \
  --replicas=1
kubectl --context "$context" rollout status \
  --timeout=120s \
  --namespace vault-system \
  deployment/vault

access_key_id="fixture-$(openssl rand -hex 16)"
secret_access_key=$(openssl rand -hex 32)
kubectl --context "$context" exec \
  --namespace vault-system \
  deployment/vault -- \
  vault kv put secret/finops/aws/kind \
  "access_key_id=$access_key_id" \
  "secret_access_key=$secret_access_key" >/dev/null
unset access_key_id secret_access_key
kubectl --context "$context" exec -i \
  --namespace vault-system \
  deployment/vault -- \
  vault policy write kind-eso-read - <<'POLICY'
path "secret/data/finops/aws/kind" {
  capabilities = ["read"]
}
POLICY
eso_token=$(kubectl --context "$context" exec \
  --namespace vault-system \
  deployment/vault -- \
  vault token create -policy=kind-eso-read -ttl=15m -field=token)
kubectl --context "$context" create secret generic vault-eso-token \
  --namespace vault-system \
  --from-literal="token=$eso_token" \
  --dry-run=client \
  -o yaml | kubectl --context "$context" apply -f -
unset eso_token

kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=120s \
  --namespace "$namespace" \
  externalsecret/aws-kind-e2e-vault-outage

recovered_access_key_id=$(kubectl --context "$context" get \
  --namespace "$namespace" \
  secret/aws-kind-e2e-outage-credentials \
  -o jsonpath='{.data.access-key-id}')
recovered_secret_access_key=$(kubectl --context "$context" get \
  --namespace "$namespace" \
  secret/aws-kind-e2e-outage-credentials \
  -o jsonpath='{.data.secret-access-key}')
if [[ -z "$recovered_access_key_id" || -z "$recovered_secret_access_key" ]]; then
  printf 'ExternalSecret did not restore both credential keys after Vault recovery\n' >&2
  exit 1
fi
unset recovered_access_key_id recovered_secret_access_key
printf 'GREEN: ExternalSecret recovered after Vault returned\n'
