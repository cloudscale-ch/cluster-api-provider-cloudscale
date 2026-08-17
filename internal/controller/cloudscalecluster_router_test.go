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
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const (
	routerUUID    = "router-uuid-123"
	subnetUUID    = "subnet-uuid-123"
	ifaceUUID     = "iface-uuid-123"
	assignedGWv4  = "10.0.0.3"
	testNetworkID = "net-uuid-123"
)

// preNet describes a pre-existing network to seed into cluster status.
type preNet struct {
	name, netID, subnetID, cidr string
}

// routerHarness holds recording Router+Subnet mocks for reconcile tests. The
// CreateInterface mock echoes the requested address back as the assigned IP,
// mirroring the cloudscale.ch API.
type routerHarness struct {
	createReqs    []*cloudscalesdk.RouterCreateRequest
	ifaceReqs     []cloudscalesdk.CreateInterfaceRequest
	subnetUpdates []*cloudscalesdk.SubnetUpdateRequest
	router        *testutils.MockRouterService
	subnet        *testutils.MockSubnetService
}

// newRouterHarness builds the standard recording mocks. status is the router
// status returned by Create (defaults to active); getFn overrides the Get mock
// (used for pre-existing / idempotent cases).
func newRouterHarness(status string, getFn func(context.Context, string) (*cloudscalesdk.Router, error)) *routerHarness {
	if status == "" {
		status = cloudscalesdk.RouterActive
	}
	h := &routerHarness{}
	h.router = &testutils.MockRouterService{
		ListFn: func(context.Context, ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(_ context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			h.createReqs = append(h.createReqs, req)
			return &cloudscalesdk.Router{UUID: routerUUID, Name: req.Name, InternetGateway: req.InternetGateway, Status: status}, nil
		},
		CreateInterfaceFn: func(_ context.Context, _ string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			h.ifaceReqs = append(h.ifaceReqs, req)
			addr := req.Addresses[0]
			return &cloudscalesdk.RouterInterface{
				UUID:      "iface-" + addr.Address,
				Network:   cloudscalesdk.NetworkStub{UUID: req.Network},
				Addresses: []cloudscalesdk.IPAddress{{Address: addr.Address, Subnet: cloudscalesdk.SubnetStub{UUID: addr.Subnet}}},
			}, nil
		},
	}
	if getFn != nil {
		h.router.GetFn = getFn
	} else {
		h.router.GetFn = func(_ context.Context, id string) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{UUID: id, Status: status}, nil
		}
	}
	h.subnet = &testutils.MockSubnetService{
		UpdateFn: func(_ context.Context, _ string, req *cloudscalesdk.SubnetUpdateRequest) error {
			h.subnetUpdates = append(h.subnetUpdates, req)
			return nil
		},
	}
	return h
}

func routerReadyCondition(cs *scopeCluster) *metav1.Condition {
	return apimeta.FindStatusCondition(cs.Status.Conditions, string(infrastructurev1beta2.RouterReadyCondition))
}

// scopeCluster is a short alias for the CloudscaleCluster type used in helpers.
type scopeCluster = infrastructurev1beta2.CloudscaleCluster

