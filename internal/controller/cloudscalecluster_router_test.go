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
	"net/url"
	"os"
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const (
	routerUUID    = "router-uuid-123"
	ifaceUUID     = "iface-uuid-123"
	routerAddress = "10.0.0.1"
	subnetUUID    = "subnet-uuid-123"
)

// activeRouter is what the API returns for a provisioned router.
func activeRouter(uuid string, interfaces ...cloudscalesdk.RouterInterface) *cloudscalesdk.Router {
	return &cloudscalesdk.Router{
		UUID:       uuid,
		Status:     cloudscalesdk.RouterActive,
		Interfaces: interfaces,
		Zone:       cloudscalesdk.ZoneStub{Slug: "rma1"},
	}
}

// routerInterface is what the API returns for an interface attached to a network,
// holding one address on that network's subnet.
func routerInterface(uuid, address string) cloudscalesdk.RouterInterface {
	return cloudscalesdk.RouterInterface{
		UUID:    uuid,
		Network: cloudscalesdk.NetworkStub{Name: "test", UUID: netUUID},
		Addresses: []cloudscalesdk.IPAddress{
			{Address: address, Subnet: cloudscalesdk.SubnetStub{UUID: subnetUUID}},
		},
	}
}

// routerSpec is a managed router attached to the default "test" network.
func routerSpec() infrastructurev1beta2.RouterSpec {
	return infrastructurev1beta2.RouterSpec{
		Name:            "test-router",
		InternetGateway: true,
		Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
			{Network: "test", Address: routerAddress, ConfigureSubnetGateway: new(true)},
		},
	}
}

// managedInterfaces is how a status for a single interface CAPCS created on the
// default "test" network looks like.
func managedInterfaces() []infrastructurev1beta2.RouterInterfaceStatus {
	return []infrastructurev1beta2.RouterInterfaceStatus{
		{Network: "test", InterfaceID: ifaceUUID, Managed: true},
	}
}

