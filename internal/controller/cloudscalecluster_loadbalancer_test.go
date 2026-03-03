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
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v6"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

const (
	testLBUUID       = "lb-uuid"
	testPoolUUID     = "pool-uuid"
	testListenerUUID = "listener-uuid"
	testHMUUID       = "hm-uuid"
)

// Test scope options for load balancer tests
type lbTestScopeOptions struct {
	loadBalancerService        *mockLoadBalancerService
	poolService                *mockLoadBalancerPoolService
	listenerService            *mockLoadBalancerListenerService
	healthMonitorService       *mockLoadBalancerHealthMonitorService
	poolMemberService          *mockLoadBalancerPoolMemberService
	lbEnabled                  bool
	algorithm                  string
	flavor                     string
	apiServerPort              int32
	healthMonitorDelayS        int
	healthMonitorTimeoutS      int
	healthMonitorUpThreshold   int
	healthMonitorDownThreshold int
}

// newFakeClientForLB creates a fake k8s client with the necessary schemes
func newFakeClientForLB(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = clusterv1.AddToScheme(scheme)
	_ = infrastructurev1beta2.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// newTestReconcilerWithClient creates a reconciler with a k8s client (needed for member management)
func newTestReconcilerWithClient(k8sClient client.Client) *CloudscaleClusterReconciler {
	return &CloudscaleClusterReconciler{
		Client:   k8sClient,
		recorder: events.NewFakeRecorder(10),
	}
}

func newTestClusterScopeWithLB(opts lbTestScopeOptions) *scope.ClusterScope {
	if opts.apiServerPort == 0 {
		opts.apiServerPort = 6443
	}
	if opts.healthMonitorDelayS == 0 {
		opts.healthMonitorDelayS = 5
	}
	if opts.healthMonitorTimeoutS == 0 {
		opts.healthMonitorTimeoutS = 3
	}
	if opts.healthMonitorUpThreshold == 0 {
		opts.healthMonitorUpThreshold = 2
	}
	if opts.healthMonitorDownThreshold == 0 {
		opts.healthMonitorDownThreshold = 3
	}

	cloudscaleClient := &cloudscale.Client{
		LoadBalancers:              opts.loadBalancerService,
		LoadBalancerPools:          opts.poolService,
		LoadBalancerListeners:      opts.listenerService,
		LoadBalancerHealthMonitors: opts.healthMonitorService,
		LoadBalancerPoolMembers:    opts.poolMemberService,
	}

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
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				Region: "rma",
				Zone:   "rma1",
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled:       &opts.lbEnabled,
					Algorithm:     opts.algorithm,
					Flavor:        opts.flavor,
					APIServerPort: opts.apiServerPort,
					HealthMonitor: infrastructurev1beta2.HealthMonitorSpec{
						DelayS:        opts.healthMonitorDelayS,
						TimeoutS:      opts.healthMonitorTimeoutS,
						UpThreshold:   opts.healthMonitorUpThreshold,
						DownThreshold: opts.healthMonitorDownThreshold,
					},
				},
			},
		},
		CloudscaleClient: cloudscaleClient,
	}
}

// ============================================================================
// Tests for reconcileLB
// ============================================================================

func TestReconcileLB_CreatesLoadBalancer(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerRequest

	lbService := &mockLoadBalancerService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancer{
				UUID:   "lb-uuid-123",
				Name:   req.Name,
				Status: "creating",
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
		flavor:              "lb-standard",
	})
	r := newTestReconciler()

	err := r.reconcileLB(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "lb-uuid-123", clusterScope.CloudscaleCluster.Status.LoadBalancerID)
	assert.Equal(t, "test-cluster-cp-lb", capturedReq.Name)
	assert.Equal(t, "rma1", capturedReq.Zone)
	assert.Equal(t, "lb-standard", capturedReq.Flavor)
}

