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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const testExistingServerUUID = "existing-server-uuid"

func TestReconcileServer_CreatesServer(t *testing.T) {
	g := NewWithT(t)
	var capturedReq *cloudscalesdk.ServerRequest

	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil // No existing server
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			capturedReq = req
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-123",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
				Interfaces: []cloudscalesdk.ServerInterface{
					{
						Type: "private",
						Addresses: []cloudscalesdk.ServerAddress{
							{Address: "10.0.0.5", Version: 4},
						},
					},
				},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(Equal("server-uuid-123"))
	g.Expect(capturedReq.Flavor).To(Equal("flex-8-4"))
	g.Expect(capturedReq.Image).To(Equal("ubuntu-24.04"))
	g.Expect(capturedReq.Zone).To(Equal("rma1"))

	// Verify condition is set
	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerRunningReason))
}

func TestReconcileServer_SetsProviderID(t *testing.T) {
	g := NewWithT(t)
	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-456",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(machineScope.CloudscaleMachine.Spec.ProviderID).ToNot(BeNil())
	g.Expect(*machineScope.CloudscaleMachine.Spec.ProviderID).To(Equal("cloudscale://server-uuid-456"))
}

func TestReconcileServer_SetsAddresses(t *testing.T) {
	g := NewWithT(t)
	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-789",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
				Interfaces: []cloudscalesdk.ServerInterface{
					{
						Type: "public",
						Addresses: []cloudscalesdk.ServerAddress{
							{Address: "185.98.123.45", Version: 4},
						},
					},
					{
						Type: "private",
						Addresses: []cloudscalesdk.ServerAddress{
							{Address: "10.0.0.10", Version: 4},
						},
					},
				},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(machineScope.CloudscaleMachine.Status.Addresses).To(HaveLen(2))
}

func TestReconcileServer_FindsExistingByTag(t *testing.T) {
	g := NewWithT(t)
	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return []cloudscalesdk.Server{
				{
					UUID:   "found-server-uuid",
					Status: "running",
					Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
				},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			g.Fail("Create should not be called when server is found by tag")
			return nil, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(Equal("found-server-uuid"))
}

func TestReconcileServer_ErrorsOnMultipleByTag(t *testing.T) {
	g := NewWithT(t)
	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return []cloudscalesdk.Server{
				{
					UUID:   "server-uuid-1",
					Status: "running",
					Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
				},
				{
					UUID:   "server-uuid-2",
					Status: "running",
					Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
				},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("found 2 servers matching tag filter"))
	g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(BeEmpty())

	// Verify error condition is set by defer
	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerErrorReason))
}

func TestDeleteServer_BasicScenarios(t *testing.T) {
	testCases := []struct {
		name         string
		serverID     string
		deleteErr    error
		wantServerID string
	}{
		{
			name:         "Deletes server",
			serverID:     "server-to-delete",
			wantServerID: "",
		},
		{
			name:         "Skips if no server",
			serverID:     "",
			wantServerID: "",
		},
		{
			name:         "Ignores already deleted",
			serverID:     "already-deleted-server",
			deleteErr:    &cloudscalesdk.ErrorResponse{StatusCode: 404},
			wantServerID: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var deletedID string
			g := NewWithT(t)

			serverService := &testutils.MockServerService{
				DeleteFn: func(ctx context.Context, id string) error {
					deletedID = id
					if tc.serverID == "" {
						g.Fail("Delete should not be called when no server exists")
					}
					return tc.deleteErr
				},
			}

			machineScope := testutils.NewMachineScope(serverService)
			machineScope.CloudscaleMachine.Status.ServerID = tc.serverID

			r := &CloudscaleMachineReconciler{
				recorder: events.NewFakeRecorder(10),
			}

			err := r.deleteServer(context.Background(), machineScope)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(Equal(tc.wantServerID))
			if tc.serverID != "" {
				g.Expect(deletedID).To(Equal(tc.serverID))
			}
		})
	}
}

