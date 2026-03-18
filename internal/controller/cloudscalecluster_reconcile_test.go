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

import (
	"context"
	"fmt"
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	cs "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// reconcileTestScope builds a ClusterScope with both network and LB services wired up.
func reconcileTestScope(opts reconcileTestOpts) *scope.ClusterScope {
	defaultGateway := ""
	return &scope.ClusterScope{
		Logger: logr.Discard(),
		Cluster: &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		},
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				Region: "rma",
				Zone:   "rma1",
				Network: infrastructurev1beta2.NetworkSpec{
					CIDR:           "10.0.0.0/24",
					GatewayAddress: &defaultGateway,
				},
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled:       ptr.To(opts.lbEnabled),
					APIServerPort: 6443,
					HealthMonitor: infrastructurev1beta2.HealthMonitorSpec{
						DelayS:        5,
						TimeoutS:      3,
						UpThreshold:   2,
						DownThreshold: 3,
					},
				},
			},
		},
		CloudscaleClient: &cs.Client{
			Networks:                   opts.networkService,
			Subnets:                    opts.subnetService,
			ServerGroups:               opts.serverGroupService,
			LoadBalancers:              opts.lbService,
			LoadBalancerPools:          opts.poolService,
			LoadBalancerListeners:      opts.listenerService,
			LoadBalancerHealthMonitors: opts.hmService,
			LoadBalancerPoolMembers:    opts.memberService,
		},
	}
}

type reconcileTestOpts struct {
	lbEnabled          bool
	networkService     *mockNetworkService
	subnetService      *mockSubnetService
	serverGroupService *mockServerGroupService
	lbService          *mockLoadBalancerService
	poolService        *mockLoadBalancerPoolService
	listenerService    *mockLoadBalancerListenerService
	hmService          *mockLoadBalancerHealthMonitorService
	memberService      *mockLoadBalancerPoolMemberService
}

// defaultMocks returns mocks that simulate all resources already provisioned and running.
func defaultMocks() reconcileTestOpts {
	return reconcileTestOpts{
		lbEnabled: true,
		serverGroupService: &mockServerGroupService{
			listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
				return nil, nil
			},
		},
		networkService: &mockNetworkService{
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
				return &cloudscalesdk.Network{UUID: id}, nil
			},
		},
		subnetService: &mockSubnetService{
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
				return &cloudscalesdk.Subnet{UUID: id}, nil
			},
		},
		lbService: &mockLoadBalancerService{
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
				return &cloudscalesdk.LoadBalancer{
					UUID:   id,
					Status: "running",
					VIPAddresses: []cloudscalesdk.VIPAddress{
						{Address: "1.2.3.4"},
					},
				}, nil
			},
		},
		poolService: &mockLoadBalancerPoolService{
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
				return &cloudscalesdk.LoadBalancerPool{UUID: id}, nil
			},
		},
		listenerService: &mockLoadBalancerListenerService{
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
				return &cloudscalesdk.LoadBalancerListener{UUID: id}, nil
			},
		},
		hmService: &mockLoadBalancerHealthMonitorService{
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
				return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: id}, nil
			},
		},
		memberService: &mockLoadBalancerPoolMemberService{
			listFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
				return nil, nil
			},
		},
	}
}

// ============================================================================
// Tests for reconcileNormal
// ============================================================================

func TestReconcileNormal_FullyProvisionedCluster(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "pool-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "listener-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = "hm-123"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: "1.2.3.4",
		Port: 6443,
	}

	// Need a fake k8s client for member reconciliation (lists CloudscaleMachines)
	fakeClient := fake.NewClientBuilder().
		WithScheme(testSchemeForReconcile()).
		Build()
	r := &CloudscaleClusterReconciler{
		Client:   fakeClient,
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Status.Initialization).ToNot(BeNil())
	g.Expect(*clusterScope.CloudscaleCluster.Status.Initialization.Provisioned).To(BeTrue())

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
}

func TestReconcileNormal_NetworkErrorStopsReconciliation(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	mocks.networkService = &mockNetworkService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
			return nil, fmt.Errorf("network api error")
		},
	}

	clusterScope := reconcileTestScope(mocks)
	r := newTestReconciler()

	_, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("network api error"))

	// Ready condition should be set by deferred setReadyCondition (False since no sub-conditions are set)
	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
}

func TestReconcileNormal_LBErrorStopsReconciliation(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	mocks.lbService = &mockLoadBalancerService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
			return nil, fmt.Errorf("lb api error")
		},
	}

	clusterScope := reconcileTestScope(mocks)
	// Network is already provisioned
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-123"

	r := newTestReconciler()

	_, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("lb api error"))
}