// recordedEvents drains everything the reconciler has emitted so far.
func recordedEvents(r *CloudscaleClusterReconciler) []string {
	var out []string
	for {
		select {
		case e := <-r.recorder.(*events.FakeRecorder).Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

// --- Managed router provisioning ---

func TestReconcileRouters_CreatesRouterAndInterface(t *testing.T) {
	g := NewWithT(t)

	var interfaceRouterID string
	var capturedCreateReq *cloudscalesdk.RouterCreateRequest
	var capturedIfaceReq cloudscalesdk.CreateInterfaceRequest
	var capturedSubnetUpdateReq *cloudscalesdk.SubnetUpdateRequest
	routerService := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			capturedCreateReq = req
			return activeRouter(routerUUID), nil
		},
		CreateInterfaceFn: func(ctx context.Context, uuid string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			interfaceRouterID = uuid
			capturedIfaceReq = req
			return &cloudscalesdk.RouterInterface{UUID: ifaceUUID}, nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			capturedSubnetUpdateReq = req
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithSubnetService(subnetService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouter(routerSpec()),
	)
	r := newTestReconciler()

	_, err := r.reconcileRouters(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(capturedCreateReq).ToNot(BeNil())
	g.Expect(capturedCreateReq.Name).To(Equal("test-router"))
	g.Expect(capturedCreateReq.InternetGateway).To(BeTrue())
	g.Expect(capturedCreateReq.Zone).To(Equal(clusterScope.CloudscaleCluster.Spec.Zone))
	g.Expect(capturedCreateReq.Tags).To(Equal(new(cloudscalesdk.TagMap{
		infrastructurev1beta2.NameCloudscaleProviderOwned + "test-cluster": "test-router",
	})))

	g.Expect(interfaceRouterID).To(Equal(routerUUID))
	g.Expect(capturedIfaceReq.Network).To(Equal(netUUID))
	g.Expect(capturedIfaceReq.Addresses).To(HaveLen(1))
	g.Expect(capturedIfaceReq.Addresses[0].Subnet).To(Equal(subnetUUID))
	g.Expect(capturedIfaceReq.Addresses[0].Address).To(Equal("10.0.0.1"))

	g.Expect(capturedSubnetUpdateReq).ToNot(BeNil())
	g.Expect(capturedSubnetUpdateReq.GatewayAddress).To(Equal(capturedIfaceReq.Addresses[0].Address))
	g.Expect(capturedSubnetUpdateReq.DNSServers).To(BeNil())

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("test-router")
	g.Expect(rs).ToNot(BeNil())
	g.Expect(rs.RouterID).To(Equal(routerUUID))
	g.Expect(rs.Managed).To(BeTrue())
	// Keyed by the CAPCS network name, holding the interface UUID (not the network's).
	g.Expect(rs.Interfaces).To(Equal(managedInterfaces()))

	// The exact, ordered events for the whole managed path. Order is what pins the
	// create-then-configure sequencing, which presence checks alone would not catch.
	g.Expect(recordedEvents(r)).To(Equal([]string{
		"Normal RouterCreated Created router test-router (router-uuid-123) in zone rma1",
		"Normal RouterInterfaceCreated Attached router test-router to network test with address 10.0.0.1",
		"Normal SubnetGatewayConfigured Configured subnet test gateway to router interface address 10.0.0.1",
	}))
}

func TestReconcileRouters_ConfiguresSubnetGateway(t *testing.T) {
	g := NewWithT(t)

	var updatedSubnetID string
	var capturedUpdate *cloudscalesdk.SubnetUpdateRequest
	updateCalls := 0
	routerService := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return activeRouter(id, routerInterface(ifaceUUID, routerAddress)), nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			updateCalls++
			updatedSubnetID = id
			capturedUpdate = req
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithSubnetService(subnetService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouter(routerSpec()),
		testutils.WithRouterStatus("test-router", routerUUID, true, managedInterfaces()),
	)
	r := newTestReconciler()

	_, err := r.reconcileRouters(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updateCalls).To(Equal(1))
	g.Expect(updatedSubnetID).To(Equal(subnetUUID))
	g.Expect(capturedUpdate.GatewayAddress).To(Equal("10.0.0.1"))
	// Recorded in status so the next reconcile skips the update.
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns.GatewayAddress).To(Equal("10.0.0.1"))

	_, err = r.reconcileRouters(context.Background(), clusterScope)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updateCalls).To(Equal(1))
}

func TestReconcileRouters_SkipsSubnetGatewayWhenNotOwner(t *testing.T) {
	g := NewWithT(t)

	routerService := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return activeRouter(id, routerInterface(ifaceUUID, routerAddress)), nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			g.Fail("Subnets.Update should not be called when configureSubnetGateway is false")
			return nil
		},
	}

	spec := routerSpec()
	spec.Interfaces[0].ConfigureSubnetGateway = new(false)

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithSubnetService(subnetService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouter(spec),
		testutils.WithRouterStatus("test-router", routerUUID, true, managedInterfaces()),
	)
	r := newTestReconciler()

	_, err := r.reconcileRouters(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

// --- Pre-existing (adopted) routers ---

// TestReconcileRouters_AdoptedInterfaceOwnership verifies the reconciler does correctly
// detect interface ownership.
func TestReconcileRouters_AdoptedInterfaceOwnership(t *testing.T) {
	const preAttachedUUID = "pre-attached-iface-uuid"

	// router in a network CAPCS knows nothing about.
	foreign := cloudscalesdk.RouterInterface{
		UUID:    "foreign-iface-uuid",
		Network: cloudscalesdk.NetworkStub{Name: "someone-elses-network", UUID: "foreign-net-uuid"},
	}
	interfaceStatus := func(uuid string, managed bool) []infrastructurev1beta2.RouterInterfaceStatus {
		return []infrastructurev1beta2.RouterInterfaceStatus{
			{Network: "test", InterfaceID: uuid, Managed: managed},
		}
	}

	const (
		mismatchEvent     = `Warning RouterInterfaceAddressMismatch Router test-router is attached to network test at "10.0.0.42", not at the requested "10.0.0.1", which therefore has no effect`
		ifaceCreatedEvent = "Normal RouterInterfaceCreated Attached router test-router to network test with address 10.0.0.1"
	)
	gatewaySetEvent := func(address string) string {
		return "Normal SubnetGatewayConfigured Configured subnet test gateway to router interface address " + address
	}

	tests := []struct {
		name string
		// live is what the API reports the router is attached to.
		live []cloudscalesdk.RouterInterface
		// adopt is the interface uuid the spec entry declares, empty when the interface should be created
		adopt string
		// status is the interface status
		status []infrastructurev1beta2.RouterInterfaceStatus
		// networkGateway is the gateway address.
		networkGateway string

		// wantErr is a substring of the error the reconcile has to fail with. The
		// reconcile is run twice either way: a collision must stay a collision.
		wantErr        string
		wantInterfaces []infrastructurev1beta2.RouterInterfaceStatus
		wantCreate     bool
		wantGatewaySet string
		// wantEvents is the exact, ordered set of events the reconcile must emit.
		wantEvents []string
	}{
		{
			// Nothing on a spec network, so CAPCS attaches its own and owns it. The
			// router's foreign interface is not recorded at all.
			name:           "unattached to the spec network",
			live:           []cloudscalesdk.RouterInterface{foreign},
			wantInterfaces: managedInterfaces(),
			wantCreate:     true,
			wantGatewaySet: routerAddress,
			wantEvents:     []string{ifaceCreatedEvent, gatewaySetEvent(routerAddress)},
		},
		{
			name:           "pre-existing attachment at the spec address is a collision",
			live:           []cloudscalesdk.RouterInterface{routerInterface(preAttachedUUID, routerAddress)},
			networkGateway: routerAddress,
			wantErr:        "set spec.routers[].interfaces[].uuid to adopt that interface",
		},
		{
			name:           "pre-existing attachment at another address is a collision",
			live:           []cloudscalesdk.RouterInterface{routerInterface(preAttachedUUID, "10.0.0.42")},
			networkGateway: "10.0.0.42",
			wantErr:        "set spec.routers[].interfaces[].uuid to adopt that interface",
		},
		{
			name:           "adopted by uuid is used and left alone",
			live:           []cloudscalesdk.RouterInterface{routerInterface(preAttachedUUID, "10.0.0.42")},
			adopt:          preAttachedUUID,
			wantInterfaces: interfaceStatus(preAttachedUUID, false),
			wantGatewaySet: "10.0.0.42",
			wantEvents:     []string{gatewaySetEvent("10.0.0.42")},
		},
		{
			name:    "adopted uuid the router does not carry",
			live:    []cloudscalesdk.RouterInterface{foreign},
			adopt:   preAttachedUUID,
			wantErr: "has no interface " + preAttachedUUID + " to adopt",
		},
		{
			name:    "adopted uuid attached to another network",
			live:    []cloudscalesdk.RouterInterface{foreign},
			adopt:   foreign.UUID,
			wantErr: "is attached to network foreign-net-uuid, not to network \"test\"",
		},
		{
			// An attach whose response was lost to a timeout: the entry written before
			// the call is what tells CAPCS this interface is its own, and it is filled
			// in as soon as the interface is seen.
			name:           "attachment recorded before its response arrived",
			live:           []cloudscalesdk.RouterInterface{routerInterface(ifaceUUID, routerAddress)},
			status:         []infrastructurev1beta2.RouterInterfaceStatus{{Network: "test", Managed: true}},
			networkGateway: routerAddress,
			wantInterfaces: managedInterfaces(),
		},
		{
			name:           "replaced interface stays managed",
			live:           []cloudscalesdk.RouterInterface{routerInterface("replacement-iface-uuid", "10.0.0.42")},
			status:         managedInterfaces(),
			networkGateway: "10.0.0.42",
			wantInterfaces: interfaceStatus("replacement-iface-uuid", true),
			wantEvents:     []string{mismatchEvent},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			created := false
			routerService := &testutils.MockRouterService{
				GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
					return activeRouter(id, tc.live...), nil
				},
				CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
					g.Fail("Create must not be called for a pre-existing router")
					return nil, nil
				},
				CreateInterfaceFn: func(ctx context.Context, uuid string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
					g.Expect(tc.wantCreate).To(BeTrue(), "CreateInterface called but the router is already attached")
					g.Expect(uuid).To(Equal(routerUUID))
					created = true
					return &cloudscalesdk.RouterInterface{UUID: ifaceUUID}, nil
				},
			}

			var gatewaySet string
			subnetService := &testutils.MockSubnetService{
				UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
					g.Expect(tc.wantGatewaySet).ToNot(BeEmpty(),
						"Subnets.Update must not be called; the gateway is already correct or the address is unknown")
					g.Expect(id).To(Equal(subnetUUID))
					gatewaySet = req.GatewayAddress
					return nil
				},
			}

			spec := routerSpec()
			spec.UUID = routerUUID
			spec.InternetGateway = false
			if tc.adopt != "" {
				// The webhook forbids requesting an address for an adopted interface.
				spec.Interfaces[0].UUID = tc.adopt
				spec.Interfaces[0].Address = ""
			}

			opts := []testutils.ClusterScopeOption{
				testutils.WithRouterService(routerService),
				testutils.WithSubnetService(subnetService),
				testutils.WithAdoptedNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
				testutils.WithRouter(spec),
			}
			if tc.status != nil {
				opts = append(opts, testutils.WithRouterStatus("test-router", routerUUID, false, tc.status))
			}
			clusterScope := testutils.NewClusterScopeOpts(opts...)
			clusterScope.CloudscaleCluster.Status.Networks[0].GatewayAddress = tc.networkGateway
			r := newTestReconciler()

			_, err := r.reconcileRouters(context.Background(), clusterScope)

			if tc.wantErr != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tc.wantErr)))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(created).To(Equal(tc.wantCreate))
			g.Expect(gatewaySet).To(Equal(tc.wantGatewaySet))

			rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("test-router")
			g.Expect(rs).ToNot(BeNil())
			g.Expect(rs.Managed).To(BeFalse(), "an adopted router is never CAPCS-managed")
			g.Expect(rs.Interfaces).To(Equal(tc.wantInterfaces))

			// The recorded gateway tracks whatever was actually written, so the next
			// reconcile knows whether there is anything left to do.
			wantRecorded := tc.networkGateway
			if tc.wantGatewaySet != "" {
				wantRecorded = tc.wantGatewaySet
			}
			g.Expect(clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test").GatewayAddress).To(Equal(wantRecorded))

			g.Expect(recordedEvents(r)).To(Equal(tc.wantEvents))

			// Reconciling again must not revisit any of it, and must not talk itself
			// out of a collision either.
			_, err = r.reconcileRouters(context.Background(), clusterScope)
			if tc.wantErr != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tc.wantErr)))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(rs.Interfaces).To(Equal(tc.wantInterfaces))
		})
	}
}

