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

	"github.com/cloudscale-ch/cloudscale-go-sdk/v6"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	cs "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

const netUUID = "net-uuid-123"

// --- Test helpers ---

func newTestClusterScope(networkService cs.NetworkService, subnetService cs.SubnetService) *scope.ClusterScope {
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
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				Region: "rma",
				Network: infrastructurev1beta2.NetworkSpec{
					Zone:           "rma1",
					CIDR:           "10.0.0.0/24",
					GatewayAddress: &defaultGateway,
				},
			},
		},
		CloudscaleClient: &cs.Client{
			Networks: networkService,
			Subnets:  subnetService,
		},
	}
}

func newTestReconciler() *CloudscaleClusterReconciler {
	return &CloudscaleClusterReconciler{
		recorder: events.NewFakeRecorder(10),
	}
}

// --- Orchestrator tests ---

func TestReconcileNetwork_CreatesBothResources(t *testing.T) {
	var capturedNetReq *cloudscale.NetworkCreateRequest
	var capturedSubReq *cloudscale.SubnetCreateRequest

	networkService := &mockNetworkService{
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			capturedNetReq = req
			return &cloudscale.Network{UUID: netUUID, Name: req.Name}, nil
		},
	}
	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			capturedSubReq = req
			return &cloudscale.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, netUUID, clusterScope.CloudscaleCluster.Status.NetworkID)
	assert.Equal(t, "subnet-uuid-123", clusterScope.CloudscaleCluster.Status.SubnetID)
	assert.Equal(t, "test-cluster", capturedNetReq.Name)
	assert.Equal(t, "rma1", capturedNetReq.Zone)
	assert.NotNil(t, capturedNetReq.AutoCreateIPV4Subnet)
	assert.False(t, *capturedNetReq.AutoCreateIPV4Subnet)
	assert.Equal(t, netUUID, capturedSubReq.Network)
	assert.Equal(t, "10.0.0.0/24", capturedSubReq.CIDR)
	assert.Equal(t, "", capturedSubReq.GatewayAddress)
}

func TestReconcileNetwork_SkipsIfBothExist(t *testing.T) {
	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: id}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			t.Fatal("Network create should not be called")
			return nil, nil
		},
	}
	subnetService := &mockSubnetService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Subnet, error) {
			return &cloudscale.Subnet{UUID: id}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			t.Fatal("Subnet create should not be called")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	clusterScope.CloudscaleCluster.Status.NetworkID = "existing-net"
	clusterScope.CloudscaleCluster.Status.SubnetID = "existing-subnet"

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "existing-net", clusterScope.CloudscaleCluster.Status.NetworkID)
	assert.Equal(t, "existing-subnet", clusterScope.CloudscaleCluster.Status.SubnetID)
}

func TestReconcileNetwork_NetworkErrorStopsSubnet(t *testing.T) {
	networkService := &mockNetworkService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Network, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			t.Fatal("Subnet create should not be called when network fails")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "api error")
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.SubnetID)
}

func TestReconcileNetwork_SubnetErrorSurfaced(t *testing.T) {
	networkService := &mockNetworkService{
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: "net-uuid"}, nil
		},
	}
	subnetService := &mockSubnetService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error) {
			return nil, fmt.Errorf("subnet api error")
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "subnet api error")
	assert.Equal(t, "net-uuid", clusterScope.CloudscaleCluster.Status.NetworkID)
}

// --- Network sub-resource tests ---

