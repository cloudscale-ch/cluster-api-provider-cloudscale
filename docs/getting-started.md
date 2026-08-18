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

| Flavor                    | Network      | Router/sNAT  | Control plane endpoint | Node connectivity | Requires                                             |
|---------------------------|--------------|--------------|------------------------|-------------------|------------------------------------------------------|
| *(default)*               | Managed      | not required | Public LB, DualStack   | Public + cluster  | —                                                    |
| `fip`                     | Pre-existing | not required | Floating IP, IPv4      | Public + cluster  | `CLOUDSCALE_NETWORK_UUID`                            |
| `pre-existing-network`    | Pre-existing | not required | Public LB, DualStack   | Public + cluster  | `CLOUDSCALE_NETWORK_UUID`                            |
| `public-lb-private-nodes` | Pre-existing | pre-existing | Public LB              | Private only      | `CLOUDSCALE_NETWORK_UUID`, with a NAT gateway set up |
| `router-nat`              | Managed      | managed      | Public LB              | Private only      | —                                                    |
| `topology`                | Managed      | not required | Public LB, DualStack   | Public + cluster  | `CLUSTER_TOPOLOGY=true` feature gate                 |
| `topology-router-nat`     | Managed      | managed      | Public LB              | Private only      | `CLUSTER_TOPOLOGY=true` feature gate                 |

These flavors just show possible configurations. You're encouraged to copy and adjust them to your needs.

## Using Templates with Topology/ClusterClass

The flavors with a `topology` prefix generate a `Cluster` that references a
[ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/) in `spec.topology.classRef`.

To use a topology flavor, you first need to apply its referenced ClusterClass:

```bash
# adjust CLUSTER_CLASS_FLAVOR to the respective cluster class yaml file.
clusterctl generate yaml \
  --from https://raw.githubusercontent.com/cloudscale-ch/cluster-api-provider-cloudscale/main/templates/{CLUSTER_CLASS_FLAVOR}.yaml \
  | kubectl apply -f -
```

Per-cluster overrides go under `spec.topology.variables`. The available variables depend on which ClusterClass flavor you use.

**Common variables** (both `topology` and `topology-router-nat`):

| Variable                                      | Type             | Required | Default                           | Description                                                                          |
|-----------------------------------------------|------------------|----------|-----------------------------------|--------------------------------------------------------------------------------------|
| `region`                                      | string           | yes      | `lpg`                             | cloudscale region (`lpg` or `rma`)                                                   |
| `credentialsRefName`                          | string           | yes      | —                                 | Name of the Secret holding API credentials (must match the Secret created in step 7) |
| `cloudscaleControlPlaneMachineFlavor`         | string           | yes      | `flex-4-2`                        | Flavor for control plane nodes                                                       |
| `cloudscaleControlPlaneMachineImage`          | string           | yes      | `custom:ubuntu-2404-kube-v1.36.0` | Image for control plane nodes                                                        |
| `cloudscaleControlPlaneMachineRootVolumeSize` | integer          | yes      | `20`                              | Root volume size in GB for control plane                                             |
| `cloudscaleWorkerMachineFlavor`               | string           | yes      | `flex-4-2`                        | Flavor for worker nodes                                                              |
| `cloudscaleWorkerMachineImage`                | string           | yes      | `custom:ubuntu-2404-kube-v1.36.0` | Image for worker nodes                                                               |
| `cloudscaleWorkerMachineRootVolumeSize`       | integer          | yes      | `20`                              | Root volume size in GB for workers                                                   |
| `cloudscaleSSHKeys`                           | array of strings | yes      | —                                 | SSH public keys for all nodes                                                        |
| `cloudscaleControlPlaneServerGroupName`       | string           | yes      | —                                 | Server group name for control plane                                                  |
| `cloudscaleWorkerServerGroupName`             | string           | yes      | —                                 | Server group name for workers                                                        |

*(Source: [`templates/cluster-class.yaml`](../templates/cluster-class.yaml))*

The `topology-router-nat` flavor adds these variables:

| Variable      | Type   | Required | Default        | Description                                                                  |
|---------------|--------|----------|----------------|------------------------------------------------------------------------------|
| `networkName` | string | yes      | `node-net`     | Logical name for the private network                                         |
| `networkCIDR` | string | yes      | `10.10.0.0/24` | CIDR for the private network (router interface address is derived from this) |
| `routerName`  | string | yes      | `router`       | Name of the managed router                                                   |

*(Source: [`templates/cluster-class-router-nat.yaml`](../templates/cluster-class-router-nat.yaml))*

Use `--flavor topology` or `--flavor topology-router-nat` in the next step if you did use ClusterClass.

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

Wait for the control plane to come up enough to hand out a kubeconfig. This
doesn't need a CCM or CNI yet:

```bash
kubectl wait --for=condition=ControlPlaneInitialized cluster/my-cluster --timeout=15m
```

## 8. Install the cloudscale CCM

CAPCS only provisions infrastructure. For a working cluster
the [cloudscale CCM](https://github.com/cloudscale-ch/cloudscale-cloud-controller-manager) must be installed on the
**workload** cluster.

CAPCS provides a [`ClusterResourceSet` you apply on the
**management** cluster](https://cluster-api.sigs.k8s.io/tasks/cluster-resource-set).
CAPI's CRS controller then deploys it into any workload cluster labelled `ccm: cloudscale` — every CAPCS template sets
this label.

**N.B.:** the CCM also needs a CLOUDSCALE_API_TOKEN in the environment set. It's recommended to use a different token
for the CCM and CAPCS.

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

Fetch the workload cluster's kubeconfig:

```bash
clusterctl get kubeconfig my-cluster > ~/.kube/my-cluster.yaml
```

Install Cilium with defaults:

```bash
KUBECONFIG=~/.kube/my-cluster.yaml cilium install
```

Nodes become `Ready` once Cilium is up. Any standard CNI works — Cilium is just
the example.

## 10. Verify

```bash
# management cluster
clusterctl describe cluster my-cluster

# workload cluster
KUBECONFIG=~/.kube/my-cluster.yaml kubectl get nodes
KUBECONFIG=~/.kube/my-cluster.yaml kubectl -n kube-system get pods
```

All nodes should report `Ready`. In `kube-system` you should see
`cloudscale-cloud-controller-manager-*` on each control plane node and
`cilium-*` on every node.

Now wait for the cluster to report fully available.

```bash
kubectl wait --for=condition=Available cluster/my-cluster --timeout=15m
```

If something is stuck, see [troubleshooting](troubleshooting.md).

## Optional: persistent storage (cloudscale CSI)

For `PersistentVolumeClaim` support, install the
[cloudscale CSI driver](https://github.com/cloudscale-ch/csi-cloudscale) on the **workload** cluster:

```bash
export KUBECONFIG=~/.kube/my-cluster.yaml

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
