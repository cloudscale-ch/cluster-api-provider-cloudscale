# Cluster API Provider for cloudscale.ch

[![Tests](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/actions/workflows/test.yml/badge.svg)](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/cloudscale-ch/cluster-api-provider-cloudscale)](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/releases/latest)

Kubernetes [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider
for [cloudscale.ch](https://www.cloudscale.ch). CAPCS provisions the cloudscale-specific
infrastructure — servers, networks, load balancers, floating IPs, server groups —
that Cluster API uses to build and manage workload Kubernetes clusters.

New to Cluster API? Read the upstream
[concepts](https://cluster-api.sigs.k8s.io/user/concepts.html) and
[quick start](https://cluster-api.sigs.k8s.io/user/quick-start.html) first; this
project only documents what is cloudscale-specific.

## Features

- Three CRDs: `CloudscaleCluster`, `CloudscaleMachine`, `CloudscaleMachineTemplate`
- Managed or pre-existing networks; public or private load balancer VIPs;
  floating IPs (IPv4/IPv6); anti-affinity server groups
- Supported regions: `lpg`, `rma`
- HA control plane; `MachineDeployment` autoscaling including
  [scale-from-zero](https://cluster-api.sigs.k8s.io/tasks/automated-machine-management/autoscaling)
  via capacity reported on `CloudscaleMachineTemplate`
- Four cluster templates: `default`, `fip`, `pre-existing-network`,
  `public-lb-private-nodes`

## Prerequisites

- cloudscale.ch account and API token
- A custom OS image imported into your cloudscale.ch project, e.g. built with
  [image-builder for OpenStack](https://image-builder.sigs.k8s.io/)
- A management Kubernetes cluster ([kind](https://kind.sigs.k8s.io/) works) and
  [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl)

## Quickstart

```bash
export CLOUDSCALE_API_TOKEN=<your-api-token>
clusterctl init --infrastructure cloudscale-ch-cloudscale
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 --worker-machine-count 2 \
  | kubectl apply -f -
clusterctl describe cluster my-cluster
```

The default template uses a managed network and a public load balancer.
[Getting Started](docs/getting-started.md) lists the required environment
variables and the other template flavors.

## Documentation

| If you are…                         | Start here                                                                                                     |
|-------------------------------------|----------------------------------------------------------------------------------------------------------------|
| New to Cluster API, or new to CAPCS | [Getting Started](docs/getting-started.md)                                                                     |
| Looking up a CRD field              | `kubectl explain cloudscalecluster.spec` (or the generated CRDs under [`config/crd/bases/`](config/crd/bases)) |
| Hitting an error                    | [Troubleshooting](docs/troubleshooting.md)                                                                     |
| Contributing to CAPCS               | [Development](docs/development.md), [CONTRIBUTING.md](CONTRIBUTING.md)                                         |
| Cutting a release                   | [Releasing](docs/releasing.md), [Testing releases](docs/testing-releases.md)                                   |

## License

Apache License 2.0