func TestReconcileNetworkResource_FindsByTag(t *testing.T) {
	networkService := &mockNetworkService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Network, error) {
			return []cloudscale.Network{
				{UUID: "found-net-uuid", Name: "test-cluster"},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			t.Fatal("Create should not be called when network is found by tag")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	r := newTestReconciler()

	err := r.reconcileNetworkResource(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "found-net-uuid", clusterScope.CloudscaleCluster.Status.NetworkID)
}

func TestReconcileNetworkResource_ErrorsOnMultiple(t *testing.T) {
	networkService := &mockNetworkService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Network, error) {
			return []cloudscale.Network{
				{UUID: "net-uuid-1"},
				{UUID: "net-uuid-2"},
			}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	r := newTestReconciler()

	err := r.reconcileNetworkResource(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 networks matching tag filter")
}

func TestReconcileNetworkResource_RecreatesIfDeletedExternally(t *testing.T) {
	var created bool

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			return nil, &cloudscale.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Network, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			created = true
			return &cloudscale.Network{UUID: "new-net-uuid", Name: req.Name}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.NetworkID = "deleted-net-uuid"

	r := newTestReconciler()

	err := r.reconcileNetworkResource(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.True(t, created, "Should create a new network when old one was deleted")
	assert.Equal(t, "new-net-uuid", clusterScope.CloudscaleCluster.Status.NetworkID)
}

// --- Subnet sub-resource tests ---

func TestReconcileSubnet_FindsByTag(t *testing.T) {
	subnetService := &mockSubnetService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error) {
			return []cloudscale.Subnet{
				{UUID: "found-subnet-uuid", CIDR: "10.0.0.0/24"},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			t.Fatal("Create should not be called when subnet is found by tag")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScope(&mockNetworkService{}, subnetService)
	clusterScope.CloudscaleCluster.Status.NetworkID = netUUID

	r := newTestReconciler()

	err := r.reconcileSubnet(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "found-subnet-uuid", clusterScope.CloudscaleCluster.Status.SubnetID)
}

func TestReconcileSubnet_ErrorsOnMultiple(t *testing.T) {
	subnetService := &mockSubnetService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error) {
			return []cloudscale.Subnet{
				{UUID: "subnet-uuid-1"},
				{UUID: "subnet-uuid-2"},
			}, nil
		},
	}

	clusterScope := newTestClusterScope(&mockNetworkService{}, subnetService)
	clusterScope.CloudscaleCluster.Status.NetworkID = netUUID

	r := newTestReconciler()

	err := r.reconcileSubnet(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 subnets matching tag filter")
}

func TestReconcileSubnet_RecreatesIfDeletedExternally(t *testing.T) {
	var created bool

	subnetService := &mockSubnetService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Subnet, error) {
			return nil, &cloudscale.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			created = true
			return &cloudscale.Subnet{UUID: "new-subnet-uuid", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(&mockNetworkService{}, subnetService)
	clusterScope.CloudscaleCluster.Status.NetworkID = netUUID
	clusterScope.CloudscaleCluster.Status.SubnetID = "deleted-subnet-uuid"

	r := newTestReconciler()

	err := r.reconcileSubnet(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.True(t, created, "Should create a new subnet when old one was deleted")
	assert.Equal(t, "new-subnet-uuid", clusterScope.CloudscaleCluster.Status.SubnetID)
}

func TestReconcileSubnet_CustomCIDR(t *testing.T) {
	var capturedReq *cloudscale.SubnetCreateRequest

	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			capturedReq = req
			return &cloudscale.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(&mockNetworkService{}, subnetService)
	clusterScope.CloudscaleCluster.Status.NetworkID = netUUID
	clusterScope.CloudscaleCluster.Spec.Network.CIDR = "192.168.0.0/16"

	r := newTestReconciler()

	err := r.reconcileSubnet(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "192.168.0.0/16", capturedReq.CIDR)
}

func TestReconcileSubnet_ExplicitGateway(t *testing.T) {
	var capturedReq *cloudscale.SubnetCreateRequest

	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			capturedReq = req
			return &cloudscale.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(&mockNetworkService{}, subnetService)
	clusterScope.CloudscaleCluster.Status.NetworkID = netUUID
	gateway := "10.0.0.254"
	clusterScope.CloudscaleCluster.Spec.Network.GatewayAddress = &gateway

	r := newTestReconciler()

	err := r.reconcileSubnet(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "10.0.0.254", capturedReq.GatewayAddress)
}

func TestReconcileSubnet_FailsIfNoNetwork(t *testing.T) {
	clusterScope := newTestClusterScope(&mockNetworkService{}, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.NetworkID = ""

	r := newTestReconciler()

	err := r.reconcileSubnet(context.Background(), clusterScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "network must be created before subnet")
}

// --- Delete tests ---

func TestDeleteNetwork_DeletesNetworkAndClearsBothIDs(t *testing.T) {
	var deletedID string

	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.NetworkID = "net-to-delete"
	clusterScope.CloudscaleCluster.Status.SubnetID = "subnet-to-cascade"

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Equal(t, "net-to-delete", deletedID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.NetworkID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.SubnetID)
}

func TestDeleteNetwork_SkipsIfNoNetwork(t *testing.T) {
	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when no network exists")
			return nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	require.NoError(t, err)
}

func TestDeleteNetwork_IgnoresAlreadyDeleted(t *testing.T) {
	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			return &cloudscale.ErrorResponse{StatusCode: 404}
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.NetworkID = "already-deleted-net"
	clusterScope.CloudscaleCluster.Status.SubnetID = "already-deleted-subnet"

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	require.NoError(t, err)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.NetworkID)
	assert.Empty(t, clusterScope.CloudscaleCluster.Status.SubnetID)
}

// --- Mock services ---

type mockNetworkService struct {
	createFn func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error)
	getFn    func(ctx context.Context, id string) (*cloudscale.Network, error)
	listFn   func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Network, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockNetworkService) Create(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockNetworkService) Get(ctx context.Context, id string) (*cloudscale.Network, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockNetworkService) List(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Network, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockNetworkService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

type mockSubnetService struct {
	createFn func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error)
	getFn    func(ctx context.Context, id string) (*cloudscale.Subnet, error)
	listFn   func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockSubnetService) Create(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockSubnetService) Get(ctx context.Context, id string) (*cloudscale.Subnet, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockSubnetService) List(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockSubnetService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
