package v1beta2

// Condition types for CloudscaleCluster
// These conditions follow the CAPI v1beta2 contract and Kubernetes API conventions.
const (
	// ReadyCondition indicates the overall readiness of the resource.
	// For CloudscaleCluster: True when all infrastructure is provisioned and endpoint is set.
	// This condition is mirrored by CAPI to the parent resource's InfrastructureReady condition.
	ReadyCondition = "Ready"

	// NetworkReadyCondition indicates whether the private network has been provisioned.
	NetworkReadyCondition = "NetworkReady"

	// LoadBalancerReadyCondition indicates whether the load balancer and all its
	// components (pool, listener, health monitor) are ready.
	LoadBalancerReadyCondition = "LoadBalancerReady"

	// PausedCondition indicates the resource reconciliation is paused.
	// True when the pause annotation is present on the resource or parent cluster.
	PausedCondition = "Paused"

	// DeletingCondition indicates the resource is being deleted.
	// True when DeletionTimestamp is set on the resource.
	DeletingCondition = "Deleting"
)

// Condition reasons for CloudscaleCluster.
const (
	// NetworkProvisionedReason indicates the network has been successfully provisioned.
	NetworkProvisionedReason = "NetworkProvisioned"

	// NetworkErrorReason indicates an error occurred during network operations.
	NetworkErrorReason = "NetworkError"

	// NetworkDeletingReason indicates the network is being deleted.
	NetworkDeletingReason = "NetworkDeleting"

	// LoadBalancerRunningReason indicates the load balancer is running and ready.
	LoadBalancerRunningReason = "LoadBalancerRunning"

	// LoadBalancerNotReadyReason indicates the load balancer exists but is not yet running.
	LoadBalancerNotReadyReason = "LoadBalancerNotReady"

	// LoadBalancerDisabledReason indicates the load balancer is disabled (external control plane).
	LoadBalancerDisabledReason = "LoadBalancerDisabled"

	// LoadBalancerErrorReason indicates an error occurred during load balancer operations.
	LoadBalancerErrorReason = "LoadBalancerError"

	// LoadBalancerDeletingReason indicates the load balancer is being deleted.
	LoadBalancerDeletingReason = "LoadBalancerDeleting"
)

// Shared condition reasons.
const (
	// DeletingReason indicates the resource is being deleted.
	DeletingReason = "Deleting"

	// PausedReason indicates the resource reconciliation is paused.
	PausedReason = "Paused"

	// ReadyReason indicates the resource is ready.
	ReadyReason = "Ready"

	// NotReadyReason indicates the resource is not ready.
	NotReadyReason = "NotReady"
)
