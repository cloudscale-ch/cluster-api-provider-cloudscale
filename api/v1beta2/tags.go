package v1beta2

// ResourceLifecycle configures the lifecycle of a resource.
type ResourceLifecycle string

const (

	// ResourceLifecycleOwned is the value we use when tagging resources to indicate
	// that the resource is considered owned and managed by the cluster,
	// and in particular that the lifecycle is tied to the lifecycle of the cluster.
	ResourceLifecycleOwned = ResourceLifecycle("owned")

	// NameCloudscaleProviderPrefix is the tag prefix we use to differentiate
	// cluster-api-provider-cloudscale owned components from other tooling that
	// uses NameKubernetesClusterPrefix
	NameCloudscaleProviderPrefix = "capcs-"

	// NameCloudscaleProviderOwned is the tag name we use to differentiate
	// cluster-api-provider-cloudscale owned components from other tooling that
	// uses NameKubernetesClusterPrefix.
	NameCloudscaleProviderOwned = NameCloudscaleProviderPrefix + "cluster-"
)
