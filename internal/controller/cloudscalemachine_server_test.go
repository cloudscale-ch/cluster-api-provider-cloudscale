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

	"github.com/cloudscale-ch/cloudscale-go-sdk/v6"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	cs "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

const testExistingServerUUID = "existing-server-uuid"

type mockServerService struct {
	createFn func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error)
	getFn    func(ctx context.Context, id string) (*cloudscale.Server, error)
	listFn   func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error)
	deleteFn func(ctx context.Context, id string) error
	updateFn func(ctx context.Context, id string, req *cloudscale.ServerUpdateRequest) error
}

func (m *mockServerService) Create(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockServerService) Get(ctx context.Context, id string) (*cloudscale.Server, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockServerService) List(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockServerService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockServerService) Update(ctx context.Context, id string, req *cloudscale.ServerUpdateRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

func newTestFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrastructurev1beta2.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func newTestMachineScopeWithServer(serverService cs.ServerService) *scope.MachineScope {
	cloudscaleClient := &cs.Client{
		Servers: serverService,
	}

	cloudscaleMachine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
		},
		Spec: infrastructurev1beta2.CloudscaleMachineSpec{
			Flavor: "flex-8-4",
			Image:  "ubuntu-24.04",
		},
	}

	bootstrapSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"value": []byte("#!/bin/bash\necho 'bootstrap script'"),
		},
	}

	fakeClient := newTestFakeClient(cloudscaleMachine, bootstrapSecret)

	machineScope, _ := scope.NewMachineScope(scope.MachineScopeParams{
		Client: fakeClient,
		Logger: logr.Discard(),
		Cluster: &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		},
		Machine: &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "default",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To("bootstrap-secret"),
				},
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
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				NetworkID: "net-uuid-123",
				SubnetID:  "subnet-uuid-123",
			},
		},
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  cloudscaleClient,
	})

	return machineScope
}

func TestReconcileServer_CreatesServer(t *testing.T) {
	g := NewWithT(t)
	var capturedReq *cloudscale.ServerRequest

	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return nil, nil // No existing server
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			capturedReq = req
			return &cloudscale.Server{
				UUID:          "server-uuid-123",
				Name:          req.Name,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				Interfaces: []cloudscale.Interface{
					{
						Type: "private",
						Addresses: []cloudscale.Address{
							{Address: "10.0.0.5", Version: 4},
						},
					},
				},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
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
	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			return &cloudscale.Server{
				UUID:          "server-uuid-456",
				Name:          req.Name,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
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
	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			return &cloudscale.Server{
				UUID:          "server-uuid-789",
				Name:          req.Name,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				Interfaces: []cloudscale.Interface{
					{
						Type: "public",
						Addresses: []cloudscale.Address{
							{Address: "185.98.123.45", Version: 4},
						},
					},
					{
						Type: "private",
						Addresses: []cloudscale.Address{
							{Address: "10.0.0.10", Version: 4},
						},
					},
				},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(machineScope.CloudscaleMachine.Status.Addresses).To(HaveLen(2))
}

func TestReconcileServer_SkipsIfAlreadyExists(t *testing.T) {
	g := NewWithT(t)
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Server, error) {
			return &cloudscale.Server{
				UUID:          id,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			t.Fatal("Create should not be called when server already exists")
			return nil, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = testExistingServerUUID

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(Equal(testExistingServerUUID))
}

func TestReconcileServer_FindsExistingByTag(t *testing.T) {
	g := NewWithT(t)
	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return []cloudscale.Server{
				{
					UUID:          "found-server-uuid",
					Status:        "running",
					ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			t.Fatal("Create should not be called when server is found by tag")
			return nil, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
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
	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return []cloudscale.Server{
				{
					UUID:          "server-uuid-1",
					Status:        "running",
					ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				},
				{
					UUID:          "server-uuid-2",
					Status:        "running",
					ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
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

func TestDeleteServer_DeletesServer(t *testing.T) {
	g := NewWithT(t)
	var deletedID string

	serverService := &mockServerService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-to-delete"

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedID).To(Equal("server-to-delete"))
	g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(BeEmpty())
}

func TestDeleteServer_SkipsIfNoServer(t *testing.T) {
	g := NewWithT(t)
	serverService := &mockServerService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when no server exists")
			return nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteServer_IgnoresAlreadyDeleted(t *testing.T) {
	g := NewWithT(t)
	serverService := &mockServerService{
		deleteFn: func(ctx context.Context, id string) error {
			return &cloudscale.ErrorResponse{StatusCode: 404}
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "already-deleted-server"

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(machineScope.CloudscaleMachine.Status.ServerID).To(BeEmpty())
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
			serverService := &mockServerService{
				getFn: func(ctx context.Context, id string) (*cloudscale.Server, error) {
					return &cloudscale.Server{
						UUID:          id,
						Status:        tc.serverStatus,
						ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
					}, nil
				},
			}

			machineScope := newTestMachineScopeWithServer(serverService)
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
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Server, error) {
			return &cloudscale.Server{
				UUID:          id,
				Status:        "changing",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = testExistingServerUUID
	machineScope.CloudscaleMachine.Status.Initialization = &infrastructurev1beta2.MachineInitializationStatus{
		Provisioned: ptr.To(true),
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

	var capturedReq *cloudscale.ServerRequest

	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			capturedReq = req
			return &cloudscale.Server{
				UUID:          "server-uuid-sg",
				Name:          req.Name,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
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

	var capturedReq *cloudscale.ServerRequest

	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			capturedReq = req
			return &cloudscale.Server{
				UUID:          "server-uuid-no-sg",
				Name:          req.Name,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.reconcileServer(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.ServerGroups).To(BeNil())
}