func TestReconcileServer_SetsServerStatusCondition(t *testing.T) {
	testCases := []struct {
		name              string
		serverStatus      string
		expectedCondition metav1.ConditionStatus
		expectedReason    string
		expectedRequeue   bool
	}{
		{"running", "running", metav1.ConditionTrue, infrastructurev1beta2.ServerRunningReason, false},
		{"changing", "changing", metav1.ConditionFalse, infrastructurev1beta2.ServerStartingReason, true},
		{"stopped", "stopped", metav1.ConditionFalse, infrastructurev1beta2.ServerStoppedReason, false},
		{"paused", "paused", metav1.ConditionFalse, infrastructurev1beta2.ServerInternalErrorReason, false},
		{"rescue_running", "rescue_running", metav1.ConditionFalse, infrastructurev1beta2.ServerInRescueModeReason, false},
		{"rescue_stopped", "rescue_stopped", metav1.ConditionFalse, infrastructurev1beta2.ServerInRescueModeReason, false},
		{"error", "error", metav1.ConditionFalse, infrastructurev1beta2.ServerInternalErrorReason, false},
		{"unknown", "unknown", metav1.ConditionFalse, infrastructurev1beta2.ServerInternalErrorReason, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			serverService := &testutils.MockServerService{
				GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
					return &cloudscalesdk.Server{
						UUID:   id,
						Status: tc.serverStatus,
						Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
					}, nil
				},
			}

			machineScope := testutils.NewMachineScope(serverService)
			machineScope.CloudscaleMachine.Status.ServerID = testExistingServerUUID

			r := &CloudscaleMachineReconciler{
				recorder: events.NewFakeRecorder(10),
			}

			result, err := r.reconcileServer(context.Background(), machineScope)

			g.Expect(err).ToNot(HaveOccurred())

			cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.Status).To(Equal(tc.expectedCondition))
			g.Expect(cond.Reason).To(Equal(tc.expectedReason))

			if tc.expectedRequeue {
				g.Expect(result.RequeueAfter).To(Equal(ServerStatusPollInterval))
			} else {
				g.Expect(result).To(Equal(ctrl.Result{}))
			}
		})
	}
}

func TestReconcileServer_ProvisionedNotModified(t *testing.T) {
	g := NewWithT(t)
	// Verify that reconcileServer does not modify the Provisioned flag
	// (that's the controller's responsibility)
	serverService := &testutils.MockServerService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:   id,
				Status: "changing",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			g.Fail("Create should not be called when server already exists")
			return nil, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = testExistingServerUUID
	machineScope.CloudscaleMachine.Status.Initialization = &infrastructurev1beta2.MachineInitializationStatus{
		Provisioned: new(true),
	}

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())

	// Should requeue since status is "changing"
	g.Expect(result.RequeueAfter).To(Equal(ServerStatusPollInterval))

	// Condition should reflect changing status with "already provisioned" reason
	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerChangingReason))

	// Provisioned should remain unchanged
	g.Expect(*machineScope.CloudscaleMachine.Status.Initialization.Provisioned).To(BeTrue())
}

func TestReconcileServer_SetsServerGroupInRequest(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.ServerRequest

	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			capturedReq = req
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-sg",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.CloudscaleMachine.Status.ServerGroupID = "server-group-uuid-123"

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.ServerGroups).To(Equal([]string{"server-group-uuid-123"}))
}

func TestReconcileServer_NoServerGroupWhenStatusEmpty(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.ServerRequest

	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			capturedReq = req
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-no-sg",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.ServerGroups).To(BeNil())
}

// --- buildInterfaceRequests tests ---

func TestBuildInterfaceRequests_DefaultsToPublicPlusFirstNetwork(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	// No interfaces in spec → uses runtime defaults

	r := &CloudscaleMachineReconciler{}

	reqs, ipFamily, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(reqs).ToNot(BeNil())
	g.Expect(*reqs).To(HaveLen(2))
	g.Expect((*reqs)[0].Network).To(Equal(InterfaceTypePublic))
	g.Expect((*reqs)[1].Network).To(Equal("net-uuid-123"))
	g.Expect(ipFamily).To(BeNil(), "runtime default path should not return ipFamily")
}

func TestBuildInterfaceRequests_NoClusterNetworksErrors(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleCluster.Status.Networks = nil

	r := &CloudscaleMachineReconciler{}

	_, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no networks provisioned"))
}

func TestBuildInterfaceRequests_PublicInterface(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Type: "public"},
	}

	r := &CloudscaleMachineReconciler{}

	reqs, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*reqs).To(HaveLen(1))
	g.Expect((*reqs)[0].Network).To(Equal(InterfaceTypePublic))
}