func TestReconcileNormal_LBPendingReturnsRequeue(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	mocks.lbService = &mockLoadBalancerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			return &cloudscalesdk.LoadBalancer{UUID: id, Status: "creating"}, nil
		},
	}

	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"

	r := newTestReconciler()

	result, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeFalse(), "should requeue when LB is pending")
	// Provisioned should be false
	g.Expect(clusterScope.CloudscaleCluster.Status.Initialization).To(BeNil())
}

func TestReconcileNormal_LBDisabledSetsProvisioned(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	mocks.lbEnabled = false

	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-123"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: "external-cp.example.com",
		Port: 6443,
	}

	r := newTestReconciler()

	result, err := r.reconcileNormal(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Status.Initialization).ToNot(BeNil())
	g.Expect(*clusterScope.CloudscaleCluster.Status.Initialization.Provisioned).To(BeTrue())
}

// ============================================================================
// Tests for reconcileDelete
// ============================================================================

func TestReconcileDelete_SuccessfulDeletion(t *testing.T) {
	g := NewWithT(t)

	var deletedLBID, deletedNetID string

	mocks := defaultMocks()
	mocks.lbService = &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedLBID = id
			return nil
		},
	}
	mocks.networkService = &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedNetID = id
			return nil
		},
	}

	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "pool-123"
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "listener-123"
	// Add finalizer so we can verify it gets removed
	clusterScope.CloudscaleCluster.Finalizers = []string{infrastructurev1beta2.ClusterFinalizer}

	r := newTestReconciler()

	result, err := r.reconcileDelete(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(deletedLBID).To(Equal("lb-123"))
	g.Expect(deletedNetID).To(Equal("net-123"))

	// Verify all LB status fields cleared
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerID).To(BeEmpty())
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID).To(BeEmpty())
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID).To(BeEmpty())
	// Verify network status cleared
	g.Expect(clusterScope.CloudscaleCluster.Status.NetworkID).To(BeEmpty())
	g.Expect(clusterScope.CloudscaleCluster.Status.SubnetID).To(BeEmpty())

	// Verify finalizer removed
	g.Expect(clusterScope.CloudscaleCluster.Finalizers).ToNot(ContainElement(infrastructurev1beta2.ClusterFinalizer))

	// Verify conditions
	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))

	deletingCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.DeletingCondition)
	g.Expect(deletingCond).ToNot(BeNil())
	g.Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
}

func TestReconcileDelete_LBDeleteErrorStopsDeletion(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	mocks.lbService = &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("lb delete failed")
		},
	}
	mocks.networkService = &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Network delete should not be called when LB delete fails")
			return nil
		},
	}

	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Finalizers = []string{infrastructurev1beta2.ClusterFinalizer}

	r := newTestReconciler()

	_, err := r.reconcileDelete(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("lb delete failed"))
	// Finalizer should NOT be removed on error
	g.Expect(clusterScope.CloudscaleCluster.Finalizers).To(ContainElement(infrastructurev1beta2.ClusterFinalizer))
}

func TestReconcileDelete_NetworkDeleteErrorStopsDeletion(t *testing.T) {
	g := NewWithT(t)

	mocks := defaultMocks()
	mocks.lbService = &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			return nil
		},
	}
	mocks.networkService = &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("network delete failed")
		},
	}

	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-123"
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Finalizers = []string{infrastructurev1beta2.ClusterFinalizer}

	r := newTestReconciler()

	_, err := r.reconcileDelete(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("network delete failed"))
	// Finalizer should NOT be removed on error
	g.Expect(clusterScope.CloudscaleCluster.Finalizers).To(ContainElement(infrastructurev1beta2.ClusterFinalizer))
}

func TestReconcileDelete_LBDisabledSkipsLBDeletion(t *testing.T) {
	g := NewWithT(t)

	var deletedNetID string

	mocks := defaultMocks()
	mocks.lbEnabled = false
	mocks.lbService = &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("LB delete should not be called when LB is disabled")
			return nil
		},
	}
	mocks.networkService = &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedNetID = id
			return nil
		},
	}

	clusterScope := reconcileTestScope(mocks)
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-123"
	clusterScope.CloudscaleCluster.Finalizers = []string{infrastructurev1beta2.ClusterFinalizer}

	r := newTestReconciler()

	result, err := r.reconcileDelete(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(deletedNetID).To(Equal("net-123"))
	g.Expect(clusterScope.CloudscaleCluster.Finalizers).ToNot(ContainElement(infrastructurev1beta2.ClusterFinalizer))
}

// testSchemeForReconcile returns a scheme with the types needed for reconcileNormal tests.
func testSchemeForReconcile() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clusterv1.AddToScheme(s)
	_ = infrastructurev1beta2.AddToScheme(s)
	return s
}
