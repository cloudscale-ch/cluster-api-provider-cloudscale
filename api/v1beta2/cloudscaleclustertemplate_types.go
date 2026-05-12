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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// CloudscaleClusterTemplateSpec defines the desired state of CloudscaleClusterTemplate
type CloudscaleClusterTemplateSpec struct {
	Template CloudscaleClusterTemplateResource `json:"template"`
}

// CloudscaleClusterTemplateResource contains spec for CloudscaleClusterSpec.
type CloudscaleClusterTemplateResource struct {
	// +optional
	ObjectMeta clusterv1.ObjectMeta  `json:"metadata,omitempty"`
	Spec       CloudscaleClusterSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=cloudscaleclustertemplates,scope=Namespaced,categories=cluster-api

// CloudscaleClusterTemplate is the Schema for the cloudscaleclustertemplates API
type CloudscaleClusterTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CloudscaleClusterTemplate
	// +required
	Spec CloudscaleClusterTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// CloudscaleClusterTemplateList contains a list of CloudscaleClusterTemplate
type CloudscaleClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CloudscaleClusterTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&CloudscaleClusterTemplate{},
		&CloudscaleClusterTemplateList{},
	)
}
