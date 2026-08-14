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
	"errors"
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const (
	routerUUID    = "router-uuid-123"
	subnetUUID    = "subnet-uuid-123"
	ifaceUUID     = "iface-uuid-123"
	assignedGWv4  = "10.0.0.5"
	explicitGWv4  = "10.0.0.1"
	testNetworkID = "net-uuid-123"
)

func TestReconcileRouters_SingleRouter_SingleInterface(t *testing.T) {
	g := NewWithT(t)

	var capturedCreate *cloudscalesdk.RouterCreateRequest
	var capturedIface *cloudscalesdk.CreateInterfaceRequest
	var capturedUpdate *cloudscalesdk.SubnetUpdateRequest

	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			capturedCreate = req
			return &cloudscalesdk.Router{UUID: routerUUID, Name: req.Name, InternetGateway: true, Status: cloudscalesdk.RouterActive}, nil
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			capturedIface = &req
			return &cloudscalesdk.RouterInterface{
				UUID:      ifaceUUID,
				Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
				Addresses: []cloudscalesdk.IPAddress{{Address: assignedGWv4, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
			}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: subnetUUID, GatewayAddress: ""}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			capturedUpdate = req
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name:            "nat-gw",
			InternetGateway: true,
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "test", Address: "10.0.0.3", ConfigureSubnetGateway: new(true)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	g.Expect(capturedCreate).NotTo(BeNil())
	g.Expect(capturedCreate.InternetGateway).To(BeTrue())
	g.Expect(capturedCreate.Name).To(Equal("nat-gw"))
	g.Expect(capturedCreate.Zone).To(Equal("rma1"))

	g.Expect(capturedIface).NotTo(BeNil())
	g.Expect(capturedIface.Network).To(Equal(testNetworkID))
	g.Expect(capturedIface.Addresses).To(HaveLen(1))
	g.Expect(capturedIface.Addresses[0].Subnet).To(Equal(subnetUUID))
	// The controller requests the webhook-defaulted per-interface address verbatim.
	g.Expect(capturedIface.Addresses[0].Address).To(Equal("10.0.0.3"))

	g.Expect(capturedUpdate).NotTo(BeNil())
	g.Expect(capturedUpdate.GatewayAddress).To(Equal(assignedGWv4))

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("nat-gw")
	g.Expect(rs).NotTo(BeNil())
	g.Expect(rs.RouterID).To(Equal(routerUUID))
	g.Expect(rs.InterfaceIDs["test"]).To(Equal(ifaceUUID))
}

func TestReconcileRouters_SingleRouter_ExplicitGateway(t *testing.T) {
	g := NewWithT(t)

	var capturedIface *cloudscalesdk.CreateInterfaceRequest
	var capturedUpdate *cloudscalesdk.SubnetUpdateRequest

	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: routerUUID, Name: req.Name, InternetGateway: true, Status: cloudscalesdk.RouterActive}, nil
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			capturedIface = &req
			return &cloudscalesdk.RouterInterface{
				UUID:      ifaceUUID,
				Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
				Addresses: []cloudscalesdk.IPAddress{{Address: explicitGWv4, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
			}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: subnetUUID, GatewayAddress: ""}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			capturedUpdate = req
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Networks[0].GatewayAddress = explicitGWv4
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name:            "nat-gw",
			InternetGateway: true,
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "test", Address: explicitGWv4, ConfigureSubnetGateway: new(true)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	g.Expect(capturedIface).NotTo(BeNil())
	g.Expect(capturedIface.Addresses[0].Address).To(Equal(explicitGWv4))
	g.Expect(capturedUpdate.GatewayAddress).To(Equal(explicitGWv4))
}

func TestReconcileRouters_SingleRouter_MultiInterface(t *testing.T) {
	g := NewWithT(t)

	const (
		net1ID     = "net-uuid-1"
		net2ID     = "net-uuid-2"
		subnet1ID  = "subnet-uuid-1"
		subnet2ID  = "subnet-uuid-2"
		iface1UUID = "iface-uuid-1"
		iface2UUID = "iface-uuid-2"
		gwIP1      = "10.1.0.5"
		gwIP2      = "10.2.0.5"
	)

	createIfaceCount := 0
	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: routerUUID, Name: req.Name, Status: cloudscalesdk.RouterActive}, nil
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			createIfaceCount++
			if req.Network == net1ID {
				return &cloudscalesdk.RouterInterface{
					UUID:      iface1UUID,
					Network:   cloudscalesdk.NetworkStub{UUID: net1ID},
					Addresses: []cloudscalesdk.IPAddress{{Address: gwIP1, Subnet: cloudscalesdk.SubnetStub{UUID: subnet1ID}}},
				}, nil
			}
			return &cloudscalesdk.RouterInterface{
				UUID:      iface2UUID,
				Network:   cloudscalesdk.NetworkStub{UUID: net2ID},
				Addresses: []cloudscalesdk.IPAddress{{Address: gwIP2, Subnet: cloudscalesdk.SubnetStub{UUID: subnet2ID}}},
			}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: id, GatewayAddress: ""}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("net1", net1ID, subnet1ID, "10.1.0.0/24"),
		testutils.WithPreExistingNetwork("net2", net2ID, subnet2ID, "10.2.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name: "shared-router",
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "net1", ConfigureSubnetGateway: new(true)},
				{Network: "net2", ConfigureSubnetGateway: new(true)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(createIfaceCount).To(Equal(2))

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("shared-router")
	g.Expect(rs).NotTo(BeNil())
	g.Expect(rs.InterfaceIDs["net1"]).To(Equal(iface1UUID))
	g.Expect(rs.InterfaceIDs["net2"]).To(Equal(iface2UUID))
}

