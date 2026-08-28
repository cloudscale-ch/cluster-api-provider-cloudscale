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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/events"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

func TestDeleteServerGroups_DeletesAll(t *testing.T) {
	g := NewWithT(t)

	var deletedIDs []string

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{UUID: "sg-1", Name: "group-1"},
				{UUID: "sg-2", Name: "group-2"},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	r := newTestReconciler()

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedIDs).To(Equal([]string{"sg-1", "sg-2"}))
}

func TestDeleteServerGroups_NoGroups_Noop(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			g.Fail("Delete should not be called when no server groups exist")
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	r := newTestReconciler()

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteServerGroups_ListError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	r := newTestReconciler()

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("listing server groups"))
}

func TestDeleteServerGroups_DeleteError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{UUID: "sg-1", Name: "group-1"},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("delete failed")
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	r := newTestReconciler()

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("deleting server group"))
}

func TestDeleteServerGroups_Ignores404(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{UUID: "sg-already-deleted", Name: "group-1"},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			return &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	r := newTestReconciler()

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteServerGroups_OwnedServerPresent_SkipsDeletion(t *testing.T) {
	g := NewWithT(t)

	// The server group has a server that is owned by our cluster
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{UUID: "sg-1", Name: "group-1", Servers: []cloudscalesdk.ServerStub{{UUID: "server-123"}}},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			// Delete should NOT be called because server group has owned servers
			g.Fail("Delete must not be called when group has owned servers")
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	fakeClient := testutils.NewFakeClient(
		&infrastructurev1beta2.CloudscaleMachine{
			Name:      "machine-1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
			Status: infrastructurev1beta2.CloudscaleMachineStatus{
				ServerID: "server-123",
			},
		},
	)
	r := &CloudscaleClusterReconciler{
		Client:   fakeClient,
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("server group still contains owned servers"))
}

func TestDeleteServerGroups_ForeignServers_Skips(t *testing.T) {
	g := NewWithT(t)
	called := false

	// Server group has a server that is NOT owned by this cluster
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{UUID: "sg-foreign", Name: "foreign-group", Servers: []cloudscalesdk.ServerStub{{UUID: "server-999"}}},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			called = true
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	fakeClient := testutils.NewFakeClient(&infrastructurev1beta2.CloudscaleMachine{
		Name:      "machine-1",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel: "test-cluster",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			ServerID: "server-123",
		},
	})
	r := &CloudscaleClusterReconciler{
		Client:   fakeClient,
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(called).To(BeFalse())
}

func TestDeleteServerGroups_EmptyGroupName_DoesNotSkip(t *testing.T) {
	g := NewWithT(t)
	var deletedID string

	// Server group with no servers should be deleted immediately
	serverGroupService := &testutils.MockServerGroupService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
			return []cloudscalesdk.ServerGroup{
				{UUID: "sg-empty", Name: "empty-group", Servers: nil},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithServerGroupService(serverGroupService))
	r := newTestReconciler()

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedID).To(Equal("sg-empty"))
}
