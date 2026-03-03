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

package controller

import (
	"context"
	"fmt"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v6"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// reconcileNetwork orchestrates network and subnet provisioning.
// A single NetworkReadyCondition covers both resources.
func (r *CloudscaleClusterReconciler) reconcileNetwork(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.NetworkErrorReason, reterr.Error())
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.NetworkProvisionedReason, "")
		}
	}()

	if err := r.reconcileNetworkResource(ctx, clusterScope); err != nil {
		return fmt.Errorf("reconciling network: %w", err)
	}

	if err := r.reconcileSubnet(ctx, clusterScope); err != nil {
		return fmt.Errorf("reconciling subnet: %w", err)
	}

	return nil
}

// reconcileNetworkResource ensures the network exists.
func (r *CloudscaleClusterReconciler) reconcileNetworkResource(ctx context.Context, clusterScope *scope.ClusterScope) error {
	id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.NetworkID,
		"network",
		clusterScope.CloudscaleClient.Networks,
		func(n cloudscalesdk.Network) string { return n.UUID },
		*r.resourceTags(clusterScope),
	)
	if err != nil {
		return err
	}
	clusterScope.CloudscaleCluster.Status.NetworkID = id
	if id != "" {
		return nil
	}

	// Create new network
	clusterScope.Info("Creating network")

	network, err := clusterScope.CloudscaleClient.Networks.Create(ctx, &cloudscalesdk.NetworkCreateRequest{
		Name:                 clusterScope.Name(),
		AutoCreateIPV4Subnet: ptr.To(false),
		ZonalResourceRequest: cloudscalesdk.ZonalResourceRequest{
			Zone: clusterScope.CloudscaleCluster.Spec.Zone,
		},
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: r.resourceTags(clusterScope),
		},
	})
	if err != nil {
		return fmt.Errorf("creating network: %w", err)
	}

	clusterScope.CloudscaleCluster.Status.NetworkID = network.UUID
	clusterScope.Info("Created network", "networkID", network.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "NetworkCreated", "CreateNetwork",
		"Created network %s in zone %s", network.UUID, clusterScope.CloudscaleCluster.Spec.Zone)

	return nil
}

// reconcileSubnet ensures the subnet exists within the network.
func (r *CloudscaleClusterReconciler) reconcileSubnet(ctx context.Context, clusterScope *scope.ClusterScope) error {
	if clusterScope.CloudscaleCluster.Status.NetworkID == "" {
		return fmt.Errorf("network must be created before subnet")
	}

	id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.SubnetID,
		"subnet",
		clusterScope.CloudscaleClient.Subnets,
		func(s cloudscalesdk.Subnet) string { return s.UUID },
		*r.resourceTags(clusterScope),
	)
	if err != nil {
		return err
	}
	clusterScope.CloudscaleCluster.Status.SubnetID = id
	if id != "" {
		return nil
	}

	// Create new subnet
	// GatewayAddress is defaulted by the webhook (empty string = no gateway)
	spec := &clusterScope.CloudscaleCluster.Spec.Network
	clusterScope.Info("Creating subnet", "cidr", spec.CIDR, "gateway", *spec.GatewayAddress)

	subnet, err := clusterScope.CloudscaleClient.Subnets.Create(ctx, &cloudscalesdk.SubnetCreateRequest{
		Network:        clusterScope.CloudscaleCluster.Status.NetworkID,
		CIDR:           spec.CIDR,
		GatewayAddress: *spec.GatewayAddress,
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: r.resourceTags(clusterScope),
		},
	})
	if err != nil {
		return fmt.Errorf("creating subnet: %w", err)
	}

	clusterScope.CloudscaleCluster.Status.SubnetID = subnet.UUID
	clusterScope.Info("Created subnet", "subnetID", subnet.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "SubnetCreated", "CreateSubnet",
		"Created subnet %s with CIDR %s", subnet.UUID, clusterScope.CloudscaleCluster.Spec.Network.CIDR)

	return nil
}

// deleteNetwork deletes the network. Subnets are cascade-deleted by the cloudscale.ch API
// when their parent network is deleted, so only the network needs explicit deletion.
func (r *CloudscaleClusterReconciler) deleteNetwork(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.NetworkErrorReason, fmt.Sprintf("Failed to delete network: %v", reterr))
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.NetworkDeletingReason, "Network has been deleted")
		}
	}()

	if clusterScope.CloudscaleCluster.Status.NetworkID == "" {
		return nil
	}

	networkID := clusterScope.CloudscaleCluster.Status.NetworkID
	clusterScope.Info("Deleting network", "networkID", networkID)

	if err := clusterScope.CloudscaleClient.Networks.Delete(ctx, networkID); err != nil {
		// Ignore 404 - network was already deleted externally
		if !cloudscale.IsNotFound(err) {
			return fmt.Errorf("deleting network: %w", err)
		}
		clusterScope.Info("Network already deleted", "networkID", networkID)
	}

	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "NetworkDeleted", "DeleteNetwork",
		"Deleted network %s", networkID)

	// Clear both IDs (subnet is cascade-deleted with the network)
	clusterScope.CloudscaleCluster.Status.NetworkID = ""
	clusterScope.CloudscaleCluster.Status.SubnetID = ""
	return nil
}
