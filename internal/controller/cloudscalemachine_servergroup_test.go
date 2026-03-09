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
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	cs "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

type mockServerGroupService struct {
	createFn func(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error)
	getFn    func(ctx context.Context, id string) (*cloudscale.ServerGroup, error)
	listFn   func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error)
	deleteFn func(ctx context.Context, id string) error
	updateFn func(ctx context.Context, id string, req *cloudscale.ServerGroupRequest) error
}

func (m *mockServerGroupService) Create(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockServerGroupService) Get(ctx context.Context, id string) (*cloudscale.ServerGroup, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockServerGroupService) List(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockServerGroupService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockServerGroupService) Update(ctx context.Context, id string, req *cloudscale.ServerGroupRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

func newTestMachineScopeWithServerGroup(serverGroupService cs.ServerGroupService) *scope.MachineScope {
	cloudscaleClient := &cs.Client{
		ServerGroups: serverGroupService,
	}

	cloudscaleMachine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
		},
		Spec: infrastructurev1beta2.CloudscaleMachineSpec{
			Flavor:      "flex-8-4",
			Image:       "ubuntu-24.04",
			ServerGroup: &infrastructurev1beta2.ServerGroupSpec{Name: "test-group"},
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

func TestReconcileServerGroup_NoServerGroup_Noop(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			t.Fatal("List should not be called when no server group is specified")
			return nil, nil
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	machineScope.CloudscaleMachine.Spec.ServerGroup = nil

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
}

func TestReconcileServerGroup_FindsExisting_SetsStatusID(t *testing.T) {
	g := NewWithT(t)

	createCalled := false
	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return []cloudscale.ServerGroup{
				{
					UUID:          "existing-group-uuid",
					Name:          "test-group",
					Type:          "anti-affinity",
					ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error) {
			createCalled = true
			return nil, nil
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(createCalled).To(BeFalse(), "Create should not be called when group exists")
	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(Equal("existing-group-uuid"))

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
}

func TestReconcileServerGroup_SkipsNonMatchingName(t *testing.T) {
	g := NewWithT(t)

	createCalled := false
	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return []cloudscale.ServerGroup{
				{
					UUID:          "other-group-uuid",
					Name:          "different-group",
					Type:          "anti-affinity",
					ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
				},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error) {
			createCalled = true
			return &cloudscale.ServerGroup{
				UUID:          "new-group-uuid",
				Name:          req.Name,
				Type:          req.Type,
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(createCalled).To(BeTrue(), "Create should be called when no matching name found")
	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(Equal("new-group-uuid"))
}

func TestReconcileServerGroup_SkipsNonMatchingZone(t *testing.T) {
	g := NewWithT(t)

	createCalled := false
	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return []cloudscale.ServerGroup{
				{
					UUID:          "other-zone-group-uuid",
					Name:          "test-group",
					Type:          "anti-affinity",
					ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "lpg1"}},
				},
			}, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error) {
			createCalled = true
			return &cloudscale.ServerGroup{
				UUID:          "new-group-uuid",
				Name:          req.Name,
				Type:          req.Type,
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(createCalled).To(BeTrue(), "Create should be called when no matching zone found")
	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(Equal("new-group-uuid"))
}

func TestReconcileServerGroup_CreatesNew_SetsStatusID(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscale.ServerGroupRequest
	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error) {
			capturedReq = req
			return &cloudscale.ServerGroup{
				UUID:          "created-group-uuid",
				Name:          req.Name,
				Type:          req.Type,
				ZonalResource: cloudscale.ZonalResource{Zone: cloudscale.Zone{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(Equal("created-group-uuid"))

	// Verify request fields
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.Name).To(Equal("test-group"))
	g.Expect(capturedReq.Type).To(Equal("anti-affinity"))
	g.Expect(capturedReq.Zone).To(Equal("rma1"))
	g.Expect(capturedReq.Tags).ToNot(BeNil())
	// Verify cluster-level tags are used (not machine-specific tags)
	expectedTagKey := machineScope.CloudscaleCluster.ClusterTagKey()
	g.Expect((*capturedReq.Tags)[expectedTagKey]).To(Equal(string(infrastructurev1beta2.ResourceLifecycleOwned)))

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
}

func TestReconcileServerGroup_ListError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("listing server groups"))

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerGroupErrorReason))
}

func TestReconcileServerGroup_CreateError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscale.ServerGroupRequest) (*cloudscale.ServerGroup, error) {
			return nil, fmt.Errorf("create failed")
		},
	}

	machineScope := newTestMachineScopeWithServerGroup(serverGroupService)
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("creating server group"))

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerGroupErrorReason))
}

func TestDeleteServerGroup_ClearsStatusID(t *testing.T) {
	g := NewWithT(t)

	machineScope := newTestMachineScopeWithServerGroup(nil)
	machineScope.CloudscaleMachine.Status.ServerGroupID = "group-to-clear"

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	r.deleteServerGroup(context.Background(), machineScope)

	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(BeEmpty())
}
