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
)

// CloudscaleMachineTemplateSpec defines the desired state of CloudscaleMachineTemplate.
type CloudscaleMachineTemplateSpec struct {
	// Template contains the machine template specification.
	Template CloudscaleMachineTemplateResource `json:"template"`
}

// CloudscaleMachineTemplateResource describes the data needed to create a CloudscaleMachine from a template.
type CloudscaleMachineTemplateResource struct {
	// Spec is the specification of the desired behavior of the machine.
	Spec CloudscaleMachineSpec `json:"spec"`
}

// CloudscaleMachineTemplateStatus defines the observed state of CloudscaleMachineTemplate.
type CloudscaleMachineTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the CloudscaleMachineTemplate resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CloudscaleMachineTemplate is the Schema for the cloudscalemachinetemplates API
type CloudscaleMachineTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CloudscaleMachineTemplate
	// +required
	Spec CloudscaleMachineTemplateSpec `json:"spec"`

	// status defines the observed state of CloudscaleMachineTemplate
	// +optional
	Status CloudscaleMachineTemplateStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CloudscaleMachineTemplateList contains a list of CloudscaleMachineTemplate
type CloudscaleMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CloudscaleMachineTemplate `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&CloudscaleMachineTemplate{},
		&CloudscaleMachineTemplateList{},
	)
}