func TestReconcileRouters_PreExistingZoneMismatchErrors(t *testing.T) {
	g := NewWithT(t)

	routerService := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return &cloudscalesdk.Router{
				UUID:   id,
				Status: cloudscalesdk.RouterActive,
				Zone:   cloudscalesdk.ZoneStub{Slug: "lpg1"},
			}, nil
		},
	}

	spec := routerSpec()
	spec.UUID = routerUUID
	spec.InternetGateway = false

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouter(spec),
	)
	r := newTestReconciler()

	_, err := r.reconcileRouters(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("expected zone"))

	// A hard error must be visible on the condition, not just returned.
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.RouterReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.RouterErrorReason))
	g.Expect(cond.Message).To(ContainSubstring("expected zone"))
}

// TestReconcileRouters_AttachesMultipleInterfaces tests multiple networks with interfaces.
func TestReconcileRouters_AttachesMultipleInterfaces(t *testing.T) {
	g := NewWithT(t)

	const (
		transitNetUUID    = "transit-net-uuid"
		transitSubnetUUID = "transit-subnet-uuid"
	)

	attachedTo := map[string]cloudscalesdk.CreateAddressRequest{}
	routerService := &testutils.MockRouterService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
			return activeRouter(routerUUID), nil
		},
		CreateInterfaceFn: func(ctx context.Context, uuid string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			g.Expect(req.Addresses).To(HaveLen(1))
			attachedTo[req.Network] = req.Addresses[0]
			return &cloudscalesdk.RouterInterface{UUID: "iface-" + req.Network}, nil
		},
	}

	gatewayUpdates := map[string]string{}
	subnetService := &testutils.MockSubnetService{
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
			gatewayUpdates[id] = req.GatewayAddress
			return nil
		},
	}

	spec := routerSpec()
	spec.Interfaces = append(spec.Interfaces, infrastructurev1beta2.RouterInterfaceSpec{
		Network: "transit", Address: "10.9.0.2", ConfigureSubnetGateway: new(false),
	})

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithSubnetService(subnetService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithPreExistingNetwork("transit", transitNetUUID, transitSubnetUUID, "10.9.0.0/24"),
		testutils.WithRouter(spec),
	)
	r := newTestReconciler()

	_, err := r.reconcileRouters(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(attachedTo).To(Equal(map[string]cloudscalesdk.CreateAddressRequest{
		netUUID:        {Subnet: subnetUUID, Address: routerAddress},
		transitNetUUID: {Subnet: transitSubnetUUID, Address: "10.9.0.2"},
	}))
	// Only the gateway-owning interface rewrites a subnet; the transit subnet keeps
	// whatever gateway its own router set.
	g.Expect(gatewayUpdates).To(Equal(map[string]string{subnetUUID: routerAddress}))

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("test-router")
	g.Expect(rs).ToNot(BeNil())
	g.Expect(rs.Interfaces).To(Equal([]infrastructurev1beta2.RouterInterfaceStatus{
		{Network: "test", InterfaceID: "iface-" + netUUID, Managed: true},
		{Network: "transit", InterfaceID: "iface-" + transitNetUUID, Managed: true},
	}))
}

