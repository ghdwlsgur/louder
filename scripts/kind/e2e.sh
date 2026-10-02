#!/usr/bin/env bash
set -euo pipefail

cluster_name="${KIND_CLUSTER_NAME:-louder-e2e}"
context="kind-$cluster_name"
export KIND_CLUSTER_NAME="$cluster_name"
image=louder-operator:local
mode="${1:-smoke}"
port_forward_pid=""

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
  storage)
    if ! command -v openssl >/dev/null 2>&1; then
      printf 'Required command not found: openssl\n' >&2
      exit 1
    fi
    ;;
  *)
    printf 'Unknown E2E mode: %s (expected smoke, secrets, or storage)\n' "$mode" >&2
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
  if [[ -n "$port_forward_pid" ]]; then
    kill "$port_forward_pid" 2>/dev/null || true
    wait "$port_forward_pid" 2>/dev/null || true
  fi
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
kubectl --context "$context" wait \
  --for=condition=Established \
  --timeout=60s \
  crd/budgetpolicies.finops.sre.local
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

if [[ "$mode" == storage ]]; then
  clickhouse_password=$(openssl rand -hex 24)
  kubectl --context "$context" create secret generic louder-clickhouse-credentials \
    --namespace cloud-cost \
    --from-literal=CLICKHOUSE_ADDR=clickhouse.cloud-cost.svc:9000 \
    --from-literal=CLICKHOUSE_DATABASE=finops \
    --from-literal=CLICKHOUSE_USERNAME=louder \
    --from-literal="CLICKHOUSE_PASSWORD=$clickhouse_password" \
    --from-literal=CLICKHOUSE_USER=louder \
    --from-literal=CLICKHOUSE_DB=finops \
    --dry-run=client -o yaml | kubectl --context "$context" apply -f -
  kubectl --context "$context" apply -f config/kind/clickhouse.yaml
  kubectl --context "$context" rollout status --timeout=180s --namespace cloud-cost deployment/clickhouse
  kubectl --context "$context" exec -i --namespace cloud-cost deployment/clickhouse -- \
    sh -c 'clickhouse-client --user="$CLICKHOUSE_USER" --password="$CLICKHOUSE_PASSWORD" --database="$CLICKHOUSE_DB" --multiquery' \
    < config/storage/clickhouse/cost_records.sql
  bash scripts/kind/assert-cloudaccount-missing-secret.sh
  bash scripts/kind/assert-collector-cronjob.sh
  COLLECTOR_JOB_NAME=kind-fixture-collector-replay bash scripts/kind/assert-collector-cronjob.sh
  bash scripts/kind/assert-ncp-monthly-collector.sh
  bash scripts/kind/assert-clickhouse-storage.sh
  bash scripts/kind/assert-budgetpolicy-cronjob.sh
  port=19000
  kubectl --context "$context" port-forward --address 127.0.0.1 --namespace cloud-cost service/clickhouse "$port:9000" >/dev/null 2>&1 &
  port_forward_pid=$!
  for attempt in $(seq 1 30); do
    if (echo >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  if ! (echo >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1; then
    printf 'ClickHouse port-forward did not become ready\n' >&2
    exit 1
  fi
  CLICKHOUSE_INTEGRATION=1 \
    CLICKHOUSE_ADDR="127.0.0.1:$port" \
    CLICKHOUSE_DATABASE=finops \
    CLICKHOUSE_USERNAME=louder \
  CLICKHOUSE_PASSWORD="$clickhouse_password" \
    GOCACHE="${GOCACHE:-/tmp/louder-go-build}" \
    go test ./internal/storage/clickhouse -run 'TestNativeReaderReturns(NormalizedFinalRecord|MonthlyNCPInvoicePeriod)' -count=1
  CLICKHOUSE_INTEGRATION=1 \
    CLICKHOUSE_ADDR="127.0.0.1:$port" \
    CLICKHOUSE_DATABASE=finops \
    CLICKHOUSE_USERNAME=louder \
    CLICKHOUSE_PASSWORD="$clickhouse_password" \
    GOCACHE="${GOCACHE:-/tmp/louder-go-build}" \
    go test ./internal/analyzer -run 'TestStored(DailyAnomaly|MonthlyNCPBudget|Budget)ProducesFakeNotifierIntents' -count=1
  unset clickhouse_password
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