func TestReconcileLB_SkipsIfAlreadyExists(t *testing.T) {
	lbService := &mockLoadBalancerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			return &cloudscalesdk.LoadBalancer{UUID: id, Status: "running"}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			t.Fatal("Create should not be called when LB already exists")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "existing-lb-uuid"

	r := newTestReconciler()

	err := r.reconcileLB(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "existing-lb-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerID)
}

func TestReconcileLB_FindsExistingByTag(t *testing.T) {
	lbService := &mockLoadBalancerService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
			return []cloudscalesdk.LoadBalancer{
				{UUID: "found-lb-uuid", Name: "test-cluster-cp-lb"},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			t.Fatal("Create should not be called when LB is found by tag")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	r := newTestReconciler()

	err := r.reconcileLB(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "found-lb-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerID)
}

func TestReconcileLB_ErrorsOnMultipleByTag(t *testing.T) {
	lbService := &mockLoadBalancerService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
			return []cloudscalesdk.LoadBalancer{
				{UUID: "lb-uuid-1", Name: "test-cluster-cp-lb"},
				{UUID: "lb-uuid-2", Name: "test-cluster-cp-lb"},
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	r := newTestReconciler()

	err := r.reconcileLB(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 load balancers matching tag filter")
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerID)
}

func TestReconcileLB_RecreatesIfDeletedExternally(t *testing.T) {
	var createdLB bool

	lbService := &mockLoadBalancerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			createdLB = true
			return &cloudscalesdk.LoadBalancer{UUID: "new-lb-uuid", Name: req.Name}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "deleted-lb-uuid"

	r := newTestReconciler()

	err := r.reconcileLB(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.True(t, createdLB, "Should create a new LB when old one was deleted")
	assert.Equal(t, "new-lb-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerID)
}

func TestReconcileLB_UsesCustomFlavor(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerRequest

	lbService := &mockLoadBalancerService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancer{UUID: "lb-uuid"}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
		flavor:              "lb-flex-2",
	})
	r := newTestReconciler()

	err := r.reconcileLB(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "lb-flex-2", capturedReq.Flavor)
}

// ============================================================================
// Tests for reconcileLBPool
// ============================================================================

func TestReconcileLBPool_CreatesPool(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerPoolRequest

	poolService := &mockLoadBalancerPoolService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancerPool{
				UUID: "pool-uuid-123",
				Name: req.Name,
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolService: poolService,
		lbEnabled:   true,
		algorithm:   "round_robin",
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"

	r := newTestReconciler()

	err := r.reconcileLBPool(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "pool-uuid-123", clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
	assert.Equal(t, "test-cluster-cp-pool", capturedReq.Name)
	assert.Equal(t, "lb-uuid", capturedReq.LoadBalancer)
	assert.Equal(t, "round_robin", capturedReq.Algorithm)
	assert.Equal(t, "tcp", capturedReq.Protocol)
}

func TestReconcileLBPool_SkipsIfAlreadyExists(t *testing.T) {
	poolService := &mockLoadBalancerPoolService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
			return &cloudscalesdk.LoadBalancerPool{UUID: id}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
			t.Fatal("Create should not be called when pool already exists")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolService: poolService,
		lbEnabled:   true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "existing-pool-uuid"

	r := newTestReconciler()

	err := r.reconcileLBPool(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "existing-pool-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
}

func TestReconcileLBPool_FindsExistingByTag(t *testing.T) {
	poolService := &mockLoadBalancerPoolService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error) {
			return []cloudscalesdk.LoadBalancerPool{
				{UUID: "found-pool-uuid", Name: "test-cluster-cp-pool"},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
			t.Fatal("Create should not be called when pool is found by tag")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolService: poolService,
		lbEnabled:   true,
	})
	r := newTestReconciler()

	err := r.reconcileLBPool(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "found-pool-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
}

func TestReconcileLBPool_ErrorsOnMultipleByTag(t *testing.T) {
	poolService := &mockLoadBalancerPoolService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error) {
			return []cloudscalesdk.LoadBalancerPool{
				{UUID: "pool-uuid-1", Name: "test-cluster-cp-pool"},
				{UUID: "pool-uuid-2", Name: "test-cluster-cp-pool"},
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolService: poolService,
		lbEnabled:   true,
	})
	r := newTestReconciler()

	err := r.reconcileLBPool(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 load balancer pools matching tag filter")
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
}

func TestReconcileLBPool_RecreatesIfDeletedExternally(t *testing.T) {
	var createdPool bool

	poolService := &mockLoadBalancerPoolService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
			return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
			createdPool = true
			return &cloudscalesdk.LoadBalancerPool{UUID: "new-pool-uuid", Name: req.Name}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolService: poolService,
		lbEnabled:   true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = "deleted-pool-uuid"

	r := newTestReconciler()

	err := r.reconcileLBPool(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.True(t, createdPool, "Should create a new pool when old one was deleted")
	assert.Equal(t, "new-pool-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
}

func TestReconcileLBPool_UsesCustomAlgorithm(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerPoolRequest

	poolService := &mockLoadBalancerPoolService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancerPool{UUID: "pool-uuid"}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolService: poolService,
		lbEnabled:   true,
		algorithm:   "least_connections",
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID

	r := newTestReconciler()

	err := r.reconcileLBPool(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "least_connections", capturedReq.Algorithm)
}

// ============================================================================
// Tests for reconcileLBListener
// ============================================================================

func TestReconcileLBListener_CreatesListener(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerListenerRequest

	listenerService := &mockLoadBalancerListenerService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancerListener{
				UUID: "listener-uuid-123",
				Name: req.Name,
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		listenerService: listenerService,
		lbEnabled:       true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID

	r := newTestReconciler()

	err := r.reconcileLBListener(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "listener-uuid-123", clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
	assert.Equal(t, "test-cluster-cp-listener", capturedReq.Name)
	assert.Equal(t, testPoolUUID, capturedReq.Pool)
	assert.Equal(t, "tcp", capturedReq.Protocol)
	assert.Equal(t, 6443, capturedReq.ProtocolPort)
}

func TestReconcileLBListener_SkipsIfAlreadyExists(t *testing.T) {
	listenerService := &mockLoadBalancerListenerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
			return &cloudscalesdk.LoadBalancerListener{UUID: id}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
			t.Fatal("Create should not be called when listener already exists")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		listenerService: listenerService,
		lbEnabled:       true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "existing-listener-uuid"

	r := newTestReconciler()

	err := r.reconcileLBListener(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "existing-listener-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
}

func TestReconcileLBListener_FindsExistingByTag(t *testing.T) {
	listenerService := &mockLoadBalancerListenerService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error) {
			return []cloudscalesdk.LoadBalancerListener{
				{UUID: "found-listener-uuid", Name: "test-cluster-cp-listener"},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
			t.Fatal("Create should not be called when listener is found by tag")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		listenerService: listenerService,
		lbEnabled:       true,
	})
	r := newTestReconciler()

	err := r.reconcileLBListener(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "found-listener-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
}

func TestReconcileLBListener_ErrorsOnMultipleByTag(t *testing.T) {
	listenerService := &mockLoadBalancerListenerService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error) {
			return []cloudscalesdk.LoadBalancerListener{
				{UUID: "listener-uuid-1", Name: "test-cluster-cp-listener"},
				{UUID: "listener-uuid-2", Name: "test-cluster-cp-listener"},
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		listenerService: listenerService,
		lbEnabled:       true,
	})
	r := newTestReconciler()

	err := r.reconcileLBListener(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 load balancer listeners matching tag filter")
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
}

func TestReconcileLBListener_RecreatesIfDeletedExternally(t *testing.T) {
	var createdListener bool

	listenerService := &mockLoadBalancerListenerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
			return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
			createdListener = true
			return &cloudscalesdk.LoadBalancerListener{UUID: "new-listener-uuid", Name: req.Name}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		listenerService: listenerService,
		lbEnabled:       true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = "deleted-listener-uuid"

	r := newTestReconciler()

	err := r.reconcileLBListener(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.True(t, createdListener, "Should create a new listener when old one was deleted")
	assert.Equal(t, "new-listener-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
}

func TestReconcileLBListener_UsesCustomPort(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerListenerRequest

	listenerService := &mockLoadBalancerListenerService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancerListener{UUID: "listener-uuid"}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		listenerService: listenerService,
		lbEnabled:       true,
		apiServerPort:   8443,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID

	r := newTestReconciler()

	err := r.reconcileLBListener(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, 8443, capturedReq.ProtocolPort)
}

// ============================================================================
// Tests for reconcileLBHealthMonitor
// ============================================================================

func TestReconcileLBHealthMonitor_CreatesMonitor(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerHealthMonitorRequest

	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancerHealthMonitor{
				UUID: "hm-uuid-123",
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		healthMonitorService: healthMonitorService,
		lbEnabled:            true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID

	r := newTestReconciler()

	err := r.reconcileLBHealthMonitor(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "hm-uuid-123", clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID)
	assert.Equal(t, testPoolUUID, capturedReq.Pool)
	assert.Equal(t, "tcp", capturedReq.Type)
	assert.Equal(t, 5, capturedReq.DelayS)
	assert.Equal(t, 3, capturedReq.TimeoutS)
	assert.Equal(t, 2, capturedReq.UpThreshold)
	assert.Equal(t, 3, capturedReq.DownThreshold)
}

func TestReconcileLBHealthMonitor_SkipsIfAlreadyExists(t *testing.T) {
	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: id}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			t.Fatal("Create should not be called when health monitor already exists")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		healthMonitorService: healthMonitorService,
		lbEnabled:            true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = "existing-hm-uuid"

	r := newTestReconciler()

	err := r.reconcileLBHealthMonitor(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "existing-hm-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID)
}

func TestReconcileLBHealthMonitor_FindsExistingByTag(t *testing.T) {
	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error) {
			return []cloudscalesdk.LoadBalancerHealthMonitor{
				{UUID: "found-hm-uuid"},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			t.Fatal("Create should not be called when health monitor is found by tag")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		healthMonitorService: healthMonitorService,
		lbEnabled:            true,
	})
	r := newTestReconciler()

	err := r.reconcileLBHealthMonitor(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "found-hm-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID)
}

func TestReconcileLBHealthMonitor_ErrorsOnMultipleByTag(t *testing.T) {
	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error) {
			return []cloudscalesdk.LoadBalancerHealthMonitor{
				{UUID: "hm-uuid-1"},
				{UUID: "hm-uuid-2"},
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		healthMonitorService: healthMonitorService,
		lbEnabled:            true,
	})
	r := newTestReconciler()

	err := r.reconcileLBHealthMonitor(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 load balancer health monitors matching tag filter")
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID)
}

func TestReconcileLBHealthMonitor_RecreatesIfDeletedExternally(t *testing.T) {
	var createdHM bool

	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			createdHM = true
			return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: "new-hm-uuid"}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		healthMonitorService: healthMonitorService,
		lbEnabled:            true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = "deleted-hm-uuid"

	r := newTestReconciler()

	err := r.reconcileLBHealthMonitor(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.True(t, createdHM, "Should create a new health monitor when old one was deleted")
	assert.Equal(t, "new-hm-uuid", clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID)
}

func TestReconcileLBHealthMonitor_UsesCustomThresholds(t *testing.T) {
	var capturedReq *cloudscalesdk.LoadBalancerHealthMonitorRequest

	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			capturedReq = req
			return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: "hm-uuid"}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		healthMonitorService:       healthMonitorService,
		lbEnabled:                  true,
		healthMonitorDelayS:        10,
		healthMonitorTimeoutS:      5,
		healthMonitorUpThreshold:   3,
		healthMonitorDownThreshold: 5,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID

	r := newTestReconciler()

	err := r.reconcileLBHealthMonitor(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, 10, capturedReq.DelayS)
	assert.Equal(t, 5, capturedReq.TimeoutS)
	assert.Equal(t, 3, capturedReq.UpThreshold)
	assert.Equal(t, 5, capturedReq.DownThreshold)
}

// ============================================================================
// Tests for reconcileLoadBalancer (main orchestrator)
// ============================================================================

func TestReconcileLoadBalancer_SkipsWhenDisabled(t *testing.T) {
	lbService := &mockLoadBalancerService{
		createFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			t.Fatal("Create should not be called when LB is disabled")
			return nil, nil
		},
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			t.Fatal("Get should not be called when LB is disabled")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           false, // Disabled for external control plane
	})

	r := newTestReconciler()

	result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

func TestReconcileLoadBalancer_WaitsForLBRunning(t *testing.T) {
	lbService := &mockLoadBalancerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			return &cloudscalesdk.LoadBalancer{
				UUID:   id,
				Status: "creating", // Not running yet
			}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID

	r := newTestReconciler()

	result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when LB is not running")
}

func TestReconcileLoadBalancer_SetsControlPlaneEndpoint(t *testing.T) {
	lbService := &mockLoadBalancerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			return &cloudscalesdk.LoadBalancer{
				UUID:   id,
				Status: "running",
				VIPAddresses: []cloudscalesdk.VIPAddress{
					{Address: "203.0.113.10"},
				},
			}, nil
		},
	}

	poolService := &mockLoadBalancerPoolService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
			return &cloudscalesdk.LoadBalancerPool{UUID: id}, nil
		},
	}

	listenerService := &mockLoadBalancerListenerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
			return &cloudscalesdk.LoadBalancerListener{UUID: id}, nil
		},
	}

	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
			return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: id}, nil
		},
	}

	poolMemberService := &mockLoadBalancerPoolMemberService{
		listFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return nil, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService:  lbService,
		poolService:          poolService,
		listenerService:      listenerService,
		healthMonitorService: healthMonitorService,
		poolMemberService:    poolMemberService,
		lbEnabled:            true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID

	r := newTestReconcilerWithClient(newFakeClientForLB())

	_, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "203.0.113.10", clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host)
	assert.Equal(t, int32(6443), clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port)
}

// ============================================================================
// Tests for deleteLoadBalancer
// ============================================================================

func TestDeleteLoadBalancer_SkipsWhenDisabled(t *testing.T) {
	lbService := &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when LB is disabled")
			return nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           false,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID

	r := newTestReconciler()

	err := r.deleteLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)
	// Status should not be cleared when LB is disabled
	assert.Equal(t, testLBUUID, clusterScope.CloudscaleCluster.Status.LoadBalancerID)
}

func TestDeleteLoadBalancer_OnlyDeletesLB(t *testing.T) {
	var deletedLBID string
	poolDeleteCalled := false
	listenerDeleteCalled := false
	hmDeleteCalled := false

	lbService := &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedLBID = id
			return nil
		},
	}

	poolService := &mockLoadBalancerPoolService{
		deleteFn: func(ctx context.Context, id string) error {
			poolDeleteCalled = true
			return nil
		},
	}

	listenerService := &mockLoadBalancerListenerService{
		deleteFn: func(ctx context.Context, id string) error {
			listenerDeleteCalled = true
			return nil
		},
	}

	healthMonitorService := &mockLoadBalancerHealthMonitorService{
		deleteFn: func(ctx context.Context, id string) error {
			hmDeleteCalled = true
			return nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService:  lbService,
		poolService:          poolService,
		listenerService:      listenerService,
		healthMonitorService: healthMonitorService,
		lbEnabled:            true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID

	r := newTestReconciler()

	err := r.deleteLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)

	// Only the LB itself should be deleted (child resources are cascade-deleted by the API)
	assert.Equal(t, testLBUUID, deletedLBID)
	assert.False(t, poolDeleteCalled, "pool delete should not be called")
	assert.False(t, listenerDeleteCalled, "listener delete should not be called")
	assert.False(t, hmDeleteCalled, "health monitor delete should not be called")
}

func TestDeleteLoadBalancer_IgnoresNotFoundErrors(t *testing.T) {
	lbService := &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error {
			return &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID

	r := newTestReconciler()

	err := r.deleteLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)
}

func TestDeleteLoadBalancer_ClearsStatusIDs(t *testing.T) {
	lbService := &mockLoadBalancerService{
		deleteFn: func(ctx context.Context, id string) error { return nil },
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		loadBalancerService: lbService,
		lbEnabled:           true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"member-1", "member-2"}

	r := newTestReconciler()

	err := r.deleteLoadBalancer(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID)
	assert.Nil(t, clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs)
}

// ============================================================================
// Tests for reconcileLBMembers
// ============================================================================

func TestReconcileLBMembers_AddsMissingMember(t *testing.T) {
	var createdReq *cloudscalesdk.LoadBalancerPoolMemberRequest

	poolMemberService := &mockLoadBalancerPoolMemberService{
		listFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return nil, nil // no existing members
		},
		createFn: func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
			createdReq = req
			return &cloudscalesdk.LoadBalancerPoolMember{UUID: "new-member-uuid", Name: req.Name, Address: req.Address}, nil
		},
	}

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-machine-1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
			},
		},
	}

	k8sClient := newFakeClientForLB(machine)
	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolMemberService: poolMemberService,
		lbEnabled:         true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-uuid"

	r := newTestReconcilerWithClient(k8sClient)

	err := r.reconcileLBMembers(context.Background(), clusterScope)

	require.NoError(t, err)
	require.NotNil(t, createdReq)
	assert.Equal(t, "cp-machine-1", createdReq.Name)
	assert.Equal(t, "10.0.0.1", createdReq.Address)
	assert.Contains(t, clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs, "new-member-uuid")
}