func TestReconcileRouters(t *testing.T) {
	cases := []struct {
		name     string
		networks []preNet
		routers  []infrastructurev1beta2.RouterSpec
		// preStatus / preGateways seed status as if a previous reconcile ran.
		preStatus   []infrastructurev1beta2.RouterStatus
		preGateways map[string]string
		status      string // router status returned by Create (default active)
		getFn       func(context.Context, string) (*cloudscalesdk.Router, error)
		assert      func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error)
	}{
		{
			name:     "single router, single interface: create router, interface, and subnet gateway",
			networks: []preNet{{"test", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			routers: []infrastructurev1beta2.RouterSpec{{
				Name:            "nat-gw",
				InternetGateway: true,
				Interfaces:      []infrastructurev1beta2.RouterInterfaceSpec{{Network: "test", Address: assignedGWv4, ConfigureSubnetGateway: new(true)}},
			}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())

				g.Expect(h.createReqs).To(HaveLen(1))
				g.Expect(h.createReqs[0].InternetGateway).To(BeTrue())
				g.Expect(h.createReqs[0].Name).To(Equal("nat-gw"))
				g.Expect(h.createReqs[0].Zone).To(Equal("rma1"))

				g.Expect(h.ifaceReqs).To(HaveLen(1))
				g.Expect(h.ifaceReqs[0].Network).To(Equal(testNetworkID))
				g.Expect(h.ifaceReqs[0].Addresses[0].Subnet).To(Equal(subnetUUID))
				g.Expect(h.ifaceReqs[0].Addresses[0].Address).To(Equal(assignedGWv4))

				g.Expect(h.subnetUpdates).To(HaveLen(1))
				g.Expect(h.subnetUpdates[0].GatewayAddress).To(Equal(assignedGWv4))

				rs := cs.Status.GetRouterStatus("nat-gw")
				g.Expect(rs).NotTo(BeNil())
				g.Expect(rs.RouterID).To(Equal(routerUUID))
				g.Expect(rs.InterfaceIDs["test"]).NotTo(BeEmpty())
			},
		},
		{
			name:     "single router, multiple interfaces: one interface created per network",
			networks: []preNet{{"net1", "net-uuid-1", "subnet-uuid-1", "10.1.0.0/24"}, {"net2", "net-uuid-2", "subnet-uuid-2", "10.2.0.0/24"}},
			routers: []infrastructurev1beta2.RouterSpec{{
				Name: "shared-router",
				Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
					{Network: "net1", Address: "10.1.0.3", ConfigureSubnetGateway: new(true)},
					{Network: "net2", Address: "10.2.0.3", ConfigureSubnetGateway: new(true)},
				},
			}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())
				g.Expect(h.ifaceReqs).To(HaveLen(2))
				rs := cs.Status.GetRouterStatus("shared-router")
				g.Expect(rs.InterfaceIDs["net1"]).NotTo(BeEmpty())
				g.Expect(rs.InterfaceIDs["net2"]).NotTo(BeEmpty())
			},
		},
		{
			name:     "pre-existing router adopted by UUID: Create is never called",
			networks: []preNet{{"test", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			routers: []infrastructurev1beta2.RouterSpec{{
				Name:       "my-router",
				UUID:       routerUUID,
				Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{{Network: "test", Address: assignedGWv4, ConfigureSubnetGateway: new(true)}},
			}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())
				g.Expect(h.createReqs).To(BeEmpty())
				g.Expect(h.ifaceReqs).To(HaveLen(1))
				g.Expect(cs.Status.GetRouterStatus("my-router").RouterID).To(Equal(routerUUID))
			},
		},
		{
			name:     "configureSubnetGateway=false: subnet gateway is not updated",
			networks: []preNet{{"transit", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			routers: []infrastructurev1beta2.RouterSpec{{
				Name:       "spoke-router",
				Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{{Network: "transit", Address: "10.0.0.4", ConfigureSubnetGateway: new(false)}},
			}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())
				g.Expect(h.ifaceReqs[0].Addresses[0].Address).To(Equal("10.0.0.4"))
				g.Expect(h.subnetUpdates).To(BeEmpty())
			},
		},
		{
			name:     "router not active: requeue, record status, set RouterReady False",
			networks: []preNet{{"test", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			status:   "changing",
			routers: []infrastructurev1beta2.RouterSpec{{
				Name:       "nat-gw",
				Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{{Network: "test", ConfigureSubnetGateway: new(true)}},
			}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))
				g.Expect(h.ifaceReqs).To(BeEmpty())
				g.Expect(cs.Status.GetRouterStatus("nat-gw").RouterID).To(Equal(routerUUID))
				cond := routerReadyCondition(cs)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			},
		},
		{
			name:     "multiple routers, first still changing: both are still created",
			networks: []preNet{{"test", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			status:   "changing",
			routers:  []infrastructurev1beta2.RouterSpec{{Name: "router-1"}, {Name: "router-2"}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))
				g.Expect(h.createReqs).To(HaveLen(2), "both routers must be created even though the first is still changing")
			},
		},
		{
			name:     "idempotent second reconcile: nothing is created or updated",
			networks: []preNet{{"test", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			getFn: func(_ context.Context, id string) (*cloudscalesdk.Router, error) {
				return &cloudscalesdk.Router{
					UUID:   id,
					Status: cloudscalesdk.RouterActive,
					Interfaces: []cloudscalesdk.RouterInterface{{
						UUID:      ifaceUUID,
						Network:   cloudscalesdk.NetworkStub{UUID: testNetworkID},
						Addresses: []cloudscalesdk.IPAddress{{Address: assignedGWv4, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}}},
					}},
				}, nil
			},
			routers: []infrastructurev1beta2.RouterSpec{{
				Name:       "nat-gw",
				Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{{Network: "test", ConfigureSubnetGateway: new(true)}},
			}},
			preStatus:   []infrastructurev1beta2.RouterStatus{{Name: "nat-gw", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}}},
			preGateways: map[string]string{"test": assignedGWv4},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())
				g.Expect(h.createReqs).To(BeEmpty())
				g.Expect(h.ifaceReqs).To(BeEmpty())
				g.Expect(h.subnetUpdates).To(BeEmpty())
			},
		},
		{
			name:     "no routers: no-op",
			networks: []preNet{{"test", testNetworkID, subnetUUID, "10.0.0.0/24"}},
			assert: func(g *WithT, h *routerHarness, cs *scopeCluster, result ctrl.Result, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result.IsZero()).To(BeTrue())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			h := newRouterHarness(tc.status, tc.getFn)

			opts := []testutils.ClusterScopeOption{
				testutils.WithRouterService(h.router),
				testutils.WithSubnetService(h.subnet),
			}
			for _, n := range tc.networks {
				opts = append(opts, testutils.WithPreExistingNetwork(n.name, n.netID, n.subnetID, n.cidr))
			}
			clusterScope := testutils.NewClusterScopeOpts(opts...)
			clusterScope.CloudscaleCluster.Spec.Routers = tc.routers
			clusterScope.CloudscaleCluster.Status.Routers = tc.preStatus
			for name, gw := range tc.preGateways {
				if ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(name); ns != nil {
					ns.GatewayAddress = gw
				}
			}

			r := newTestReconciler()
			result, err := r.reconcileRouters(context.Background(), clusterScope)
			tc.assert(g, h, clusterScope.CloudscaleCluster, result, err)
		})
	}
}

