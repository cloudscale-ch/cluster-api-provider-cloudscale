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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.Equal(t, cluster, scope.Cluster)
	assert.Equal(t, cloudscaleCluster, scope.CloudscaleCluster)
	assert.Equal(t, cloudscaleClient, scope.CloudscaleClient)
}

func TestNewClusterScope_NilClient(t *testing.T) {
	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            nil,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	require.Error(t, err)
	assert.Nil(t, scope)
	assert.Contains(t, err.Error(), "client is required")
}

func TestNewClusterScope_NilCluster(t *testing.T) {
	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           nil,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	require.Error(t, err)
	assert.Nil(t, scope)
	assert.Contains(t, err.Error(), "cluster is required")
}

func TestNewClusterScope_NilCloudscaleCluster(t *testing.T) {
	fakeClient := newFakeClient()

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: nil,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	require.Error(t, err)
	assert.Nil(t, scope)
	assert.Contains(t, err.Error(), "cloudscaleCluster is required")
}

func TestNewClusterScope_NilCloudscaleClient(t *testing.T) {
	cloudscaleCluster := newTestCloudscaleCluster()
	fakeClient := newFakeClient(cloudscaleCluster)

	scope, err := NewClusterScope(ClusterScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  nil,
	})

	require.Error(t, err)
	assert.Nil(t, scope)
	assert.Contains(t, err.Error(), "cloudscaleClient is required")
}

// ============================================================================
// Tests for Name and Namespace
// ============================================================================

func TestClusterScope_Name(t *testing.T) {
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

	require.NoError(t, err)
	assert.Equal(t, "test-cluster", scope.Name())
}

func TestClusterScope_Namespace(t *testing.T) {
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

	require.NoError(t, err)
	assert.Equal(t, "test-namespace", scope.Namespace())
}

// ============================================================================
// Tests for Close
// ============================================================================

func TestClusterScope_Close(t *testing.T) {
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
	require.NoError(t, err)

	// Modify status to verify patch happens
	scope.CloudscaleCluster.Status.NetworkID = "patched-network-id"

	err = scope.Close(context.Background())
	require.NoError(t, err)

	// Verify the status was patched by fetching the object again
	updated := &infrastructurev1beta2.CloudscaleCluster{}
	err = fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      cloudscaleCluster.Name,
		Namespace: cloudscaleCluster.Namespace,
	}, updated)
	require.NoError(t, err)
	assert.Equal(t, "patched-network-id", updated.Status.NetworkID)
}
