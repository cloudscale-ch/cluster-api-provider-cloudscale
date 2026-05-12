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

// Orchestrator-level smoke tests for the machine reconciler. Server status
// transitions, error propagation, server-group handling, and delete edge
// cases are covered by cloudscalemachine_server_test.go and
// cloudscalemachine_servergroup_test.go. These tests verify only the wiring
// reconcileNormal applies on top of those sub-reconcilers.

import (
	"context"
	"fmt"
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

func newTestMachineReconciler(objs ...client.Object) *CloudscaleMachineReconciler {
	fakeClient := testutils.NewFakeClient(objs...)
	return &CloudscaleMachineReconciler{
		Client:   fakeClient,
		Scheme:   fakeClient.Scheme(),
		recorder: events.NewFakeRecorder(10),
	}
}

// TestMachineReconcileNormal_ServerRunning_SetsProvisioned is the happy-path
// smoke for reconcileNormal: with a running server, the orchestrator sets
// Initialization.Provisioned and ReadyCondition=True.
func TestMachineReconcileNormal_ServerRunning_SetsProvisioned(t *testing.T) {
	g := NewWithT(t)

	serverService := &testutils.MockServerService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:          id,
				Status:        "running",
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
				Interfaces: []cloudscalesdk.Interface{
					{
						Type: "private",
						Addresses: []cloudscalesdk.Address{
							{Address: "10.0.0.5", Version: 4},
						},
					},
				},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(machineScope.CloudscaleMachine.Status.Initialization).ToNot(BeNil())
	g.Expect(*machineScope.CloudscaleMachine.Status.Initialization.Provisioned).To(BeTrue())

	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
}

// TestMachineReconcileNormal_BootstrapDataNotReady verifies that reconcileServer
// is not invoked when bootstrap data is missing and the ServerReadyCondition
// reflects the wait. This gating lives only in reconcileNormal.
func TestMachineReconcileNormal_BootstrapDataNotReady(t *testing.T) {
	g := NewWithT(t)

	serverService := &testutils.MockServerService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			g.Fail("Server Get should not be called when bootstrap data is not ready")
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			g.Fail("Server Create should not be called when bootstrap data is not ready")
			return nil, nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.Machine.Spec.Bootstrap.DataSecretName = nil

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(machineScope.CloudscaleMachine.Status.Initialization).To(BeNil())

	serverCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
	g.Expect(serverCond).ToNot(BeNil())
	g.Expect(serverCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(serverCond.Reason).To(Equal(infrastructurev1beta2.WaitingForBootstrapDataReason))
}

func TestMachineReconcileNormal_ServerError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverService := &testutils.MockServerService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, fmt.Errorf("server api error")
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	// No ServerID set, so it will try to List
	r := newTestMachineReconciler()

	_, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("server api error"))

	// ReadyCondition should be set by defer (False since ServerReadyCondition is False due to error)
	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
}

// TestMachineReconcileDelete_Success is the happy-path smoke for reconcileDelete:
// the server is deleted, the finalizer is removed, and the Deleting/Ready
// conditions are set accordingly.
func TestMachineReconcileDelete_Success(t *testing.T) {
	g := NewWithT(t)

	var deletedID string

	serverService := &testutils.MockServerService{
		DeleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	machineScope := testutils.NewMachineScope(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-to-delete"
	machineScope.CloudscaleMachine.Finalizers = []string{infrastructurev1beta2.MachineFinalizer}

	r := newTestMachineReconciler()

	result, err := r.reconcileDelete(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(deletedID).To(Equal("server-to-delete"))
	g.Expect(machineScope.CloudscaleMachine.Finalizers).ToNot(ContainElement(infrastructurev1beta2.MachineFinalizer))

	deletingCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.DeletingCondition)
	g.Expect(deletingCond).ToNot(BeNil())
	g.Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))

	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.DeletingReason))
}

