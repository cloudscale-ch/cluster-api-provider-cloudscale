# Cluster API Provider for cloudscale.ch

[![Tests](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/actions/workflows/test.yml/badge.svg)](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/cloudscale-ch/cluster-api-provider-cloudscale)](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/cloudscale-ch/cluster-api-provider-cloudscale.svg)](https://pkg.go.dev/github.com/cloudscale-ch/cluster-api-provider-cloudscale)
[![Goreportcard](https://goreportcard.com/badge/github.com/cloudscale-ch/cluster-api-provider-cloudscale)](https://goreportcard.com/report/github.com/cloudscale-ch/cluster-api-provider-cloudscale)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/cloudscale-ch/cluster-api-provider-cloudscale)

Kubernetes [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider
for [cloudscale.ch](https://www.cloudscale.ch). CAPCS provisions the cloudscale-specific
infrastructure — servers, networks, load balancers, floating IPs, server groups —
that Cluster API uses to build and manage workload Kubernetes clusters.

New to Cluster API? Read the upstream
[concepts](https://cluster-api.sigs.k8s.io/user/concepts.html) and
[quick start](https://cluster-api.sigs.k8s.io/user/quick-start.html) first; this
project only documents what is cloudscale-specific.

## Features

- Managed or pre-existing networks; public or private load balancer VIPs;
  floating IPs (IPv4/IPv6); anti-affinity server groups
- HA control plane; `MachineDeployment` autoscaling including
  [scale-from-zero](https://cluster-api.sigs.k8s.io/tasks/automated-machine-management/autoscaling)
  via capacity reported on `CloudscaleMachineTemplate`
- [ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/) support

## Compatibility

### Cluster-API Versions

Currently, CAPCS requires CAPI version >= v1.13.0 and is compatible only with the v1beta2 CRD versions of CAPI.

### Kubernetes Versions

The cloudscale provider is able to install and manage
the [versions of Kubernetes supported by the Cluster API (CAPI) project](https://cluster-api.sigs.k8s.io/reference/versions.html#supported-versions-matrix-by-provider-or-component).

## Prerequisites

- cloudscale.ch account and API token
- A custom OS image imported into your cloudscale.ch project, e.g. built with
  [image-builder for OpenStack](https://image-builder.sigs.k8s.io/)
- A management Kubernetes cluster ([kind](https://kind.sigs.k8s.io/) works) and
  [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl)

## Quickstart

This quickstart assumes you already know how Cluster-API works and have the prerequisites ready to use.
For a more detailed introduction, please read [our getting started guide](docs/getting-started.md).

```bash
export CLOUDSCALE_API_TOKEN=<your-api-token>

# initialize the Cluster-API management controllers
clusterctl init --infrastructure cloudscale-ch-cloudscale

# Generate and apply the cluster definition
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 --worker-machine-count 2 \
  | kubectl apply -f -

# Describe the status of the cluster
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
| Setting up monitoring or tracing    | [Observability](docs/observability.md)                                                                         |
| Hitting an error                    | [Troubleshooting](docs/troubleshooting.md)                                                                     |
| Contributing to CAPCS               | [Development](docs/development.md), [CONTRIBUTING.md](CONTRIBUTING.md)                                         |
| Cutting a release                   | [Releasing](docs/releasing.md), [Testing releases](docs/testing-releases.md)                                   |

## License

Apache License 2.0
