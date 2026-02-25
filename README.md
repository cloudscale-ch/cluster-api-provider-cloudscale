# Cluster API Provider for cloudscale.ch

Kubernetes [Cluster API](https://cluster-api.sigs.k8s.io/) infrastructure provider for [cloudscale.ch](https://www.cloudscale.ch).

**Status**: early development

## Features

- **CloudscaleCluster**: Network, Subnet, Load Balancer management
- **CloudscaleMachine**: Server provisioning with cloud-init
- **CloudscaleMachineTemplate**: Immutable machine templates for KubeadmControlPlane/MachineDeployment

## Prerequisites

- Go 1.25+
- Docker
- kubectl
- Access to a Kubernetes cluster (kind for development)
- cloudscale.ch API token

## Development

This is a kubebuilder-scaffolded project and for new APIs, Webhooks, etc. kubebuilder
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

### Tilt

The easiest way to work on this provider is by using the 
[Tilt setup](https://cluster-api.sigs.k8s.io/developer/core/tilt.html) of Cluster-API.

Refer to the linked documentation on how to set up your local tilt. An example `tilt-settings.yaml` is provided here:

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
extra_args:
  cloudscale:
    - "--zap-log-level=5"
```

## License

Apache License 2.0