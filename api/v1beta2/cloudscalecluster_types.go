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

// IPFamily represents an IP family configuration.
// Valid values depend on the field — each consumer declares its own
// +kubebuilder:validation:Enum subset.
type IPFamily string

const (
	IPFamilyIPv4      IPFamily = "IPv4"
	IPFamilyIPv6      IPFamily = "IPv6"
	IPFamilyDualStack IPFamily = "DualStack"
)

// CloudscaleClusterSpec defines the desired state of CloudscaleCluster
type CloudscaleClusterSpec struct {
	// Region is the cloudscale.ch region the cluster is provisioned in.
	// Determines the default zone and the set of available flavors.
	// Immutable after cluster creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=rma;lpg
	Region string `json:"region"`

	// Zone is the cloudscale.ch zone within Region.
	// Defaults to Region + "1" (e.g., "rma1", "lpg1"). Set explicitly only when
	// the region offers multiple zones and you need to pin the cluster to one.
	// Immutable after cluster creation.
	// +optional
	Zone string `json:"zone,omitempty"`

	// CredentialsRef references the Secret containing the cloudscale.ch API token.
	// +kubebuilder:validation:Required
	CredentialsRef CloudscaleCredentialsReference `json:"credentialsRef"`

	// ControlPlaneEndpoint represents the endpoint to communicate with the control plane.
	// This is set automatically from the load balancer's VIP address or floating IP.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitzero"`

	// Networks define the private networks for this cluster.
	// Referenced by name from machine interface specs and LB config.
	// If empty, defaults to a single managed network named after the cluster.
	// +listType=map
	// +listMapKey=name
	// +optional
	Networks []NetworkSpec `json:"networks,omitempty"`

	// Routers define cloudscale.ch routers managed or adopted by CAPCS.
	// Each router can be attached to one or more networks via its interfaces.
	// Routers are provisioned after networks and deleted before networks.
	// +listType=map
	// +listMapKey=name
	// +optional
	Routers []RouterSpec `json:"routers,omitempty"`

	// ControlPlaneLoadBalancer configures the load balancer for the control plane.
	// +optional
	ControlPlaneLoadBalancer LoadBalancerSpec `json:"controlPlaneLoadBalancer,omitzero"`

	// FloatingIP configures a floating IP for a stable control plane endpoint.
	// When the load balancer is enabled (recommended), the floating IP is assigned
	// to the LB, providing a stable IP that survives LB recreation.
	// When using a pre-existing floating IP without a load balancer, the user must
	// configure a dummy interface on the control plane servers (see cloudscale.ch docs)
	// and ensure the control-plane machine template includes a public interface
	// ({type: public}), as cloudscale.ch requires a public IPv4 address to assign
	// a floating IP to a server.
	// Managed floating IPs require the load balancer to be enabled.
	// Floating IPs cannot be attached to a load balancer with a private VIP
	// (i.e. one whose ControlPlaneLoadBalancer.Network is set).
	// +optional
	FloatingIP *FloatingIPSpec `json:"floatingIP,omitempty"`
}

// CloudscaleCredentialsReference references a Secret holding the cloudscale.ch
// API token used to provision this cluster's infrastructure. The Secret must
// contain a key named "token" with the raw token string as its value.
type CloudscaleCredentialsReference struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Secret. Defaults to the
	// CloudscaleCluster's own namespace if unset.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NetworkSpec defines a private network for the cluster.
// Exactly one of UUID or CIDR must be specified.
type NetworkSpec struct {
	// Name identifies this network within the cluster.
	// Used to reference this network from machine interface specs and LB config.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// UUID references a pre-existing cloudscale.ch network.
	// The network is not deleted on cluster teardown.
	// Mutually exclusive with CIDR.
	// +optional
	UUID string `json:"uuid,omitempty"`

	// CIDR defines the subnet for a controller-managed network.
	// The network and subnet are created and deleted by CAPCS.
	// Mutually exclusive with UUID.
	// +optional
	CIDR string `json:"cidr,omitempty"`

	// GatewayAddress controls the gateway for this network's subnet.
	// When no router interface references this network: the value is set as the
	// subnet's static gateway at creation time (empty means no gateway).
	// When a router interface references this network: the value is requested as
	// the router interface's IP on the subnet. If empty and a router interface
	// references this network, CAPCS defaults to network-address + 3, because
	// cloudscale.ch reserves .1 and .2 for DNS.
	// In both cases the subnet's gateway is then updated to the actual IP.
	// Only applicable when CIDR is set (managed network). Immutable after creation.
	// +optional
	GatewayAddress string `json:"gatewayAddress,omitempty"`
}

