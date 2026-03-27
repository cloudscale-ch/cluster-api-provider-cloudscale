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
	"errors"
	"fmt"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// reconcileNetwork orchestrates network and subnet provisioning for all networks
// defined in spec.networks. A single NetworkReadyCondition covers all networks.
func (r *CloudscaleClusterReconciler) reconcileNetwork(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.NetworkErrorReason, reterr.Error())
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.NetworkProvisionedReason, "")
		}
	}()

	if len(clusterScope.CloudscaleCluster.Spec.Networks) == 0 {
		return fmt.Errorf("no networks defined in spec")
	}

	for _, netSpec := range clusterScope.CloudscaleCluster.Spec.Networks {
		if netSpec.UUID != "" {
			if err := r.reconcileBYONetwork(ctx, clusterScope, netSpec); err != nil {
				return fmt.Errorf("reconciling BYO network %q: %w", netSpec.Name, err)
			}
		} else {
			if err := r.reconcileManagedNetwork(ctx, clusterScope, netSpec); err != nil {
				return fmt.Errorf("reconciling managed network %q: %w", netSpec.Name, err)
			}
		}
	}

	return nil
}

// reconcileBYONetwork validates a BYO network exists and discovers its subnet.
// The subnet is discovered once and cached in status. Subsequent reconciles
// short-circuit if the network and subnet IDs are already populated.
// This is intentional: BYO networks are managed externally, so CAPCS does not
// re-verify them. If the network/subnet is reconfigured externally, the next
// machine creation will fail at the cloudscale API level.
func (r *CloudscaleClusterReconciler) reconcileBYONetwork(ctx context.Context, clusterScope *scope.ClusterScope, netSpec infrastructurev1beta2.NetworkSpec) error {
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(netSpec.Name)
	if ns != nil && ns.NetworkID != "" && ns.SubnetID != "" {
		return nil
	}

	network, err := clusterScope.CloudscaleClient.Networks.Get(ctx, netSpec.UUID)
	if err != nil {
		return fmt.Errorf("getting BYO network %s: %w", netSpec.UUID, err)
	}

	if len(network.Subnets) == 0 {
		return fmt.Errorf("BYO network %s has no subnets", netSpec.UUID)
	}

	r.setNetworkStatus(clusterScope, netSpec.Name, network.UUID, network.Subnets[0].UUID, false)
	clusterScope.Info("Discovered BYO network", "name", netSpec.Name, "networkID", network.UUID, "subnetID", network.Subnets[0].UUID)

	return nil
}

// reconcileManagedNetwork ensures a managed network and its subnet exist.
func (r *CloudscaleClusterReconciler) reconcileManagedNetwork(ctx context.Context, clusterScope *scope.ClusterScope, netSpec infrastructurev1beta2.NetworkSpec) error {
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(netSpec.Name)

	// Reconcile the network resource
	var networkID string
	if ns != nil {
		networkID = ns.NetworkID
	}

	tags := r.networkTags(clusterScope, netSpec.Name)

	_, resolvedNetworkID, err := ensureResource(ctx, clusterScope,
		networkID,
		fmt.Sprintf("network/%s", netSpec.Name),
		clusterScope.CloudscaleClient.Networks,
		func(n cloudscalesdk.Network) string { return n.UUID },
		tags,
	)
	if err != nil {
		return err
	}

	if resolvedNetworkID == "" {
		// Create new network
		clusterScope.Info("Creating network", "name", netSpec.Name)
		network, err := clusterScope.CloudscaleClient.Networks.Create(ctx, &cloudscalesdk.NetworkCreateRequest{
			Name:                 netSpec.Name,
			AutoCreateIPV4Subnet: ptr.To(false),
			ZonalResourceRequest: cloudscalesdk.ZonalResourceRequest{
				Zone: clusterScope.CloudscaleCluster.Spec.Zone,
			},
			TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
				Tags: ptr.To(tags),
			},
		})
		if err != nil {
			return fmt.Errorf("creating network: %w", err)
		}
		resolvedNetworkID = network.UUID
		clusterScope.Info("Created network", "name", netSpec.Name, "networkID", network.UUID)
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "NetworkCreated", "CreateNetwork",
			"Created network %s (%s) in zone %s", netSpec.Name, network.UUID, clusterScope.CloudscaleCluster.Spec.Zone)
	}

	// Reconcile the subnet
	var subnetID string
	if ns != nil {
		subnetID = ns.SubnetID
	}

	_, resolvedSubnetID, err := ensureResource(ctx, clusterScope,
		subnetID,
		fmt.Sprintf("subnet/%s", netSpec.Name),
		clusterScope.CloudscaleClient.Subnets,
		func(s cloudscalesdk.Subnet) string { return s.UUID },
		tags,
	)
	if err != nil {
		return err
	}

	if resolvedSubnetID == "" {
		// Create new subnet
		clusterScope.Info("Creating subnet", "name", netSpec.Name, "cidr", netSpec.CIDR, "gateway", netSpec.GatewayAddress)
		subnet, err := clusterScope.CloudscaleClient.Subnets.Create(ctx, &cloudscalesdk.SubnetCreateRequest{
			Network:        resolvedNetworkID,
			CIDR:           netSpec.CIDR,
			GatewayAddress: netSpec.GatewayAddress,
			TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
				Tags: ptr.To(tags),
			},
		})
		if err != nil {
			return fmt.Errorf("creating subnet: %w", err)
		}
		resolvedSubnetID = subnet.UUID
		clusterScope.Info("Created subnet", "name", netSpec.Name, "subnetID", subnet.UUID)
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "SubnetCreated", "CreateSubnet",
			"Created subnet %s (%s) with CIDR %s", netSpec.Name, subnet.UUID, netSpec.CIDR)
	}

	r.setNetworkStatus(clusterScope, netSpec.Name, resolvedNetworkID, resolvedSubnetID, true)
	return nil
}

