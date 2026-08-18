package v1beta2

// Condition types for CloudscaleCluster and CloudscaleMachine resources.
// These conditions follow the CAPI v1beta2 contract and Kubernetes API conventions.
const (
	// ReadyCondition indicates the overall readiness of the resource.
	// For CloudscaleCluster: True when all infrastructure is provisioned and endpoint is set.
	// For CloudscaleMachine: True when the server exists and is running.
	// This condition is mirrored by CAPI to the parent resource's InfrastructureReady condition.
	ReadyCondition = "Ready"

	// NetworkReadyCondition indicates whether the private network and subnet have been provisioned.
	// Only applicable to CloudscaleCluster.
	NetworkReadyCondition = "NetworkReady"

	// LoadBalancerReadyCondition indicates whether the load balancer and all its
	// components (pool, listener, health monitor) are ready.
	// Only applicable to CloudscaleCluster.
	LoadBalancerReadyCondition = "LoadBalancerReady"

	// ServerGroupReadyCondition indicates whether the server group has been provisioned.
	// Only applicable to CloudscaleMachine.
	ServerGroupReadyCondition = "ServerGroupReady"

	// ServerReadyCondition indicates whether the server has been provisioned.
	// Only applicable to CloudscaleMachine.
	ServerReadyCondition = "ServerReady"

	// PausedCondition indicates the resource reconciliation is paused.
	// True when the pause annotation is present on the resource or parent cluster.
	PausedCondition = "Paused"

	// RouterReadyCondition indicates all spec.routers are provisioned, active, and have their
	// interfaces attached. Also set to True when no routers are defined.
	RouterReadyCondition = "RouterReady"

	// FloatingIPReadyCondition indicates whether the floating IP has been provisioned and assigned.
	// Only applicable to CloudscaleCluster when spec.floatingIP is set.
	FloatingIPReadyCondition = "FloatingIPReady"

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

	// RouterProvisionedReason indicates all routers and their interfaces are provisioned.
	RouterProvisionedReason = "RouterProvisioned"

	// RouterNotReadyReason indicates a router exists but its status is not yet active.
	RouterNotReadyReason = "RouterNotReady"

	// RouterErrorReason indicates an error occurred during router operations.
	RouterErrorReason = "RouterError"

	// RouterDisabledReason indicates spec.routers is empty.
	RouterDisabledReason = "RouterDisabled"

	// RouterDeletingReason indicates routers are being deleted.
	RouterDeletingReason = "RouterDeleting"

	// FloatingIPProvisionedReason indicates the floating IP has been successfully provisioned and assigned.
	FloatingIPProvisionedReason = "FloatingIPProvisioned"

	// FloatingIPDisabledReason indicates no floating IP is configured.
	FloatingIPDisabledReason = "FloatingIPDisabled"

	// FloatingIPErrorReason indicates an error occurred during floating IP operations.
	FloatingIPErrorReason = "FloatingIPError"

	// FloatingIPDeletingReason indicates the floating IP is being deleted.
	FloatingIPDeletingReason = "FloatingIPDeleting"
)

// Condition reasons for CloudscaleMachine.
const (
	// ServerRunningReason indicates the server is running and ready.
	ServerRunningReason = "ServerRunning"

	// ServerStartingReason indicates the server is being created (status: changing).
	ServerStartingReason = "ServerStarting"

	// ServerChangingReason indicates the server is transitioning (status: changing, after already provisioned).
	ServerChangingReason = "ServerChanging"

	// ServerStoppedReason indicates the server is stopped.
	ServerStoppedReason = "ServerStopped"

	// ServerInRescueModeReason indicates the server is in rescue mode.
	ServerInRescueModeReason = "ServerInRescueMode"

	// ServerInternalErrorReason indicates an internal error with the server (paused/error/unknown status).
	ServerInternalErrorReason = "ServerInternalError"

	// ServerErrorReason indicates an error occurred during server operations.
	ServerErrorReason = "ServerError"

	// ServerDeletingReason indicates the server is being deleted.
	ServerDeletingReason = "ServerDeleting"

	// ServerDeletedExternallyReason indicates the server was deleted outside of CAPI.
	ServerDeletedExternallyReason = "ServerDeletedExternally"

	// ServerGroupReadyReason indicates the server group has been successfully provisioned.
	ServerGroupReadyReason = "ServerGroupReady"

	// ServerGroupErrorReason indicates an error occurred during server group operations.
	ServerGroupErrorReason = "ServerGroupError"

	// WaitingForClusterInfrastructureReason indicates the machine is waiting for
	// the cluster infrastructure to be provisioned.
	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"

	// WaitingForBootstrapDataReason indicates the machine is waiting for
	// bootstrap data to be available.
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"
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
