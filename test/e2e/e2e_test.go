//go:build e2e

/*
Copyright 2026 cloudscale.ch.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
)

// Workload cluster lifecycle tests verify basic cluster provisioning on cloudscale.
// PostMachinesProvisioned validates that all expected cloudscale resources (network,
// subnet, LB, servers) are present. The HA variant adds server groups for anti-affinity.
//
// QuickStartSpec exercises the full create-wait-validate-delete lifecycle without the
// overhead of upgrades or pivots.
var _ = Describe("Workload cluster lifecycle", Label("lifecycle"), func() {
	Context("With single control-plane node", func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
				Flavor:                   new(""),
				ControlPlaneMachineCount: new(int64(1)),
				WorkerMachineCount:       new(int64(1)),
				PostMachinesProvisioned:  validateCloudscaleResources,
			}
		})
	})

	Context("With HA control-plane", Label("ha"), func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
				Flavor:                   new("ha"),
				ControlPlaneMachineCount: new(int64(3)),
				WorkerMachineCount:       new(int64(2)),
				PostMachinesProvisioned:  validateCloudscaleResources,
			}
		})
	})
})

var _ = Describe("Workload cluster-class topology", Label("topology"), func() {
	Context("With ClusterClass topology", func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
				Flavor:                   new("topology"),
				ControlPlaneMachineCount: new(int64(1)),
				WorkerMachineCount:       new(int64(1)),
				PostMachinesProvisioned:  validateCloudscaleResources,
			}
		})
	})
})

// Pre-existing networking tests verify cluster provisioning against a pre-existing network.
// All contexts are skipped when CLOUDSCALE_NETWORK_UUID is not set. The pre-existing network must
// provide internet egress (e.g. Support-arranged NAT) for the private-nodes contexts,
// otherwise kubeadm bootstrap hangs.
var _ = Describe("Pre-existing networking", Label("pre-existing-networking"), func() {
	BeforeEach(func() {
		if _, ok := e2eConfig.Variables["CLOUDSCALE_NETWORK_UUID"]; !ok {
			Skip("CLOUDSCALE_NETWORK_UUID not set, skipping pre-existing networking tests")
		}
	})

	// With pre-existing network: public LB, machines dual-attached (pre-existing network + public interfaces).
	Context("With pre-existing network", func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
				Flavor:                   new("pre-existing-network"),
				ControlPlaneMachineCount: new(int64(1)),
				WorkerMachineCount:       new(int64(1)),
				PostMachinesProvisioned:  validateCloudscaleResources,
			}
		})
	})

	// With public LB, machines attached only to the pre-existing network (no public interface).
	Context("With public LB, private nodes", func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
				Flavor:                   new("public-lb-private-nodes"),
				ControlPlaneMachineCount: new(int64(1)),
				WorkerMachineCount:       new(int64(1)),
				PostMachinesProvisioned:  validateCloudscaleResources,
			}
		})
	})

	// Floating IP on a LB.
	Context("With Floating IP on CP server", func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
				Flavor:                   new("fip"),
				ControlPlaneMachineCount: new(int64(1)),
				WorkerMachineCount:       new(int64(1)),
				PostMachinesProvisioned:  validateCloudscaleResources,
			}
		})
	})
})

// Cluster upgrade tests verify in-place Kubernetes version upgrades by rolling
// control-plane and worker nodes to new machine images. Conformance tests are skipped
// (SkipConformanceTests: true) to keep runtime reasonable — the separate conformance
// spec covers that.
var _ = Describe("Cluster upgrade", Label("upgrade"), func() {
	capi_e2e.ClusterUpgradeConformanceSpec(ctx, func() capi_e2e.ClusterUpgradeConformanceSpecInput {
		return capi_e2e.ClusterUpgradeConformanceSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			SkipCleanup:              skipCleanup,
			SkipConformanceTests:     true,
			InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
			ControlPlaneMachineCount: new(int64(1)),
			WorkerMachineCount:       new(int64(1)),
		}
	})
})

// Self-hosted tests verify the pivot workflow: CAPI management components are moved
// from the bootstrap kind cluster into the workload cluster via clusterctl move.
// SkipUpgrade is set to isolate the pivot test from upgrade mechanics. This catches
// regressions in our provider's ability to manage itself after pivot.
var _ = Describe("Self-hosted cluster", Label("self-hosted"), func() {
	capi_e2e.SelfHostedSpec(ctx, func() capi_e2e.SelfHostedSpecInput {
		return capi_e2e.SelfHostedSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			SkipCleanup:              skipCleanup,
			InfrastructureProvider:   new("cloudscale-ch-cloudscale"),
			SkipUpgrade:              true,
			ControlPlaneMachineCount: new(int64(1)),
			WorkerMachineCount:       new(int64(1)),
		}
	})
})

// MD remediation tests verify that unhealthy worker machines are automatically
// replaced via MachineHealthCheck. The CAPI MachineDeploymentRemediationSpec
// marks a worker node unhealthy and validates that MHC detects and replaces
// the machine. Runs weekly.
//
// KCP remediation (KCPRemediationSpec) was intentionally left out — it requires
// VMs to call back to the management cluster API (via wait-signal.sh), which is
// not possible when the management cluster is a local Kind cluster.
var _ = Describe("MD remediation", Label("md-remediation"), func() {
	capi_e2e.MachineDeploymentRemediationSpec(ctx, func() capi_e2e.MachineDeploymentRemediationSpecInput {
		return capi_e2e.MachineDeploymentRemediationSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: new("cloudscale-ch-cloudscale"),
		}
	})
})

// Kubernetes conformance runs the official K8s conformance suite (via kubetest) against
// a provisioned workload cluster.
// This ensures our provider produces clusters that pass the K8s conformance bar.
var _ = Describe("Kubernetes conformance", Label("conformance"), func() {
	capi_e2e.K8SConformanceSpec(ctx, func() capi_e2e.K8SConformanceSpecInput {
		return capi_e2e.K8SConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: new("cloudscale-ch-cloudscale"),
		}
	})
})
