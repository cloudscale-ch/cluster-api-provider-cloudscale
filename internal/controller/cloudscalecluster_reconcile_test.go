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

package controller

// Orchestrator-level smoke tests for the cluster reconciler. Error paths,
// requeue logic, LB-disabled handling, and sub-resource state transitions
// are covered by the per-reconciler test files (network, loadbalancer,
// floatingip, servergroup). These tests only verify the top-level wiring:
// reconcileNormal sets Provisioned + ReadyCondition once everything is in
// place; reconcileDelete removes children and the finalizer.

import (
	"context"
	"testing"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

// defaultProvisionedScope returns a ClusterScope wired to mocks that simulate
// every cloudscale resource as already provisioned and running.
func defaultProvisionedScope() *scope.ClusterScope {
	return testutils.NewClusterScopeOpts(
		testutils.WithLBEnabled(true),
		testutils.WithGeneration(1),
		testutils.WithServerGroupService(&testutils.MockServerGroupService{
			ListFn: func(ctx context.Context, _ ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
				return nil, nil
			},
		}),
		testutils.WithNetworkService(&testutils.MockNetworkService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
				return &cloudscalesdk.Network{UUID: id}, nil
			},
			DeleteFn: func(ctx context.Context, _ string) error { return nil },
		}),
		testutils.WithSubnetService(&testutils.MockSubnetService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
				return &cloudscalesdk.Subnet{UUID: id}, nil
			},
		}),
		testutils.WithLBService(&testutils.MockLoadBalancerService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
				return &cloudscalesdk.LoadBalancer{
					UUID:         id,
					Status:       LoadBalancerRunningStatus,
					VIPAddresses: []cloudscalesdk.VIPAddress{{Address: "1.2.3.4"}},
				}, nil
			},
			DeleteFn: func(ctx context.Context, _ string) error { return nil },
		}),
		testutils.WithPoolService(&testutils.MockLoadBalancerPoolService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
				return &cloudscalesdk.LoadBalancerPool{UUID: id}, nil
			},
		}),
		testutils.WithListenerService(&testutils.MockLoadBalancerListenerService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
				return &cloudscalesdk.LoadBalancerListener{UUID: id}, nil
			},
		}),
		testutils.WithHMService(&testutils.MockLoadBalancerHealthMonitorService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
				return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: id}, nil
			},
		}),
		testutils.WithMemberService(&testutils.MockLoadBalancerPoolMemberService{
			ListFn: func(ctx context.Context, _ string, _ ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
				return nil, nil
			},
		}),
	)
}

// TestReconcileNormal_FullyProvisionedCluster is the top-level happy-path smoke
// test for reconcileNormal: with every sub-resource present, the orchestrator
// must flip Initialization.Provisioned to true and surface ReadyCondition=True.
func TestReconcileNormal_FullyProvisionedCluster(t *testing.T) {
	g := NewWithT(t)

	clusterScope := defaultProvisionedScope()
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "net-123", SubnetID: "subnet-123", Managed: true},
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "pool-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "listener-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = "hm-123"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: "1.2.3.4",
		Port: 6443,
	}
	clusterScope.Cluster.Status.Initialization.ControlPlaneInitialized = new(true)

	r := newTestReconciler()

	result, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Status.Initialization).ToNot(BeNil())
	g.Expect(*clusterScope.CloudscaleCluster.Status.Initialization.Provisioned).To(BeTrue())

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
}

// TestReconcileNormal_PollsUntilControlPlaneInitialized verifies a fully
// provisioned cluster keeps re-queuing while the control plane is not yet
// initialized, but provisioning and the control plane endpoint are set
// regardless of that poll. Ensuring the LB-health poll never blocks provisioning,
// so the control plane it waits on can come up (no bootstrap deadlock).
func TestReconcileNormal_PollsUntilControlPlaneInitialized(t *testing.T) {
	g := NewWithT(t)

	clusterScope := defaultProvisionedScope()
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "net-123", SubnetID: "subnet-123", Managed: true},
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "pool-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "listener-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = "hm-123"
	// No explicit endpoint and ControlPlaneInitialized is false: the endpoint is set from the LB VIP, and reconcileNormal keeps polling.

	r := newTestReconciler()

	result, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(10*time.Second), "must keep polling until the control plane is initialized")
	g.Expect(clusterScope.CloudscaleCluster.Status.Initialization).ToNot(BeNil())
	g.Expect(*clusterScope.CloudscaleCluster.Status.Initialization.Provisioned).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("1.2.3.4"))
}

// TestReconcileDelete_SuccessfulDeletion is the top-level happy-path smoke test
// for reconcileDelete: deletes the LB and the network, clears all status IDs,
// removes the finalizer, and sets the Deleting/Ready conditions.
func TestReconcileDelete_SuccessfulDeletion(t *testing.T) {
	g := NewWithT(t)

	clusterScope := defaultProvisionedScope()
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "net-123", SubnetID: "subnet-123", Managed: true},
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "pool-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "listener-123"
	clusterScope.CloudscaleCluster.Finalizers = []string{infrastructurev1beta2.ClusterFinalizer}

	r := newTestReconciler()

	result, err := r.reconcileDelete(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerID).To(BeEmpty())
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID).To(BeEmpty())
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID).To(BeEmpty())
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(BeNil())
	g.Expect(clusterScope.CloudscaleCluster.Finalizers).ToNot(ContainElement(infrastructurev1beta2.ClusterFinalizer))

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))

	deletingCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.DeletingCondition)
	g.Expect(deletingCond).ToNot(BeNil())
	g.Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
}
