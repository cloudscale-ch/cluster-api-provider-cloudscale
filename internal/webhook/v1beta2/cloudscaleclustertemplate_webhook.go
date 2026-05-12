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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// nolint:unused
// log is for logging in this package.
var cloudscaleclustertemplatelog = logf.Log.WithName("cloudscaleclustertemplate-resource")

// SetupCloudscaleClusterTemplateWebhookWithManager registers the webhook for CloudscaleClusterTemplate in the manager.
func SetupCloudscaleClusterTemplateWebhookWithManager(mgr ctrl.Manager, regionInfo *cloudscale.RegionInfo) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1beta2.CloudscaleClusterTemplate{}).
		WithValidator(&CloudscaleClusterTemplateCustomValidator{RegionInfo: regionInfo}).
		WithDefaulter(&CloudscaleClusterTemplateCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscaleclustertemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclustertemplates,verbs=create;update,versions=v1beta2,name=mcloudscaleclustertemplate-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleClusterTemplateCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CloudscaleClusterTemplate when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CloudscaleClusterTemplateCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CloudscaleClusterTemplate.
func (d *CloudscaleClusterTemplateCustomDefaulter) Default(_ context.Context, clusterTemplate *infrastructurev1beta2.CloudscaleClusterTemplate) error {
	cloudscaleclustertemplatelog.Info("Defaulting for CloudscaleClusterTemplate", "name", clusterTemplate.GetName())

	clusterSpecDefault(&clusterTemplate.Spec.Template.Spec)

	return nil
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscaleclustertemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclustertemplates,verbs=create;update,versions=v1beta2,name=vcloudscaleclustertemplate-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleClusterTemplateCustomValidator struct is responsible for validating the CloudscaleClusterTemplate resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CloudscaleClusterTemplateCustomValidator struct {
	RegionInfo *cloudscale.RegionInfo
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleClusterTemplate.
func (v *CloudscaleClusterTemplateCustomValidator) ValidateCreate(_ context.Context, clusterTemplate *infrastructurev1beta2.CloudscaleClusterTemplate) (admission.Warnings, error) {
	cloudscaleclustertemplatelog.Info("Validation for CloudscaleClusterTemplate upon creation", "name", clusterTemplate.GetName())

	allErrs := clusterSpecValidateCreate(clusterTemplate.Spec.Template.Spec, v.RegionInfo, field.NewPath("spec", "template", "spec"))

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleClusterTemplate"},
			clusterTemplate.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleClusterTemplate.
func (v *CloudscaleClusterTemplateCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *infrastructurev1beta2.CloudscaleClusterTemplate) (admission.Warnings, error) {
	cloudscaleclustertemplatelog.Info("Validation for CloudscaleClusterTemplate upon update", "name", newObj.GetName())

	if !reflect.DeepEqual(newObj.Spec, oldObj.Spec) {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleClusterTemplate"},
			newObj.Name,
			field.ErrorList{field.Forbidden(field.NewPath("spec"), "CloudscaleClusterTemplate.Spec is immutable")},
		)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleClusterTemplate.
func (v *CloudscaleClusterTemplateCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1beta2.CloudscaleClusterTemplate) (admission.Warnings, error) {
	cloudscaleclustertemplatelog.Info("Validation for CloudscaleClusterTemplate upon deletion", "name", obj.GetName())
	return nil, nil
}
