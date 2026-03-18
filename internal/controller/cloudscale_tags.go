package controller

import (
	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
)

// clusterOwnershipTags returns the tags that identify resources owned by a cluster.
// Used for both creating and listing cluster-scoped resources (networks, subnets,
// load balancers, server groups).
func clusterOwnershipTags(cluster *infrastructurev1beta2.CloudscaleCluster) cloudscalesdk.TagMap {
	return cloudscalesdk.TagMap{
		cluster.ClusterTagKey(): string(infrastructurev1beta2.ResourceLifecycleOwned),
	}
}
