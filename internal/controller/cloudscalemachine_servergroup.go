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
	"sync"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/observability"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// serverGroupMu serializes server group creation to prevent duplicates
// when multiple machines reconcile concurrently. The cloudscale API does
// not return a conflict on duplicate creation, so we serialize here.
// Safe: leader election guarantees a single replica.
var serverGroupMu sync.Mutex

// reconcileServerGroup ensures the server group exists if specified.
// Server groups are zone-scoped and created once per unique name+zone combination.
func (r *CloudscaleMachineReconciler) reconcileServerGroup(ctx context.Context, machineScope *scope.MachineScope) (_ ctrl.Result, reterr error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleMachineReconciler.reconcileServerGroup")
	defer done()

	logger.Info("Reconciling server group")

	defer func() {
		if reterr != nil {
			r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerGroupErrorReason, reterr.Error())
		} else {
			r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerGroupReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.ServerGroupReadyReason, "")
		}
	}()

	if machineScope.CloudscaleMachine.Spec.ServerGroup == nil {
		return ctrl.Result{}, nil // No server group requested
	}

	// If we already have a server group ID, verify it still exists
	if machineScope.CloudscaleMachine.Status.ServerGroupID != "" {
		getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
		_, err := machineScope.CloudscaleClient.ServerGroups.Get(getCtx, machineScope.CloudscaleMachine.Status.ServerGroupID)
		cancel()
		if err == nil {
			return ctrl.Result{}, nil
		}
		if !cloudscale.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("getting server group: %w", err)
		}
		// Server group was deleted externally, fall through to re-create
		machineScope.CloudscaleMachine.Status.ServerGroupID = ""
	}

	// Serialize server group list+create to prevent duplicates under concurrent reconciles.
	serverGroupMu.Lock()
	defer serverGroupMu.Unlock()

	zone := machineScope.CloudscaleCluster.Spec.Zone
	groupName := machineScope.CloudscaleMachine.Spec.ServerGroup.Name

	// Search for existing server group by name and zone using cluster-level tags
	// so that all machines in the cluster can find the same server group.
	listCtx, cancelList := context.WithTimeout(ctx, cloudscale.ReadTimeout)
	groups, err := machineScope.CloudscaleClient.ServerGroups.List(listCtx, cloudscalesdk.WithTagFilter(clusterOwnershipTags(machineScope.CloudscaleCluster)))
	cancelList()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("listing server groups: %w", err)
	}

	for _, g := range groups {
		if g.Name == groupName && g.Zone.Slug == zone {
			machineScope.CloudscaleMachine.Status.ServerGroupID = g.UUID
			return ctrl.Result{}, nil
		}
	}

	// Create new server group with cluster-level tags
	req := &cloudscalesdk.ServerGroupRequest{
		Name: groupName,
		Type: "anti-affinity",
		Zone: zone,
		Tags: new(clusterOwnershipTags(machineScope.CloudscaleCluster)),
	}

	createCtx, cancelCreate := context.WithTimeout(ctx, cloudscale.WriteTimeout)
	group, err := machineScope.CloudscaleClient.ServerGroups.Create(createCtx, req)
	cancelCreate()
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			requeueAfter := 5 * time.Second
			machineScope.Info("Server group creation timed out, waiting before retry", "requeueAfter", requeueAfter)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating server group: %w", err)
	}

	machineScope.CloudscaleMachine.Status.ServerGroupID = group.UUID
	return ctrl.Result{}, nil
}

// deleteServerGroup clears the server group reference from the machine status.
// The actual server group API resource is deleted by the cluster controller during
// cluster deletion, since server groups are shared across machines.
func (r *CloudscaleMachineReconciler) deleteServerGroup(_ context.Context, machineScope *scope.MachineScope) {
	if machineScope.CloudscaleMachine.Spec.ServerGroup == nil {
		return
	}

	machineScope.CloudscaleMachine.Status.ServerGroupID = ""
}