func TestReconcileRouters_PreExistingRouter_AdoptByUUID(t *testing.T) {
	g := NewWithT(t)

	createCalled := false
	var capturedIface *cloudscalesdk.CreateInterfaceRequest

	routerSvc := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			g.Expect(id).To(Equal(routerUUID))
			return &cloudscalesdk.Router{UUID: routerUUID, Status: cloudscalesdk.RouterActive}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			createCalled = true
			return nil, errors.New("create must not be called for pre-existing router")
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			capturedIface = &req
			return &cloudscalesdk.RouterInterface{
				UUID:      ifaceUUID,
				Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
				Addresses: []cloudscalesdk.IPAddress{{Address: assignedGWv4, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
			}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: id, GatewayAddress: ""}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name: "my-router",
			UUID: routerUUID,
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "test", ConfigureSubnetGateway: new(true)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(createCalled).To(BeFalse())
	g.Expect(capturedIface).NotTo(BeNil())

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("my-router")
	g.Expect(rs).NotTo(BeNil())
	g.Expect(rs.RouterID).To(Equal(routerUUID))
}

func TestReconcileRouters_ConfigureSubnetGateway_False(t *testing.T) {
	g := NewWithT(t)

	subnetGetCalled := false
	subnetUpdateCalled := false
	var capturedIface *cloudscalesdk.CreateInterfaceRequest

	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: routerUUID, Status: cloudscalesdk.RouterActive}, nil
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			capturedIface = &req
			return &cloudscalesdk.RouterInterface{
				UUID:      ifaceUUID,
				Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
				Addresses: []cloudscalesdk.IPAddress{{Address: "10.0.0.4", Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
			}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			subnetGetCalled = true
			return &cloudscalesdk.Subnet{UUID: id}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			subnetUpdateCalled = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("transit", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name: "spoke-router",
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "transit", Address: "10.0.0.4", ConfigureSubnetGateway: new(false)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	// A non-owner interface requests its own sibling address, not the subnet gateway.
	g.Expect(capturedIface).NotTo(BeNil())
	g.Expect(capturedIface.Addresses[0].Address).To(Equal("10.0.0.4"))

	// Subnet.Get and Subnet.Update must NOT be called when ConfigureSubnetGateway is false.
	g.Expect(subnetGetCalled).To(BeFalse())
	g.Expect(subnetUpdateCalled).To(BeFalse())
}

func TestReconcileRouters_SharedNetwork_HubAndSpoke(t *testing.T) {
	// Three routers attach to one shared transit network. Each interface must
	// request its own distinct, webhook-defaulted address (no "already allocated"
	// collision), and only the ConfigureSubnetGateway owner updates the subnet.
	g := NewWithT(t)

	requestedAddrs := []string{}
	subnetUpdates := 0

	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: "router-" + req.Name, Name: req.Name, Status: cloudscalesdk.RouterActive}, nil
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			addr := req.Addresses[0].Address
			requestedAddrs = append(requestedAddrs, addr)
			return &cloudscalesdk.RouterInterface{
				UUID:      "iface-" + addr,
				Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
				Addresses: []cloudscalesdk.IPAddress{{Address: addr, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
			}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: id, GatewayAddress: ""}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			subnetUpdates++
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("transit", testNetworkID, subnetUUID, "10.10.3.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name: "cp-router",
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "transit", Address: "10.10.3.4", ConfigureSubnetGateway: new(false)},
			},
		},
		{
			Name: "worker-router",
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "transit", Address: "10.10.3.5", ConfigureSubnetGateway: new(false)},
			},
		},
		{
			Name:            "gw-router",
			InternetGateway: true,
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "transit", Address: "10.10.3.3", ConfigureSubnetGateway: new(true)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	// Each router requested its own distinct address — no collision.
	g.Expect(requestedAddrs).To(ConsistOf("10.10.3.4", "10.10.3.5", "10.10.3.3"))
	// Only the gateway owner (gw-router) updates the subnet gateway.
	g.Expect(subnetUpdates).To(Equal(1))
}

