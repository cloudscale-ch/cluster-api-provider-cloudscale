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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v6"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			LoadBalancers:              opts.lbService,
			LoadBalancerPools:          opts.poolService,
			LoadBalancerListeners:      opts.listenerService,
			LoadBalancerHealthMonitors: opts.hmService,
			LoadBalancerPoolMembers:    opts.memberService,
		},
	}
}

type reconcileTestOpts struct {
	lbEnabled       bool
	networkService  *mockNetworkService
	subnetService   *mockSubnetService
	lbService       *mockLoadBalancerService
	poolService     *mockLoadBalancerPoolService
	listenerService *mockLoadBalancerListenerService
	hmService       *mockLoadBalancerHealthMonitorService
	memberService   *mockLoadBalancerPoolMemberService
}

// defaultMocks returns mocks that simulate all resources already provisioned and running.
func defaultMocks() reconcileTestOpts {
	return reconcileTestOpts{
		lbEnabled: true,
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
	assert.NotNil(t, clusterScope.CloudscaleCluster.Status.Initialization)
	assert.True(t, *clusterScope.CloudscaleCluster.Status.Initialization.Provisioned)

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
}

func TestReconcileNormal_NetworkErrorStopsReconciliation(t *testing.T) {
	mocks := defaultMocks()
	mocks.networkService = &mockNetworkService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
			return nil, fmt.Errorf("network api error")
		},
	}

	clusterScope := reconcileTestScope(mocks)
	r := newTestReconciler()

	_, err := r.reconcileNormal(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "network api error")

	// Ready condition should be set by deferred setReadyCondition (False since no sub-conditions are set)
	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
}

func TestReconcileNormal_LBErrorStopsReconciliation(t *testing.T) {
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

	require.Error(t, err)
	assert.Contains(t, err.Error(), "lb api error")
}

func TestReconcileNormal_LBPendingReturnsRequeue(t *testing.T) {
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

	require.NoError(t, err)
	assert.False(t, result.IsZero(), "should requeue when LB is pending")
	// Provisioned should be false
	assert.Nil(t, clusterScope.CloudscaleCluster.Status.Initialization)
}

func TestReconcileNormal_LBDisabledSetsProvisioned(t *testing.T) {
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
	assert.NotNil(t, clusterScope.CloudscaleCluster.Status.Initialization)
	assert.True(t, *clusterScope.CloudscaleCluster.Status.Initialization.Provisioned)
}

// ============================================================================
// Tests for reconcileDelete
// ============================================================================

func TestReconcileDelete_SuccessfulDeletion(t *testing.T) {
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
	assert.Equal(t, "lb-123", deletedLBID)
	assert.Equal(t, "net-123", deletedNetID)

	// Verify all LB status fields cleared
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
	// Verify network status cleared
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.NetworkID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.SubnetID)

	// Verify finalizer removed
	assert.NotContains(t, clusterScope.CloudscaleCluster.Finalizers, infrastructurev1beta2.ClusterFinalizer)

	// Verify conditions
	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)

	deletingCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.DeletingCondition)
	require.NotNil(t, deletingCond)
	assert.Equal(t, metav1.ConditionTrue, deletingCond.Status)
}

func TestReconcileDelete_LBDeleteErrorStopsDeletion(t *testing.T) {
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

	require.Error(t, err)
	assert.Contains(t, err.Error(), "lb delete failed")
	// Finalizer should NOT be removed on error
	assert.Contains(t, clusterScope.CloudscaleCluster.Finalizers, infrastructurev1beta2.ClusterFinalizer)
}

func TestReconcileDelete_NetworkDeleteErrorStopsDeletion(t *testing.T) {
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

	require.Error(t, err)
	assert.Contains(t, err.Error(), "network delete failed")
	// Finalizer should NOT be removed on error
	assert.Contains(t, clusterScope.CloudscaleCluster.Finalizers, infrastructurev1beta2.ClusterFinalizer)
}

func TestReconcileDelete_LBDisabledSkipsLBDeletion(t *testing.T) {
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
	assert.Equal(t, "net-123", deletedNetID)
	assert.NotContains(t, clusterScope.CloudscaleCluster.Finalizers, infrastructurev1beta2.ClusterFinalizer)
}

// testSchemeForReconcile returns a scheme with the types needed for reconcileNormal tests.
func testSchemeForReconcile() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clusterv1.AddToScheme(s)
	_ = infrastructurev1beta2.AddToScheme(s)
	return s
}