// TestMachineReconcileDelete_ServerError_PreservesFinalizer tests that the finalizer is preserved when
// reconciling a machine for deletion fails due to a server-side error during cleanup.
func TestMachineReconcileDelete_ServerError_PreservesFinalizer(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(&testutils.MockServerService{
		DeleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("server delete failed")
		},
	})
	machineScope.CloudscaleMachine.Status.ServerID = "server-to-delete"
	machineScope.CloudscaleMachine.Finalizers = []string{infrastructurev1beta2.MachineFinalizer}

	r := newTestMachineReconciler()

	_, err := r.reconcileDelete(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("server delete failed"))

	// Finalizer should NOT be removed on error
	g.Expect(machineScope.CloudscaleMachine.Finalizers).To(ContainElement(infrastructurev1beta2.MachineFinalizer))
}

// TestMachineSetReadyCondition tests the orchestrator's condition-rollup logic
// for the machine. The mirroring of ServerReadyCondition into ReadyCondition
// is unique to setReadyCondition and not exercised by the leaf tests.
func TestMachineSetReadyCondition(t *testing.T) {
	cases := []struct {
		name              string
		serverReadyCond   *metav1.Condition
		expectReadyStatus metav1.ConditionStatus
		expectReadyReason string
		expectMsgContains string
	}{
		{
			name: "ServerReady=True propagates Ready=True",
			serverReadyCond: &metav1.Condition{
				Type:   infrastructurev1beta2.ServerReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: infrastructurev1beta2.ServerRunningReason,
			},
			expectReadyStatus: metav1.ConditionTrue,
			expectReadyReason: infrastructurev1beta2.ReadyReason,
		},
		{
			name: "ServerReady=False propagates Ready=False with same reason/message",
			serverReadyCond: &metav1.Condition{
				Type:    infrastructurev1beta2.ServerReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrastructurev1beta2.ServerStartingReason,
				Message: "Server is starting",
			},
			expectReadyStatus: metav1.ConditionFalse,
			expectReadyReason: infrastructurev1beta2.ServerStartingReason,
			expectMsgContains: "Server is starting",
		},
		{
			name:              "no sub-conditions yields Ready=False/NotReady",
			serverReadyCond:   nil,
			expectReadyStatus: metav1.ConditionFalse,
			expectReadyReason: infrastructurev1beta2.NotReadyReason,
			expectMsgContains: "Waiting for",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			machine := &infrastructurev1beta2.CloudscaleMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  "default",
					Generation: 1,
				},
			}
			if tc.serverReadyCond != nil {
				conditions.Set(machine, *tc.serverReadyCond)
			}

			newTestMachineReconciler().setReadyCondition(machine)

			readyCond := conditions.Get(machine, infrastructurev1beta2.ReadyCondition)
			g.Expect(readyCond).ToNot(BeNil())
			g.Expect(readyCond.Status).To(Equal(tc.expectReadyStatus))
			g.Expect(readyCond.Reason).To(Equal(tc.expectReadyReason))
			if tc.expectMsgContains != "" {
				g.Expect(readyCond.Message).To(ContainSubstring(tc.expectMsgContains))
			}
		})
	}
}

// TestMachineReconcile_ResourceNotFound exercises the Reconcile entry point
// for a missing CloudscaleMachine: should not error.
func TestMachineReconcile_ResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	r := newTestMachineReconciler()

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
}

// TestMachineReconcile_NoOwnerMachine exercises the Reconcile entry point
// when no owner Machine references the CloudscaleMachine: should no-op.
func TestMachineReconcile_NoOwnerMachine(t *testing.T) {
	g := NewWithT(t)

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
		},
		Spec: infrastructurev1beta2.CloudscaleMachineSpec{
			Flavor: "flex-8-4",
			Image:  "ubuntu-24.04",
		},
	}

	r := newTestMachineReconciler(machine)

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: machine.Name, Namespace: machine.Namespace},
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
}