// RouterSpec defines a cloudscale.ch router managed or adopted by CAPCS.
// Exactly one of UUID (pre-existing) or InternetGateway (managed) governs behaviour.
type RouterSpec struct {
	// Name identifies this router within the cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// UUID references a pre-existing cloudscale.ch router by UUID.
	// CAPCS attaches and detaches interfaces but does not create or delete the router.
	// Mutually exclusive with InternetGateway. Immutable after creation.
	// +optional
	UUID string `json:"uuid,omitempty"`

	// InternetGateway enables SNAT for outbound internet access on a managed router.
	// When false (default), the router performs pure L3 routing between attached networks.
	// Only valid when UUID is not set. Immutable after creation.
	// +optional
	InternetGateway bool `json:"internetGateway,omitempty"`

	// Interfaces lists the networks this router is attached to.
	// +optional
	Interfaces []RouterInterfaceSpec `json:"interfaces,omitempty"`
}

// RouterInterfaceSpec defines a single network attachment for a router.
type RouterInterfaceSpec struct {
	// Network references spec.networks[].name.
	// +kubebuilder:validation:Required
	Network string `json:"network"`

	// Address is the IP requested for this router interface within the referenced
	// network's subnet. The cloudscale.ch API requires an explicit address per
	// interface, so the admission webhook defaults this for managed (CIDR) networks:
	// the ConfigureSubnetGateway owner gets network+3 (mirrored to the network's
	// gatewayAddress), and additional interfaces on the same network get network+4,
	// network+5, ... in order. It may be set explicitly to override the default.
	// Immutable after creation.
	// +optional
	Address string `json:"address,omitempty"`

	// ConfigureSubnetGateway, when true (default), sets the subnet's gatewayAddress
	// to this router interface's assigned IP, making the router the default route for
	// servers on that subnet.
	// Set to false for transit/backbone networks where a different router (e.g. the
	// internet-gateway router) owns the subnet gateway.
	// Immutable after creation.
	// +kubebuilder:default=true
	// +optional
	ConfigureSubnetGateway *bool `json:"configureSubnetGateway,omitempty"`
}

