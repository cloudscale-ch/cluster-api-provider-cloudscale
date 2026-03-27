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

	"github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
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
				Networks: []infrastructurev1beta2.NetworkSpec{
					{
						Name: "test",
						CIDR: "10.0.0.0/24",
					},
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
	g := NewWithT(t)

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

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal(netUUID))
	g.Expect(ns.SubnetID).To(Equal("subnet-uuid-123"))
	g.Expect(capturedNetReq.Name).To(Equal("test"))
	g.Expect(capturedNetReq.Zone).To(Equal("rma1"))
	g.Expect(capturedNetReq.AutoCreateIPV4Subnet).ToNot(BeNil())
	g.Expect(*capturedNetReq.AutoCreateIPV4Subnet).To(BeFalse())
	g.Expect(capturedSubReq.Network).To(Equal(netUUID))
	g.Expect(capturedSubReq.CIDR).To(Equal("10.0.0.0/24"))
	g.Expect(capturedSubReq.GatewayAddress).To(Equal(""))
}

func TestReconcileNetwork_SkipsIfBothExist(t *testing.T) {
	g := NewWithT(t)

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
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "existing-net", SubnetID: "existing-subnet", Managed: true},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("existing-net"))
	g.Expect(ns.SubnetID).To(Equal("existing-subnet"))
}

func TestReconcileNetwork_NetworkErrorStopsSubnet(t *testing.T) {
	g := NewWithT(t)

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

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("api error"))
	g.Expect(clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")).To(BeNil())
}

func TestReconcileNetwork_SubnetErrorSurfaced(t *testing.T) {
	g := NewWithT(t)

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

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("subnet api error"))
}

// --- Managed network sub-resource tests (via reconcileNetwork orchestrator) ---

func TestReconcileNetwork_FindsByTag(t *testing.T) {
	g := NewWithT(t)

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

	clusterScope := newTestClusterScope(networkService, subnetService)
	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("found-net-uuid"))
	g.Expect(ns.SubnetID).To(Equal("found-subnet-uuid"))
}

func TestReconcileNetwork_ErrorsOnMultipleNetworks(t *testing.T) {
	g := NewWithT(t)

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

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("found 2 network/tests matching tag filter"))
}

func TestReconcileNetwork_RecreatesIfDeletedExternally(t *testing.T) {
	g := NewWithT(t)

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
	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			return &cloudscale.Subnet{UUID: "new-subnet-uuid", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "deleted-net-uuid", Managed: true},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(created).To(BeTrue(), "Should create a new network when old one was deleted")
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("new-net-uuid"))
}

// --- Subnet sub-resource tests (via reconcileNetwork orchestrator) ---

func TestReconcileNetwork_SubnetFindsByTag(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: netUUID, Name: req.Name}, nil
		},
	}
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

	clusterScope := newTestClusterScope(networkService, subnetService)
	// Pre-populate network status so the network part is resolved via Get
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: netUUID, Managed: true},
	}
	// The networkService Get should return the existing network
	networkService.getFn = func(ctx context.Context, id string) (*cloudscale.Network, error) {
		return &cloudscale.Network{UUID: id}, nil
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.SubnetID).To(Equal("found-subnet-uuid"))
}

func TestReconcileNetwork_SubnetErrorsOnMultiple(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: id}, nil
		},
	}
	subnetService := &mockSubnetService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Subnet, error) {
			return []cloudscale.Subnet{
				{UUID: "subnet-uuid-1"},
				{UUID: "subnet-uuid-2"},
			}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: netUUID, Managed: true},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("found 2 subnet/tests matching tag filter"))
}

func TestReconcileNetwork_SubnetRecreatesIfDeletedExternally(t *testing.T) {
	g := NewWithT(t)

	var created bool

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: id}, nil
		},
	}
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

	clusterScope := newTestClusterScope(networkService, subnetService)
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: netUUID, SubnetID: "deleted-subnet-uuid", Managed: true},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(created).To(BeTrue(), "Should create a new subnet when old one was deleted")
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.SubnetID).To(Equal("new-subnet-uuid"))
}

func TestReconcileNetwork_CustomCIDR(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscale.SubnetCreateRequest

	networkService := &mockNetworkService{
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: netUUID, Name: req.Name}, nil
		},
	}
	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			capturedReq = req
			return &cloudscale.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	clusterScope.CloudscaleCluster.Spec.Networks[0].CIDR = "192.168.0.0/16"

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq.CIDR).To(Equal("192.168.0.0/16"))
}

