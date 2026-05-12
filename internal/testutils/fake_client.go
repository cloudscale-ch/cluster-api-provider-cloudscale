package testutils

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
)

// NewFakeClient returns a fake client with corev1, clusterv1, and infrastructurev1beta2
// schemes registered.
func NewFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1beta2.AddToScheme(scheme)
	_ = infrastructurev1beta2.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}
