# Getting Started

A CAPCS cluster has two parts: a **management cluster** running CAPCS and core
Cluster API controllers, and a **workload cluster** whose servers, network,
and load balancer CAPCS provisions on cloudscale.ch. Once the workload cluster
comes up you install a CCM, a CNI, and (optionally) a CSI driver into it.

For Cluster API fundamentals (concepts, `clusterctl`, upgrades) see the
[upstream documentation](https://cluster-api.sigs.k8s.io/) — this guide only
covers what is cloudscale-specific.

## Contents

- [Prerequisites](#prerequisites)
- [1. Build and import a custom OS image](#1-build-and-import-a-custom-os-image)
- [2. Create a management cluster](#2-create-a-management-cluster)
- [3. Configure cloudscale credentials](#3-configure-cloudscale-credentials)
- [4. Install CAPCS on the management cluster](#4-install-capcs-on-the-management-cluster)
- [5. Configure template variables](#5-configure-template-variables)
- [6. Pick a cluster template flavor](#6-pick-a-cluster-template-flavor)
- [7. Generate and apply the workload cluster](#7-generate-and-apply-the-workload-cluster)
- [8. Install the cloudscale CCM](#8-install-the-cloudscale-ccm)
- [9. Install a CNI (Cilium)](#9-install-a-cni-cilium)
- [10. Verify](#10-verify)
- [Optional: persistent storage (cloudscale CSI)](#optional-persistent-storage-cloudscale-csi)
- [Clean up](#clean-up)
- [Next steps](#next-steps)

## Prerequisites

- A cloudscale.ch account and an API token with read/write scope from the
  [control panel](https://control.cloudscale.ch/). Keep it out of version
  control.
- [`clusterctl`](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl) in version >= 1.13.0.
- `kubectl`
- The [`cilium` CLI](https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default/)
  (used in step 9) or use another CNI you prefer.
- `helm` for the optional CSI driver.

## 1. Build and import a custom OS image

CAPCS does not publish a pre-built image. Build one with
[image-builder for OpenStack](https://image-builder.sigs.k8s.io/capi/providers/openstack)
and upload it to your cloudscale.ch project via the control panel or API.

The image bakes in `kubeadm`, `kubelet`, and the container runtime — its
Kubernetes minor version must match the `KUBERNETES_VERSION` you later pass to
`clusterctl generate cluster`.

Once imported, note the exact image name. You pass it as
`CLOUDSCALE_MACHINE_IMAGE` with a `custom:` prefix, e.g.
`custom:ubuntu-2404-kube-v1.36.0`.

## 2. Create a management cluster

Any conformant Kubernetes cluster works. Local [kind](https://kind.sigs.k8s.io/)
is the simplest:

```bash
kind create cluster --name capcs-mgmt
```

## 3. Configure cloudscale credentials

`clusterctl` resolves template variables from either the shell environment or
its config file at `~/.config/cluster-api/clusterctl.yaml`. Pick one:

**Shell:**

```bash
export CLOUDSCALE_API_TOKEN=<your-api-token>
```

**Config file** (`~/.config/cluster-api/clusterctl.yaml`):

```yaml
CLOUDSCALE_API_TOKEN: <your-api-token>
# any other CLOUDSCALE_* variable from step 5 can live here too
```

See [`clusterctl` configuration](https://cluster-api.sigs.k8s.io/clusterctl/configuration)
for the full format.

## 4. Install CAPCS on the management cluster

If you plan to use the `topology` flavor, export `CLUSTER_TOPOLOGY=true` first
so cluster-api core enables the
[ClusterClass feature gate](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/).

```bash
clusterctl init --infrastructure cloudscale-ch-cloudscale
```

This installs the Cluster API core, kubeadm bootstrap, kubeadm control plane,
and CAPCS components.

## 5. Configure template variables

`clusterctl generate cluster` substitutes these into the chosen template:

| Variable                                  | Description                                           | Example                           |
|-------------------------------------------|-------------------------------------------------------|-----------------------------------|
| `CLOUDSCALE_API_TOKEN`                    | API token used by the workload cluster's CAPCS Secret | `abc123...`                       |
| `CLOUDSCALE_REGION`                       | cloudscale.ch region                                  | `lpg` or `rma`                    |
| `CLOUDSCALE_MACHINE_IMAGE`                | Name of your imported custom image                    | `custom:ubuntu-2404-kube-v1.36.0` |
| `CLOUDSCALE_CONTROL_PLANE_MACHINE_FLAVOR` | Flavor for control plane nodes                        | `flex-4-2`                        |
| `CLOUDSCALE_WORKER_MACHINE_FLAVOR`        | Flavor for worker nodes                               | `flex-4-2`                        |
| `CLOUDSCALE_ROOT_VOLUME_SIZE`             | Root volume size in GB                                | `50`                              |
| `CLOUDSCALE_SSH_PUBLIC_KEY`               | SSH public key added to every node                    | `ssh-ed25519 AAAA...`             |
| `CLOUDSCALE_NETWORK_UUID`                 | Pre-existing network UUID (depends on flavor)         | `2db69ba3-...`                    |

Set them in your shell, or keep them in `clusterctl.yaml` alongside the token.

## 6. Pick a cluster template flavor

| Flavor                    | Network                  | Control plane endpoint | Node connectivity | Requires                                             |
|---------------------------|--------------------------|------------------------|-------------------|------------------------------------------------------|
| *(default)*               | Managed, `172.18.0.0/24` | Public LB, DualStack   | Public + cluster  | —                                                    |
| `fip`                     | Pre-existing             | Floating IP, IPv4      | Public + cluster  | `CLOUDSCALE_NETWORK_UUID`                            |
| `pre-existing-network`    | Pre-existing             | Public LB, DualStack   | Public + cluster  | `CLOUDSCALE_NETWORK_UUID`                            |
| `public-lb-private-nodes` | Pre-existing + NAT       | Public LB              | Private only      | `CLOUDSCALE_NETWORK_UUID`, with a NAT gateway set up |
| `router-nat`              | Managed + managed router | Public LB              | Private only      | —                                                    |
| `topology`                | Managed, `172.18.0.0/24` | Public LB, DualStack   | Public + cluster  | `CLUSTER_TOPOLOGY=true` feature gate                 |
| `topology-router-nat`     | Managed + managed router | Public LB              | Private only      | `CLUSTER_TOPOLOGY=true` feature gate                 |

These flavors just show possible configurations. You're encouraged to copy and adjust them to your needs.

If you do need a NAT gateway set up for your cluster, set up a private network and contact cloudscale support with the
following details:

- Desired name of the gateway
- UUID of the network (in control panel details)
- Optional: desired IP of the gateway in the network

### Using the topology flavor

The `topology` flavor generates a `Cluster` that references the `quick-start`
[ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
shipped in `templates/cluster-class.yaml`. Apply it once per namespace before
generating any topology cluster:

```bash
clusterctl generate yaml \
  --from https://raw.githubusercontent.com/cloudscale-ch/cluster-api-provider-cloudscale/main/templates/cluster-class.yaml \
  | kubectl apply -f -
```

Per-cluster overrides go under `spec.topology.variables`. See `templates/cluster-class.yaml` for the full
variable list and defaults.

Caveat: the `quick-start` ClusterClass exposes a narrower configurability than the traditional templates — no floating
IP, no pre-existing network, no private load balancer, and pod/service CIDRs are pinned to
`192.168.0.0/16` / `10.96.0.0/12`. Adjust the template for your use-case.

Use `--flavor topology` in the next step if you did use ClusterClass.

### Using the router-nat ClusterClass

The `topology-router-nat` flavor is the ClusterClass equivalent of `router-nat`: nodes have
no public interface and reach the internet only through a managed router doing SNAT. It
references the separate `router-nat` ClusterClass in `templates/cluster-class-router-nat.yaml`,
which must likewise be applied once per namespace:

```bash
clusterctl generate yaml \
  --from https://raw.githubusercontent.com/cloudscale-ch/cluster-api-provider-cloudscale/main/templates/cluster-class-router-nat.yaml \
  | kubectl apply -f -
```

On top of the `quick-start` variables it adds three:

| Variable      | Default        | Purpose                                                                                                                                                                      |
|---------------|----------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `networkName` | `node-net`     | Logical name of the cluster's private network. Patched into the router interface and into every machine's interface list, so it stays consistent across the three templates. |
| `networkCIDR` | `10.10.0.0/24` | Subnet of that network. The router interface address is derived from it per cluster, so changing the CIDR moves the address with it.                                         |
| `routerName`  | `router`       | Name of the managed router.                                                                                                                                                  |

The shipped `topology-router-nat` template sets `networkName` and `routerName` from the
cluster name, so clusters sharing the class stay distinguishable in the cloudscale.ch control
panel. Give each cluster its own `networkCIDR` if you intend to route between them.

Caveat: these three variables and the router's `internetGateway` are **create-time only**.
Networks and routers are immutable once a cluster exists, so changing them on a live `Cluster`
leaves it with `TopologyReconciled=False` and a `Forbidden` error from the CAPCS admission
webhook. Create a new cluster instead.

Use `--flavor topology-router-nat` in the next step for this variant.

## 7. Generate and apply the workload cluster

Make sure to adjust the `--flavor` flag to the right flavor or omit it for the default template.

```bash
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale \
  --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 \
  --flavor <name or omit> \
  > my-cluster.yaml

kubectl apply -f my-cluster.yaml
```

Inspect `my-cluster.yaml` before
applying — it includes a Secret holding `CLOUDSCALE_API_TOKEN`, which CAPCS
references via `CloudscaleCluster.spec.credentialsRef`.

Watch progress:

```bash
clusterctl describe cluster my-cluster
```

## 8. Install the cloudscale CCM

CAPCS only provisions infrastructure. For a working cluster
the [cloudscale CCM](https://github.com/cloudscale-ch/cloudscale-cloud-controller-manager) must be installed on the
**workload** cluster.

CAPCS provides a [`ClusterResourceSet` you apply on the **management** cluster](https://cluster-api.sigs.k8s.io/tasks/cluster-resource-set).
CAPI's CRS controller then deploys it into any workload cluster labelled `ccm: cloudscale` — every CAPCS template sets this label.

**N.B.:** the CCM also needs a CLOUDSCALE_API_TOKEN in the environment set. It's recommended to use a different token for the CCM and CAPCS.

```bash
# the namespace must match the namespace of the workload cluster definition
curl -L https://raw.githubusercontent.com/cloudscale-ch/cluster-api-provider-cloudscale/main/templates/addons/ccm.yaml \
  | envsubst | kubectl apply -n ${NAMESPACE} -f -
```

This creates a ConfigMap with the CCM manifests, a Secret with the API token,
and the `ClusterResourceSet` that wires them together. Workload-cluster nodes
will sit tainted with `node.cloudprovider.kubernetes.io/uninitialized` until
the CCM starts and removes the taint — that is expected.

## 9. Install a CNI (Cilium)

Fetch the workload cluster's kubeconfig and point `KUBECONFIG` at it:

```bash
clusterctl get kubeconfig my-cluster > ~/.kube/my-cluster.yaml
export KUBECONFIG=~/.kube/my-cluster.yaml
```

Install Cilium with defaults:

```bash
cilium install
```

Nodes become `Ready` once Cilium is up. Any standard CNI works — Cilium is just
the example.

## 10. Verify

```bash
# management cluster
clusterctl describe cluster <name>

# workload cluster
kubectl get nodes
kubectl -n kube-system get pods
```

All nodes should report `Ready`. In `kube-system` you should see
`cloudscale-cloud-controller-manager-*` on each control plane node and
`cilium-*` on every node.

If something is stuck, see [troubleshooting](troubleshooting.md).

## Optional: persistent storage (cloudscale CSI)

For `PersistentVolumeClaim` support, install the
[cloudscale CSI driver](https://github.com/cloudscale-ch/csi-cloudscale) on the
**workload** cluster. With `KUBECONFIG` still pointing at the workload cluster:

```bash
kubectl -n kube-system create secret generic cloudscale \
  --from-literal=access-token="$CLOUDSCALE_API_TOKEN"

helm repo add csi-cloudscale https://cloudscale-ch.github.io/csi-cloudscale
helm repo update
helm install csi-cloudscale csi-cloudscale/csi-cloudscale -n kube-system
```

The chart ships the `cloudscale-volume-ssd` (default) and
`cloudscale-volume-bulk` storage classes, plus LUKS-encrypted variants. See
the upstream repo for version compatibility and storage class options.

## Clean up

```bash
kubectl delete cluster my-cluster
```

Deleting the `Cluster` cascades through CAPCS, which removes the servers, load
balancer, floating IPs, server groups, and any managed networks it created.
Pre-existing networks supplied via `CLOUDSCALE_NETWORK_UUID` are left intact.

**N.B.:** Always delete the cluster. Do not delete other custom resources before the cluster is deleted.

## Next steps

- Look up CRD fields with `kubectl explain cloudscalecluster.spec` (or browse
  the CRDs in [`config/crd/bases/`](../config/crd/bases))
- Read the [troubleshooting guide](troubleshooting.md) when something gets
  stuck
- Upstream Cluster API tasks (upgrades, scaling, MachineHealthChecks, etc.)
  are documented at <https://cluster-api.sigs.k8s.io/tasks/>