func TestReconcileRouters_RequeueWhileNotActive(t *testing.T) {
	g := NewWithT(t)

	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: routerUUID, Status: "changing"}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name: "nat-gw",
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "test", ConfigureSubnetGateway: new(true)},
			},
		},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("nat-gw")
	g.Expect(rs).NotTo(BeNil())
	g.Expect(rs.RouterID).To(Equal(routerUUID))
}

func TestReconcileRouters_Idempotent(t *testing.T) {
	g := NewWithT(t)

	createRouterCalled := false
	createIfaceCalled := false
	subnetUpdateCalled := false

	routerSvc := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{
				UUID:   routerUUID,
				Status: cloudscalesdk.RouterActive,
				Interfaces: []cloudscalesdk.RouterInterface{{
					UUID:      ifaceUUID,
					Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
					Addresses: []cloudscalesdk.IPAddress{{Address: assignedGWv4, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
				}},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			createRouterCalled = true
			return nil, errors.New("create must not be called on idempotent reconcile")
		},
		CreateInterfaceFn: func(ctx context.Context, id string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			createIfaceCalled = true
			return nil, errors.New("CreateInterface must not be called when interface already exists")
		},
	}
	subnetSvc := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: id, GatewayAddress: assignedGWv4}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			subnetUpdateCalled = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{
			Name: "nat-gw",
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
				{Network: "test", ConfigureSubnetGateway: new(true)},
			},
		},
	}
	// Pre-populate status as if previously reconciled.
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{Name: "nat-gw", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}},
	}
	clusterScope.CloudscaleCluster.Status.Networks[0].GatewayAddress = assignedGWv4

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(createRouterCalled).To(BeFalse())
	g.Expect(createIfaceCalled).To(BeFalse())
	g.Expect(subnetUpdateCalled).To(BeFalse())
}

func TestReconcileRouters_NoRouters_Noop(t *testing.T) {
	g := NewWithT(t)

	routerSvc := &testutils.MockRouterService{}
	subnetSvc := &testutils.MockSubnetService{}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	// spec.Routers is empty by default.

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
}

func TestReconcileRouters_MultiRouter_FirstChanging_SecondStillCreated(t *testing.T) {
	// When the first router is still in "changing" status, the second router must
	// still be created in the same reconcile cycle (no early return on first requeue).
	g := NewWithT(t)

	createCount := 0
	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil // no existing routers
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			createCount++
			return &cloudscalesdk.Router{UUID: routerUUID, Name: req.Name, Status: "changing"}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "router-1"},
		{Name: "router-2"},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	g.Expect(createCount).To(Equal(2), "both routers must be created even though the first is still changing")
}

func TestReconcileRouters_RequeueWithNilError_SetsNotReadyCondition(t *testing.T) {
	// When reconcileRouters returns (RequeueAfter, nil) because a router is not
	// yet active, RouterReadyCondition must be set to False so operators can see
	// the cluster is waiting — not left unset from the previous reconcile.
	g := NewWithT(t)

	routerSvc := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: routerUUID, Status: "changing"}, nil
		},
	}
	subnetSvc := &testutils.MockSubnetService{}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithSubnetService(subnetSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "nat-gw"},
	}

	r := newTestReconciler()
	result, err := r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))

	cond := apimeta.FindStatusCondition(
		clusterScope.CloudscaleCluster.Status.Conditions,
		string(infrastructurev1beta2.RouterReadyCondition),
	)
	g.Expect(cond).NotTo(BeNil(), "RouterReadyCondition must be set when requeuing")
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse), "RouterReadyCondition must be False while waiting")
}

func TestDeleteRouters_Managed_WithInterfaces_DeletesInterfacesFirst(t *testing.T) {
	// A managed router with tracked interfaces must have its interfaces deleted
	// before the router itself is deleted.
	g := NewWithT(t)

	deleteIfaceCalled := false
	deleteRouterCalled := false
	routerSvc := &testutils.MockRouterService{
		DeleteInterfaceFn: func(ctx context.Context, routerID, ifaceID string) error {
			g.Expect(deleteRouterCalled).To(BeFalse(), "DeleteInterface must be called before Delete")
			deleteIfaceCalled = true
			return nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			g.Expect(deleteIfaceCalled).To(BeTrue(), "Delete must be called after DeleteInterface")
			deleteRouterCalled = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "nat-gw"},
	}
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{Name: "nat-gw", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}},
	}

	r := newTestReconciler()
	err := r.deleteRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(deleteIfaceCalled).To(BeTrue())
	g.Expect(deleteRouterCalled).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].RouterID).To(Equal(""))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].InterfaceIDs).To(BeEmpty())
}