// deleteHarness records interface/router delete calls in order and can inject errors.
type deleteHarness struct {
	order          []string
	deleteIfaceErr error
	deleteErr      error
}

func (h *deleteHarness) service() *testutils.MockRouterService {
	return &testutils.MockRouterService{
		DeleteInterfaceFn: func(_ context.Context, _, ifaceID string) error {
			if h.deleteIfaceErr != nil {
				return h.deleteIfaceErr
			}
			h.order = append(h.order, "iface:"+ifaceID)
			return nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			if h.deleteErr != nil {
				return h.deleteErr
			}
			h.order = append(h.order, "router:"+id)
			return nil
		},
	}
}

func TestDeleteRouters(t *testing.T) {
	cases := []struct {
		name           string
		routers        []infrastructurev1beta2.RouterSpec
		preStatus      []infrastructurev1beta2.RouterStatus
		deleteIfaceErr error
		deleteErr      error
		assert         func(g *WithT, h *deleteHarness, cs *scopeCluster, err error)
	}{
		{
			name:      "managed router with interfaces: interfaces deleted before the router",
			routers:   []infrastructurev1beta2.RouterSpec{{Name: "nat-gw"}},
			preStatus: []infrastructurev1beta2.RouterStatus{{Name: "nat-gw", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}}},
			assert: func(g *WithT, h *deleteHarness, cs *scopeCluster, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(h.order).To(Equal([]string{"iface:" + ifaceUUID, "router:" + routerUUID}))
				g.Expect(cs.Status.Routers[0].RouterID).To(BeEmpty())
				g.Expect(cs.Status.Routers[0].InterfaceIDs).To(BeEmpty())
			},
		},
		{
			name:      "pre-existing router: interfaces detached, router left in place",
			routers:   []infrastructurev1beta2.RouterSpec{{Name: "my-router", UUID: routerUUID}},
			preStatus: []infrastructurev1beta2.RouterStatus{{Name: "my-router", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}}},
			assert: func(g *WithT, h *deleteHarness, cs *scopeCluster, err error) {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(h.order).To(Equal([]string{"iface:" + ifaceUUID}), "router itself must not be deleted")
				g.Expect(cs.Status.Routers[0].InterfaceIDs).NotTo(HaveKey("test"))
			},
		},
		{
			name:           "interface delete error: router is not deleted and status is retained",
			routers:        []infrastructurev1beta2.RouterSpec{{Name: "nat-gw"}},
			preStatus:      []infrastructurev1beta2.RouterStatus{{Name: "nat-gw", RouterID: routerUUID, InterfaceIDs: map[string]string{"test": ifaceUUID}}},
			deleteIfaceErr: errors.New("API 500 deleting interface"),
			assert: func(g *WithT, h *deleteHarness, cs *scopeCluster, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(h.order).NotTo(ContainElement("router:" + routerUUID))
				g.Expect(cs.Status.Routers[0].RouterID).To(Equal(routerUUID))
				g.Expect(cs.Status.Routers[0].InterfaceIDs).To(HaveKey("test"))
			},
		},
		{
			name:      "router delete error: surfaced and RouterID retained",
			routers:   []infrastructurev1beta2.RouterSpec{{Name: "nat-gw"}},
			preStatus: []infrastructurev1beta2.RouterStatus{{Name: "nat-gw", RouterID: routerUUID}},
			deleteErr: errors.New("API 500 internal server error"),
			assert: func(g *WithT, h *deleteHarness, cs *scopeCluster, err error) {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("internal server error"))
				g.Expect(cs.Status.Routers[0].RouterID).To(Equal(routerUUID))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			h := &deleteHarness{deleteIfaceErr: tc.deleteIfaceErr, deleteErr: tc.deleteErr}

			clusterScope := testutils.NewClusterScopeOpts(
				testutils.WithRouterService(h.service()),
				testutils.WithPreExistingNetwork("test", testNetworkID, subnetUUID, "10.0.0.0/24"),
			)
			clusterScope.CloudscaleCluster.Spec.Routers = tc.routers
			clusterScope.CloudscaleCluster.Status.Routers = tc.preStatus

			r := newTestReconciler()
			err := r.deleteRouters(context.Background(), clusterScope)
			tc.assert(g, h, clusterScope.CloudscaleCluster, err)
		})
	}
}
