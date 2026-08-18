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
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// testResource is a simple type used for testing ensureResource.
type testResource struct {
	UUID string
}

// mockGetListService implements getListService[testResource] for testing.
type mockGetListService struct {
	getFn  func(ctx context.Context, id string) (*testResource, error)
	listFn func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error)
}

func (m *mockGetListService) Get(ctx context.Context, id string) (*testResource, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockGetListService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func testClusterScope() *scope.ClusterScope {
	return &scope.ClusterScope{
		Logger: logr.Discard(),
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}
}

func extractTestUUID(r testResource) string {
	return r.UUID
}

var testTags = cloudscalesdk.TagMap{"test-key": "test-value"}

func TestEnsureResource_ExistingID_Found(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		getFn: func(ctx context.Context, id string) (*testResource, error) {
			return &testResource{UUID: id}, nil
		},
	}

	resource, id, err := ensureResource(context.Background(), testClusterScope(), "existing-123", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(id).To(Equal("existing-123"))
	g.Expect(resource).ToNot(BeNil())
	g.Expect(resource.UUID).To(Equal("existing-123"))
}

func TestEnsureResource_ExistingID_NotFound_FallsThrough(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		getFn: func(ctx context.Context, id string) (*testResource, error) {
			return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error) {
			return nil, nil // empty list
		},
	}

	resource, id, err := ensureResource(context.Background(), testClusterScope(), "deleted-123", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(id).To(Equal(""), "should return empty ID so caller creates the resource")
	g.Expect(resource).To(BeNil())
}

func TestEnsureResource_ExistingID_GetError(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		getFn: func(ctx context.Context, id string) (*testResource, error) {
			return nil, fmt.Errorf("api connection error")
		},
	}

	_, _, err := ensureResource(context.Background(), testClusterScope(), "existing-123", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("api connection error"))
}

func TestEnsureResource_NoID_ListFindsOne(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error) {
			return []testResource{{UUID: "adopted-123"}}, nil
		},
	}

	resource, id, err := ensureResource(context.Background(), testClusterScope(), "", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(id).To(Equal("adopted-123"))
	g.Expect(resource).ToNot(BeNil())
	g.Expect(resource.UUID).To(Equal("adopted-123"))
}

func TestEnsureResource_NoID_ListFindsMultiple(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error) {
			return []testResource{
				{UUID: "resource-1"},
				{UUID: "resource-2"},
			}, nil
		},
	}

	_, _, err := ensureResource(context.Background(), testClusterScope(), "", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("found 2 test resources matching tag filter"))
}

func TestEnsureResource_NoID_ListFindsNone(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error) {
			return nil, nil
		},
	}

	resource, id, err := ensureResource(context.Background(), testClusterScope(), "", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(id).To(Equal(""), "should return empty ID so caller creates the resource")
	g.Expect(resource).To(BeNil())
}

func TestEnsureResource_NoID_ListError(t *testing.T) {
	g := NewWithT(t)

	svc := &mockGetListService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]testResource, error) {
			return nil, fmt.Errorf("list api error")
		},
	}

	_, _, err := ensureResource(context.Background(), testClusterScope(), "", "test resource", svc, extractTestUUID, testTags)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("list api error"))
}
