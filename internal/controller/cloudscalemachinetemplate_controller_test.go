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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

func newTestFlavorInfo() *cloudscale.FlavorInfo {
	return cloudscale.NewFlavorInfo([]cloudscalesdk.Flavor{
		{Slug: "flex-4-2", Name: "Flex-4-2", VCPUCount: 2, MemoryGB: 4},
		{Slug: "flex-8-4", Name: "Flex-8-4", VCPUCount: 4, MemoryGB: 8},
		{Slug: "plus-16-8", Name: "Plus-16-8", VCPUCount: 8, MemoryGB: 16},
		{Slug: "gpu2-640-80-4-400", Name: "GPU2-640-80-4-400", VCPUCount: 80, MemoryGB: 640, GPU: &cloudscalesdk.FlavorGPU{
			Name:         "RTX PRO 6000 Max-Q",
			Count:        4,
			VRAMPerGPUGB: 96,
		}},
	})
}

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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
					Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
						Spec: infrastructurev1beta2.CloudscaleMachineSpec{
							Flavor: "flex-8-4",
							Image:  "ubuntu-24.04",
						},
					},
				},
			},
			flavorInfo:              newTestFlavorInfo(),
			expectedCPU:             "4",
			expectedMemory:          "8Gi",
			expectedGPU:             "",
			expectCapacityPopulated: true,
		},
		{
			name: "populates GPU for gpu flavor",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
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
			flavorInfo:              newTestFlavorInfo(),
			expectedCPU:             "80",
			expectedMemory:          "640Gi",
			expectedGPU:             "4",
			expectCapacityPopulated: true,
		},
		{
			name: "unknown flavor does not populate capacity",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
					Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
						Spec: infrastructurev1beta2.CloudscaleMachineSpec{
							Flavor: "unknown-flavor",
							Image:  "ubuntu-24.04",
						},
					},
				},
			},
			flavorInfo:              newTestFlavorInfo(),
			expectCapacityPopulated: false,
		},
		{
			name: "nil FlavorInfo does not populate capacity",
			template: &infrastructurev1beta2.CloudscaleMachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
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
				NamespacedName: types.NamespacedName{
					Name:      tt.template.Name,
					Namespace: tt.template.Namespace,
				},
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
		FlavorInfo: newTestFlavorInfo(),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	}
	result, err := reconciler.Reconcile(context.Background(), req)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
}
