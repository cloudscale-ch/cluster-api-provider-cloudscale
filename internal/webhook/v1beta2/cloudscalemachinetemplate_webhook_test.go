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
	"context"
	"testing"

	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

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

// ctxWithDryRun returns a context carrying an admission.Request with the given dry-run flag.
func ctxWithDryRun(dryRun bool) context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(dryRun)},
	})
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

	_, err := validator.ValidateUpdate(ctxWithDryRun(false), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

func TestMachineTemplateValidateUpdate_ImageChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}
	obj.Spec.Template.Spec.Image = "ubuntu-22.04"

	_, err := validator.ValidateUpdate(ctxWithDryRun(false), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

// A spec change during a topology dry-run (annotation set + DryRun=true) must be allowed,
// so the topology controller can detect the diff and rotate the template.
func TestMachineTemplateValidateUpdate_DryRunSkipsImmutability(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}

	obj.Spec.Template.Spec.Flavor = "flex-16-8"
	obj.SetAnnotations(map[string]string{clusterv1.TopologyDryRunAnnotation: ""})

	_, err := validator.ValidateUpdate(ctxWithDryRun(true), oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// Dry-run without the topology annotation must be rejected (not a topology rotation).
func TestMachineTemplateValidateUpdate_DryRunWithoutAnnotationStillImmutable(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}

	obj.Spec.Template.Spec.Flavor = "flex-16-8"

	_, err := validator.ValidateUpdate(ctxWithDryRun(true), oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.template.spec"))
}

// Non-dry-run with the annotation must be rejected since it would be a real write.
func TestMachineTemplateValidateUpdate_AnnotationSetButNotDryRunStillImmutable(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj := newMachineTemplateWebhookTestObjects()
	validator := CloudscaleMachineTemplateCustomValidator{FlavorInfo: testutils.NewTestFlavorInfo()}

	obj.Spec.Template.Spec.Flavor = "flex-16-8"
	obj.SetAnnotations(map[string]string{clusterv1.TopologyDryRunAnnotation: ""})

	_, err := validator.ValidateUpdate(ctxWithDryRun(false), oldObj, obj)
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
