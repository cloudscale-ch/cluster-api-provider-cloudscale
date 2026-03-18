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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

func newTestMachineReconciler() *CloudscaleMachineReconciler {
	return &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}
}

// ============================================================================
// Tests for reconcileNormal
// ============================================================================

func TestMachineReconcileNormal_ServerRunning_SetsProvisioned(t *testing.T) {
	g := NewWithT(t)

	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
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

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	// Provisioned should be set to true
	g.Expect(machineScope.CloudscaleMachine.Status.Initialization).ToNot(BeNil())
	g.Expect(*machineScope.CloudscaleMachine.Status.Initialization.Provisioned).To(BeTrue())

	// ReadyCondition should be True
	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
}

func TestMachineReconcileNormal_BootstrapDataNotReady(t *testing.T) {
	g := NewWithT(t)

	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			t.Fatal("Server Get should not be called when bootstrap data is not ready")
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
			t.Fatal("Server Create should not be called when bootstrap data is not ready")
			return nil, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	// Set DataSecretName to nil to simulate bootstrap data not ready
	machineScope.Machine.Spec.Bootstrap.DataSecretName = nil

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	// Provisioned should NOT be set
	g.Expect(machineScope.CloudscaleMachine.Status.Initialization).To(BeNil())

	// ServerReadyCondition should indicate waiting for bootstrap data
	serverCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
	g.Expect(serverCond).ToNot(BeNil())
	g.Expect(serverCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(serverCond.Reason).To(Equal(infrastructurev1beta2.WaitingForBootstrapDataReason))
}

func TestMachineReconcileNormal_ServerChanging_DoesNotSetProvisioned(t *testing.T) {
	g := NewWithT(t)

	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:          id,
				Status:        "changing",
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(ServerStatusPollInterval), "should requeue when server is changing")

	// Provisioned should NOT be set since server is not running
	g.Expect(machineScope.CloudscaleMachine.Status.Initialization).To(BeNil())
}

func TestMachineReconcileNormal_ServerError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
			return nil, fmt.Errorf("server api error")
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
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

// ============================================================================
// Tests for reconcileDelete
// ============================================================================

func TestMachineReconcileDelete_Success(t *testing.T) {
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
	machineScope.CloudscaleMachine.Finalizers = []string{infrastructurev1beta2.MachineFinalizer}

	r := newTestMachineReconciler()

	result, err := r.reconcileDelete(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(deletedID).To(Equal("server-to-delete"))

	// Finalizer should be removed
	g.Expect(machineScope.CloudscaleMachine.Finalizers).ToNot(ContainElement(infrastructurev1beta2.MachineFinalizer))

	// DeletingCondition should be set
	deletingCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.DeletingCondition)
	g.Expect(deletingCond).ToNot(BeNil())
	g.Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))

	// ReadyCondition should be False
	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.DeletingReason))
}

func TestMachineReconcileDelete_ServerError_PreservesFinalizer(t *testing.T) {
	g := NewWithT(t)

	serverService := &mockServerService{
		deleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("server delete failed")
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-to-delete"
	machineScope.CloudscaleMachine.Finalizers = []string{infrastructurev1beta2.MachineFinalizer}

	r := newTestMachineReconciler()

	_, err := r.reconcileDelete(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("server delete failed"))

	// Finalizer should NOT be removed on error
	g.Expect(machineScope.CloudscaleMachine.Finalizers).To(ContainElement(infrastructurev1beta2.MachineFinalizer))
}

func TestMachineReconcileDelete_NoServer(t *testing.T) {
	g := NewWithT(t)

	serverService := &mockServerService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when no server exists")
			return nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	// No ServerID set
	machineScope.CloudscaleMachine.Finalizers = []string{infrastructurev1beta2.MachineFinalizer}

	r := newTestMachineReconciler()

	result, err := r.reconcileDelete(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())

	// Finalizer should be removed even when there's no server
	g.Expect(machineScope.CloudscaleMachine.Finalizers).ToNot(ContainElement(infrastructurev1beta2.MachineFinalizer))
}

// ============================================================================
// Tests for setReadyCondition
// ============================================================================

func TestMachineSetReadyCondition_ServerReady(t *testing.T) {
	g := NewWithT(t)

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-machine",
			Namespace:  "default",
			Generation: 1,
		},
	}
	conditions.Set(machine, metav1.Condition{
		Type:   infrastructurev1beta2.ServerReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: infrastructurev1beta2.ServerRunningReason,
	})

	r := newTestMachineReconciler()
	r.setReadyCondition(machine)

	readyCond := conditions.Get(machine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.ReadyReason))
}

func TestMachineSetReadyCondition_ServerNotReady(t *testing.T) {
	g := NewWithT(t)

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-machine",
			Namespace:  "default",
			Generation: 1,
		},
	}
	conditions.Set(machine, metav1.Condition{
		Type:    infrastructurev1beta2.ServerReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  infrastructurev1beta2.ServerStartingReason,
		Message: "Server is starting",
	})

	r := newTestMachineReconciler()
	r.setReadyCondition(machine)

	readyCond := conditions.Get(machine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.ServerStartingReason))
	g.Expect(readyCond.Message).To(Equal("Server is starting"))
}

func TestMachineSetReadyCondition_NoConditions(t *testing.T) {
	g := NewWithT(t)

	machine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-machine",
			Namespace:  "default",
			Generation: 1,
		},
	}

	r := newTestMachineReconciler()
	r.setReadyCondition(machine)

	// When no sub-conditions are set, the machine is not ready — a missing
	// ServerReadyCondition means we haven't verified the server yet.
	readyCond := conditions.Get(machine, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).ToNot(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.NotReadyReason))
	g.Expect(readyCond.Message).To(ContainSubstring("Waiting for"))
}

func TestMachineReconcileNormal_AlreadyProvisioned_StaysProvisioned(t *testing.T) {
	g := NewWithT(t)

	// When a machine is already provisioned and server is still running, Provisioned should remain true
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:          id,
				Status:        "running",
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"
	machineScope.CloudscaleMachine.Status.Initialization = &infrastructurev1beta2.MachineInitializationStatus{
		Provisioned: ptr.To(true),
	}

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(*machineScope.CloudscaleMachine.Status.Initialization.Provisioned).To(BeTrue())
}

func TestMachineReconcileNormal_ServerChanging_AlreadyProvisioned_StaysProvisioned(t *testing.T) {
	g := NewWithT(t)

	// When a server moves to "changing" but was already provisioned, Provisioned remains true
	// because reconcileNormal only sets Provisioned=true, never reverts it
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
			return &cloudscalesdk.Server{
				UUID:          id,
				Status:        "changing",
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"
	machineScope.CloudscaleMachine.Status.Initialization = &infrastructurev1beta2.MachineInitializationStatus{
		Provisioned: ptr.To(true),
	}

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(ServerStatusPollInterval))
	// Provisioned should remain true (never reverted)
	g.Expect(*machineScope.CloudscaleMachine.Status.Initialization.Provisioned).To(BeTrue())
}

// ============================================================================
// Tests for Reconcile entry-point
// ============================================================================

func TestMachineReconcile_ResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	fakeClient := newTestFakeClient()
	r := &CloudscaleMachineReconciler{
		Client:   fakeClient,
		Scheme:   fakeClient.Scheme(),
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
}

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

	fakeClient := newTestFakeClient(machine)
	r := &CloudscaleMachineReconciler{
		Client:   fakeClient,
		Scheme:   fakeClient.Scheme(),
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: machine.Name, Namespace: machine.Namespace},
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
}