// TestReconcileRouters_UnprovisionedNetworkErrors guards the ordering contract: routers are
// reconciled after networks, so an interface pointing at a network that has no status yet
// means something upstream went wrong and must not be papered over.
func TestReconcileRouters_UnprovisionedNetworkErrors(t *testing.T) {
	g := NewWithT(t)

	routerService := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return activeRouter(id), nil
		},
		CreateInterfaceFn: func(ctx context.Context, uuid string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
			g.Fail("CreateInterface must not be called for a network that is not provisioned")
			return nil, nil
		},
	}

	spec := routerSpec()
	spec.UUID = routerUUID
	spec.InternetGateway = false
	spec.Interfaces[0].Network = "not-provisioned-yet"

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouter(spec),
	)
	r := newTestReconciler()

	_, err := r.reconcileRouters(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(`network "not-provisioned-yet" not found in status`))
}

// --- Condition reporting ---

func TestReconcileRouters_ConditionReflectsProgress(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() (*testutils.MockRouterService, []testutils.ClusterScopeOption)
		wantRequeue  bool
		wantStatus   metav1.ConditionStatus
		wantReason   string
		wantAttached bool
	}{
		{
			name: "no routers is ready but disabled",
			setup: func() (*testutils.MockRouterService, []testutils.ClusterScopeOption) {
				return &testutils.MockRouterService{}, nil
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: infrastructurev1beta2.RouterDisabledReason,
		},
		{
			name: "router not active yet requeues without claiming ready",
			setup: func() (*testutils.MockRouterService, []testutils.ClusterScopeOption) {
				svc := &testutils.MockRouterService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
						return &cloudscalesdk.Router{
							UUID:   id,
							Status: "unknown",
							Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
						}, nil
					},
					CreateInterfaceFn: func(ctx context.Context, uuid string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
						return nil, errors.New("CreateInterface must not be called while the router is unknown")
					},
				}
				return svc, []testutils.ClusterScopeOption{
					testutils.WithRouter(routerSpec()),
					testutils.WithRouterStatus("test-router", routerUUID, true, nil),
				}
			},
			wantRequeue: true,
			wantStatus:  metav1.ConditionFalse,
			wantReason:  infrastructurev1beta2.RouterNotReadyReason,
		},
		{
			name: "provisioned router is ready",
			setup: func() (*testutils.MockRouterService, []testutils.ClusterScopeOption) {
				svc := &testutils.MockRouterService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
						return activeRouter(id, routerInterface(ifaceUUID, routerAddress)), nil
					},
				}
				return svc, []testutils.ClusterScopeOption{
					testutils.WithRouter(routerSpec()),
					testutils.WithRouterStatus("test-router", routerUUID, true, managedInterfaces()),
				}
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: infrastructurev1beta2.RouterProvisionedReason,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			routerService, extraOpts := tc.setup()
			opts := append([]testutils.ClusterScopeOption{
				testutils.WithRouterService(routerService),
				testutils.WithSubnetService(&testutils.MockSubnetService{
					UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error { return nil },
				}),
				testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
			}, extraOpts...)
			clusterScope := testutils.NewClusterScopeOpts(opts...)
			// The gateway is already in place, so these cases only exercise the condition.
			clusterScope.CloudscaleCluster.Status.Networks[0].GatewayAddress = routerAddress
			r := newTestReconciler()

			result, err := r.reconcileRouters(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.wantRequeue {
				g.Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				g.Expect(result.RequeueAfter).To(BeZero())
			}

			cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.RouterReadyCondition)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.Status).To(Equal(tc.wantStatus))
			g.Expect(cond.Reason).To(Equal(tc.wantReason))
		})
	}
}