func TestReconcileLBMembers_RemovesStaleMember(t *testing.T) {
	var deletedMemberID string

	poolMemberService := &mockLoadBalancerPoolMemberService{
		listFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return []cloudscalesdk.LoadBalancerPoolMember{
				{UUID: "stale-member-uuid", Name: "removed-machine", Address: "10.0.0.99"},
			}, nil
		},
		deleteFn: func(ctx context.Context, poolID, memberID string) error {
			deletedMemberID = memberID
			return nil
		},
	}

	// No machines in cluster
	k8sClient := newFakeClientForLB()
	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolMemberService: poolMemberService,
		lbEnabled:         true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"stale-member-uuid"}

	r := newTestReconcilerWithClient(k8sClient)

	err := r.reconcileLBMembers(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "stale-member-uuid", deletedMemberID)
	assert.NotContains(t, clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs, "stale-member-uuid")
}

func TestReconcileLBMembers_UpdatesChangedAddress(t *testing.T) {
	var updatedMemberID string
	var updatedReq *cloudscalesdk.LoadBalancerPoolMemberRequest

	poolMemberService := &mockLoadBalancerPoolMemberService{
		listFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return []cloudscalesdk.LoadBalancerPoolMember{
				{UUID: "member-uuid-1", Name: "cp-machine-1", Address: "10.0.0.1"},
			}, nil
		},
		updateFn: func(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error {
			updatedMemberID = memberID
			updatedReq = req
			return nil
		},
	}

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-machine-1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.2"}, // changed IP
			},
		},
	}

	k8sClient := newFakeClientForLB(machine)
	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolMemberService: poolMemberService,
		lbEnabled:         true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"member-uuid-1"}

	r := newTestReconcilerWithClient(k8sClient)

	err := r.reconcileLBMembers(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "member-uuid-1", updatedMemberID)
	require.NotNil(t, updatedReq)
	assert.Equal(t, "10.0.0.2", updatedReq.Address)
}