func TestReconcileNetwork_ExplicitGateway(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscale.SubnetCreateRequest

	networkService := &mockNetworkService{
		createFn: func(ctx context.Context, req *cloudscale.NetworkCreateRequest) (*cloudscale.Network, error) {
			return &cloudscale.Network{UUID: netUUID, Name: req.Name}, nil
		},
	}
	subnetService := &mockSubnetService{
		createFn: func(ctx context.Context, req *cloudscale.SubnetCreateRequest) (*cloudscale.Subnet, error) {
			capturedReq = req
			return &cloudscale.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, subnetService)
	clusterScope.CloudscaleCluster.Spec.Networks[0].GatewayAddress = "10.0.0.254"

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq.GatewayAddress).To(Equal("10.0.0.254"))
}

// --- Delete tests ---

func TestDeleteNetwork_DeletesNetworkAndClearsStatus(t *testing.T) {
	g := NewWithT(t)

	var deletedID string

	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "net-to-delete", SubnetID: "subnet-to-cascade", Managed: true},
	}

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedID).To(Equal("net-to-delete"))
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(BeNil())
}

func TestDeleteNetwork_SkipsIfNoNetwork(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when no network exists")
			return nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteNetwork_IgnoresAlreadyDeleted(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			return &cloudscale.ErrorResponse{StatusCode: 404}
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "already-deleted-net", SubnetID: "already-deleted-subnet", Managed: true},
	}

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(BeNil())
}

func TestDeleteNetwork_SkipsBYONetwork(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called for BYO networks")
			return nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "byo", NetworkID: "byo-net", SubnetID: "byo-subnet", Managed: false},
	}

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(BeNil())
}

func TestDeleteNetwork_PartialFailureKeepsFailedInStatus(t *testing.T) {
	g := NewWithT(t)

	var deletedIDs []string

	networkService := &mockNetworkService{
		deleteFn: func(ctx context.Context, id string) error {
			if id == "net-2" {
				return fmt.Errorf("api timeout")
			}
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "net-a", NetworkID: "net-1", Managed: true},
		{Name: "net-b", NetworkID: "net-2", Managed: true},
		{Name: "net-c", NetworkID: "net-3", Managed: true},
	}

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("deleting network net-b"))

	// net-1 and net-3 were successfully deleted
	g.Expect(deletedIDs).To(ConsistOf("net-1", "net-3"))

	// Only the failed network remains in status
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(HaveLen(1))
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks[0].Name).To(Equal("net-b"))
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks[0].NetworkID).To(Equal("net-2"))
}

// --- BYO network tests ---

func TestReconcileNetwork_BYOCachedShortCircuits(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			t.Fatal("Get should not be called when BYO status is cached")
			return nil, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "byo-net", UUID: "byo-uuid"},
	}
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "byo-net", NetworkID: "byo-uuid", SubnetID: "byo-subnet-uuid", Managed: false},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("byo-net")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("byo-uuid"))
	g.Expect(ns.SubnetID).To(Equal("byo-subnet-uuid"))
}

func TestReconcileNetwork_BYOFetchesAndSetsStatus(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			g.Expect(id).To(Equal("byo-uuid"))
			return &cloudscale.Network{
				UUID: "byo-uuid",
				Subnets: []cloudscale.SubnetStub{
					{UUID: "discovered-subnet-uuid"},
				},
			}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "byo-net", UUID: "byo-uuid"},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("byo-net")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("byo-uuid"))
	g.Expect(ns.SubnetID).To(Equal("discovered-subnet-uuid"))
	g.Expect(ns.Managed).To(BeFalse())
}

func TestReconcileNetwork_BYOGetError(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			return nil, fmt.Errorf("network not found")
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "byo-net", UUID: "byo-uuid"},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("network not found"))
}

func TestReconcileNetwork_BYONoSubnetsErrors(t *testing.T) {
	g := NewWithT(t)

	networkService := &mockNetworkService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Network, error) {
			return &cloudscale.Network{
				UUID:    "byo-uuid",
				Subnets: []cloudscale.SubnetStub{},
			}, nil
		},
	}

	clusterScope := newTestClusterScope(networkService, &mockSubnetService{})
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "byo-net", UUID: "byo-uuid"},
	}

	r := newTestReconciler()

	err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("has no subnets"))
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