// TestReconcileRouters_TimeoutsRequeue verifies all timeouts requeue to recover the state.
func TestReconcileRouters_TimeoutsRequeue(t *testing.T) {
	timeout := func(op, path string) error {
		return &url.Error{Op: op, URL: "https://api.example.com/v1/" + path, Err: os.ErrDeadlineExceeded}
	}

	tests := []struct {
		name          string
		routerService func() *testutils.MockRouterService
		subnetService func() *testutils.MockSubnetService
		seedStatus    bool
		// wantInterfaces is the interface status the timed-out write must leave behind.
		wantInterfaces []infrastructurev1beta2.RouterInterfaceStatus
	}{
		{
			name: "router create",
			routerService: func() *testutils.MockRouterService {
				return &testutils.MockRouterService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Router, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.RouterCreateRequest) (*cloudscalesdk.Router, error) {
						return nil, timeout("Post", "routers")
					},
				}
			},
		},
		{
			name: "interface create",
			routerService: func() *testutils.MockRouterService {
				return &testutils.MockRouterService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
						return activeRouter(id), nil
					},
					CreateInterfaceFn: func(ctx context.Context, uuid string, req cloudscalesdk.CreateInterfaceRequest) (*cloudscalesdk.RouterInterface, error) {
						return nil, timeout("Post", "routers/"+uuid+"/interfaces")
					},
				}
			},
			seedStatus: true,
			// The attach may have landed even though its response did not, so the
			// entry recorded before the call has to survive the requeue. Without it
			// the next reconcile cannot tell the interface from one the router
			// already carried.
			wantInterfaces: []infrastructurev1beta2.RouterInterfaceStatus{{Network: "test", Managed: true}},
		},
		{
			name: "subnet gateway update",
			routerService: func() *testutils.MockRouterService {
				return &testutils.MockRouterService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
						return activeRouter(id, routerInterface(ifaceUUID, routerAddress)), nil
					},
				}
			},
			subnetService: func() *testutils.MockSubnetService {
				return &testutils.MockSubnetService{
					UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error {
						return timeout("Patch", "subnets/"+id)
					},
				}
			},
			seedStatus: true,
			// The attach itself succeeded, so its interface is recorded in full.
			wantInterfaces: managedInterfaces(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			subnetService := &testutils.MockSubnetService{
				UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.SubnetUpdateRequest) error { return nil },
			}
			if tc.subnetService != nil {
				subnetService = tc.subnetService()
			}

			opts := []testutils.ClusterScopeOption{
				testutils.WithRouterService(tc.routerService()),
				testutils.WithSubnetService(subnetService),
				testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
				testutils.WithRouter(routerSpec()),
			}
			if tc.seedStatus {
				opts = append(opts, testutils.WithRouterStatus("test-router", routerUUID, true, nil))
			}
			clusterScope := testutils.NewClusterScopeOpts(opts...)
			r := newTestReconciler()

			result, err := r.reconcileRouters(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.RequeueAfter).To(Equal(routerRequeueAfter))

			cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.RouterReadyCondition)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.RouterNotReadyReason))

			if rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus("test-router"); rs != nil {
				g.Expect(rs.Interfaces).To(Equal(tc.wantInterfaces))
			}
		})
	}
}

