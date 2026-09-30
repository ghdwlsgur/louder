# sre-core Platform Constraints

## 1. Purpose

This document describes the Kubernetes deployment constraints for the `sre-core` platform.

It represents the observed environment as of **2026-09-30**.

Before real deployment, re-check current state:

```bash
python3 scripts/check_cluster.py --context innogrid-core-sre
```

Every direct kubectl command targeting this shared platform must explicitly include:

```bash
--context innogrid-core-sre
```

Disposable local tests may use their explicit kind context (for example, `--context kind-louder-e2e`). Do not use a production context for local E2E work or omit `--context` from kubectl commands.

---

## 2. Platform

| Item | Value |
|---|---|
| Kubernetes | v1.35.5 |
| CRD API | `apiextensions.k8s.io/v1` |
| Runtime | containerd 2.2.x |
| CNI | Cilium + kube-proxy |
| Ingress | ingress-nginx |
| IngressClass | `nginx` |
| Certificates | cert-manager |
| ClusterIssuer | `vault-issuer` |
| Secrets | External Secrets Operator |
| ClusterSecretStore | `vault-backend` |
| Monitoring | kube-prometheus-stack |
| GitOps | ArgoCD |
| Internal registry | `harbor.sre.local` |

The cluster currently has substantial spare CPU and memory capacity.

Do not treat that as permission to omit resource requests/limits.

---

## 3. Topology

The cluster spans two infrastructure domains.

```text
zone=incheon
└── Incheon OpenStack

zone=seoul
└── Seoul on-prem OpenStack
```

Useful labels:

| Label | Values |
|---|---|
| `topology.kubernetes.io/zone` | `incheon`, `seoul` |
| `sre-core/az` | `incheon`, `onprem` |
| `sre-core/host-ip` | physical host IP on on-prem workers |

The on-prem worker VMs are not equal to independent physical failure domains.

Multiple worker VMs may share one physical host.

### Physical-host HA

Do not assume `kubernetes.io/hostname` anti-affinity provides physical-host HA.

Use:

```yaml
topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: sre-core/host-ip
    whenUnsatisfiable: DoNotSchedule
    labelSelector:
      matchLabels:
        app: <your-app>
```

when physical-host separation matters.

Control-plane nodes carry:

```text
node-role.kubernetes.io/control-plane:NoSchedule
```

Ordinary operator/application workloads should not require control-plane tolerations.

---

## 4. Storage

Available StorageClasses:

| StorageClass | Provisioner | Access | Reclaim | Notes |
|---|---|---|---|---|
| `csi-cinder-sc-delete` | Cinder | RWO | Delete | default |
| `csi-cinder-sc-retain` | Cinder | RWO | Retain | retained data |
| `csi-cinder-netapp-iscsi` | Cinder | RWO | Delete | NetApp iSCSI |
| `nfs-seoul` | NFS subdir | RWX | Delete | shared storage |
| `buildcache-seoul` | NFS subdir | RWX | Delete | build cache |

All currently use:

```text
volumeBindingMode: Immediate
allowVolumeExpansion: true
```

### Critical Cinder constraint

Cinder CSI points to the **Incheon OpenStack** environment.

Seoul on-prem workers are VMs from a different OpenStack environment.

The scheduler cannot safely infer this because the Cinder topology label is `zone=nova` across all nodes.

Therefore:

> Any workload using a Cinder PVC must explicitly run in `topology.kubernetes.io/zone=incheon`.

Required pattern:

```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: topology.kubernetes.io/zone
              operator: In
              values:
                - incheon
```

If RWX is required, use NFS.

Never rely on the default StorageClass implicitly for stateful platform components.

Always declare the StorageClass.

---

## 5. Operator deployment

### Admission webhook certificates

If an admission webhook is introduced, prefer the existing cert-manager convention.

ClusterIssuer:

```text
vault-issuer
```

CA injection example:

```yaml
metadata:
  annotations:
    cert-manager.io/inject-ca-from: <namespace>/<certificate-name>
```

Avoid a custom self-managed CA bootstrap unless there is a concrete reason.

### CRD deployment

CRDs may be large enough to hit the legacy last-applied annotation size limit.

For ArgoCD-managed CRDs, use:

```yaml
syncPolicy:
  syncOptions:
    - ServerSideApply=true
    - CreateNamespace=true
```

### RBAC

Prefer namespace-scoped watches when cluster-wide scope is unnecessary.

The cluster already runs several controllers with broad privileges; that is not a reason to add more.

---

