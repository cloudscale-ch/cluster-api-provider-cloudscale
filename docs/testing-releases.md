# Testing Releases

This document describes how to test a release before and after tagging.

## Testing Before Tagging

### Build release manifests locally

```bash
make release-manifests IMG=quay.io/cloudscalech/cluster-api-cloudscale-controller:$TAG
```

### Inspect the generated artifacts in `dist/`

- **`infrastructure-components.yaml`** — verify:
    - The controller image tag matches the intended version
    - CRDs are present and well-formed
    - RBAC roles and bindings are correct
    - The controller Deployment has appropriate resource limits
- **`metadata.yaml`** — verify the release series is correct
- **`cluster-template.yaml`** — verify the template is complete and references are valid

### Build and test the Docker image locally

```bash
make docker-build IMG=quay.io/cloudscalech/cluster-api-cloudscale-controller:$TAG
```

## Testing After Release

### Ensure Version is published

After building a release it takes a while until `clusterctl` picks it up.

Verify if the new release is visible
on [Go Packages](https://pkg.go.dev/github.com/cloudscale-ch/cluster-api-provider-cloudscale?tab=versions) before
proceeding.

### Install on a management cluster

Set up the pre-requisites as outlined in [README.md](../README.md).

On a management cluster with `clusterctl` configured:

```bash
clusterctl init --infrastructure cloudscale-ch-cloudscale
```

Verify the provider version is correct:

```bash
kubectl get providers -A
```

Verify the controller starts successfully:

```bash
kubectl get pods -n capcs-system
kubectl logs -n capcs-system deploy/capcs-controller-manager
```

### Create a workload cluster

Generate a workload cluster manifest and apply it:

```bash
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale \
  --kubernetes-version v1.36.0 \
  --control-plane-machine-count 1 \
  --worker-machine-count 1 \
  > my-cluster.yaml

kubectl apply -f my-cluster.yaml
```

Monitor provisioning:

```bash
clusterctl describe cluster my-cluster
kubectl get clusters,cloudscaleclusters,machines,cloudscalemachines
```

The cluster should eventually reach a `Provisioned` phase.

### Clean up

```bash
kubectl delete cluster my-cluster
```

## E2E Tests

E2E tests run on schedule against the dev build:

| Schedule                        | Tests                                                    | Workflow           |
|---------------------------------|----------------------------------------------------------|--------------------|
| Nightly (2 AM UTC)              | Lifecycle (1 CP + 1 worker)                              | `e2e-nightly.yml`  |
| Weekly (Sunday 3 AM UTC)        | HA, upgrades, self-hosted, remediation, conformance-fast | `e2e-weekly.yml`   |
| Biweekly (1st & 15th, 3 AM UTC) | Full K8s conformance                                     | `e2e-biweekly.yml` |

For release candidates, trigger a manual e2e run via the `test-e2e.yml` workflow dispatch in GitHub Actions. Select the
test suite(s) to run and the branch/tag to test against. The same workflow is also how maintainers run a broader suite
against a PR branch — see [Running E2E on a PR](development.md#running-e2e-on-a-pr) for the mechanics.

See `test/e2e/` for test infrastructure details and `Makefile` for individual e2e targets (`test-e2e-lifecycle`,
`test-e2e-ha`, etc.).
