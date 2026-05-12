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

// validClusterScopeParams returns a fully-populated parameter set; tests
// blank out individual fields to provoke validation errors.
func validClusterScopeParams(t *testing.T) ClusterScopeParams {
	t.Helper()
	cloudscaleCluster := newTestCloudscaleCluster()
	return ClusterScopeParams{
		Client:            newFakeClient(cloudscaleCluster),
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  newTestCloudscaleClient(),
	}
}

func TestNewClusterScope_Success(t *testing.T) {
	g := NewWithT(t)
	params := validClusterScopeParams(t)

	scope, err := NewClusterScope(params)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope).ToNot(BeNil())
	g.Expect(scope.Cluster).To(Equal(params.Cluster))
	g.Expect(scope.CloudscaleCluster).To(Equal(params.CloudscaleCluster))
	g.Expect(scope.CloudscaleClient).To(Equal(params.CloudscaleClient))
	g.Expect(scope.Name()).To(Equal("test-cluster"))
	g.Expect(scope.Namespace()).To(Equal("test-namespace"))
}

func TestNewClusterScope_Validation(t *testing.T) {
	cases := []struct {
		name      string
		blank     func(p *ClusterScopeParams)
		errPhrase string
	}{
		{"nil Client", func(p *ClusterScopeParams) { p.Client = nil }, "client is required"},
		{"nil Cluster", func(p *ClusterScopeParams) { p.Cluster = nil }, "cluster is required"},
		{"nil CloudscaleCluster", func(p *ClusterScopeParams) { p.CloudscaleCluster = nil }, "cloudscaleCluster is required"},
		{"nil CloudscaleClient", func(p *ClusterScopeParams) { p.CloudscaleClient = nil }, "cloudscaleClient is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			params := validClusterScopeParams(t)
			tc.blank(&params)

			scope, err := NewClusterScope(params)
			g.Expect(err).To(HaveOccurred())
			g.Expect(scope).To(BeNil())
			g.Expect(err.Error()).To(ContainSubstring(tc.errPhrase))
		})
	}
}

func TestClusterScope_Close_PatchesStatus(t *testing.T) {
	g := NewWithT(t)
	params := validClusterScopeParams(t)
	fakeClient := params.Client

	scope, err := NewClusterScope(params)
	g.Expect(err).ToNot(HaveOccurred())

	scope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "patched-network-id", Managed: true},
	}

	g.Expect(scope.Close(context.Background())).To(Succeed())

	updated := &infrastructurev1beta2.CloudscaleCluster{}
	g.Expect(fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      params.CloudscaleCluster.Name,
		Namespace: params.CloudscaleCluster.Namespace,
	}, updated)).To(Succeed())
	g.Expect(updated.Status.Networks).To(HaveLen(1))
	g.Expect(updated.Status.Networks[0].NetworkID).To(Equal("patched-network-id"))
}