func TestReconcileLBMembers_NoopWhenInSync(t *testing.T) {
	poolMemberService := &mockLoadBalancerPoolMemberService{
		listFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return []cloudscalesdk.LoadBalancerPoolMember{
				{UUID: "member-uuid-1", Name: "cp-machine-1", Address: "10.0.0.1"},
			}, nil
		},
		createFn: func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
			t.Fatal("Create should not be called when in sync")
			return nil, nil
		},
		deleteFn: func(ctx context.Context, poolID, memberID string) error {
			t.Fatal("Delete should not be called when in sync")
			return nil
		},
		updateFn: func(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error {
			t.Fatal("Update should not be called when in sync")
			return nil
		},
	}

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-machine-1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
			},
		},
	}

	k8sClient := newFakeClientForLB(machine)
	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolMemberService: poolMemberService,
		lbEnabled:         true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"member-uuid-1"}

	r := newTestReconcilerWithClient(k8sClient)

	err := r.reconcileLBMembers(context.Background(), clusterScope)

	require.NoError(t, err)
}

// ============================================================================
// Tests for getDesiredLoadBalancerMembers
// ============================================================================

func TestGetDesiredLoadBalancerMembers_SkipsMachinesWithoutIP(t *testing.T) {
	machineWithIP := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-machine-1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
			},
		},
	}
	machineWithoutIP := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-machine-2",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: nil, // no addresses yet
		},
	}

	k8sClient := newFakeClientForLB(machineWithIP, machineWithoutIP)
	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		lbEnabled: true,
	})
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-uuid"

	r := newTestReconcilerWithClient(k8sClient)

	members, err := r.getDesiredLoadBalancerMembers(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Len(t, members, 1)
	assert.Equal(t, "cp-machine-1", members[0].Name)
	assert.Equal(t, "10.0.0.1", members[0].Address)
}

