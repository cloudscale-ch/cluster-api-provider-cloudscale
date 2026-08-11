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
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cluster-api/util/topology"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// cloudscalemachinetemplatelog is for logging in this package.
var cloudscalemachinetemplatelog = logf.Log.WithName("cloudscalemachinetemplate-resource")

// SetupCloudscaleMachineTemplateWebhookWithManager registers the webhook for CloudscaleMachineTemplate in the manager.
func SetupCloudscaleMachineTemplateWebhookWithManager(mgr ctrl.Manager, flavorInfo *cloudscale.FlavorInfo) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1beta2.CloudscaleMachineTemplate{}).
		WithValidator(&CloudscaleMachineTemplateCustomValidator{
			FlavorInfo: flavorInfo,
		}).
		WithDefaulter(&CloudscaleMachineTemplateCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalemachinetemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachinetemplates,verbs=create;update,versions=v1beta2,name=mcloudscalemachinetemplate-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleMachineTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CloudscaleMachineTemplate when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CloudscaleMachineTemplateCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CloudscaleMachineTemplate.
func (d *CloudscaleMachineTemplateCustomDefaulter) Default(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachineTemplate) error {
	cloudscalemachinetemplatelog.Info("Defaulting for CloudscaleMachineTemplate", "name", obj.GetName())

	defaultInterfaceIPFamily(obj.Spec.Template.Spec.Interfaces)

	return nil
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalemachinetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachinetemplates,verbs=create;update,versions=v1beta2,name=vcloudscalemachinetemplate-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleMachineTemplateCustomValidator struct is responsible for validating the CloudscaleMachineTemplate resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CloudscaleMachineTemplateCustomValidator struct {
	FlavorInfo *cloudscale.FlavorInfo
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachineTemplate.
func (v *CloudscaleMachineTemplateCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachineTemplate) (admission.Warnings, error) {
	cloudscalemachinetemplatelog.Info("Validation for CloudscaleMachineTemplate upon creation", "name", obj.GetName())

	allErrs := validateMachineSpec(&obj.Spec.Template.Spec, v.FlavorInfo, field.NewPath("spec", "template", "spec"))
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleMachineTemplate"},
			obj.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachineTemplate.
func (v *CloudscaleMachineTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *infrastructurev1beta2.CloudscaleMachineTemplate) (admission.Warnings, error) {
	cloudscalemachinetemplatelog.Info("Validation for CloudscaleMachineTemplate upon update", "name", newObj.GetName())

	// The CAPI topology controller performs an SSA dry-run against the existing template to
	// detect whether a template rotation is required (e.g. after a ClusterClass variable change).
	// When it does so it sets the TopologyDryRunAnnotation. In that specific case we must skip the
	// immutability check so the dry-run succeeds and the controller can rotate the template.
	// This is a mandatory part of the CAPI InfraMachineTemplate contract for ClusterClass support.
	// See: https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-machine
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an admission.Request inside context: %v", err))
	}

	// MachineTemplate spec is fully immutable (CAPI convention) except during a topology dry-run.
	if !topology.IsDryRunRequest(req, newObj) &&
		!reflect.DeepEqual(newObj.Spec.Template.Spec, oldObj.Spec.Template.Spec) {
		allErrs := field.ErrorList{
			field.Forbidden(
				field.NewPath("spec", "template", "spec"),
				"field is immutable"),
		}
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleMachineTemplate"},
			newObj.Name, allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachineTemplate.
func (v *CloudscaleMachineTemplateCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachineTemplate) (admission.Warnings, error) {
	cloudscalemachinetemplatelog.Info("Validation for CloudscaleMachineTemplate upon deletion", "name", obj.GetName())
	return nil, nil
}
