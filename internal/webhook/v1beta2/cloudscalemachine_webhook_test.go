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
	"k8s.io/utils/ptr"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

var _ = Describe("CloudscaleMachine Webhook", func() {
	var (
		obj       *infrastructurev1beta2.CloudscaleMachine
		oldObj    *infrastructurev1beta2.CloudscaleMachine
		validator CloudscaleMachineCustomValidator
		defaulter CloudscaleMachineCustomDefaulter
	)

	BeforeEach(func() {
		obj = &infrastructurev1beta2.CloudscaleMachine{
			Spec: infrastructurev1beta2.CloudscaleMachineSpec{
				Flavor:         "flex-8-4",
				Image:          "ubuntu-24.04",
				RootVolumeSize: 50,
			},
		}
		oldObj = &infrastructurev1beta2.CloudscaleMachine{
			Spec: infrastructurev1beta2.CloudscaleMachineSpec{
				Flavor:         "flex-8-4",
				Image:          "ubuntu-24.04",
				RootVolumeSize: 50,
			},
		}
		validator = CloudscaleMachineCustomValidator{}
		defaulter = CloudscaleMachineCustomDefaulter{}
	})

	Context("When creating CloudscaleMachine under Defaulting Webhook", func() {
		It("Should not modify the spec", func() {
			original := obj.DeepCopy()
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec).To(Equal(original.Spec))
		})
	})

	Context("When creating CloudscaleMachine under Validating Webhook", func() {
		It("Should accept a valid spec", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should accept valid user tags", func() {
			obj.Spec.Tags = map[string]string{
				"env":  "production",
				"team": "platform",
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject tags with capcs- prefix", func() {
			obj.Spec.Tags = map[string]string{
				"capcs-cluster-test": "owned",
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("capcs-"))
		})
	})

	Context("When updating CloudscaleMachine under Validating Webhook", func() {
		It("Should accept no changes", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject flavor change", func() {
			obj.Spec.Flavor = "flex-16-8"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.flavor"))
		})

		It("Should allow tag changes", func() {
			obj.Spec.Tags = map[string]string{
				"env": "staging",
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject image change", func() {
			obj.Spec.Image = "ubuntu-22.04"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.image"))
		})

		It("Should reject rootVolumeSize change", func() {
			obj.Spec.RootVolumeSize = 100

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rootVolumeSize"))
		})

		It("Should reject providerID change once set", func() {
			oldObj.Spec.ProviderID = ptr.To("cloudscale://aaa")
			obj.Spec.ProviderID = ptr.To("cloudscale://bbb")

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.providerID"))
		})

		It("Should allow providerID to be set when nil", func() {
			oldObj.Spec.ProviderID = nil
			obj.Spec.ProviderID = ptr.To("cloudscale://aaa")

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject reserved prefix tags on update", func() {
			obj.Spec.Tags = map[string]string{
				"capcs-machine": "test",
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("capcs-"))
		})

		It("Should report multiple immutable field errors", func() {
			obj.Spec.Image = "ubuntu-22.04"
			obj.Spec.RootVolumeSize = 100

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.image"))
			Expect(err.Error()).To(ContainSubstring("spec.rootVolumeSize"))
		})
	})

	Context("When deleting CloudscaleMachine under Validating Webhook", func() {
		It("Should always succeed", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