## 6. Monitoring

Prometheus selectors require:

```yaml
metadata:
  labels:
    release: monitoring
```

for:

- `ServiceMonitor`
- `PodMonitor`
- `PrometheusRule`

Without this label, the resource may be silently ignored.

Example:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    release: monitoring
```

The current Alertmanager human delivery path uses a null receiver.

Therefore:

- Prometheus is still required for observability
- do not assume a PrometheusRule reaches a human
- Phase 1 human cost notifications go through the project Teams notifier

Recommended metrics:

```text
cloudcost_collection_total
cloudcost_collection_failures_total
cloudcost_collection_duration_seconds
cloudcost_records_collected_total
cloudcost_last_success_timestamp
cloudcost_data_freshness_seconds
cloudcost_notification_total
cloudcost_notification_failures_total
cloudcost_reconcile_errors_total
```

---

## 7. Secret platform

Vault is the source of truth.

ESO is already installed.

ClusterSecretStore:

```text
vault-backend
```

Do not put plaintext Secret manifests in Git.

See:

```text
docs/secrets.md
```

---

## 8. GitOps

ArgoCD is the deployment path.

Sources commonly use:

- internal GitLab repositories
- upstream Helm repositories

Most existing applications are intentionally **manual sync**.

Do not enable:

```yaml
syncPolicy:
  automated:
```

by default.

Changing sync behavior requires explicit approval.

Do not design production deployment around ad-hoc `kubectl apply`.

Direct kubectl is appropriate for:

- read-only inspection
- explicitly approved troubleshooting
- temporary validation

---

## 9. Image policy

Internal registry:

```text
harbor.sre.local
```

Externally hosted images can run, but internally maintained production images should be published or mirrored to Harbor.

Runtime containers should:

- run as non-root where practical
- use explicit resource requests/limits
- avoid unnecessary Linux capabilities
- use a read-only root filesystem where practical
- define probes/health endpoints where relevant
- avoid debug/tooling bloat in production images

The cluster currently has no Pod Security Admission labels.

Do not treat that as permission to run privileged pods.

Security context must be explicit.

---

## 10. Network

The CNI is Cilium.

NetworkPolicy usage is currently limited, so the cluster is broadly open between pods.

Do not introduce a namespace-wide default-deny policy as an accidental side effect of this project.

If policy is required:

- prefer standard Kubernetes `NetworkPolicy`
- preserve DNS
- preserve ClickHouse connectivity
- preserve CSP public API egress
- preserve Teams egress
- preserve ESO/Vault behavior

Use Cilium-specific policy only when standard NetworkPolicy is insufficient.

---

## 11. Existing CRD groups

Known groups include:

```text
generators.external-secrets.io
monitoring.coreos.com
cilium.io
external-secrets.io
cert-manager.io
argoproj.io
acme.cert-manager.io
bitnami.com
```

Before introducing a CRD group or Kind, verify it does not conflict.

Preferred project API group:

```text
finops.sre.local
```

Example:

```text
finops.sre.local/v1alpha1
```

---

## 12. Known operational hazards

### Single replicas

Several existing workloads run with one replica.

Do not copy this pattern without evaluating availability requirements.

### New-node drift

Some node bootstrap details may drift from current cluster runtime configuration.

Do not assume newly added nodes perfectly match existing nodes.

### Mixed OS

The cluster contains both:

```text
Ubuntu 24.04
Rocky Linux 9.6 / 9.7
```

Avoid host-kernel or host-path assumptions unless verified on both environments.

### Alertmanager null receiver

PrometheusRule does not imply human notification.

### kubelet configuration

Do not rely on manual node edits that may be overwritten by kubeadm-managed configuration.

---

## 13. Deployment checklist

Before proposing or applying manifests:

```text
[ ] Every kubectl command uses --context innogrid-core-sre
[ ] Current cluster state was re-checked
[ ] StorageClass is explicit
[ ] Cinder workloads are pinned to zone=incheon
[ ] RWX workloads use NFS
[ ] Physical-host HA uses sre-core/host-ip where needed
[ ] ServiceMonitor/PodMonitor/PrometheusRule include release=monitoring
[ ] Secret material is not in Git
[ ] Operator references K8s Secret, not Vault directly
[ ] ArgoCD CRDs use ServerSideApply=true
[ ] automated sync was not enabled without approval
[ ] control-plane tolerations were not added unnecessarily
[ ] resource requests/limits are set
[ ] securityContext is explicit
[ ] internally maintained images are published to harbor.sre.local
```