func TestBuildInterfaceRequests_NamedNetworkFound(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
	}

	r := &CloudscaleMachineReconciler{}

	reqs, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*reqs).To(HaveLen(1))
	g.Expect((*reqs)[0].Network).To(Equal("net-uuid-123"))
}

func TestBuildInterfaceRequests_NamedNetworkNotFound(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "nonexistent"},
	}

	r := &CloudscaleMachineReconciler{}

	_, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not found in cluster status"))
}

func TestBuildInterfaceRequests_NamedNetworkNotProvisioned(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "", SubnetID: "subnet-uuid-123", Managed: true},
	}
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
	}

	r := &CloudscaleMachineReconciler{}

	_, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not yet provisioned"))
}

func TestBuildInterfaceRequests_MixedPublicAndNetwork(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
		{Type: "public"},
	}

	r := &CloudscaleMachineReconciler{}

	reqs, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*reqs).To(HaveLen(2))
	g.Expect((*reqs)[0].Network).To(Equal("net-uuid-123"))
	g.Expect((*reqs)[1].Network).To(Equal(InterfaceTypePublic))
}

func TestBuildInterfaceRequests_InvalidInterfaceErrors(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{}, // neither type nor network
	}

	r := &CloudscaleMachineReconciler{}

	_, _, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("must have either type or network"))
}

func TestBuildInterfaceRequests_ReturnsIPFamilyFromPublicInterface(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
		{Type: "public", IPFamily: new(infrastructurev1beta2.IPFamilyDualStack)},
	}

	r := &CloudscaleMachineReconciler{}

	_, ipFamily, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ipFamily).ToNot(BeNil())
	g.Expect(*ipFamily).To(Equal(infrastructurev1beta2.IPFamilyDualStack))
}

func TestBuildInterfaceRequests_NilIPFamilyWhenNoPublicInterface(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{})
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
	}

	r := &CloudscaleMachineReconciler{}

	_, ipFamily, err := r.buildInterfaceRequests(machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ipFamily).To(BeNil())
}

// --- ipFamilyToUseIPV6 tests ---

func TestIPFamilyToUseIPV6_DualStack(t *testing.T) {
	g := NewWithT(t)
	result := ipFamilyToUseIPV6(new(infrastructurev1beta2.IPFamilyDualStack))
	g.Expect(result).ToNot(BeNil())
	g.Expect(*result).To(BeTrue())
}

func TestIPFamilyToUseIPV6_IPv4(t *testing.T) {
	g := NewWithT(t)
	result := ipFamilyToUseIPV6(new(infrastructurev1beta2.IPFamilyIPv4))
	g.Expect(result).ToNot(BeNil())
	g.Expect(*result).To(BeFalse())
}

func TestIPFamilyToUseIPV6_Nil(t *testing.T) {
	g := NewWithT(t)
	result := ipFamilyToUseIPV6(nil)
	g.Expect(result).To(BeNil())
}

// --- UseIPV6 integration via reconcileServer ---

func TestReconcileServer_SetsUseIPV6DualStack(t *testing.T) {
	g := NewWithT(t)
	var capturedReq *cloudscalesdk.ServerRequest

	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			capturedReq = req
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-ipv6",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
		{Type: "public", IPFamily: new(infrastructurev1beta2.IPFamilyDualStack)},
	}

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.UseIPV6).ToNot(BeNil())
	g.Expect(*capturedReq.UseIPV6).To(BeTrue())
}

func TestReconcileServer_SetsUseIPV6IPv4Only(t *testing.T) {
	g := NewWithT(t)
	var capturedReq *cloudscalesdk.ServerRequest

	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			capturedReq = req
			return &cloudscalesdk.Server{
				UUID:   "server-uuid-ipv4",
				Name:   req.Name,
				Status: "running",
				Zone:   cloudscalesdk.ZoneStub{Slug: "rma1"},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.CloudscaleMachine.Spec.Interfaces = []infrastructurev1beta2.InterfaceSpec{
		{Network: "test"},
		{Type: "public", IPFamily: new(infrastructurev1beta2.IPFamilyIPv4)},
	}

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.UseIPV6).ToNot(BeNil())
	g.Expect(*capturedReq.UseIPV6).To(BeFalse())
}
