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

package v1beta2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

var _ = Describe("CloudscaleMachineTemplate Webhook", func() {
	var (
		obj       *infrastructurev1beta2.CloudscaleMachineTemplate
		oldObj    *infrastructurev1beta2.CloudscaleMachineTemplate
		validator CloudscaleMachineTemplateCustomValidator
		defaulter CloudscaleMachineTemplateCustomDefaulter
	)

	BeforeEach(func() {
		obj = &infrastructurev1beta2.CloudscaleMachineTemplate{
			Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
				Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
					Spec: infrastructurev1beta2.CloudscaleMachineSpec{
						Flavor:         "flex-8-4",
						Image:          "ubuntu-24.04",
						RootVolumeSize: 50,
					},
				},
			},
		}
		oldObj = &infrastructurev1beta2.CloudscaleMachineTemplate{
			Spec: infrastructurev1beta2.CloudscaleMachineTemplateSpec{
				Template: infrastructurev1beta2.CloudscaleMachineTemplateResource{
					Spec: infrastructurev1beta2.CloudscaleMachineSpec{
						Flavor:         "flex-8-4",
						Image:          "ubuntu-24.04",
						RootVolumeSize: 50,
					},
				},
			},
		}
		validator = CloudscaleMachineTemplateCustomValidator{}
		defaulter = CloudscaleMachineTemplateCustomDefaulter{}
	})

	Context("When creating CloudscaleMachineTemplate under Defaulting Webhook", func() {
		It("Should not modify the spec", func() {
			original := obj.DeepCopy()
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec).To(Equal(original.Spec))
		})
	})

	Context("When creating CloudscaleMachineTemplate under Validating Webhook", func() {
		It("Should accept a valid template", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject tags with capcs- prefix", func() {
			obj.Spec.Template.Spec.Tags = map[string]string{
				"capcs-cluster-test": "owned",
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("capcs-"))
		})
	})

	Context("When updating CloudscaleMachineTemplate under Validating Webhook", func() {
		It("Should accept no changes", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject flavor change", func() {
			obj.Spec.Template.Spec.Flavor = "flex-16-8"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
		})

		It("Should reject image change", func() {
			obj.Spec.Template.Spec.Image = "ubuntu-22.04"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
		})

		It("Should reject rootVolumeSize change", func() {
			obj.Spec.Template.Spec.RootVolumeSize = 100

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
		})

		It("Should reject tags change", func() {
			obj.Spec.Template.Spec.Tags = map[string]string{
				"env": "staging",
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
		})
	})

	Context("When deleting CloudscaleMachineTemplate under Validating Webhook", func() {
		It("Should always succeed", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
