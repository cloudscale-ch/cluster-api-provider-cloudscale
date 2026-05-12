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
	"sigs.k8s.io/cluster-api/util/conditions"
)

const (
	// MachineFinalizer allows cleanup of resources before removal from the API.
	MachineFinalizer = "cloudscalemachine.infrastructure.cluster.x-k8s.io"
)

// CloudscaleMachineSpec defines the desired state of CloudscaleMachine
type CloudscaleMachineSpec struct {
	// ProviderID is the unique identifier as specified by the cloud provider.
	// Format: cloudscale://<server-uuid>
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Flavor is the cloudscale.ch server flavor slug, e.g. "flex-4-2" or
	// "plus-8-4". List available flavors via the cloudscale API
	// (`GET /v1/flavors`) or the control panel.
	// Immutable after machine creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Flavor string `json:"flavor"`

	// Image identifies the OS image used to boot the server. One of:
	// - a public image slug (e.g. "ubuntu-24.04"),
	// - a custom image slug (e.g. "custom:ubuntu-2404-kube-v1.36.0"), or
	// - a custom image UUID.
	// For Kubernetes nodes you typically want a custom image built with
	// image-builder (https://image-builder.sigs.k8s.io/) that already contains
	// kubelet, containerd, and the chosen Kubernetes version.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// RootVolumeSize is the root volume size in GB. Minimum 10. If unset, the
	// cloudscale.ch default for the chosen flavor is used.
	// +kubebuilder:validation:Minimum=10
	// +optional
	RootVolumeSize int `json:"rootVolumeSize,omitempty"`

	// Tags are user-defined key/value pairs applied to the server as cloudscale
	// tags. CAPCS additionally sets its own ownership tag with the key
	// "capcs-cluster-<cluster-name>"; do not set keys with the "capcs-" prefix.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// ServerGroup configures anti-affinity placement.
	// When specified, machines in the same server group are placed on different physical hosts.
	// N.B.: Only **up to 4 machines** can be placed in the same server group.
	// +optional
	ServerGroup *ServerGroupSpec `json:"serverGroup,omitempty"`

	// Interfaces define the network interfaces to attach to the server.
	// When omitted, the controller defaults to the first cluster network and a public interface
	// at runtime (cross-resource resolution that the webhook cannot do).
	// If the cluster uses a floating IP without a load balancer, the control-plane
	// machine template must explicitly include a public interface ({type: public}).
	// cloudscale.ch requires a public IPv4 address on the server to assign a floating IP.
	// +listType=atomic
	// +optional
	Interfaces []InterfaceSpec `json:"interfaces,omitempty"`
}

// InterfaceSpec defines a network interface to attach to a server.
// Exactly one of Type or Network must be specified.
type InterfaceSpec struct {
	// Type is "public" for a public internet interface.
	// Mutually exclusive with Network.
	// +kubebuilder:validation:Enum=public
	// +optional
	Type string `json:"type,omitempty"`

	// Network references a named network from CloudscaleCluster.spec.networks.
	// Mutually exclusive with Type.
	// +optional
	Network string `json:"network,omitempty"`

	// IPFamily controls IPv4/IPv6 for a public interface.
	// Only valid when Type is "public".
	// Maps to the cloudscale API's per-server use_ipv6 setting:
	//   - IPv4: use_ipv6=false (IPv4 only)
	//   - DualStack: use_ipv6=true (IPv4 + IPv6)
	// +kubebuilder:validation:Enum=IPv4;DualStack
	// +optional
	IPFamily *IPFamily `json:"ipFamily,omitempty"`
}

// ServerGroupSpec configures server group placement for anti-affinity.
// cloudscale.ch limits a single server group to 4 servers; to scale a pool
// beyond that, split it across multiple MachineDeployments each pointing at a
// CloudscaleMachineTemplate with a distinct ServerGroupSpec.Name.
type ServerGroupSpec struct {
	// Name is the server group name. Machines with the same server group name
	// in the same zone are placed on different physical hosts. The group is
	// created automatically the first time CAPCS sees the name.
	// Immutable after machine creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// MachineInitializationStatus contains v1beta2 initialization tracking for CloudscaleMachine.
type MachineInitializationStatus struct {
	// Provisioned indicates that the machine infrastructure has been provisioned.
	// True when the server is running and ready.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// CloudscaleMachineStatus defines the observed state of CloudscaleMachine.
type CloudscaleMachineStatus struct {
	// Initialization contains v1beta2 initialization tracking.
	// +optional
	Initialization *MachineInitializationStatus `json:"initialization,omitempty"`

	// ServerID is the cloudscale.ch server UUID.
	// +optional
	ServerID string `json:"serverID,omitempty"`

	// Addresses contains the machine's addresses.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// ServerGroupID is the cloudscale.ch server group UUID.
	// +optional
	ServerGroupID string `json:"serverGroupID,omitempty"`

	// conditions represent the current state of the CloudscaleMachine resource.
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
// +kubebuilder:resource:path=cloudscalemachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster"
// +kubebuilder:printcolumn:name="Provisioned",type="string",JSONPath=".status.initialization.provisioned",description="Machine provisioned"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="cloudscale.ch server ID"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object"

// CloudscaleMachine represents a single cloudscale.ch server backing a CAPI
// Machine. Most spec fields are immutable after creation — to change them,
// roll the owning MachineDeployment or KubeadmControlPlane.
type CloudscaleMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CloudscaleMachine
	// +required
	Spec CloudscaleMachineSpec `json:"spec"`

	// status defines the observed state of CloudscaleMachine
	// +optional
	Status CloudscaleMachineStatus `json:"status,omitzero"`
}

// ensures CloudscaleMachine implements conditions.Setter
var _ conditions.Setter = &CloudscaleMachine{}

// GetConditions returns the conditions for the CloudscaleMachine.
// This implements the conditions.Getter interface from CAPI util/conditions.
func (m *CloudscaleMachine) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets the conditions for the CloudscaleMachine.
// This implements the conditions.Setter interface from CAPI util/conditions.
func (m *CloudscaleMachine) SetConditions(cond []metav1.Condition) {
	m.Status.Conditions = cond
}

// MachineTagKey generates the tag key for machines associated with a cluster.
func (m *CloudscaleMachine) MachineTagKey(clusterName string) string {
	return NameCloudscaleProviderOwned + clusterName
}

// +kubebuilder:object:root=true

// CloudscaleMachineList contains a list of CloudscaleMachine
type CloudscaleMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CloudscaleMachine `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&CloudscaleMachine{},
		&CloudscaleMachineList{},
	)
}