// --- Deletion ---

func TestDeleteRouters_DeletesManagedRouterAndInterfaces(t *testing.T) {
	g := NewWithT(t)

	var deletedRouterID string
	deletedInterfaces := map[string]string{}
	routerService := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return activeRouter(id, routerInterface(ifaceUUID, routerAddress)), nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			deletedRouterID = id
			return nil
		},
		DeleteInterfaceFn: func(ctx context.Context, routerUUID, interfaceUUID string) error {
			deletedInterfaces[routerUUID] = interfaceUUID
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouterStatus("test-router", routerUUID, true, managedInterfaces()),
	)
	r := newTestReconciler()

	err := r.deleteRouters(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedInterfaces).To(Equal(map[string]string{routerUUID: ifaceUUID}))
	g.Expect(deletedRouterID).To(Equal(routerUUID))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers).To(BeEmpty())
}

// TestDeleteRouters_PreservesForeignInterfacesOnAdoptedRouter guards the case where a
// user points CAPCS at a shared router: deleting the cluster must remove the interfaces
// CAPCS attached and nothing else, and must leave the router itself alone.
func TestDeleteRouters_PreservesForeignInterfacesOnAdoptedRouter(t *testing.T) {
	g := NewWithT(t)

	preAttached := infrastructurev1beta2.RouterInterfaceStatus{
		Network: "shared", InterfaceID: "pre-attached-iface-uuid", Managed: false,
	}

	var deletedInterfaces []string
	routerService := &testutils.MockRouterService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
			return activeRouter(id,
				routerInterface(ifaceUUID, routerAddress),
				cloudscalesdk.RouterInterface{
					UUID:    preAttached.InterfaceID,
					Network: cloudscalesdk.NetworkStub{Name: "shared", UUID: "shared-net-uuid"},
				},
			), nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			g.Fail("Delete should not be called for a pre-existing router")
			return nil
		},
		DeleteInterfaceFn: func(ctx context.Context, routerUUID, interfaceUUID string) error {
			deletedInterfaces = append(deletedInterfaces, interfaceUUID)
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(
		testutils.WithRouterService(routerService),
		// "shared" is the user's network, so CAPCS never deletes it.
		// "test" is one CAPCS manages.
		testutils.WithAdoptedNetwork("shared", "shared-net-uuid", "shared-subnet-uuid", "10.1.0.0/24"),
		testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
		testutils.WithRouterStatus("shared-router", routerUUID, false,
			append(managedInterfaces(), preAttached)),
	)
	r := newTestReconciler()

	err := r.deleteRouters(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	// Only the CAPCS-created interface is detached
	g.Expect(deletedInterfaces).To(Equal([]string{ifaceUUID}))
	// The adopted router stays in status, keeping the interface it came with.
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers).To(HaveLen(1))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].Name).To(Equal("shared-router"))
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].Managed).To(BeFalse())
	g.Expect(clusterScope.CloudscaleCluster.Status.Routers[0].Interfaces).To(Equal(
		[]infrastructurev1beta2.RouterInterfaceStatus{preAttached}))
}

