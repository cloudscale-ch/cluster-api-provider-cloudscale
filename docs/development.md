# Development

For contributors working on CAPCS itself. End-user docs are in
[Getting Started](getting-started.md) and [Troubleshooting](troubleshooting.md).

## Architecture sketch

CAPCS is a kubebuilder-scaffolded infrastructure provider.

```
api/v1beta2/                CRD types
internal/controller/        Reconcilers, one file per cloudscale resource (network, LB, FIP, server group, server)
internal/webhook/v1beta2/   Defaulting + validating webhooks (one per CRD)
internal/cloudscale/        SDK wrapper: shared HTTP transport, flavor/region helpers, per-cluster services
internal/credentials/       Resolves the per-cluster API token from `credentialsRef`
internal/scope/             Per-cluster / per-machine reconciliation scope objects
cmd/main.go                 Manager setup, controller wiring, leader election, webhook registration
```

`CloudscaleClusterTemplate` has a webhook but no reconciler — CAPI core's
topology controller consumes it to materialize a `CloudscaleCluster` for each
`Cluster` whose ClusterClass references the template.

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
make test-e2e-topology  # topology-flavor E2E (ClusterClass quick-start)
make test-e2e           # full conformance-fast E2E suite (slow)
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

For the `topology` flavor, `clusterctl generate --from` only consumes a single
cluster-template file, so the `quick-start` ClusterClass in
`templates/cluster-class.yaml` must also be applied separately (or bundled with
a kustomize overlay).

## Tilt

The best way to develop CAPCS is using Cluster APIs [Tilt setup](https://cluster-api.sigs.k8s.io/developer/core/tilt.html). It runs
out of a local clone of [cluster-api](https://github.com/kubernetes-sigs/cluster-api), **not** out of this repository.

Drop a `tilt-settings.yaml` into the cluster-api directory:

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
# optional, if wanting to deploy the observability stack
#deploy_observability:
#  - grafana
#  - kube-state-metrics
#  - loki
#  - metrics-server
#  - prometheus
#  - alloy
#  - parca
#  - tempo
```

Then `tilt up` from the cluster-api directory.

The `deploy_observability` block is processed by the cluster-api Tiltfile and
brings up Prometheus, Grafana, Tempo, and friends in the management cluster;
see [Cluster API's Tilt documentation](https://cluster-api.sigs.k8s.io/developer/core/tilt)
for what each component does and how to reach the resulting UIs. CAPCS's
`ServiceMonitor` is auto-discovered once the prometheus kustomization is
enabled. For production metric/tracing setup, see
[Observability](observability.md).

## Deploying locally without Tilt

For a quick manual check of a locally-built image, without pulling in the Tilt toolchain,
deploy CAPCS against a `kind` cluster:

```bash
# create kind cluster and deploy cluster api and capcs with a locally built image
make dev-deploy

# re-deploy in case you made changes
make dev-redeploy

# clean up the kind cluster
make dev-clean
```

By default `dev-deploy` writes clusterctl overrides to `~/.cluster-api/overrides` (`CLUSTERCTL_OVERRIDES_DIR`). That is
the home-dir location clusterctl falls back to on both Linux and macOS. Note that
clusterctl actually prefers `$XDG_CONFIG_HOME/cluster-api/overrides` and only falls back
to the home dir when that directory does **not** exist. 
If you keep an XDG overrides directory (e.g. you exported
`XDG_CONFIG_HOME`), point the targets at it:

```bash
make dev-deploy CLUSTERCTL_OVERRIDES_DIR="$XDG_CONFIG_HOME/cluster-api/overrides"
```

## Tests

| Layer   | Location                                  | What it covers                                                                        |
|---------|-------------------------------------------|---------------------------------------------------------------------------------------|
| Unit    | `*_test.go` next to each file             | Pure logic; cloudscale API mocked                                                     |
| envtest | `internal/controller/suite_test.go` setup | Reconcilers against a real apiserver + etcd, cloudscale API mocked                    |
| E2E     | `test/e2e/`                               | Real workload clusters on cloudscale.ch (see [Testing Releases](testing-releases.md)) |

PRs do not run E2E automatically. Run the relevant suite locally before
submitting (`make test-e2e-lifecycle` at minimum); reviewers can run additional
suites or trigger the `test-e2e.yml` workflow manually after reviewing the
diff is safe — see [Running E2E on a PR](#running-e2e-on-a-pr).

## Running E2E on a PR

**When to use it.** A PR touches reconcilers, webhooks, or files under
`templates/` and the reviewer wants to ensure e2e runs through with these changes.

**Prerequisites.** Maintainer role on
`cloudscale-ch/cluster-api-provider-cloudscale`. The `e2e-tests`
concurrency group means at most one e2e run is in flight at a time.

**Triggering from the GitHub UI.** Actions → "E2E Tests (Manual)" → "Run
workflow". In the "Use workflow from" dropdown pick the PR branch (`gh pr view
<num>` shows the head branch), choose the make target, and click "Run workflow".
PRs from forks are not directly selectable — push the head to a branch on the
upstream repo first:

```bash
gh pr checkout <num>
git push upstream HEAD:pr/<num>
```

then dispatch against `pr/<num>`.

**Triggering with `gh`.** Use `gh workflow run`:

```bash
gh workflow run test-e2e.yml \
  --ref <pr-branch> \
  -f test_target=test-e2e-lifecycle
```

`--ref` accepts branch or tag names but not `pull/<num>/head`; for fork PRs use
the push-to-upstream workaround above. Watch the run:

```bash
gh run list --workflow=test-e2e.yml --limit 5
gh run watch <run-id>
```

**Cleaning up if the run is killed.** cloudscale-side cleanup runs inside the
e2e suite itself; if the workflow is cancelled mid-run, dangling cloudscale
resources may need manual cleanup via the cloudscale control panel.

## Releases

See [Releasing](releasing.md) for the tag-and-publish flow and
[Testing Releases](testing-releases.md) for post-release verification.

## Notes for AI agent contributors

If you are an AI agent contributing changes, read [`AGENTS.md`](../AGENTS.md) at
the repo root — it covers kubebuilder rules, auto-generated files to leave
alone, and project-specific conventions in more detail.
