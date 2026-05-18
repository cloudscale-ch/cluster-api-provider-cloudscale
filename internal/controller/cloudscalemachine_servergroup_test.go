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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

func TestReconcileServerGroup_NoServerGroup_Noop(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			g.Fail("List should not be called when no server group is specified")
			return nil, nil
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	machineScope.CloudscaleMachine.Spec.ServerGroup = nil

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
}

func TestReconcileServerGroup_FindsExisting_SetsStatusID(t *testing.T) {
	g := NewWithT(t)

	createCalled := false
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{
					UUID:          "existing-group-uuid",
					Name:          "test-group",
					Type:          "anti-affinity",
					ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
				},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error) {
			createCalled = true
			return nil, nil
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

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
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{
					UUID:          "other-group-uuid",
					Name:          "different-group",
					Type:          "anti-affinity",
					ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
				},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error) {
			createCalled = true
			return &cloudscalesdk.ServerGroup{
				UUID:          "new-group-uuid",
				Name:          req.Name,
				Type:          req.Type,
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(createCalled).To(BeTrue(), "Create should be called when no matching name found")
	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(Equal("new-group-uuid"))
}

func TestReconcileServerGroup_SkipsNonMatchingZone(t *testing.T) {
	g := NewWithT(t)

	createCalled := false
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{
					UUID:          "other-zone-group-uuid",
					Name:          "test-group",
					Type:          "anti-affinity",
					ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "lpg1"}},
				},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error) {
			createCalled = true
			return &cloudscalesdk.ServerGroup{
				UUID:          "new-group-uuid",
				Name:          req.Name,
				Type:          req.Type,
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(createCalled).To(BeTrue(), "Create should be called when no matching zone found")
	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(Equal("new-group-uuid"))
}

func TestReconcileServerGroup_CreatesNew_SetsStatusID(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.ServerGroupRequest
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error) {
			capturedReq = req
			return &cloudscalesdk.ServerGroup{
				UUID:          "created-group-uuid",
				Name:          req.Name,
				Type:          req.Type,
				ZonalResource: cloudscalesdk.ZonalResource{Zone: cloudscalesdk.ZoneStub{Slug: "rma1"}},
			}, nil
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

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

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("listing server groups"))

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerGroupErrorReason))
}

func TestReconcileServerGroup_CreateError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error) {
			return nil, fmt.Errorf("create failed")
		},
	}

	machineScope := testutils.NewMachineScope(nil, testutils.WithMachineServerGroupService(serverGroupService), testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	_, err := r.reconcileServerGroup(context.Background(), machineScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("creating server group"))

	cond := conditions.Get(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.ServerGroupErrorReason))
}

func TestDeleteServerGroup_ClearsStatusID(t *testing.T) {
	g := NewWithT(t)

	machineScope := testutils.NewMachineScope(nil, testutils.WithServerGroup(&infrastructurev1beta2.ServerGroupSpec{Name: "test-group"}))
	machineScope.CloudscaleMachine.Status.ServerGroupID = "group-to-clear"

	r := &CloudscaleMachineReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	r.deleteServerGroup(context.Background(), machineScope)

	g.Expect(machineScope.CloudscaleMachine.Status.ServerGroupID).To(BeEmpty())
}
