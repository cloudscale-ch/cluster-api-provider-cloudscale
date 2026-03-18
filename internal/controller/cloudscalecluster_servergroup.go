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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"

	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// deleteServerGroups deletes all server groups owned by the cluster.
// This is called during cluster deletion, after all machines have been removed.
func (r *CloudscaleClusterReconciler) deleteServerGroups(ctx context.Context, clusterScope *scope.ClusterScope) error {
	groups, err := clusterScope.CloudscaleClient.ServerGroups.List(ctx,
		cloudscalesdk.WithTagFilter(clusterOwnershipTags(clusterScope.CloudscaleCluster)))
	if err != nil {
		return fmt.Errorf("listing server groups: %w", err)
	}

	for _, g := range groups {
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
