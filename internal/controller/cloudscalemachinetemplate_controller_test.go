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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

func TestCloudscaleMachineTemplateReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrastructurev1beta2.AddToScheme(scheme)

	tests := []struct {
		name                    string
		template                *infrastructurev1beta2.CloudscaleMachineTemplate
		flavorInfo              *cloudscale.FlavorInfo
		expectedCPU             string
		expectedMemory          string
		expectedGPU             string
		expectCapacityPopulated bool
	}{
		{
			name: "populates status.capacity with cpu and memory",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				Name:      "test-template",
				Namespace: "default",
				Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
					Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
						Spec: infrastructurev1beta2.CloudscaleMachineSpec{
							Flavor: "flex-8-4",
							Image:  "ubuntu-24.04",
						},
					},
				},
			},
			flavorInfo:              testutils.NewTestFlavorInfo(),
			expectedCPU:             "4",
			expectedMemory:          "8Gi",
			expectedGPU:             "",
			expectCapacityPopulated: true,
		},
		{
			name: "populates GPU for gpu flavor",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				Name:      "test-template",
				Namespace: "default",
				Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
					Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
						Spec: infrastructurev1beta2.CloudscaleMachineSpec{
							Flavor:         "gpu2-640-80-4-400",
							Image:          "ubuntu-24.04",
							RootVolumeSize: 100,
						},
					},
				},
			},
			flavorInfo:              testutils.NewTestFlavorInfo(),
			expectedCPU:             "80",
			expectedMemory:          "640Gi",
			expectedGPU:             "4",
			expectCapacityPopulated: true,
		},
		{
			name: "unknown flavor does not populate capacity",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				Name:      "test-template",
				Namespace: "default",
				Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
					Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
						Spec: infrastructurev1beta2.CloudscaleMachineSpec{
							Flavor: "unknown-flavor",
							Image:  "ubuntu-24.04",
						},
					},
				},
			},
			flavorInfo:              testutils.NewTestFlavorInfo(),
			expectCapacityPopulated: false,
		},
		{
			name: "nil FlavorInfo does not populate capacity",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				Name:      "test-template",
				Namespace: "default",
				Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
					Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
						Spec: infrastructurev1beta2.CloudscaleMachineSpec{
							Flavor: "flex-8-4",
							Image:  "ubuntu-24.04",
						},
					},
				},
			},
			flavorInfo:              nil,
			expectCapacityPopulated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create fake client with the template
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.template).
				WithStatusSubresource(tt.template).
				Build()

			reconciler := &CloudscaleMachineTemplateReconciler{
				Client:     fakeClient,
				Scheme:     scheme,
				FlavorInfo: tt.flavorInfo,
			}

			// Reconcile
			req := ctrl.Request{
				Name:      tt.template.Name,
				Namespace: tt.template.Namespace,
			}
			_, err := reconciler.Reconcile(context.Background(), req)
			g.Expect(err).NotTo(HaveOccurred())

			// Get the updated template
			updatedTemplate := &infrastructurev1beta2.CloudscaleMachineTemplate{}
			err = fakeClient.Get(context.Background(), req.NamespacedName, updatedTemplate)
			g.Expect(err).NotTo(HaveOccurred())

			if !tt.expectCapacityPopulated {
				g.Expect(updatedTemplate.Status.Capacity).To(BeNil())
				return
			}

			// Check capacity is populated
			g.Expect(updatedTemplate.Status.Capacity).NotTo(BeNil())

			// Check CPU
			cpu := updatedTemplate.Status.Capacity[corev1.ResourceCPU]
			g.Expect(cpu.String()).To(Equal(tt.expectedCPU))

			// Check Memory
			mem := updatedTemplate.Status.Capacity[corev1.ResourceMemory]
			g.Expect(mem.String()).To(Equal(tt.expectedMemory))

			// Check GPU
			if tt.expectedGPU != "" {
				gpu := updatedTemplate.Status.Capacity[cloudscale.ResourceNvidiaGPU]
				g.Expect(gpu.String()).To(Equal(tt.expectedGPU))
			}
		})
	}
}

func TestCloudscaleMachineTemplateReconciler_Reconcile_NotFound(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrastructurev1beta2.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &CloudscaleMachineTemplateReconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		FlavorInfo: testutils.NewTestFlavorInfo(),
	}

	req := ctrl.Request{
		Name:      "nonexistent",
		Namespace: "default",
	}
	result, err := reconciler.Reconcile(context.Background(), req)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
}