func TestGetDesiredLoadBalancerMembers_NoMachines(t *testing.T) {
	k8sClient := newFakeClientForLB()
	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		lbEnabled: true,
	})

	r := newTestReconcilerWithClient(k8sClient)

	members, err := r.getDesiredLoadBalancerMembers(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Empty(t, members)
}

// ============================================================================
// Tests for createLoadBalancerMember / deleteLoadBalancerMember
// ============================================================================

func TestCreateLoadBalancerMember_AppendsToStatus(t *testing.T) {
	poolMemberService := &mockLoadBalancerPoolMemberService{
		createFn: func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
			return &cloudscalesdk.LoadBalancerPoolMember{UUID: "new-uuid", Name: req.Name}, nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolMemberService: poolMemberService,
		lbEnabled:         true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"existing-uuid"}

	r := newTestReconciler()

	err := r.createLoadBalancerMember(context.Background(), clusterScope, cloudscalesdk.LoadBalancerPoolMemberRequest{
		Name:    "cp-machine-1",
		Address: "10.0.0.1",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"existing-uuid", "new-uuid"}, clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs)
}

func TestDeleteLoadBalancerMember_RemovesFromStatus(t *testing.T) {
	poolMemberService := &mockLoadBalancerPoolMemberService{
		deleteFn: func(ctx context.Context, poolID, memberID string) error {
			return nil
		},
	}

	clusterScope := newTestClusterScopeWithLB(lbTestScopeOptions{
		poolMemberService: poolMemberService,
		lbEnabled:         true,
	})
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"keep-uuid", "remove-uuid"}

	r := newTestReconciler()

	err := r.deleteLoadBalancerMember(context.Background(), clusterScope, cloudscalesdk.LoadBalancerPoolMember{
		UUID: "remove-uuid",
		Name: "cp-machine-old",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"keep-uuid"}, clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs)
}

