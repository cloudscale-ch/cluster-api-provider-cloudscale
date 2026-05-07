# Cluster API Provider for cloudscale.ch

[![Tests](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/actions/workflows/test.yml/badge.svg)](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/cloudscale-ch/cluster-api-provider-cloudscale)](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/releases/latest)

Kubernetes [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider
for [cloudscale.ch](https://www.cloudscale.ch).

## Features

- **CloudscaleCluster**: Multi-network management (managed or pre-existing), Load Balancer (public or private VIP),
  Floating IP
  support
- **CloudscaleMachine**: Server provisioning with cloud-init and configurable network interfaces
- **CloudscaleMachineTemplate**: Immutable machine templates for KubeadmControlPlane/MachineDeployment

## Prerequisites

- A Kubernetes cluster to use as a management cluster ([kind](https://kind.sigs.k8s.io/) works)
- [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl)
- A [cloudscale.ch](https://www.cloudscale.ch) account and API token
- A custom image imported into cloudscale. Images can e.g. be generated
  using [image-builder Openstack](https://image-builder.sigs.k8s.io/)

## Quickstart

### Initialize the management cluster

```bash
export CLOUDSCALE_API_TOKEN=<your-api-token>

clusterctl init --infrastructure cloudscale-ch-cloudscale
```

### Generate and apply a workload cluster

Set the [required environment variables](#environment-variables), then generate and apply the cluster manifest:

```bash
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale \
  --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 \
  | kubectl apply -f -
```

This uses the default template (public nodes, managed network). See [Cluster Templates](#cluster-templates) for other
network topologies.

Watch the cluster come up:

```bash
clusterctl describe cluster my-cluster
```

## Environment Variables

| Variable                                  | Description                             | Example                           |
|-------------------------------------------|-----------------------------------------|-----------------------------------|
| `CLOUDSCALE_API_TOKEN`                    | cloudscale.ch API token                 | `abc123...`                       |
| `CLOUDSCALE_SSH_PUBLIC_KEY`               | SSH public key added to nodes           | `ssh-ed25519 AAAA...`             |
| `CLOUDSCALE_REGION`                       | cloudscale.ch region                    | `lpg` or `rma`                    |
| `CLOUDSCALE_MACHINE_IMAGE`                | Server image for nodes                  | `custom:ubuntu-2404-kube-v1.xx.x` |
| `CLOUDSCALE_CONTROL_PLANE_MACHINE_FLAVOR` | Flavor for control plane nodes          | `flex-4-2`                        |
| `CLOUDSCALE_WORKER_MACHINE_FLAVOR`        | Flavor for worker nodes                 | `flex-4-2`                        |
| `CLOUDSCALE_ROOT_VOLUME_SIZE`             | Root volume size in GB                  | `50`                              |
| `CLOUDSCALE_NETWORK_UUID`                 | Pre-Existing cloudscale.ch network UUID | `2db69ba3-...`                    |

> **Note:** `CLOUDSCALE_NETWORK_UUID` is required by the `fip`, `public-lb-private-nodes`, and `pre-existing-network`
> template flavors. It is not needed for the default template.

## Cluster Templates

CAPCS ships several cluster templates for different network topologies. Use `clusterctl generate cluster` with the
`--flavor` flag to select one:

```bash
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale \
  --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 2 \
  --flavor <flavor-name> \
  | kubectl apply -f -
```

| Flavor                    | Network                   | CP Endpoint           | Node Connectivity | Extra Env Vars            | Notes                |
|---------------------------|---------------------------|-----------------------|-------------------|---------------------------|----------------------|
| *(default)*               | Managed (`10.100.0.0/24`) | Public LB (DualStack) | Public + cluster  | —                         |                      |
| `fip`                     | Pre-Existing              | Floating IP (IPv4)    | Public + cluster  | `CLOUDSCALE_NETWORK_UUID` |                      |
| `public-lb-private-nodes` | Pre-Existing + NAT        | Public LB             | Private only      | `CLOUDSCALE_NETWORK_UUID` | Requires NAT gateway |
| `pre-existing-network`    | Pre-Existing              | Public LB (DualStack) | Public + cluster  | `CLOUDSCALE_NETWORK_UUID` |                      |

## Development

This is a kubebuilder-scaffolded project. For new APIs, Webhooks, etc. [kubebuilder](https://book.kubebuilder.io/)
commands should be used.

```bash
# Run tests
make test

# Generate manifests
make manifests

# Generate code
make generate

# Run E2E tests (requires CLOUDSCALE_API_TOKEN)
make test-e2e
```

### E2E Tests

E2E tests are built on the [CAPI e2e test framework](https://pkg.go.dev/sigs.k8s.io/cluster-api/test/e2e)
(Ginkgo-based) and provision real clusters on cloudscale.ch. Tests use Ginkgo labels for
filtering and are split into suites of increasing cost, scheduled accordingly:

| Suite                   | Label                     | Description                                                                              | ~Duration | Schedule | Make target                        |
|-------------------------|---------------------------|------------------------------------------------------------------------------------------|-----------|----------|------------------------------------|
| Lifecycle               | `lifecycle`               | 1 CP + 1 worker: create, validate cloudscale resources, delete                           | < 5 min   | Nightly  | `test-e2e-lifecycle`               |
| HA lifecycle            | `ha`                      | 3 CP + 2 workers with anti-affinity server groups                                        | < 10 min  | Weekly   | `test-e2e-ha`                      |
| Cluster upgrade         | `upgrade`                 | Rolling K8s version upgrade (v1.34 → v1.35)                                              | < 10 min  | Weekly   | `test-e2e-upgrade`                 |
| Self-hosted             | `self-hosted`             | clusterctl move (pivot) to workload cluster. Requires container image in public registry | < 15 min  | Weekly   | `test-e2e-self-hosted`             |
| MD remediation          | `md-remediation`          | MachineHealthCheck auto-replacement of unhealthy workers                                 | < 10 min  | Weekly   | `test-e2e-md-remediation`          |
| Pre-Existing networking | `pre-existing-networking` | Pre-Existing network: public-LB + private-nodes and floating-IP variants                 | < 10 min  | Weekly   | `test-e2e-pre-existing-networking` |
| Conformance (fast)      | `conformance`             | K8s conformance, skip Serial tests                                                       | < 60 min  | Weekly   | `test-e2e-conformance-fast`        |
| Conformance (full)      | `conformance`             | Full K8s conformance including Serial tests                                              | < 120 min | Biweekly | `test-e2e-conformance`             |

Durations are approximate from a real CI run; conformance varies with cluster size.

**Why this split?** The single-CP lifecycle test is the cheapest smoke test and runs
nightly to catch regressions early. HA, upgrade, self-hosted, and remediation tests are more
resource-intensive and run weekly. Private networking tests require `CLOUDSCALE_NETWORK_UUID` to be set and are
skipped otherwise. Full K8s conformance is the most expensive and runs biweekly
(1st + 15th of month). All suites can be triggered manually via the `test-e2e.yml` workflow
dispatch. E2E tests share a concurrency group so only one suite runs at a time.

Any run involving the self-hosted spec requires the container image to be published to our registry. The self-hosted
spec moves the management cluster to the first workload cluster. That workload cluster doesn't have access to the
locally
built images and therefore needs a published container image.

For PRs, no e2e test is automatically run. It is advised to run them locally before submitting, as well as for a
reviewer
to run them locally and/or manually triggering the workflow **after** reviewing the code is safe.

### Tilt

The easiest way to work on this provider is by using the
[Tilt setup](https://cluster-api.sigs.k8s.io/developer/core/tilt.html) of Cluster-API.

Refer to the linked documentation on how to set up your local tilt. This requires cloning
[Cluster-API core](https://github.com/kubernetes-sigs/cluster-api) to your host. The necessary commands need to be
executed in the
Cluster-API core repository (**not** in this repository).

An example `tilt-settings.yaml`, which should also be placed in the Cluster-API core repository, is provided here:

```yaml
default_registry: "" # change if you use a remote image registry
provider_repos:
  # This refers to your provider directory and loads settings
  # from `tilt-provider.yaml`
  - path/to/local/clone/cluster-api-provider-cloudscale
enable_providers:
  - cloudscale
  - kubeadm-bootstrap
  - kubeadm-control-plane
deploy_cert_manager: true
kustomize_substitutions:
  CLOUDSCALE_API_TOKEN: "INSERT_TOKEN_HERE"
  CLOUDSCALE_SSH_PUBLIC_KEY: "INSERT_SSH_PUBLIC_KEY_HERE"
  CLOUDSCALE_REGION: "lpg"
  CLOUDSCALE_CONTROL_PLANE_MACHINE_FLAVOR: "flex-4-2"
  CLOUDSCALE_WORKER_MACHINE_FLAVOR: "flex-4-2"
  CLOUDSCALE_MACHINE_IMAGE: "IMAGE_NAME"
  CLOUDSCALE_ROOT_VOLUME_SIZE: "50"
  # Required for pre-existing network flavors (fip, public-lb-private-nodes, pre-existing-network):
  # CLOUDSCALE_NETWORK_UUID: "UUID_HERE"
extra_args:
  cloudscale:
    - "--zap-log-level=5"
template_dirs:
  docker:
    - ./test/infrastructure/docker/templates
  cloudscale:
    - path/to/local/clone/cluster-api-provider-cloudscale/templates
```

## License

Apache License 2.0
