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
	"k8s.io/utils/ptr"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

func newMachineWebhookTestObjects() (
	obj *infrastructurev1beta2.CloudscaleMachine,
	oldObj *infrastructurev1beta2.CloudscaleMachine,
) {
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
	return
}

// ============================================================================
// Tests for CloudscaleMachine Defaulting Webhook
// ============================================================================

func TestMachineDefaulting_NoModification(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineWebhookTestObjects()
	defaulter := CloudscaleMachineCustomDefaulter{}
	original := obj.DeepCopy()

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec).To(Equal(original.Spec))
}

// ============================================================================
// Tests for CloudscaleMachine Validating Webhook - Create
// ============================================================================

func TestMachineValidateCreate_ValidSpec(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineValidateCreate_ValidUserTags(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Tags = map[string]string{
		"env":  "production",
		"team": "platform",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineValidateCreate_ReservedTagPrefix(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Tags = map[string]string{
		"capcs-cluster-test": "owned",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("capcs-"))
}

// ============================================================================
// Tests for CloudscaleMachine Validating Webhook - Update
// ============================================================================

func TestMachineValidateUpdate_NoChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineValidateUpdate_FlavorChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Flavor = "flex-16-8"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.flavor"))
}

func TestMachineValidateUpdate_TagChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Tags = map[string]string{
		"env": "staging",
	}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineValidateUpdate_ImageChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Image = "ubuntu-22.04"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.image"))
}

func TestMachineValidateUpdate_RootVolumeSizeChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.RootVolumeSize = 100

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.rootVolumeSize"))
}

func TestMachineValidateUpdate_ProviderIDChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	oldObj.Spec.ProviderID = ptr.To("cloudscale://aaa")
	obj.Spec.ProviderID = ptr.To("cloudscale://bbb")

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.providerID"))
}

func TestMachineValidateUpdate_ProviderIDSetWhenNil(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	oldObj.Spec.ProviderID = nil
	obj.Spec.ProviderID = ptr.To("cloudscale://aaa")

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineValidateUpdate_ReservedPrefixTags(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Tags = map[string]string{
		"capcs-machine": "test",
	}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("capcs-"))
}

func TestMachineValidateUpdate_MultipleImmutableChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}
	obj.Spec.Image = "ubuntu-22.04"
	obj.Spec.RootVolumeSize = 100

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.image"))
	g.Expect(err.Error()).To(ContainSubstring("spec.rootVolumeSize"))
}

// ============================================================================
// Tests for CloudscaleMachine Validating Webhook - Delete
// ============================================================================

func TestMachineValidateDelete_AlwaysSucceeds(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineWebhookTestObjects()
	validator := CloudscaleMachineCustomValidator{}

	_, err := validator.ValidateDelete(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}
