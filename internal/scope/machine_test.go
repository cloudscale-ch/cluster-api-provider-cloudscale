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
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

func newTestMachine() *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
		},
	}
}

func newTestCloudscaleMachine() *infrastructurev1beta2.CloudscaleMachine {
	return &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "test-namespace",
		},
		Spec: infrastructurev1beta2.CloudscaleMachineSpec{
			Flavor: "flex-8-4",
			Image:  "ubuntu-24.04",
		},
	}
}

// ============================================================================
// Tests for NewMachineScope
// ============================================================================

func TestNewMachineScope_Success(t *testing.T) {
	g := NewWithT(t)

	cluster := newTestCluster()
	machine := newTestMachine()
	cloudscaleCluster := newTestCloudscaleCluster()
	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleCluster, cloudscaleMachine)
	cloudscaleClient := newTestCloudscaleClient()

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           cluster,
		Machine:           machine,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  cloudscaleClient,
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope).ToNot(BeNil())
	g.Expect(scope.Cluster).To(Equal(cluster))
	g.Expect(scope.Machine).To(Equal(machine))
	g.Expect(scope.CloudscaleCluster).To(Equal(cloudscaleCluster))
	g.Expect(scope.CloudscaleMachine).To(Equal(cloudscaleMachine))
	g.Expect(scope.CloudscaleClient).To(Equal(cloudscaleClient))
}

func TestNewMachineScope_NilClient(t *testing.T) {
	g := NewWithT(t)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            nil,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: newTestCloudscaleMachine(),
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("client is required"))
}

func TestNewMachineScope_NilCluster(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           nil,
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cluster is required"))
}

func TestNewMachineScope_NilMachine(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           nil,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("machine is required"))
}

func TestNewMachineScope_NilCloudscaleCluster(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: nil,
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cloudscaleCluster is required"))
}

func TestNewMachineScope_NilCloudscaleMachine(t *testing.T) {
	g := NewWithT(t)

	fakeClient := newFakeClient()

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: nil,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cloudscaleMachine is required"))
}

func TestNewMachineScope_NilCloudscaleClient(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  nil,
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(scope).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("cloudscaleClient is required"))
}

// ============================================================================
// Tests for Name and Namespace
// ============================================================================

func TestMachineScope_Name(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope.Name()).To(Equal("test-machine"))
}

func TestMachineScope_Namespace(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope.Namespace()).To(Equal("test-namespace"))
}

// ============================================================================
// Tests for IsControlPlane
// ============================================================================

func TestMachineScope_IsControlPlane_True(t *testing.T) {
	g := NewWithT(t)

	machine := newTestMachine()
	machine.Labels = map[string]string{
		clusterv1.MachineControlPlaneLabel: "",
	}
	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           machine,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope.IsControlPlane()).To(BeTrue())
}

func TestMachineScope_IsControlPlane_False(t *testing.T) {
	g := NewWithT(t)

	machine := newTestMachine()
	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           machine,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope.IsControlPlane()).To(BeFalse())
}

// ============================================================================
// Tests for GetBootstrapData
// ============================================================================

func TestMachineScope_GetBootstrapData_Success(t *testing.T) {
	g := NewWithT(t)

	machine := newTestMachine()
	machine.Spec.Bootstrap.DataSecretName = ptr.To("bootstrap-secret")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"value": []byte("#cloud-config\nruncmd:\n  - echo hello"),
		},
	}

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine, secret)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           machine,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	data, err := scope.GetBootstrapData(context.Background())

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(Equal("#cloud-config\nruncmd:\n  - echo hello"))
}

func TestMachineScope_GetBootstrapData_NilSecretName(t *testing.T) {
	g := NewWithT(t)

	machine := newTestMachine()
	// DataSecretName is nil

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           machine,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	_, err = scope.GetBootstrapData(context.Background())

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("bootstrap data secret name is nil"))
}

func TestMachineScope_GetBootstrapData_SecretNotFound(t *testing.T) {
	g := NewWithT(t)

	machine := newTestMachine()
	machine.Spec.Bootstrap.DataSecretName = ptr.To("nonexistent-secret")

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           machine,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	_, err = scope.GetBootstrapData(context.Background())

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("getting bootstrap data secret"))
}

func TestMachineScope_GetBootstrapData_MissingValueKey(t *testing.T) {
	g := NewWithT(t)

	machine := newTestMachine()
	machine.Spec.Bootstrap.DataSecretName = ptr.To("bootstrap-secret")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"other-key": []byte("some data"),
		},
	}

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine, secret)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           machine,
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	_, err = scope.GetBootstrapData(context.Background())

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("missing 'value' key"))
}

// ============================================================================
// Tests for GetProviderID / SetProviderID
// ============================================================================

func TestMachineScope_GetProviderID_WhenSet(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	cloudscaleMachine.Spec.ProviderID = ptr.To("cloudscale://server-uuid")
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(scope.GetProviderID()).To(Equal("cloudscale://server-uuid"))
}

func TestMachineScope_GetProviderID_WhenNil(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	// ProviderID is nil
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(scope.GetProviderID()).To(Equal(""))
}

func TestMachineScope_SetProviderID(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	scope.SetProviderID("new-server-uuid")

	g.Expect(scope.GetProviderID()).To(Equal("cloudscale://new-server-uuid"))
	g.Expect(*scope.CloudscaleMachine.Spec.ProviderID).To(Equal("cloudscale://new-server-uuid"))
}

// ============================================================================
// Tests for Close
// ============================================================================

func TestMachineScope_Close(t *testing.T) {
	g := NewWithT(t)

	cloudscaleMachine := newTestCloudscaleMachine()
	fakeClient := newFakeClient(cloudscaleMachine)

	scope, err := NewMachineScope(MachineScopeParams{
		Client:            fakeClient,
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	})
	g.Expect(err).ToNot(HaveOccurred())

	// Modify status to verify patch happens
	scope.CloudscaleMachine.Status.ServerID = "patched-server-id"

	err = scope.Close(context.Background())
	g.Expect(err).ToNot(HaveOccurred())

	// Verify the status was patched by fetching the object again
	updated := &infrastructurev1beta2.CloudscaleMachine{}
	err = fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      cloudscaleMachine.Name,
		Namespace: cloudscaleMachine.Namespace,
	}, updated)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.Status.ServerID).To(Equal("patched-server-id"))
}
