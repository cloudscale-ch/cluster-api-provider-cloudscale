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
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

// validateMachineSpec validates a CloudscaleMachineSpec at creation time.
func validateMachineSpec(spec *infrastructurev1beta2.CloudscaleMachineSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateTags(spec.Tags, fldPath.Child("tags"))...)
	return allErrs
}

// validateMachineSpecUpdate validates immutability rules and tags when updating a CloudscaleMachineSpec.
func validateMachineSpecUpdate(newSpec, oldSpec *infrastructurev1beta2.CloudscaleMachineSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Image is immutable
	if newSpec.Image != oldSpec.Image {
		allErrs = append(allErrs, field.Forbidden(
			fldPath.Child("image"),
			"field is immutable"))
	}

	// Flavor is immutable. Cloudscale API permits updating the flavor, but only if the
	// server is in stopped state. In the case of the CAPCS operator, it doesn't make sense
	// to support updating the flavor, therefore, but instead requires creating a new machine.
	if newSpec.Flavor != oldSpec.Flavor {
		allErrs = append(allErrs, field.Forbidden(
			fldPath.Child("flavor"),
			"field is immutable"))
	}

	// RootVolumeSize is immutable
	if newSpec.RootVolumeSize != oldSpec.RootVolumeSize {
		allErrs = append(allErrs, field.Forbidden(
			fldPath.Child("rootVolumeSize"),
			"field is immutable"))
	}

	// ProviderID is immutable once set
	if oldSpec.ProviderID != nil {
		if newSpec.ProviderID == nil || *newSpec.ProviderID != *oldSpec.ProviderID {
			allErrs = append(allErrs, field.Forbidden(
				fldPath.Child("providerID"),
				"field is immutable once set"))
		}
	}

	// Tags are mutable but still validated for reserved prefix
	allErrs = append(allErrs, validateTags(newSpec.Tags, fldPath.Child("tags"))...)

	return allErrs
}

// validateTags rejects tags with the reserved capcs- prefix.
func validateTags(tags map[string]string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for key := range tags {
		if strings.HasPrefix(key, infrastructurev1beta2.NameCloudscaleProviderPrefix) {
			allErrs = append(allErrs, field.Forbidden(
				fldPath.Key(key),
				"tag key prefix "+infrastructurev1beta2.NameCloudscaleProviderPrefix+" is reserved for internal use"))
		}
	}
	return allErrs
}
