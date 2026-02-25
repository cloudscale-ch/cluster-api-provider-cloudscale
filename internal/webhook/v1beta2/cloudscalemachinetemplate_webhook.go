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

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

// nolint:unused
// log is for logging in this package.
var cloudscalemachinetemplatelog = logf.Log.WithName("cloudscalemachinetemplate-resource")

// SetupCloudscaleMachineTemplateWebhookWithManager registers the webhook for CloudscaleMachineTemplate in the manager.
func SetupCloudscaleMachineTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1beta2.CloudscaleMachineTemplate{}).
		WithValidator(&CloudscaleMachineTemplateCustomValidator{}).
		WithDefaulter(&CloudscaleMachineTemplateCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalemachinetemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachinetemplates,verbs=create;update,versions=v1beta2,name=mcloudscalemachinetemplate-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleMachineTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CloudscaleMachineTemplate when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CloudscaleMachineTemplateCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CloudscaleMachineTemplate.
func (d *CloudscaleMachineTemplateCustomDefaulter) Default(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachineTemplate) error {
	cloudscalemachinetemplatelog.Info("Defaulting for CloudscaleMachineTemplate", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalemachinetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachinetemplates,verbs=create;update,versions=v1beta2,name=vcloudscalemachinetemplate-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleMachineTemplateCustomValidator struct is responsible for validating the CloudscaleMachineTemplate resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CloudscaleMachineTemplateCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachineTemplate.
func (v *CloudscaleMachineTemplateCustomValidator) ValidateCreate(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachineTemplate) (admission.Warnings, error) {
	cloudscalemachinetemplatelog.Info("Validation for CloudscaleMachineTemplate upon creation", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachineTemplate.
func (v *CloudscaleMachineTemplateCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *infrastructurev1beta2.CloudscaleMachineTemplate) (admission.Warnings, error) {
	cloudscalemachinetemplatelog.Info("Validation for CloudscaleMachineTemplate upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleMachineTemplate.
func (v *CloudscaleMachineTemplateCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1beta2.CloudscaleMachineTemplate) (admission.Warnings, error) {
	cloudscalemachinetemplatelog.Info("Validation for CloudscaleMachineTemplate upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
