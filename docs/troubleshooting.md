# Troubleshooting

Cloudscale-specific failure modes for CAPCS. For generic Cluster API issues
(bootstrap, certificates, MachineHealthCheck, etc.) see the
[upstream troubleshooting guide](https://cluster-api.sigs.k8s.io/user/troubleshooting.html).

## Where to look first

```bash
# Cluster-level status, conditions, and child resources
clusterctl describe cluster <name>

# Cloudscale infrastructure conditions
kubectl describe cloudscalecluster <name>
kubectl describe cloudscalemachine <name>

# Controller logs
kubectl -n capcs-system logs deploy/capcs-controller-manager -f
```

Most problems surface as a `Ready: False` condition with a `Reason` and `Message`
on the `CloudscaleCluster` or `CloudscaleMachine` — read those before diving
into logs.

## Authentication: `401 Unauthorized` from the cloudscale API

**Symptom:** controller logs show `401` from `api.cloudscale.ch`; `CloudscaleCluster`
stays `Ready: False` with an auth-related message.

**Common causes:**

- The credentials Secret is missing the `token` key, or the value is empty.
- `credentialsRef.namespace` points to a namespace that doesn't contain the
  Secret (it defaults to the `CloudscaleCluster`'s own namespace if unset).
- The token was revoked or scoped read-only in the cloudscale.ch control panel.

**Fix:** verify the Secret:

```bash
kubectl get secret <name> -o jsonpath='{.data.token}' | base64 -d
```

Re-create it with read/write scope if needed and let the controller requeue.

## Image: server creation fails with "image not found"

**Symptom:** `CloudscaleMachine` stuck `Ready: False`; cloudscale API returns
404 when creating the server.

**Cause:** the value of `spec.image` (set via `CLOUDSCALE_MACHINE_IMAGE`)
doesn't match a custom image imported into your cloudscale.ch project. CAPCS
does not ship a public image.

**Fix:** build and import an image with
[image-builder for OpenStack](https://image-builder.sigs.k8s.io/capi/providers/openstack)
and reference its exact name (typically `custom:<slug>`).

## Network: cluster stuck Provisioning, CIDR overlap

**Symptom:** workers are `Ready` but the control-plane load balancer never goes
healthy; pod-to-LB traffic from inside the cluster fails.

**Cause:** the network CIDR set on `CloudscaleCluster.spec.networks[].cidr`
overlaps with the CNI's pod or service range. The default Cilium cluster-pool
range is `10.0.0.0/8`, so any network CIDR inside that range collides.

**Verify:** Check the route table of the servers using `ip route`.

**Fix:** keep the network CIDR outside the CNI's IPAM range. The default
template uses `172.18.0.0/24` for this reason. If you must use a different
range, reconfigure your CNI to match.

## Network: wrong pre-existing network UUID

**Symptom:** `CloudscaleCluster` rejected by the webhook, or accepted but
reconciliation fails with `network not found`.

**Cause:** `CLOUDSCALE_NETWORK_UUID` doesn't exist in the cloudscale.ch project
the API token belongs to, or it exists in a different region.

**Fix:** look up the network in the cloudscale.ch control panel, confirm region
matches `CloudscaleCluster.spec.region`, and update the UUID.

## Load balancer stuck in `degraded` or `error`

**Symptom:** `clusterctl describe` shows the LB condition as `degraded` or
`error`; the control plane endpoint is unreachable.

**Cause:** the cloudscale LB has reported a non-running status. CAPCS does not
block reconciliation on `degraded`/`error` (it does block on `changing`), so
stale pool members will still be removed — but a persistent non-running status
points at an issue on the LB itself or its backends.

**Fix:** check the LB in the cloudscale.ch control panel; verify pool members
correspond to live control plane machines on the expected port. If a control
plane Machine was deleted and replaced, give the reconciler a minute to drop
the old member, then re-check.

## Server group: cluster cannot scale beyond 4 nodes per pool

**Symptom:** `MachineDeployment` scale-up stops at 4; new `CloudscaleMachine`
creation rejected by the cloudscale API.

**Cause:** cloudscale.ch limits a server group to 4 servers. CAPCS places all
machines from one `CloudscaleMachineTemplate` into the server group named in
`spec.serverGroup.name` (if defined).

**Fix:** split the workload across multiple `MachineDeployment`s, each
referencing a `CloudscaleMachineTemplate` with a distinct
`spec.serverGroup.name`.

## Webhook rejection: `unknown flavor`

**Symptom:** `kubectl apply` fails with a webhook validation error on
`spec.flavor` (or `spec.template.spec.flavor` for `CloudscaleMachineTemplate`).

**Cause:** the webhook validates `flavor` against the live list of flavors
fetched from the cloudscale API. The value doesn't match any known flavor slug.

**Fix:** list available flavors via the cloudscale API or control panel and pick
a slug that exists there.

## Webhook validation: common rejections

Other validations that commonly trip people up:

| Rejection                                                               | What it means                                                                                                                                            |
|-------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `exactly one of uuid or cidr must be specified` on `spec.networks[*]`   | Each network entry references either a pre-existing network (uuid) or a managed one (cidr)                                                               |
| `gateway must be within CIDR <cidr>`                                    | `networks[*].gatewayAddress` is outside the network's own CIDR                                                                                           |
| `floating IPs cannot be attached to a load balancer with a private VIP` | Combine a public LB with a floating IP, or drop one of them                                                                                              |
| `exactly one of ipFamily or ip must be specified` on `floatingIP`       | Set `ipFamily` to let CAPCS allocate, or `ip` to reuse a pre-existing floating IP                                                                        |
| `field is immutable after cluster creation`                             | Most cloudscale-side topology fields (region, zone, networks, floating IP, etc.) cannot be changed once the cluster exists                               |
| `field is immutable` on `CloudscaleMachine.spec`                        | Most machine spec fields (flavor, image, server group, …) cannot be changed once the machine exists — recreate via `MachineDeployment` rollout instead   |
| `CloudscaleClusterTemplate.Spec is immutable`                           | Override `quick-start` ClusterClass variables on the `Cluster` (`spec.topology.variables`) instead of mutating the `CloudscaleClusterTemplate` directly. |

When in doubt, run `kubectl explain cloudscalecluster.spec.<field>` — the
generated CRDs carry the rules the webhook enforces.

## `topology` flavor: cluster is admitted but never reconciles

**Symptom:** `kubectl apply` of a `topology`-flavor manifest succeeds, but
`clusterctl describe cluster <name>` shows no progress and no `CloudscaleCluster`
object exists in the namespace.

**Cause:** the `CLUSTER_TOPOLOGY` feature gate on cluster-api core is disabled,
so the topology controller never materializes infrastructure from the
`quick-start` ClusterClass.

**Fix:** re-run `clusterctl init` with `CLUSTER_TOPOLOGY=true` exported. See the
upstream
[ClusterClass docs](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
for what the gate enables.
