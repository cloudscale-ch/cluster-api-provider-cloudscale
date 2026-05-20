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

// CloudscaleClusterTemplateSpec defines the desired state of CloudscaleClusterTemplate.
type CloudscaleClusterTemplateSpec struct {
	// Template is the embedded resource the CAPI topology controller stamps out
	// into a CloudscaleCluster for each Cluster whose ClusterClass references
	// this CloudscaleClusterTemplate.
	Template CloudscaleClusterTemplateResource `json:"template"`
}

// CloudscaleClusterTemplateResource describes the CloudscaleCluster that the
// topology controller materializes from this template.
type CloudscaleClusterTemplateResource struct {
	// ObjectMeta supplies labels and annotations that propagate to the
	// generated CloudscaleCluster. The name/namespace fields are ignored:
	// the topology controller derives those from the owning Cluster.
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Spec embeds CloudscaleClusterSpec verbatim and shares the same defaulting
	// and validation logic via the cluster webhook helpers
	// (clusterSpecDefault / clusterSpecValidateCreate).
	// Immutable after creation; override per-cluster fields via
	// spec.topology.variables on the Cluster instead of mutating this spec.
	Spec CloudscaleClusterSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=cloudscaleclustertemplates,scope=Namespaced,categories=cluster-api

// CloudscaleClusterTemplate is a template embedded in a ClusterClass that the
// CAPI topology controller uses to materialize a CloudscaleCluster for every
// Cluster whose spec.topology.classRef resolves to a ClusterClass referencing
// this object. Unlike CloudscaleCluster, this CRD has no controller and no
// status — it is consumed only at Cluster creation time by CAPI core.
// Its spec is immutable after creation (enforced by the validating webhook);
// per-cluster overrides go through ClusterClass variables, not template edits.
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