// LoadBalancerSpec defines the load balancer configuration for the control plane.
type LoadBalancerSpec struct {
	// Enabled controls whether a load balancer is created for the control plane.
	// Set to false for external control planes (e.g., hosted control plane) where the endpoint
	// is provided externally, or when using a floating IP without a load balancer.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Algorithm is the cloudscale.ch load-balancing algorithm.
	// - "round_robin" (default): rotate requests across healthy backends.
	// - "least_connections": send each request to the backend with the fewest active connections.
	// - "source_ip": hash the client IP so the same client lands on the same backend.
	// +kubebuilder:validation:Enum=round_robin;least_connections;source_ip
	// +kubebuilder:default="round_robin"
	// +optional
	Algorithm string `json:"algorithm,omitempty"`

	// Flavor is the cloudscale.ch load balancer flavor slug. Defaults to
	// "lb-standard".
	// +kubebuilder:default="lb-standard"
	// +optional
	Flavor string `json:"flavor,omitempty"`

	// APIServerPort is the LB listener port exposed for the Kubernetes API
	// server. Defaults to 6443. The pool always targets the API server on the
	// control plane nodes' 6443.
	// +kubebuilder:default=6443
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	APIServerPort int32 `json:"apiServerPort,omitempty"`

	// Network places the LB VIP on a private network (internal LB).
	// References spec.networks[].name. Omit for a public LB.
	// When multiple networks are defined, either this field or PoolMemberNetwork
	// must be set so the LB pool members can be registered against a specific
	// subnet.
	// +optional
	Network string `json:"network,omitempty"`

	// PoolMemberNetwork selects the network whose subnet the LB pool members are
	// registered on (i.e. the network the control-plane machines attach to).
	// References spec.networks[].name. Defaults to Network when set, else the first
	// network. Set this when the control-plane nodes live on a different network
	// than the VIP (Network), e.g. a public VIP with private control-plane nodes,
	// or a private VIP on a dedicated access network. Immutable after creation.
	// +optional
	PoolMemberNetwork string `json:"poolMemberNetwork,omitempty"`

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

// FloatingIPSpec configures a floating IP for the control plane endpoint.
// Exactly one of IPFamily or Address must be specified.
type FloatingIPSpec struct {
	// IPFamily creates a new floating IP with this IP version.
	// A floating IP is a single address, so DualStack is not valid here.
	// Mutually exclusive with Address.
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +optional
	IPFamily *IPFamily `json:"ipFamily,omitempty"`

	// Address references a pre-existing floating IP by its address.
	// cloudscale.ch identifies floating IPs by their IP address rather than by UUID.
	// The floating IP is not deleted on cluster teardown.
	// Mutually exclusive with IPFamily.
	// +optional
	Address string `json:"address,omitempty"`
}

// RouterStatus tracks the provisioned state of a single router.
type RouterStatus struct {
	// Name matches spec.routers[].name.
	Name string `json:"name"`

	// RouterID is the cloudscale.ch router UUID.
	// +optional
	RouterID string `json:"routerID,omitempty"`

	// InterfaceIDs maps network name to interface UUID for each CAPCS-created interface.
	// Used during deletion to remove only interfaces CAPCS created on pre-existing routers.
	// +optional
	InterfaceIDs map[string]string `json:"interfaceIDs,omitempty"`
}

// CloudscaleClusterStatus defines the observed state of CloudscaleCluster.
type CloudscaleClusterStatus struct {
	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Initialization contains v1beta2 initialization tracking.
	// +optional
	Initialization *ClusterInitializationStatus `json:"initialization,omitempty"`

	// Networks track the status of each network defined in spec.networks.
	// +listType=map
	// +listMapKey=name
	// +optional
	Networks []NetworkStatus `json:"networks,omitempty"`

	// Routers track the provisioned state of each router defined in spec.routers.
	// +listType=map
	// +listMapKey=name
	// +optional
	Routers []RouterStatus `json:"routers,omitempty"`

	// FloatingIP is the cloudscale.ch floating IP.
	// +optional
	FloatingIP string `json:"floatingIP,omitempty"`

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
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NetworkStatus tracks the provisioned state of a single network.
type NetworkStatus struct {
	// Name matches the logical name from spec.networks[].name.
	Name string `json:"name"`

	// NetworkID is the cloudscale.ch network UUID.
	// +optional
	NetworkID string `json:"networkID,omitempty"`

	// SubnetID is the cloudscale.ch subnet UUID.
	// +optional
	SubnetID string `json:"subnetID,omitempty"`

	// CIDR is the subnet CIDR block.
	// Set from spec for managed networks or discovered from the API for pre-existing networks.
	// +optional
	CIDR string `json:"cidr,omitempty"`

	// Managed indicates whether CAPCS manages this network's lifecycle.
	// false for pre-existing networks (referenced by UUID), true for CAPCS-created networks (defined by CIDR).
	Managed bool `json:"managed"`

	// GatewayAddress is the subnet gateway IP that CAPCS has configured on the router interface.
	// Empty until the router interface is attached and the subnet gateway is updated.
	// +optional
	GatewayAddress string `json:"gatewayAddress,omitempty"`
}

// ClusterInitializationStatus contains v1beta2 initialization tracking for CloudscaleCluster.
type ClusterInitializationStatus struct {
	// Provisioned indicates that all cluster infrastructure has been provisioned.
	// True when Network, Subnet, Load Balancer, and Control Plane Endpoint are ready.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// GetNetworkStatus returns the NetworkStatus for the given network name, or nil if not found.
func (s *CloudscaleClusterStatus) GetNetworkStatus(name string) *NetworkStatus {
	for i := range s.Networks {
		if s.Networks[i].Name == name {
			return &s.Networks[i]
		}
	}
	return nil
}

// GetRouterStatus returns the RouterStatus for the given router name, or nil if not found.
func (s *CloudscaleClusterStatus) GetRouterStatus(name string) *RouterStatus {
	for i := range s.Routers {
		if s.Routers[i].Name == name {
			return &s.Routers[i]
		}
	}
	return nil
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=cloudscaleclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster"
// +kubebuilder:printcolumn:name="Provisioned",type="string",JSONPath=".status.initialization.provisioned",description="Infrastructure provisioned"
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.region",description="cloudscale.ch region"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="Control plane endpoint"

// CloudscaleCluster is the cloudscale.ch infrastructure for a CAPI Cluster.
// It owns the networks, control-plane load balancer, optional floating IP, and
// server groups that back the cluster's machines.
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
