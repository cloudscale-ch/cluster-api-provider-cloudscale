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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

// cloudscalemachinelog is for logging in this package.
var cloudscalemachinelog = logf.Log.WithName("cloudscalemachine-resource")

// SetupCloudscaleMachineWebhookWithManager registers the webhook for CloudscaleMachine in the manager.
func SetupCloudscaleMachineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1beta2.CloudscaleMachine{}).
		WithValidator(&CloudscaleMachineCustomValidator{}).
		WithDefaulter(&CloudscaleMachineCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalemachine,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachines,verbs=create;update,versions=v1beta2,name=mcloudscalemachine-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleMachineCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CloudscaleMachine when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CloudscaleMachineCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CloudscaleMachine.
func (d *CloudscaleMachineCustomDefaulter) Default(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachine) error {
	cloudscalemachinelog.Info("Defaulting for CloudscaleMachine", "name", obj.GetName())
	return nil
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalemachine,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachines,verbs=create;update,versions=v1beta2,name=vcloudscalemachine-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleMachineCustomValidator struct is responsible for validating the CloudscaleMachine resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CloudscaleMachineCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachine.
func (v *CloudscaleMachineCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachine) (admission.Warnings, error) {
	cloudscalemachinelog.Info("Validation for CloudscaleMachine upon creation", "name", obj.GetName())

	allErrs := validateMachineSpec(&obj.Spec, field.NewPath("spec"))
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.GroupVersion.Group, Kind: "CloudscaleMachine"},
			obj.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachine.
func (v *CloudscaleMachineCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *infrastructurev1beta2.CloudscaleMachine) (admission.Warnings, error) {
	cloudscalemachinelog.Info("Validation for CloudscaleMachine upon update", "name", newObj.GetName())

	allErrs := validateMachineSpecUpdate(&newObj.Spec, &oldObj.Spec, field.NewPath("spec"))
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.GroupVersion.Group, Kind: "CloudscaleMachine"},
			newObj.Name, allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachine.
func (v *CloudscaleMachineCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachine) (admission.Warnings, error) {
	cloudscalemachinelog.Info("Validation for CloudscaleMachine upon deletion", "name", obj.GetName())
	return nil, nil
}
