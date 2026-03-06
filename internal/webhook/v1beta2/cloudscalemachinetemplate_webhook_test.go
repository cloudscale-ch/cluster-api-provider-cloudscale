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
	"testing"

	. "github.com/onsi/gomega"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

func newMachineTemplateWebhookTestObjects() (
	obj *infrastructurev1beta2.CloudscaleMachineTemplate,
	oldObj *infrastructurev1beta2.CloudscaleMachineTemplate,
) {
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
	return
}

// ============================================================================
// Tests for CloudscaleMachineTemplate Defaulting Webhook
// ============================================================================

func TestMachineTemplateDefaulting_NoModification(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	defaulter := CloudscaleMachineTemplateCustomDefaulter{}
	original := obj.DeepCopy()

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec).To(Equal(original.Spec))
}

// ============================================================================
// Tests for CloudscaleMachineTemplate Validating Webhook - Create
// ============================================================================

func TestMachineTemplateValidateCreate_ValidTemplate(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineTemplateValidateCreate_ReservedTagPrefix(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}
	obj.Spec.Template.Spec.Tags = map[string]string{
		"capcs-cluster-test": "owned",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("capcs-"))
}

// ============================================================================
// Tests for CloudscaleMachineTemplate Validating Webhook - Update
// ============================================================================

func TestMachineTemplateValidateUpdate_NoChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineTemplateValidateUpdate_FlavorChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}
	obj.Spec.Template.Spec.Flavor = "flex-16-8"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

func TestMachineTemplateValidateUpdate_ImageChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}
	obj.Spec.Template.Spec.Image = "ubuntu-22.04"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

func TestMachineTemplateValidateUpdate_RootVolumeSizeChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}
	obj.Spec.Template.Spec.RootVolumeSize = 100

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

func TestMachineTemplateValidateUpdate_TagsChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}
	obj.Spec.Template.Spec.Tags = map[string]string{
		"env": "staging",
	}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

// ============================================================================
// Tests for CloudscaleMachineTemplate Validating Webhook - Delete
// ============================================================================

func TestMachineTemplateValidateDelete_AlwaysSucceeds(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{}

	_, err := validator.ValidateDelete(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}