// TestDeleteRouters_ResolvesInterfacesAgainstLiveRouter ensures interfaces are deleted based on the current
// router's API response.
func TestDeleteRouters_ResolvesInterfacesAgainstLiveRouter(t *testing.T) {
	tests := []struct {
		name string
		// live is what the API reports the router is attached to, nil for a router that
		// no longer exists.
		live       []cloudscalesdk.RouterInterface
		routerGone bool
		interfaces []infrastructurev1beta2.RouterInterfaceStatus

		wantDetached []string
	}{
		{
			// Recorded before the attach existed, so the network is all there is to go on.
			name:         "attachment with no recorded uuid is resolved by network",
			live:         []cloudscalesdk.RouterInterface{routerInterface(ifaceUUID, routerAddress)},
			interfaces:   []infrastructurev1beta2.RouterInterfaceStatus{{Network: "test", Managed: true}},
			wantDetached: []string{ifaceUUID},
		},
		{
			// The attach never landed after all: there is nothing to detach, and the
			// entry must not hold the router's deletion back.
			name:       "attachment that never landed is dropped",
			live:       nil,
			interfaces: []infrastructurev1beta2.RouterInterfaceStatus{{Network: "test", Managed: true}},
		},
		{
			name:       "recorded interface that is already detached is dropped",
			live:       nil,
			interfaces: managedInterfaces(),
		},
		{
			name:       "router already gone takes its interfaces with it",
			routerGone: true,
			interfaces: managedInterfaces(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			var detached []string
			routerService := &testutils.MockRouterService{
				GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
					if tc.routerGone {
						return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
					}
					return activeRouter(id, tc.live...), nil
				},
				DeleteFn: func(ctx context.Context, id string) error { return nil },
				DeleteInterfaceFn: func(ctx context.Context, routerUUID, interfaceUUID string) error {
					detached = append(detached, interfaceUUID)
					return nil
				},
			}

			clusterScope := testutils.NewClusterScopeOpts(
				testutils.WithRouterService(routerService),
				testutils.WithPreExistingNetwork("test", netUUID, subnetUUID, "10.0.0.0/24"),
				testutils.WithRouterStatus("test-router", routerUUID, true, tc.interfaces),
			)
			r := newTestReconciler()

			err := r.deleteRouters(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(detached).To(Equal(tc.wantDetached))
			// Whatever happened to the interfaces, the router itself is gone from status.
			g.Expect(clusterScope.CloudscaleCluster.Status.Routers).To(BeEmpty())
		})
	}
}

