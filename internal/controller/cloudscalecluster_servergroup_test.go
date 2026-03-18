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

	"github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	cs "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

func newTestClusterScopeWithServerGroups(serverGroupService cs.ServerGroupService) *scope.ClusterScope {
	return &scope.ClusterScope{
		Logger: logr.Discard(),
		Cluster: &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
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
		},
		CloudscaleClient: &cs.Client{
			ServerGroups: serverGroupService,
		},
	}
}

func TestDeleteServerGroups_DeletesAll(t *testing.T) {
	g := NewWithT(t)

	var deletedIDs []string

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return []cloudscale.ServerGroup{
				{UUID: "sg-1", Name: "group-1"},
				{UUID: "sg-2", Name: "group-2"},
			}, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}

	clusterScope := newTestClusterScopeWithServerGroups(serverGroupService)
	r := &CloudscaleClusterReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedIDs).To(Equal([]string{"sg-1", "sg-2"}))
}

func TestDeleteServerGroups_NoGroups_Noop(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return nil, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when no server groups exist")
			return nil
		},
	}

	clusterScope := newTestClusterScopeWithServerGroups(serverGroupService)
	r := &CloudscaleClusterReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteServerGroups_ListError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	clusterScope := newTestClusterScopeWithServerGroups(serverGroupService)
	r := &CloudscaleClusterReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("listing server groups"))
}

func TestDeleteServerGroups_DeleteError_PropagatesError(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return []cloudscale.ServerGroup{
				{UUID: "sg-1", Name: "group-1"},
			}, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("delete failed")
		},
	}

	clusterScope := newTestClusterScopeWithServerGroups(serverGroupService)
	r := &CloudscaleClusterReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("deleting server group"))
}

func TestDeleteServerGroups_Ignores404(t *testing.T) {
	g := NewWithT(t)

	serverGroupService := &mockServerGroupService{
		listFn: func(ctx context.Context, modifiers ...cloudscale.ListRequestModifier) ([]cloudscale.ServerGroup, error) {
			return []cloudscale.ServerGroup{
				{UUID: "sg-already-deleted", Name: "group-1"},
			}, nil
		},
		deleteFn: func(ctx context.Context, id string) error {
			return &cloudscale.ErrorResponse{StatusCode: 404}
		},
	}

	clusterScope := newTestClusterScopeWithServerGroups(serverGroupService)
	r := &CloudscaleClusterReconciler{
		recorder: events.NewFakeRecorder(10),
	}

	err := r.deleteServerGroups(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}
