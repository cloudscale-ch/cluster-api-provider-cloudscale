# Getting Started

This guide walks you through provisioning your first workload Kubernetes cluster
on [cloudscale.ch](https://www.cloudscale.ch) with CAPCS. For Cluster API
fundamentals (concepts, `clusterctl`, upgrades) see the
[upstream documentation](https://cluster-api.sigs.k8s.io/) — this guide only
covers what is cloudscale-specific.

## Prerequisites

1. **cloudscale.ch account and API token.** Create a token with read/write
   permissions in the [cloudscale.ch control panel](https://control.cloudscale.ch/).
   Keep it out of version control.
2. **Custom OS image** imported into your cloudscale.ch project. CAPCS does not
   publish a pre-built image — build one with
   [image-builder for OpenStack](https://image-builder.sigs.k8s.io/capi/providers/openstack)
   targeting the Kubernetes version you want, then upload it via the cloudscale.ch
   control panel or API. The image name you set there is what you pass as
   `CLOUDSCALE_MACHINE_IMAGE` (with `custom:` as a prefix).
3. **Management cluster.** Any conformant Kubernetes cluster works; a local
   [kind](https://kind.sigs.k8s.io/) cluster is the easiest starting point.
4. **`clusterctl`.** Install it per the
   [upstream instructions](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl).
5. **(Optional) Pre-existing network with NAT gateway.** Required for the `fip`,
   `pre-existing-network`, and `public-lb-private-nodes` template flavors. Create
   it in the cloudscale.ch control panel, contact support to setup the NAT gateway, and note its UUID.

## 1. Install the provider on the management cluster

```bash
export CLOUDSCALE_API_TOKEN=<your-api-token>
clusterctl init --infrastructure cloudscale-ch-cloudscale
```

`clusterctl init` also installs the Cluster API core, kubeadm bootstrap, and
kubeadm control plane components if they aren't already present.

## 2. Configure environment variables

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
| `CLOUDSCALE_NETWORK_UUID`                 | Pre-existing network UUID (non-default flavors only)  | `2db69ba3-...`                    |

Set them once in your shell, or keep them in `clusterctl`'s config file at
`~/.config/cluster-api/clusterctl.yaml`.

## 3. Pick a cluster template flavor

| Flavor                    | Network                  | Control plane endpoint | Node connectivity | Requires                                             |
|---------------------------|--------------------------|------------------------|-------------------|------------------------------------------------------|
| *(default)*               | Managed, `172.18.0.0/24` | Public LB, DualStack   | Public + cluster  | —                                                    |
| `fip`                     | Pre-existing             | Floating IP, IPv4      | Public + cluster  | `CLOUDSCALE_NETWORK_UUID`                            |
| `pre-existing-network`    | Pre-existing             | Public LB, DualStack   | Public + cluster  | `CLOUDSCALE_NETWORK_UUID`                            |
| `public-lb-private-nodes` | Pre-existing + NAT       | Public LB              | Private only      | `CLOUDSCALE_NETWORK_UUID`, with a NAT gateway set up |

The default's `172.18.0.0/24` network CIDR is chosen so it does not overlap with
the default Cilium cluster-pool range (`10.0.0.0/8`). If you change
`networks[].cidr` to a value inside your CNI's pod or service range, the control
plane load balancer's health checks will break — adjust the CNI accordingly.

## 4. Generate and apply the cluster

```bash
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale \
  --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 \
  --flavor pre-existing-network \
  > my-cluster.yaml

kubectl apply -f my-cluster.yaml
```

Omit `--flavor` for the default template. Inspect `my-cluster.yaml` before
applying — it includes a Secret holding `CLOUDSCALE_API_TOKEN`, which CAPCS
references via `CloudscaleCluster.spec.credentialsRef`.

Watch progress:

```bash
clusterctl describe cluster my-cluster
```

## 5. Get the kubeconfig and install a CNI

```bash
clusterctl get kubeconfig my-cluster > my-cluster.kubeconfig
export KUBECONFIG=$(pwd)/my-cluster.kubeconfig
```

The cluster has no CNI installed yet — nodes will stay `NotReady` until you
install one. Any standard CNI works; if you choose Cilium, keep its IPAM range
clear of the network CIDR you used in step 3.

## 6. Install the cloudscale Cloud Controller Manager

CAPCS provisions infrastructure, but services of type `LoadBalancer` and the
`ProviderID` on nodes come from the
[cloudscale CCM](https://github.com/cloudscale-ch/cloudscale-cloud-controller-manager).
The CCM is shipped as a `ClusterResourceSet` you apply on the **management**
cluster; CAPI then deploys it into any workload cluster labelled
`ccm: cloudscale` (all CAPCS templates set this label):

```bash
curl -L https://raw.githubusercontent.com/cloudscale-ch/cluster-api-provider-cloudscale/main/templates/addons/ccm.yaml \
  | envsubst | kubectl apply -f -
```

This creates a ConfigMap with the CCM manifests, a Secret with the API token,
and the `ClusterResourceSet` that wires them together.

## 7. Clean up

```bash
kubectl delete cluster my-cluster
```

Deleting the `Cluster` cascades through CAPCS, which removes the servers, load
balancer, floating IPs, server groups, and any managed networks it created.
Pre-existing networks supplied via `CLOUDSCALE_NETWORK_UUID` are left intact.

## Next steps

- Look up CRD fields with `kubectl explain cloudscalecluster.spec` (or browse the
  CRDs in [`config/crd/bases/`](../config/crd/bases))
- Read the [troubleshooting guide](troubleshooting.md) when something gets stuck
- Upstream Cluster API tasks (upgrades, scaling, MachineHealthChecks, etc.) are
  documented at <https://cluster-api.sigs.k8s.io/tasks/>
