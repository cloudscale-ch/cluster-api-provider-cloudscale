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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Server, error) {
			return &cloudscale.Server{
				UUID:          id,
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
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	require.NoError(t, err)
	assert.True(t, result.IsZero())

	// Provisioned should be set to true
	require.NotNil(t, machineScope.CloudscaleMachine.Status.Initialization)
	assert.True(t, *machineScope.CloudscaleMachine.Status.Initialization.Provisioned)

	// ReadyCondition should be True
	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
}

func TestMachineReconcileNormal_BootstrapDataNotReady(t *testing.T) {
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Server, error) {
			t.Fatal("Server Get should not be called when bootstrap data is not ready")
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerRequest) (*cloudscale.Server, error) {
			t.Fatal("Server Create should not be called when bootstrap data is not ready")
			return nil, nil
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	// Set DataSecretName to nil to simulate bootstrap data not ready
	machineScope.Machine.Spec.Bootstrap.DataSecretName = nil

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	require.NoError(t, err)
	assert.True(t, result.IsZero())

	// Provisioned should NOT be set
	assert.Nil(t, machineScope.CloudscaleMachine.Status.Initialization)

	// ServerReadyCondition should indicate waiting for bootstrap data
	serverCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition)
	require.NotNil(t, serverCond)
	assert.Equal(t, metav1.ConditionFalse, serverCond.Status)
	assert.Equal(t, infrastructurev1beta2.WaitingForBootstrapDataReason, serverCond.Reason)
}

func TestMachineReconcileNormal_ServerChanging_DoesNotSetProvisioned(t *testing.T) {
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
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	require.NoError(t, err)
	assert.Equal(t, ServerStatusPollInterval, result.RequeueAfter, "should requeue when server is changing")

	// Provisioned should NOT be set since server is not running
	assert.Nil(t, machineScope.CloudscaleMachine.Status.Initialization)
}

func TestMachineReconcileNormal_ServerError_PropagatesError(t *testing.T) {
	serverService := &mockServerService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.Server, error) {
			return nil, fmt.Errorf("server api error")
		},
	}

	machineScope := newTestMachineScopeWithServer(serverService)
	// No ServerID set, so it will try to List
	r := newTestMachineReconciler()

	_, err := r.reconcileNormal(context.Background(), machineScope)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "server api error")

	// ReadyCondition should be set by defer (False since ServerReadyCondition is False due to error)
	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
}

// ============================================================================
// Tests for reconcileDelete
// ============================================================================

func TestMachineReconcileDelete_Success(t *testing.T) {
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
	assert.Equal(t, "server-to-delete", deletedID)

	// Finalizer should be removed
	assert.NotContains(t, machineScope.CloudscaleMachine.Finalizers, infrastructurev1beta2.MachineFinalizer)

	// DeletingCondition should be set
	deletingCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.DeletingCondition)
	require.NotNil(t, deletingCond)
	assert.Equal(t, metav1.ConditionTrue, deletingCond.Status)

	// ReadyCondition should be False
	readyCond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, infrastructurev1beta2.DeletingReason, readyCond.Reason)
}

func TestMachineReconcileDelete_ServerError_PreservesFinalizer(t *testing.T) {
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

	require.Error(t, err)
	assert.Contains(t, err.Error(), "server delete failed")

	// Finalizer should NOT be removed on error
	assert.Contains(t, machineScope.CloudscaleMachine.Finalizers, infrastructurev1beta2.MachineFinalizer)
}

func TestMachineReconcileDelete_NoServer(t *testing.T) {
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())

	// Finalizer should be removed even when there's no server
	assert.NotContains(t, machineScope.CloudscaleMachine.Finalizers, infrastructurev1beta2.MachineFinalizer)
}

// ============================================================================
// Tests for setReadyCondition
// ============================================================================

func TestMachineSetReadyCondition_ServerReady(t *testing.T) {
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
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, infrastructurev1beta2.ReadyReason, readyCond.Reason)
}

func TestMachineSetReadyCondition_ServerNotReady(t *testing.T) {
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
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, infrastructurev1beta2.ServerStartingReason, readyCond.Reason)
	assert.Equal(t, "Server is starting", readyCond.Message)
}

func TestMachineSetReadyCondition_NoConditions(t *testing.T) {
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
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, infrastructurev1beta2.NotReadyReason, readyCond.Reason)
	assert.Contains(t, readyCond.Message, "Waiting for")
}

func TestMachineReconcileNormal_AlreadyProvisioned_StaysProvisioned(t *testing.T) {
	// When a machine is already provisioned and server is still running, Provisioned should remain true
	serverService := &mockServerService{
		getFn: func(ctx context.Context, id string) (*cloudscale.Server, error) {
			return &cloudscale.Server{
				UUID:          id,
				Status:        "running",
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
	assert.True(t, *machineScope.CloudscaleMachine.Status.Initialization.Provisioned)
}

func TestMachineReconcileNormal_ServerChanging_AlreadyProvisioned_StaysProvisioned(t *testing.T) {
	// When a server moves to "changing" but was already provisioned, Provisioned remains true
	// because reconcileNormal only sets Provisioned=true, never reverts it
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
	machineScope.CloudscaleMachine.Status.ServerID = "server-123"
	machineScope.CloudscaleMachine.Status.Initialization = &infrastructurev1beta2.MachineInitializationStatus{
		Provisioned: ptr.To(true),
	}

	r := newTestMachineReconciler()

	result, err := r.reconcileNormal(context.Background(), machineScope)

	require.NoError(t, err)
	assert.Equal(t, ServerStatusPollInterval, result.RequeueAfter)
	// Provisioned should remain true (never reverted)
	assert.True(t, *machineScope.CloudscaleMachine.Status.Initialization.Provisioned)
}

// ============================================================================
// Tests for Reconcile entry-point
// ============================================================================

func TestMachineReconcile_ResourceNotFound(t *testing.T) {
	fakeClient := newTestFakeClient()
	r := &CloudscaleMachineReconciler{
		Client:   fakeClient,
		Scheme:   fakeClient.Scheme(),
		recorder: events.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.True(t, result.IsZero())
}

func TestMachineReconcile_NoOwnerMachine(t *testing.T) {
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

	require.NoError(t, err)
	assert.True(t, result.IsZero())
}
