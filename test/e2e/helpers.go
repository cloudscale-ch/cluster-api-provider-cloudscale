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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

// validateCloudscaleResources validates that cloudscale-specific resources are properly created.
// This is called by QuickStartSpec after machines are provisioned.
func validateCloudscaleResources(proxy framework.ClusterProxy, namespace, clusterName string) {
	ctx := context.Background()
	c := proxy.GetClient()

	By("Validating CloudscaleCluster resources")

	// Resolve CloudscaleCluster via the Cluster's infrastructureRef — under ClusterClass
	// topology, the InfraCluster name is generated with a suffix and does not match clusterName.
	cluster := &clusterv1.Cluster{}
	Expect(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster)).To(Succeed(), "Failed to get Cluster")

	cloudscaleCluster := &infrav1beta2.CloudscaleCluster{}
	key := client.ObjectKey{Namespace: namespace, Name: cluster.Spec.InfrastructureRef.Name}
	Expect(c.Get(ctx, key, cloudscaleCluster)).To(Succeed(), "Failed to get CloudscaleCluster")

	// Validate all network resources are created
	Expect(cloudscaleCluster.Status.Networks).NotTo(BeEmpty(), "At least one network should be defined in status")
	for i, net := range cloudscaleCluster.Status.Networks {
		Expect(net.NetworkID).NotTo(BeEmpty(), "Network %d (%s) should have NetworkID", i, net.Name)
		Expect(net.SubnetID).NotTo(BeEmpty(), "Network %d (%s) should have SubnetID", i, net.Name)
	}

	// Validate load balancer resources (if enabled - default is true)
	if ptr.Deref(cloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		Expect(cloudscaleCluster.Status.LoadBalancerID).NotTo(BeEmpty(), "LoadBalancerID should be set")
		Expect(cloudscaleCluster.Status.LoadBalancerPoolID).NotTo(BeEmpty(), "LoadBalancerPoolID should be set")
		Expect(cloudscaleCluster.Status.LoadBalancerListenerID).NotTo(BeEmpty(), "LoadBalancerListenerID should be set")
		Expect(cloudscaleCluster.Status.LoadBalancerHealthMonitorID).NotTo(BeEmpty(), "LoadBalancerHealthMonitorID should be set")
	}

	// Validate floating IP (if configured)
	if cloudscaleCluster.Spec.FloatingIP != nil {
		Expect(cloudscaleCluster.Status.FloatingIP).NotTo(BeEmpty(), "FloatingIP should be set when floating IP is configured")
	}

	// Validate provisioned status
	Expect(ptr.Deref(cloudscaleCluster.Status.Initialization.Provisioned, false)).To(BeTrue(), "CloudscaleCluster should be provisioned")

	// Validate control plane endpoint
	Expect(cloudscaleCluster.Spec.ControlPlaneEndpoint).NotTo(BeZero(), "ControlPlaneEndpoint should be set")
	Expect(cloudscaleCluster.Spec.ControlPlaneEndpoint.Host).NotTo(BeEmpty(), "ControlPlaneEndpoint.Host should be set")
	Expect(cloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)), "ControlPlaneEndpoint.Port should be 6443")

	By("Validating CloudscaleMachine resources")

	// List CloudscaleMachines
	machineList := &infrav1beta2.CloudscaleMachineList{}
	Expect(c.List(ctx, machineList, client.InNamespace(namespace))).To(Succeed(), "Failed to list CloudscaleMachines")

	for _, machine := range machineList.Items {
		// Validate each machine has a server ID
		Expect(machine.Status.ServerID).NotTo(BeEmpty(), "Machine %s should have ServerID", machine.Name)

		// Validate provisioned status
		Expect(ptr.Deref(machine.Status.Initialization.Provisioned, false)).To(BeTrue(), "Machine %s should be provisioned", machine.Name)

		// Validate addresses
		Expect(machine.Status.Addresses).NotTo(BeEmpty(), "Machine %s should have addresses", machine.Name)
	}
}