// Mock services for Load Balancer components

type mockLoadBalancerService struct {
	createFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error)
	getFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error)
	listFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error)
	deleteFn func(ctx context.Context, id string) error
	updateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerRequest) error
}

func (m *mockLoadBalancerService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockLoadBalancerService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockLoadBalancerService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockLoadBalancerService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockLoadBalancerService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

type mockLoadBalancerPoolService struct {
	createFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error)
	getFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error)
	listFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error)
	deleteFn func(ctx context.Context, id string) error
	updateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerPoolRequest) error
}

func (m *mockLoadBalancerPoolService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockLoadBalancerPoolService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockLoadBalancerPoolService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockLoadBalancerPoolService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockLoadBalancerPoolService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerPoolRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

type mockLoadBalancerListenerService struct {
	createFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error)
	getFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error)
	listFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error)
	deleteFn func(ctx context.Context, id string) error
	updateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerListenerRequest) error
}

func (m *mockLoadBalancerListenerService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockLoadBalancerListenerService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockLoadBalancerListenerService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockLoadBalancerListenerService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockLoadBalancerListenerService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerListenerRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

type mockLoadBalancerHealthMonitorService struct {
	createFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error)
	getFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error)
	listFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error)
	deleteFn func(ctx context.Context, id string) error
	updateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) error
}

