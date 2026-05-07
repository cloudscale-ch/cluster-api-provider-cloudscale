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
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// errServerGroupNotEmpty is returned when a server group still contains owned servers
// and cannot be deleted yet.
var errServerGroupNotEmpty = errors.New("server group still contains owned servers")

// deleteServerGroups deletes all server groups owned by the cluster.
// This is called during cluster deletion, after all machines have been removed.
// When owned servers are still present in a group, the function returns
// errServerGroupNotEmpty to signal a graceful requeue, preventing the
// "Cannot delete non-empty server group" API error.
func (r *CloudscaleClusterReconciler) deleteServerGroups(ctx context.Context, clusterScope *scope.ClusterScope) error {
	ownedServerIDs, err := r.getOwnedServerIDs(ctx, clusterScope)
	if err != nil {
		return fmt.Errorf("listing owned servers: %w", err)
	}

	groups, err := clusterScope.CloudscaleClient.ServerGroups.List(ctx,
		cloudscalesdk.WithTagFilter(clusterOwnershipTags(clusterScope.CloudscaleCluster)))
	if err != nil {
		return fmt.Errorf("listing server groups: %w", err)
	}

	for _, g := range groups {
		hasOwnedServers := false
		hasForeignServers := false
		for _, s := range g.Servers {
			if ownedServerIDs.Has(s.UUID) {
				hasOwnedServers = true
			} else {
				hasForeignServers = true
			}
		}

		if hasOwnedServers {
			clusterScope.Info("Waiting for owned servers to leave server group",
				"serverGroupID", g.UUID, "name", g.Name)
			return errServerGroupNotEmpty
		}

		if hasForeignServers {
			clusterScope.Info("Server group contains foreign servers, skipping deletion",
				"serverGroupID", g.UUID, "name", g.Name)
			continue
		}

		clusterScope.Info("Deleting server group", "serverGroupID", g.UUID, "name", g.Name)
		if err := clusterScope.CloudscaleClient.ServerGroups.Delete(ctx, g.UUID); err != nil {
			if !cloudscale.IsNotFound(err) {
				return fmt.Errorf("deleting server group %s: %w", g.UUID, err)
			}
			clusterScope.Info("Server group already deleted", "serverGroupID", g.UUID)
		}
	}

	return nil
}

// getOwnedServerIDs returns a set of server UUIDs from CloudscaleMachine resources
// that belong to the given cluster.
func (r *CloudscaleClusterReconciler) getOwnedServerIDs(ctx context.Context, clusterScope *scope.ClusterScope) (sets.Set[string], error) {
	machineList := &infrav1beta2.CloudscaleMachineList{}
	if err := r.List(ctx, machineList,
		client.InNamespace(clusterScope.CloudscaleCluster.Namespace),
		client.MatchingLabels{clusterv1.ClusterNameLabel: clusterScope.Cluster.Name},
	); err != nil {
		return nil, fmt.Errorf("listing CloudscaleMachines: %w", err)
	}

	ids := sets.New[string]()
	for _, m := range machineList.Items {
		if m.Status.ServerID != "" {
			ids.Insert(m.Status.ServerID)
		}
	}
	return ids, nil
}
