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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
)

const (
	// ClusterFinalizer allows cleanup of resources before removal from the API.
	ClusterFinalizer = "cloudscalecluster.infrastructure.cluster.x-k8s.io"
)

// CloudscaleClusterSpec defines the desired state of CloudscaleCluster
type CloudscaleClusterSpec struct {
	// Region is the cloudscale.ch region (e.g., "rma", "lpg").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=rma;lpg
	Region string `json:"region"`

	// CredentialsRef references the Secret containing the cloudscale.ch API token.
	// +kubebuilder:validation:Required
	CredentialsRef CloudscaleCredentialsReference `json:"credentialsRef"`

	// ControlPlaneEndpoint represents the endpoint to communicate with the control plane.
	// This is set automatically from the load balancer's VIP address.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitzero"`

	// Network contains network configuration for the cluster.
	// +optional
	Network NetworkSpec `json:"network,omitzero"`

	// ControlPlaneLoadBalancer configures the load balancer for the control plane.
	// +optional
	ControlPlaneLoadBalancer LoadBalancerSpec `json:"controlPlaneLoadBalancer,omitzero"`
}

// CloudscaleCredentialsReference references a Secret containing the API token.
type CloudscaleCredentialsReference struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Secret. Defaults to the cluster namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NetworkSpec defines the network configuration.
type NetworkSpec struct {
	// Zone is the cloudscale.ch zone for the network (e.g., "rma1", "lpg1").
	// Defaults to region + "1" if not specified.
	// +optional
	Zone string `json:"zone,omitempty"`

	// CIDR is the CIDR block for the private network subnet.
	// +kubebuilder:default="10.0.0.0/24"
	// +optional
	CIDR string `json:"cidr,omitempty"`

	// GatewayAddress is the gateway IP address for the subnet.
	// By default, no gateway is configured on the private network subnet. This ensures
	// that outbound internet traffic uses the public network interface, which is required
	// for the Cloud Controller Manager to reach the cloudscale.ch API.
	// Set this to a specific IP address (e.g., "10.0.0.1") only if you have configured
	// a NAT gateway or similar infrastructure on the private network.
	// +optional
	GatewayAddress *string `json:"gatewayAddress,omitempty"`
}

// LoadBalancerSpec defines the load balancer configuration for the control plane.
type LoadBalancerSpec struct {
	// Enabled controls whether a load balancer is created for the control plane.
	// Set to false for external control planes (e.g., hosted control plane) where the endpoint
	// is provided externally.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Algorithm is the load balancing algorithm.
	// +kubebuilder:validation:Enum=round_robin;least_connections;source_ip
	// +kubebuilder:default="round_robin"
	// +optional
	Algorithm string `json:"algorithm,omitempty"`

	// Flavor is the load balancer flavor (size).
	// +kubebuilder:default="lb-standard"
	// +optional
	Flavor string `json:"flavor,omitempty"`

	// APIServerPort is the port for the Kubernetes API server.
	// +kubebuilder:default=6443
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	APIServerPort int32 `json:"apiServerPort,omitempty"`

	// HealthMonitor configures the load balancer health monitor.
	// +optional
	HealthMonitor HealthMonitorSpec `json:"healthMonitor,omitempty"`
}

// HealthMonitorSpec configures the load balancer health monitor.
type HealthMonitorSpec struct {
	// DelayS is the interval between health checks in seconds.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +optional
	DelayS int `json:"delayS,omitempty"`

	// TimeoutS is the health check timeout in seconds.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +optional
	TimeoutS int `json:"timeoutS,omitempty"`

	// UpThreshold is the number of successful checks to mark healthy.
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	UpThreshold int `json:"upThreshold,omitempty"`

	// DownThreshold is the number of failed checks to mark unhealthy.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	DownThreshold int `json:"downThreshold,omitempty"`
}

// CloudscaleClusterStatus defines the observed state of CloudscaleCluster.
type CloudscaleClusterStatus struct {
	// Initialization contains v1beta2 initialization tracking.
	// +optional
	Initialization *ClusterInitializationStatus `json:"initialization,omitempty"`

	// NetworkID is the cloudscale.ch network UUID.
	// +optional
	NetworkID string `json:"networkID,omitempty"`

	// SubnetID is the cloudscale.ch subnet UUID.
	// +optional
	SubnetID string `json:"subnetID,omitempty"`

	// LoadBalancerID is the cloudscale.ch load balancer UUID.
	// +optional
	LoadBalancerID string `json:"loadBalancerID,omitempty"`

	// LoadBalancerPoolID is the cloudscale.ch load balancer pool UUID for the API server.
	// +optional
	LoadBalancerPoolID string `json:"loadBalancerPoolID,omitempty"`

	// LoadBalancerListenerID is the cloudscale.ch load balancer listener UUID for the API server.
	// +optional
	LoadBalancerListenerID string `json:"loadBalancerListenerID,omitempty"`

	// LoadBalancerHealthMonitorID is the cloudscale.ch load balancer health monitor UUID.
	// +optional
	LoadBalancerHealthMonitorID string `json:"loadBalancerHealthMonitorID,omitempty"`

	// LoadBalancerMemberIDs are the list of nodes attached to the loadBalancer.
	// +optional
	LoadBalancerMemberIDs []string `json:"loadBalancerMemberIDs,omitempty"`

	// conditions represent the current state of the CloudscaleCluster resource.
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

// ClusterInitializationStatus contains v1beta2 initialization tracking for CloudscaleCluster.
type ClusterInitializationStatus struct {
	// Provisioned indicates that all cluster infrastructure has been provisioned.
	// True when Network, Subnet, Load Balancer, and Control Plane Endpoint are ready.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=cloudscaleclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster"
// +kubebuilder:printcolumn:name="Provisioned",type="string",JSONPath=".status.initialization.provisioned",description="Infrastructure provisioned"
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.region",description="cloudscale.ch region"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="Control plane endpoint"

// CloudscaleCluster is the Schema for the cloudscaleclusters API
type CloudscaleCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CloudscaleCluster
	// +required
	Spec CloudscaleClusterSpec `json:"spec"`

	// status defines the observed state of CloudscaleCluster
	// +optional
	Status CloudscaleClusterStatus `json:"status,omitzero"`
}

// ensures CloudscaleCluster implements conditions.Setter
var _ conditions.Setter = &CloudscaleCluster{}

// GetConditions returns the conditions for the CloudscaleCluster.
// This implements the conditions.Getter interface from CAPI util/conditions.
func (c *CloudscaleCluster) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets the conditions for the CloudscaleCluster.
// This implements the conditions.Setter interface from CAPI util/conditions.
func (c *CloudscaleCluster) SetConditions(conds []metav1.Condition) {
	c.Status.Conditions = conds
}

// CredentialsSecretRef returns an ObjectReference to the credentials Secret.
func (c *CloudscaleCluster) CredentialsSecretRef() corev1.ObjectReference {
	ns := c.Spec.CredentialsRef.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	return corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       c.Spec.CredentialsRef.Name,
		Namespace:  ns,
	}
}

// ClusterTagKey generates the key for resources associated with a cluster.
func (c *CloudscaleCluster) ClusterTagKey() string {
	return NameCloudscaleProviderOwned + c.Name
}

// +kubebuilder:object:root=true

// CloudscaleClusterList contains a list of CloudscaleCluster
type CloudscaleClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CloudscaleCluster `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&CloudscaleCluster{},
		&CloudscaleClusterList{},
	)
}