// deleteNetwork deletes all managed networks. BYO networks are left untouched.
// Subnets are cascade-deleted by the cloudscale.ch API when their parent network is deleted.
// On partial failure, successfully deleted networks are removed from status so that
// only undeleted networks remain for the next reconcile attempt.
func (r *CloudscaleClusterReconciler) deleteNetwork(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.NetworkErrorReason, fmt.Sprintf("Failed to delete network: %v", reterr))
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.NetworkReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.NetworkDeletingReason, "Networks have been deleted")
		}
	}()

	var remaining []infrastructurev1beta2.NetworkStatus
	var errs []error

	for _, ns := range clusterScope.CloudscaleCluster.Status.Networks {
		if !ns.Managed {
			clusterScope.Info("Skipping BYO network deletion", "name", ns.Name, "networkID", ns.NetworkID)
			continue
		}

		if ns.NetworkID == "" {
			continue
		}

		clusterScope.Info("Deleting network", "name", ns.Name, "networkID", ns.NetworkID)
		if err := clusterScope.CloudscaleClient.Networks.Delete(ctx, ns.NetworkID); err != nil {
			if !cloudscale.IsNotFound(err) {
				remaining = append(remaining, ns)
				errs = append(errs, fmt.Errorf("deleting network %s: %w", ns.Name, err))
				continue
			}
			clusterScope.Info("Network already deleted", "name", ns.Name, "networkID", ns.NetworkID)
		}

		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "NetworkDeleted", "DeleteNetwork",
			"Deleted network %s (%s)", ns.Name, ns.NetworkID)
	}

	clusterScope.CloudscaleCluster.Status.Networks = remaining
	return errors.Join(errs...)
}

// setNetworkStatus updates or appends the network status entry for the given name.
func (r *CloudscaleClusterReconciler) setNetworkStatus(clusterScope *scope.ClusterScope, name, networkID, subnetID string, managed bool) {
	for i, ns := range clusterScope.CloudscaleCluster.Status.Networks {
		if ns.Name == name {
			clusterScope.CloudscaleCluster.Status.Networks[i].NetworkID = networkID
			clusterScope.CloudscaleCluster.Status.Networks[i].SubnetID = subnetID
			clusterScope.CloudscaleCluster.Status.Networks[i].Managed = managed
			return
		}
	}
	clusterScope.CloudscaleCluster.Status.Networks = append(clusterScope.CloudscaleCluster.Status.Networks, infrastructurev1beta2.NetworkStatus{
		Name:      name,
		NetworkID: networkID,
		SubnetID:  subnetID,
		Managed:   managed,
	})
}

// networkTags returns the tags for a specific named network, combining cluster ownership with network name.
func (r *CloudscaleClusterReconciler) networkTags(clusterScope *scope.ClusterScope, networkName string) cloudscalesdk.TagMap {
	tags := cloudscalesdk.TagMap{
		infrastructurev1beta2.NameCloudscaleProviderOwned + clusterScope.Cluster.Name: networkName,
	}
	return tags
}
