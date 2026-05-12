# Development

For contributors working on CAPCS itself. End-user docs are in
[Getting Started](getting-started.md) and [Troubleshooting](troubleshooting.md).

## Architecture sketch

CAPCS is a kubebuilder-scaffolded infrastructure provider. Three CRDs, three
reconcilers, a webhook per CRD, and a thin wrapper around the cloudscale-go-sdk.

```
api/v1beta2/                CRD types (CloudscaleCluster, CloudscaleMachine, CloudscaleMachineTemplate)
internal/controller/        Reconcilers, one file per cloudscale resource (network, LB, FIP, server group, server)
internal/webhook/v1beta2/   Defaulting + validating webhooks (one per CRD)
internal/cloudscale/        SDK wrapper: shared HTTP transport, flavor/region helpers, per-cluster services
internal/credentials/       Resolves the per-cluster API token from `credentialsRef`
internal/scope/             Per-cluster / per-machine reconciliation scope objects
cmd/main.go                 Manager setup, controller wiring, leader election, webhook registration
```

A few conventions to know before touching code:

- **Webhooks own all defaulting and validation.** Controllers must never repeat
  validation logic — if a field needs a default or a check, it goes in the
  webhook so behavior stays consistent between `kubectl apply` and the
  reconcile loop.
- **Ownership tags.** Cloudscale resources are tagged with the key
  `capcs-cluster-<cluster-name>` so the reconciler can identify what it owns
  and clean it up. See `api/v1beta2/tags.go` and `internal/controller/cloudscale_tags.go`.
- **Shared HTTP transport.** Per-cluster cloudscale clients share an
  `http.Transport` (see `internal/cloudscale/services.go`) so connection
  pooling works across reconciliations.

## Setup

You need:

- Go (version pinned in `go.mod`)
- [kind](https://kind.sigs.k8s.io/), [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl),
  `kubectl`, `kustomize`
- [Tilt](https://tilt.dev/) for the inner-loop workflow
- A cloudscale.ch API token (export `CLOUDSCALE_API_TOKEN`)
- A cloudscale.ch custom image (see [Getting Started](getting-started.md#prerequisites))

## Make targets

```bash
make test               # unit tests + envtest (runs fmt, vet, generate, manifests)
make manifests          # regenerate CRDs / webhook config from kubebuilder markers
make generate           # regenerate deepcopy code
make lint               # golangci-lint
make build              # build the manager binary

make test-e2e-lifecycle # smallest E2E suite — single CP + 1 worker
make test-e2e           # full conformance-fast E2E suite (slow, real cloudscale)
```

E2E suites and their cadence are documented in
[Testing Releases](testing-releases.md).

## Iterating on cluster templates locally

When you change a file under `templates/`, you can test it before it ships in a
release by pointing `clusterctl generate` at the local file:

```bash
clusterctl generate cluster my-cluster \
  --infrastructure cloudscale-ch-cloudscale \
  --kubernetes-version v1.36.0 \
  --from templates/cluster-template-fip.yaml \
  | kubectl apply -f -
```

This is a contributor flow only — end users consume published flavors via
`--flavor` (see [Getting Started](getting-started.md#3-pick-a-cluster-template-flavor)).

## Tilt

The fastest inner loop is Cluster API's
[Tilt setup](https://cluster-api.sigs.k8s.io/developer/core/tilt.html). It runs
out of a local clone of [cluster-api](https://github.com/kubernetes-sigs/cluster-api),
**not** out of this repository.

Drop a `tilt-settings.yaml` next to the cluster-api checkout:

```yaml
default_registry: ""
provider_repos:
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
  # Required for the fip / public-lb-private-nodes / pre-existing-network flavors:
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

Then `tilt up` from the cluster-api checkout.

## Tests

| Layer   | Location                                  | What it covers                                                                        |
|---------|-------------------------------------------|---------------------------------------------------------------------------------------|
| Unit    | `*_test.go` next to each file             | Pure logic; cloudscale API mocked                                                     |
| envtest | `internal/controller/suite_test.go` setup | Reconcilers against a real apiserver + etcd, cloudscale API mocked                    |
| E2E     | `test/e2e/`                               | Real workload clusters on cloudscale.ch (see [Testing Releases](testing-releases.md)) |

PRs do not run E2E automatically. Run the relevant suite locally before
submitting (`make test-e2e-lifecycle` at minimum); reviewers can run additional
suites or trigger the `test-e2e.yml` workflow manually after reviewing the
diff is safe.

## Releases

See [Releasing](releasing.md) for the tag-and-publish flow and
[Testing Releases](testing-releases.md) for post-release verification.

## Notes for AI agent contributors

If you are an AI agent contributing changes, read [`AGENTS.md`](../AGENTS.md) at
the repo root — it covers kubebuilder rules, auto-generated files to leave
alone, and project-specific conventions in more detail.