func (m *mockLoadBalancerHealthMonitorService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockLoadBalancerHealthMonitorService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockLoadBalancerHealthMonitorService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockLoadBalancerHealthMonitorService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockLoadBalancerHealthMonitorService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

type mockLoadBalancerPoolMemberService struct {
	createFn func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error)
	getFn    func(ctx context.Context, poolID, memberID string) (*cloudscalesdk.LoadBalancerPoolMember, error)
	listFn   func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error)
	deleteFn func(ctx context.Context, poolID, memberID string) error
	updateFn func(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error
}

func (m *mockLoadBalancerPoolMemberService) Create(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
	if m.createFn != nil {
		return m.createFn(ctx, poolID, req)
	}
	return nil, nil
}

func (m *mockLoadBalancerPoolMemberService) Get(ctx context.Context, poolID, memberID string) (*cloudscalesdk.LoadBalancerPoolMember, error) {
	if m.getFn != nil {
		return m.getFn(ctx, poolID, memberID)
	}
	return nil, nil
}

func (m *mockLoadBalancerPoolMemberService) List(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
	if m.listFn != nil {
		return m.listFn(ctx, poolID, modifiers...)
	}
	return nil, nil
}

func (m *mockLoadBalancerPoolMemberService) Delete(ctx context.Context, poolID, memberID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, poolID, memberID)
	}
	return nil
}

func (m *mockLoadBalancerPoolMemberService) Update(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, poolID, memberID, req)
	}
	return nil
}