func TestDeleteRouters_BasicScenarios(t *testing.T) {
	tests := []struct {
		name          string
		routerService func(g *WithT) *testutils.MockRouterService
		statusSetup   func() []infrastructurev1beta2.RouterStatus
		wantErr       bool
		wantStatusLen int
	}{
		{
			name: "never provisioned router is dropped without any API call",
			routerService: func(g *WithT) *testutils.MockRouterService {
				return &testutils.MockRouterService{
					DeleteFn: func(ctx context.Context, id string) error {
						g.Fail("Delete should not be called for a router that was never created")
						return nil
					},
				}
			},
			statusSetup: func() []infrastructurev1beta2.RouterStatus {
				return []infrastructurev1beta2.RouterStatus{{Name: "test-router", Managed: true}}
			},
			wantStatusLen: 0,
		},
		{
			name: "already deleted router is ignored",
			routerService: func(g *WithT) *testutils.MockRouterService {
				return &testutils.MockRouterService{
					DeleteFn: func(ctx context.Context, id string) error {
						return &cloudscalesdk.ErrorResponse{StatusCode: 404}
					},
				}
			},
			statusSetup: func() []infrastructurev1beta2.RouterStatus {
				return []infrastructurev1beta2.RouterStatus{{Name: "test-router", RouterID: routerUUID, Managed: true}}
			},
			wantStatusLen: 0,
		},
		{
			name: "failed router deletion keeps it in status",
			routerService: func(g *WithT) *testutils.MockRouterService {
				return &testutils.MockRouterService{
					DeleteFn: func(ctx context.Context, id string) error {
						return errors.New("boom")
					},
				}
			},
			statusSetup: func() []infrastructurev1beta2.RouterStatus {
				return []infrastructurev1beta2.RouterStatus{{Name: "test-router", RouterID: routerUUID, Managed: true}}
			},
			wantErr:       true,
			wantStatusLen: 1,
		},
		{
			name: "failed interface deletion keeps the router for the next attempt",
			routerService: func(g *WithT) *testutils.MockRouterService {
				return &testutils.MockRouterService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Router, error) {
						return activeRouter(id, routerInterface(ifaceUUID, routerAddress)), nil
					},
					DeleteFn: func(ctx context.Context, id string) error {
						g.Fail("Router must not be deleted while its interfaces are still attached")
						return nil
					},
					DeleteInterfaceFn: func(ctx context.Context, routerUUID, interfaceUUID string) error {
						return errors.New("boom")
					},
				}
			},
			statusSetup: func() []infrastructurev1beta2.RouterStatus {
				return []infrastructurev1beta2.RouterStatus{{
					Name: "test-router", RouterID: routerUUID, Managed: true,
					Interfaces: managedInterfaces(),
				}}
			},
			wantErr:       true,
			wantStatusLen: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			clusterScope := testutils.NewClusterScopeOpts(testutils.WithRouterService(tc.routerService(g)))
			clusterScope.CloudscaleCluster.Status.Routers = tc.statusSetup()
			r := newTestReconciler()

			err := r.deleteRouters(context.Background(), clusterScope)

			wantReason := infrastructurev1beta2.RouterDeletingReason
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				wantReason = infrastructurev1beta2.RouterErrorReason
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(clusterScope.CloudscaleCluster.Status.Routers).To(HaveLen(tc.wantStatusLen))

			// Deletion always leaves the condition False; only the reason distinguishes
			// "torn down" from "could not tear down".
			cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.RouterReadyCondition)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(cond.Reason).To(Equal(wantReason))
		})
	}
}
