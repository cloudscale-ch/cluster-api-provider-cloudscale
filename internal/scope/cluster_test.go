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

package scope

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = infrastructurev1beta2.AddToScheme(scheme)
	return scheme
}

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(objs...).
		WithStatusSubresource(objs...).
		Build()
}

func newTestCluster() *clusterv1.Cluster {
	return &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
	}
}

func newTestCloudscaleCluster() *infrastructurev1beta2.CloudscaleCluster {
	return &infrastructurev1beta2.CloudscaleCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: infrastructurev1beta2.CloudscaleClusterSpec{
			Region: "rma",
		},
	}
}

func newTestCloudscaleClient() *cloudscale.Client {
	return &cloudscale.Client{}
}

// ============================================================================
// Tests for NewClusterScope
// ============================================================================

func TestNewClusterScope_Success(t *testing.T) {
	g := NewWithT(t)

	cluster := newTestCluster()
	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)
	cloudscaleClient := newTestCloudscaleClient()

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           cluster,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  cloudscaleClient,
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope).ToNot(BeNil())
	g.Expect(scope.Cluster).To(Equal(cluster))
	g.Expect(scope.CloudscaleCluster).To(Equal(cloudscaleCluster))
	g.Expect(scope.CloudscaleClient).To(Equal(cloudscaleClient))
}

func TestNewClusterScope_NilClient(t *testing.T) {
	g := NewWithT(t)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            nil,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("client is required"))
}

func TestNewClusterScope_NilCluster(t *testing.T) {
	g := NewWithT(t)

	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           nil,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cluster is required"))
}

func TestNewClusterScope_NilCloudscaleCluster(t *testing.T) {
	g := NewWithT(t)

	fakeClient := newFakeClient()

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: nil,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cloudscaleCluster is required"))
}

func TestNewClusterScope_NilCloudscaleClient(t *testing.T) {
	g := NewWithT(t)

	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  nil,
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cloudscaleClient is required"))
}

// ============================================================================
// Tests for Name and Namespace
// ============================================================================

func TestClusterScope_Name(t *testing.T) {
	g := NewWithT(t)

	cluster := newTestCluster()
	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           cluster,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope.Name()).To(Equal("test-cluster"))
}

func TestClusterScope_Namespace(t *testing.T) {
	g := NewWithT(t)

	cluster := newTestCluster()
	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           cluster,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope.Namespace()).To(Equal("test-namespace"))
}

// ============================================================================
// Tests for Close
// ============================================================================

func TestClusterScope_Close(t *testing.T) {
	g := NewWithT(t)

	cluster := newTestCluster()
	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           cluster,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	// Modify status to verify patch happens
	scope.CloudscaleCluster.Status.NetworkID = "patched-network-id"

	err = scope.Close(context.Background())
	g.Expect(err).ToNot(HaveOccurred())

	// Verify the status was patched by fetching the object again
	updated := &infrastructurev1beta2.CloudscaleCluster{}
	err = fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      cloudscaleCluster.Name,
		Namespace: cloudscaleCluster.Namespace,
	}, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.Status.NetworkID).To(Equal("patched-network-id"))
}
