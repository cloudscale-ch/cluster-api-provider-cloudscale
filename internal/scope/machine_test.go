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

// validMachineScopeParams returns a fully populated MachineScopeParams; tests
// blank fields to drive validation errors.
func validMachineScopeParams(t *testing.T) (MachineScopeParams, *infrastructurev1beta2.CloudscaleMachine) {
	t.Helper()
	cloudscaleMachine := newTestCloudscaleMachine()
	return MachineScopeParams{
		Client:            newFakeClient(cloudscaleMachine),
		Logger:            logr.Discard(),
		Cluster:           newTestCluster(),
		Machine:           newTestMachine(),
		CloudscaleCluster: newTestCloudscaleCluster(),
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  newTestCloudscaleClient(),
	}, cloudscaleMachine
}

func TestNewMachineScope_Success(t *testing.T) {
	g := NewWithT(t)
	params, _ := validMachineScopeParams(t)

	scope, err := NewMachineScope(params)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scope).ToNot(BeNil())
	g.Expect(scope.Cluster).To(Equal(params.Cluster))
	g.Expect(scope.Machine).To(Equal(params.Machine))
	g.Expect(scope.CloudscaleCluster).To(Equal(params.CloudscaleCluster))
	g.Expect(scope.CloudscaleMachine).To(Equal(params.CloudscaleMachine))
	g.Expect(scope.CloudscaleClient).To(Equal(params.CloudscaleClient))
	g.Expect(scope.Name()).To(Equal("test-machine"))
	g.Expect(scope.Namespace()).To(Equal("test-namespace"))
}

func TestNewMachineScope_Validation(t *testing.T) {
	cases := []struct {
		name      string
		blank     func(p *MachineScopeParams)
		errPhrase string
	}{
		{"nil Client", func(p *MachineScopeParams) { p.Client = nil }, "client is required"},
		{"nil Cluster", func(p *MachineScopeParams) { p.Cluster = nil }, "cluster is required"},
		{"nil Machine", func(p *MachineScopeParams) { p.Machine = nil }, "machine is required"},
		{"nil CloudscaleCluster", func(p *MachineScopeParams) { p.CloudscaleCluster = nil }, "cloudscaleCluster is required"},
		{"nil CloudscaleMachine", func(p *MachineScopeParams) { p.CloudscaleMachine = nil }, "cloudscaleMachine is required"},
		{"nil CloudscaleClient", func(p *MachineScopeParams) { p.CloudscaleClient = nil }, "cloudscaleClient is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			params, _ := validMachineScopeParams(t)
			tc.blank(&params)

			scope, err := NewMachineScope(params)
			g.Expect(err).To(HaveOccurred())
			g.Expect(scope).To(BeNil())
			g.Expect(err.Error()).To(ContainSubstring(tc.errPhrase))
		})
	}
}

func TestMachineScope_IsControlPlane(t *testing.T) {
	cases := []struct {
		name           string
		labels         map[string]string
		isControlPlane bool
	}{
		{"no labels", nil, false},
		{"control-plane label set", map[string]string{clusterv1.MachineControlPlaneLabel: ""}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			params, _ := validMachineScopeParams(t)
			params.Machine = newTestMachine()
			params.Machine.Labels = tc.labels

			scope, err := NewMachineScope(params)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(scope.IsControlPlane()).To(Equal(tc.isControlPlane))
		})
	}
}

func TestMachineScope_GetBootstrapData(t *testing.T) {
	cases := []struct {
		name       string
		secretName *string
		secret     *corev1.Secret
		wantData   string
		wantErrSub string
	}{
		{
			name:       "happy path",
			secretName: ptr.To("bootstrap-secret"),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-secret", Namespace: "test-namespace"},
				Data:       map[string][]byte{"value": []byte("#cloud-config\nruncmd:\n  - echo hello")},
			},
			wantData: "#cloud-config\nruncmd:\n  - echo hello",
		},
		{
			name:       "nil DataSecretName",
			secretName: nil,
			wantErrSub: "bootstrap data secret name is nil",
		},
		{
			name:       "secret not found",
			secretName: ptr.To("nonexistent-secret"),
			wantErrSub: "getting bootstrap data secret",
		},
		{
			name:       "missing value key",
			secretName: ptr.To("bootstrap-secret"),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-secret", Namespace: "test-namespace"},
				Data:       map[string][]byte{"other-key": []byte("some data")},
			},
			wantErrSub: "missing 'value' key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			machine := newTestMachine()
			machine.Spec.Bootstrap.DataSecretName = tc.secretName

			cloudscaleMachine := newTestCloudscaleMachine()
			objs := []client.Object{cloudscaleMachine}
			if tc.secret != nil {
				objs = append(objs, tc.secret)
			}

			scope, err := NewMachineScope(MachineScopeParams{
				Client:            newFakeClient(objs...),
				Logger:            logr.Discard(),
				Cluster:           newTestCluster(),
				Machine:           machine,
				CloudscaleCluster: newTestCloudscaleCluster(),
				CloudscaleMachine: cloudscaleMachine,
				CloudscaleClient:  newTestCloudscaleClient(),
			})
			g.Expect(err).ToNot(HaveOccurred())

			data, err := scope.GetBootstrapData(context.Background())
			if tc.wantErrSub != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.wantErrSub))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(data).To(Equal(tc.wantData))
		})
	}
}

func TestMachineScope_ProviderID(t *testing.T) {
	cases := []struct {
		name           string
		seeded         *string
		setTo          string
		wantInitialGet string
		wantAfterSet   string
	}{
		{
			name:           "already set is read back verbatim",
			seeded:         ptr.To("cloudscale://server-uuid"),
			wantInitialGet: "cloudscale://server-uuid",
		},
		{
			name:           "nil returns empty string",
			seeded:         nil,
			wantInitialGet: "",
		},
		{
			name:         "Set prefixes raw uuid with cloudscale://",
			seeded:       nil,
			setTo:        "new-server-uuid",
			wantAfterSet: "cloudscale://new-server-uuid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			params, cloudscaleMachine := validMachineScopeParams(t)
			cloudscaleMachine.Spec.ProviderID = tc.seeded

			scope, err := NewMachineScope(params)
			g.Expect(err).ToNot(HaveOccurred())

			if tc.setTo != "" {
				scope.SetProviderID(tc.setTo)
				g.Expect(scope.GetProviderID()).To(Equal(tc.wantAfterSet))
				g.Expect(*scope.CloudscaleMachine.Spec.ProviderID).To(Equal(tc.wantAfterSet))
				return
			}
			g.Expect(scope.GetProviderID()).To(Equal(tc.wantInitialGet))
		})
	}
}

func TestMachineScope_Close_PatchesStatus(t *testing.T) {
	g := NewWithT(t)
	params, cloudscaleMachine := validMachineScopeParams(t)
	fakeClient := params.Client

	scope, err := NewMachineScope(params)
	g.Expect(err).ToNot(HaveOccurred())

	scope.CloudscaleMachine.Status.ServerID = "patched-server-id"

	g.Expect(scope.Close(context.Background())).To(Succeed())

	updated := &infrastructurev1beta2.CloudscaleMachine{}
	g.Expect(fakeClient.Get(context.Background(), client.ObjectKey{
		Name:      cloudscaleMachine.Name,
		Namespace: cloudscaleMachine.Namespace,
	}, updated)).To(Succeed())
	g.Expect(updated.Status.ServerID).To(Equal("patched-server-id"))
}
