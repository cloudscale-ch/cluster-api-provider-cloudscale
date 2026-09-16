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
	"sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
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
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineTemplateValidateCreate_InvalidFlavor(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
	obj.Spec.Template.Spec.Flavor = "nonexistent-flavor"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec.flavor"))
	g.Expect(err.Error()).To(ContainSubstring("unknown flavor"))
}

func TestMachineTemplateValidateCreate_ReservedTagPrefix(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
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

func TestMachineTemplateValidateUpdate_FlavorChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
	obj.Spec.Template.Spec.Flavor = "flex-16-8"

	_, err := validator.ValidateUpdate(newAdmissionContext(false), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

func TestMachineTemplateValidateUpdate_ImageChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
	obj.Spec.Template.Spec.Image = "ubuntu-22.04"

	_, err := validator.ValidateUpdate(newAdmissionContext(false), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

// ============================================================================
// Tests for CloudscaleMachineTemplate Validating Webhook - Delete
// ============================================================================

func TestMachineTemplateValidateDelete_AlwaysSucceeds(t *testing.T) {
	g := NewWithT(t)
	obj, _ := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}

	_, err := validator.ValidateDelete(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// ============================================================================
// Tests for ClusterClass dry-run support
// ============================================================================

func TestMachineTemplateValidateUpdate_DryRunWithChangedSpec(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}

	// Simulate a topology-driven template rotation: the spec changed.
	obj.Spec.Template.Spec.Flavor = "flex-16-8"

	// Annotate the new object with the topology dry-run marker.
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[v1beta2.TopologyDryRunAnnotation] = ""

	_, err := validator.ValidateUpdate(newAdmissionContext(true), oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestMachineTemplateValidateUpdate_DryRunWithoutAnnotationStillDenied(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
	obj.Spec.Template.Spec.Flavor = "flex-16-8"

	// No TopologyDryRunAnnotation on the object.
	_, err := validator.ValidateUpdate(newAdmissionContext(true), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

func TestMachineTemplateValidateUpdate_AnnotationWithoutDryRunStillDenied(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
	obj.Spec.Template.Spec.Flavor = "flex-16-8"

	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[v1beta2.TopologyDryRunAnnotation] = ""

	// DryRun is false.
	_, err := validator.ValidateUpdate(newAdmissionContext(false), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}