func TestDeleteRouters_Managed(t *testing.T) {
	g := NewWithT(t)

	var deletedRouter string
	routerSvc := &testutils.MockRouterService{
		DeleteFn: func(ctx context.Context, id string) error {
			deletedRouter = id
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "nat-gw"},
	}
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{Name: "nat-gw", RouterID: routerUUID},
	}

	r := newTestReconciler()
	err := r.deleteRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(deletedRouter).To(Equal(routerUUID))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].RouterID).To(Equal(""))
}

func TestDeleteRouters_PreExisting_DetachOnly(t *testing.T) {
	g := NewWithT(t)

	deletedRouterCalled := false
	deletedIfaces := map[string]bool{}

	routerSvc := &testutils.MockRouterService{
		DeleteFn: func(ctx context.Context, id string) error {
			deletedRouterCalled = true
			return errors.New("delete must not be called for pre-existing router")
		},
		DeleteInterfaceFn: func(ctx context.Context, routerID, ifaceID string) error {
			deletedIfaces[ifaceID] = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "my-router", UUID: routerUUID},
	}
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{
			Name:         "my-router",
			RouterID:     routerUUID,
			InterfaceIDs: map[string]string{"test": ifaceUUID},
		},
	}

	r := newTestReconciler()
	err := r.deleteRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(deletedRouterCalled).To(BeFalse())
	g.Expect(deletedIfaces[ifaceUUID]).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].InterfaceIDs).NotTo(HaveKey("test"),
		"interface UUID must be cleared from status once the interface is confirmed gone")
}

func TestDeleteRouters_Managed_InterfaceDeleteError_RouterNotDeleted(t *testing.T) {
	// When an interface delete fails, the router must not be deleted and the
	// interface must stay in status so the next reconcile retries.
	g := NewWithT(t)

	deleteRouterCalled := false
	routerSvc := &testutils.MockRouterService{
		DeleteInterfaceFn: func(ctx context.Context, routerID, ifaceID string) error {
			return errors.New("API 500 deleting interface")
		},
		DeleteFn: func(ctx context.Context, id string) error {
			deleteRouterCalled = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "nat-gw"},
	}
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{Name: "nat-gw", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}},
	}

	r := newTestReconciler()
	err := r.deleteRouters(context.Background(), clusterScope)
	g.Expect(err).To(HaveOccurred())
	g.Expect(deleteRouterCalled).To(BeFalse())
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].RouterID).To(Equal(routerUUID))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].InterfaceIDs).To(HaveKey("test"))
}

func TestDeleteRouters_Managed_DeleteError_Surfaced(t *testing.T) {
	// A failing router delete must be surfaced as an error and leave RouterID in
	// status so the next reconcile retries.
	g := NewWithT(t)

	routerSvc := &testutils.MockRouterService{
		DeleteFn: func(ctx context.Context, id string) error {
			return errors.New("API 500 internal server error")
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "nat-gw"},
	}
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{Name: "nat-gw", RouterID: routerUUID},
	}

	r := newTestReconciler()
	err := r.deleteRouters(context.Background(), clusterScope)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("internal server error"))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].RouterID).To(Equal(routerUUID))
}

func TestDeleteRouters_PreExisting_MultiInterface(t *testing.T) {
	// A pre-existing router with two interfaces must have both interfaces detached
	// and cleared from status, without deleting the router itself.
	g := NewWithT(t)

	const (
		iface1UUID = "iface-uuid-1"
		iface2UUID = "iface-uuid-2"
	)

	deletedIfaces := map[string]bool{}

	routerSvc := &testutils.MockRouterService{
		DeleteInterfaceFn: func(ctx context.Context, routerID, ifaceID string) error {
			deletedIfaces[ifaceID] = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerSvc),
		testutils.WithPreExistingNetwork("net1", "net-uuid-1", "subnet-uuid-1", "10.1.0.0/24"),
		testutils.WithPreExistingNetwork("net2", "net-uuid-2", "subnet-uuid-2", "10.2.0.0/24"),
	)
	clusterScope.CloudscaleCluster.Spec.Routers = []infrastructurev1beta2.RouterSpec{
		{Name: "my-router", UUID: routerUUID},
	}
	clusterScope.CloudscaleCluster.Status.Routers = []infrastructurev1beta2.RouterStatus{
		{
			Name:     "my-router",
			RouterID: routerUUID,
			InterfaceIDs: map[string]string{
				"net1": iface1UUID,
				"net2": iface2UUID,
			},
		},
	}

	r := newTestReconciler()
	err := r.deleteRouters(context.Background(), clusterScope)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(deletedIfaces[iface1UUID]).To(BeTrue())
	g.Expect(deletedIfaces[iface2UUID]).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].InterfaceIDs).NotTo(HaveKey("net1"))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].InterfaceIDs).NotTo(HaveKey("net2"))
}
