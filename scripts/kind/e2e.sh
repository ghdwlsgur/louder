#!/usr/bin/env bash
set -euo pipefail

cluster_name="${KIND_CLUSTER_NAME:-louder-e2e}"
context="kind-$cluster_name"
export KIND_CLUSTER_NAME="$cluster_name"
image=louder-operator:local
mode="${1:-smoke}"

case "$mode" in
  smoke) ;;
  secrets)
    for tool in helm openssl; do
      if ! command -v "$tool" >/dev/null 2>&1; then
        printf 'Required command not found: %s\n' "$tool" >&2
        exit 1
      fi
    done
    ;;
  *)
    printf 'Unknown E2E mode: %s (expected smoke or secrets)\n' "$mode" >&2
    exit 1
    ;;
esac

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
  status=$?
  if [[ "$status" -ne 0 && "$mode" == secrets ]]; then
    kubectl --context "$context" get pods --all-namespaces -o wide || true
    kubectl --context "$context" describe deployment vault --namespace vault-system || true
    kubectl --context "$context" describe pods --namespace vault-system -l app.kubernetes.io/name=vault || true
  fi
  kind delete cluster --name "$cluster_name" --quiet
  return "$status"
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

if [[ "$mode" == smoke ]]; then
  bash scripts/kind/assert-cloudaccount-missing-secret.sh
  bash scripts/kind/assert-collector-cronjob.sh
  exit
fi

kubectl --context "$context" create namespace vault-system
vault_root_token=$(openssl rand -hex 32)
kubectl --context "$context" create secret generic vault-bootstrap-token \
  --namespace vault-system \
  --from-literal="token=$vault_root_token" \
  --dry-run=client \
  -o yaml | kubectl --context "$context" apply -f -
unset vault_root_token

kubectl --context "$context" apply -f config/kind/vault.yaml
kubectl --context "$context" rollout status \
  --timeout=120s \
  --namespace vault-system \
  deployment/vault

helm repo add external-secrets https://charts.external-secrets.io --force-update
helm repo update external-secrets
helm upgrade --install external-secrets external-secrets/external-secrets \
  --kube-context "$context" \
  --namespace external-secrets \
  --create-namespace \
  --version 2.11.0 \
  --wait \
  --timeout 5m

kubectl --context "$context" wait \
  --for=condition=Established \
  --timeout=120s \
  crd/clustersecretstores.external-secrets.io \
  crd/externalsecrets.external-secrets.io

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

kubectl --context "$context" apply -f config/kind/secret-flow/cluster-secret-store.yaml
kubectl --context "$context" wait \
  --for=condition=Ready=True \
  --timeout=120s \
  clustersecretstore/vault-kind
kubectl --context "$context" apply -f config/kind/secret-flow/external-secret.yaml
kubectl --context "$context" wait \
  --for=condition=Ready=False \
  --timeout=30s \
  --namespace cloud-cost \
  externalsecret/aws-kind-e2e-credentials
red_reason=$(kubectl --context "$context" get \
  --namespace cloud-cost \
  externalsecret/aws-kind-e2e-credentials \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}')
if [[ "$red_reason" != SecretSyncedError ]]; then
  printf 'Expected missing Vault data to fail with SecretSyncedError, got %q\n' "$red_reason" >&2
  exit 1
fi
printf 'RED: ExternalSecret reports SecretSyncedError while the Vault path is absent\n'
kubectl --context "$context" delete \
  --namespace cloud-cost \
  externalsecret/aws-kind-e2e-credentials \
  --ignore-not-found

access_key_id="fixture-$(openssl rand -hex 16)"
secret_access_key=$(openssl rand -hex 32)
kubectl --context "$context" exec \
  --namespace vault-system \
  deployment/vault -- \
  vault kv put secret/finops/aws/kind \
  "access_key_id=$access_key_id" \
  "secret_access_key=$secret_access_key" >/dev/null
unset access_key_id secret_access_key

bash scripts/kind/assert-vault-eso-secret-flow.sh
bash scripts/kind/run-vault-outage-scenario.sh
