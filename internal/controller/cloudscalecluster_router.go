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
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/observability"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// routerStatusPollInterval is how long to wait before re-checking a router that
// is not yet active.
const routerStatusPollInterval = 5 * time.Second

// reconcileRouters ensures all routers defined in spec.routers are provisioned
// and have their interfaces attached. Sets RouterReadyCondition.
func (r *CloudscaleClusterReconciler) reconcileRouters(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	ctx, _, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.reconcileRouters")
	defer done()

	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterErrorReason, reterr.Error())
		}
	}()

	if len(clusterScope.CloudscaleCluster.Spec.Routers) == 0 {
		conditions.Set(clusterScope.CloudscaleCluster, metav1.Condition{
			Type:    infrastructurev1beta2.RouterReadyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  infrastructurev1beta2.RouterDisabledReason,
			Message: "",
		})
		return ctrl.Result{}, nil
	}

	// Process all routers before returning, collecting the earliest requeue time.
	// This ensures routers are not serialized: if router[0] is still "changing",
	// router[1] is still created in the same reconcile cycle.
	var minResult ctrl.Result
	for _, routerSpec := range clusterScope.CloudscaleCluster.Spec.Routers {
		result, err := r.reconcileRouter(ctx, clusterScope, routerSpec)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !result.IsZero() && (minResult.IsZero() || result.RequeueAfter < minResult.RequeueAfter) {
			minResult = result
		}
	}

	if !minResult.IsZero() {
		r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterNotReadyReason, "Waiting for routers to become ready")
		return minResult, nil
	}

	r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.RouterProvisionedReason, "")
	return ctrl.Result{}, nil
}

// reconcileRouter reconciles a single router spec entry.
func (r *CloudscaleClusterReconciler) reconcileRouter(ctx context.Context, clusterScope *scope.ClusterScope, routerSpec infrastructurev1beta2.RouterSpec) (ctrl.Result, error) {
	var router *cloudscalesdk.Router
	var routerID string
	var err error

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerSpec.Name)
	if rs != nil {
		routerID = rs.RouterID
	}

	if routerSpec.UUID != "" {
		// Pre-existing router: adopt by UUID.
		getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
		router, err = clusterScope.CloudscaleClient.Routers.Get(getCtx, routerSpec.UUID)
		cancel()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("getting pre-existing router %q: %w", routerSpec.Name, err)
		}
		routerID = router.UUID
	} else {
		// Managed router: adopt by tag or create.
		tags := r.routerTags(clusterScope, routerSpec.Name)
		router, routerID, err = ensureResource(ctx, clusterScope,
			routerID,
			fmt.Sprintf("router/%s", routerSpec.Name),
			clusterScope.CloudscaleClient.Routers,
			func(rt cloudscalesdk.Router) string { return rt.UUID },
			tags,
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		if routerID == "" {
			clusterScope.Info("Creating router", "name", routerSpec.Name)
			createCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
			defer cancel()
			router, err = clusterScope.CloudscaleClient.Routers.Create(createCtx, &cloudscalesdk.RouterCreateRequest{
				Name:            routerSpec.Name,
				InternetGateway: routerSpec.InternetGateway,
				ZonalResourceRequest: cloudscalesdk.ZonalResourceRequest{
					Zone: clusterScope.CloudscaleCluster.Spec.Zone,
				},
				TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
					Tags: new(tags),
				},
			})
			if err != nil {
				if cloudscale.IsTimeoutError(err) {
					clusterScope.Info("Router creation timed out, waiting before retry", "requeueAfter", createNetworkTimeoutRequeueAfter)
					return ctrl.Result{RequeueAfter: createNetworkTimeoutRequeueAfter}, nil
				}
				return ctrl.Result{}, fmt.Errorf("creating router %q: %w", routerSpec.Name, err)
			}
			routerID = router.UUID
			clusterScope.Info("Created router", "name", routerSpec.Name, "routerID", routerID)
			r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "RouterCreated", "CreateRouter",
				"Created router %s (%s) in zone %s", routerSpec.Name, routerID, clusterScope.CloudscaleCluster.Spec.Zone)
		}
	}

	r.setRouterStatus(clusterScope, routerSpec.Name, routerID)

	if router.Status != cloudscalesdk.RouterActive {
		clusterScope.Info("Waiting for router to become active", "name", routerSpec.Name, "status", router.Status)
		r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterNotReadyReason,
			fmt.Sprintf("Router %s is not yet active (status: %s)", routerSpec.Name, router.Status))
		return ctrl.Result{RequeueAfter: routerStatusPollInterval}, nil
	}

	for _, ifaceSpec := range routerSpec.Interfaces {
		result, err := r.reconcileRouterInterface(ctx, clusterScope, routerSpec, router, routerID, ifaceSpec)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !result.IsZero() {
			return result, nil
		}
	}

	return ctrl.Result{}, nil
}

// reconcileRouterInterface ensures a single router interface is attached and the subnet gateway is configured.
func (r *CloudscaleClusterReconciler) reconcileRouterInterface(ctx context.Context, clusterScope *scope.ClusterScope, routerSpec infrastructurev1beta2.RouterSpec, router *cloudscalesdk.Router, routerID string, ifaceSpec infrastructurev1beta2.RouterInterfaceSpec) (ctrl.Result, error) {
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(ifaceSpec.Network)
	if ns == nil {
		return ctrl.Result{}, fmt.Errorf("network %q not found in status for router %q interface", ifaceSpec.Network, routerSpec.Name)
	}

	iface := routerInterfaceForNetwork(router, ns.NetworkID)
	var assignedIP string

	if iface == nil {
		clusterScope.Info("Creating router interface", "router", routerSpec.Name, "network", ifaceSpec.Network, "address", ifaceSpec.Address)
		// The cloudscale.ch API requires an explicit address per interface. The
		// admission webhook defaults ifaceSpec.Address deterministically (see
		// RouterInterfaceSpec.Address); the controller only requests it.
		addr := cloudscalesdk.CreateAddressRequest{Subnet: ns.SubnetID, Address: ifaceSpec.Address}
		createCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
		defer cancel()
		createdIface, err := clusterScope.CloudscaleClient.Routers.CreateInterface(createCtx, routerID, cloudscalesdk.CreateInterfaceRequest{
			Network:   ns.NetworkID,
			Addresses: []cloudscalesdk.CreateAddressRequest{addr},
		})
		if err != nil {
			if cloudscale.IsTimeoutError(err) {
				clusterScope.Info("Router interface creation timed out, waiting before retry", "requeueAfter", createNetworkTimeoutRequeueAfter)
				return ctrl.Result{RequeueAfter: createNetworkTimeoutRequeueAfter}, nil
			}
			return ctrl.Result{}, fmt.Errorf("creating router interface for router %q network %q: %w", routerSpec.Name, ifaceSpec.Network, err)
		}
		r.setRouterInterfaceID(clusterScope, routerSpec.Name, ifaceSpec.Network, createdIface.UUID)
		assignedIP = routerInterfaceAddress(createdIface, ns.SubnetID)
		clusterScope.Info("Created router interface", "router", routerSpec.Name, "network", ifaceSpec.Network, "assignedIP", assignedIP)
	} else {
		assignedIP = routerInterfaceAddress(iface, ns.SubnetID)
		// Record the interface UUID if we don't have it yet (e.g. adopted from pre-existing status).
		rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerSpec.Name)
		if rs != nil && rs.InterfaceIDs[ifaceSpec.Network] == "" {
			r.setRouterInterfaceID(clusterScope, routerSpec.Name, ifaceSpec.Network, iface.UUID)
		}
	}

	if assignedIP == "" {
		clusterScope.Info("Router interface address not yet available, waiting before retry", "router", routerSpec.Name, "network", ifaceSpec.Network)
		return ctrl.Result{RequeueAfter: routerStatusPollInterval}, nil
	}

	if !ptr.Deref(ifaceSpec.ConfigureSubnetGateway, true) {
		return ctrl.Result{}, nil
	}

	if ns.GatewayAddress != assignedIP {
		clusterScope.Info("Configuring subnet gateway", "router", routerSpec.Name, "network", ifaceSpec.Network, "gatewayAddress", assignedIP)
		updateCtx, cancelUpdate := context.WithTimeout(ctx, cloudscale.WriteTimeout)
		defer cancelUpdate()
		if updErr := clusterScope.CloudscaleClient.Subnets.Update(updateCtx, ns.SubnetID, &cloudscalesdk.SubnetUpdateRequest{
			GatewayAddress: assignedIP,
		}); updErr != nil {
			if cloudscale.IsTimeoutError(updErr) {
				clusterScope.Info("Subnet gateway update timed out, waiting before retry", "requeueAfter", createNetworkTimeoutRequeueAfter)
				return ctrl.Result{RequeueAfter: createNetworkTimeoutRequeueAfter}, nil
			}
			return ctrl.Result{}, fmt.Errorf("updating subnet gateway for router %q network %q: %w", routerSpec.Name, ifaceSpec.Network, updErr)
		}
		r.setNetworkGatewayAddress(clusterScope, ifaceSpec.Network, assignedIP)
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "SubnetGatewayConfigured", "ConfigureSubnetGateway",
			"Configured subnet %s gateway to router interface address %s", ifaceSpec.Network, assignedIP)
	}

	return ctrl.Result{}, nil
}

// deleteRouters deletes routers. It runs before deleteNetwork because a network
// cannot be deleted while a router interface is attached. Interfaces are always
// removed first (for both managed and pre-existing routers); for managed routers
// the router itself is deleted afterwards. Deletes are synchronous, so a
// successful call means the resource is gone.
func (r *CloudscaleClusterReconciler) deleteRouters(ctx context.Context, clusterScope *scope.ClusterScope) error {
	r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterDeletingReason, "Deleting routers")

	var errs []error

	preExistingByName := make(map[string]bool, len(clusterScope.CloudscaleCluster.Spec.Routers))
	for _, routerSpec := range clusterScope.CloudscaleCluster.Spec.Routers {
		if routerSpec.UUID != "" {
			preExistingByName[routerSpec.Name] = true
		}
	}

	for i := range clusterScope.CloudscaleCluster.Status.Routers {
		rs := &clusterScope.CloudscaleCluster.Status.Routers[i]
		logger := clusterScope.WithValues("name", rs.Name, "routerID", rs.RouterID)

		// Delete tracked interfaces first (for all routers).
		if len(rs.InterfaceIDs) > 0 {
			errs = append(errs, r.deleteRouterInterfaces(ctx, clusterScope, rs, logger)...)
			if len(rs.InterfaceIDs) > 0 {
				continue // some interfaces failed to delete; retry before removing the router
			}
		}

		// Pre-existing router: leave the router itself, only detach interfaces.
		if preExistingByName[rs.Name] {
			continue
		}

		// Managed router: safe to delete now that interfaces are gone.
		if rs.RouterID == "" {
			continue
		}

		delCtx, cancelDel := context.WithTimeout(ctx, cloudscale.DeleteTimeout)
		err := clusterScope.CloudscaleClient.Routers.Delete(delCtx, rs.RouterID)
		cancelDel()
		if err != nil && !cloudscale.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting router %s: %w", rs.Name, err))
			continue
		}

		logger.Info("Deleted router")
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "RouterDeleted", "DeleteRouter",
			"Deleted router %s (%s)", rs.Name, rs.RouterID)
		rs.RouterID = ""
	}

	return errors.Join(errs...)
}

// deleteRouterInterfaces deletes all CAPCS-tracked interfaces on a router.
// Deletes are synchronous: interfaces that are removed (or already gone) are
// dropped from rs.InterfaceIDs in place; entries that fail are left to retry.
func (r *CloudscaleClusterReconciler) deleteRouterInterfaces(
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	rs *infrastructurev1beta2.RouterStatus,
	logger interface{ Info(string, ...any) },
) (errs []error) {
	for networkName, ifaceUUID := range rs.InterfaceIDs {
		delCtx, cancel := context.WithTimeout(ctx, cloudscale.DeleteTimeout)
		err := clusterScope.CloudscaleClient.Routers.DeleteInterface(delCtx, rs.RouterID, ifaceUUID)
		cancel()
		if err != nil && !cloudscale.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting router interface for %s/%s: %w", rs.Name, networkName, err))
			continue
		}
		logger.Info("Deleted router interface", "network", networkName, "ifaceUUID", ifaceUUID)
		delete(rs.InterfaceIDs, networkName)
	}
	return errs
}

// routerTags returns the ownership tags for a specific named router.
func (r *CloudscaleClusterReconciler) routerTags(clusterScope *scope.ClusterScope, routerName string) cloudscalesdk.TagMap {
	return cloudscalesdk.TagMap{
		infrastructurev1beta2.NameCloudscaleProviderOwned + clusterScope.Cluster.Name: routerName,
	}
}

// setRouterStatus upserts the router status entry for the given name and routerID.
func (r *CloudscaleClusterReconciler) setRouterStatus(clusterScope *scope.ClusterScope, name, routerID string) {
	for i := range clusterScope.CloudscaleCluster.Status.Routers {
		if clusterScope.CloudscaleCluster.Status.Routers[i].Name == name {
			clusterScope.CloudscaleCluster.Status.Routers[i].RouterID = routerID
			return
		}
	}
	clusterScope.CloudscaleCluster.Status.Routers = append(clusterScope.CloudscaleCluster.Status.Routers, infrastructurev1beta2.RouterStatus{
		Name:     name,
		RouterID: routerID,
	})
}

// setRouterInterfaceID records the interface UUID in the router status entry.
func (r *CloudscaleClusterReconciler) setRouterInterfaceID(clusterScope *scope.ClusterScope, routerName, networkName, ifaceUUID string) {
	for i := range clusterScope.CloudscaleCluster.Status.Routers {
		if clusterScope.CloudscaleCluster.Status.Routers[i].Name == routerName {
			if clusterScope.CloudscaleCluster.Status.Routers[i].InterfaceIDs == nil {
				clusterScope.CloudscaleCluster.Status.Routers[i].InterfaceIDs = make(map[string]string)
			}
			clusterScope.CloudscaleCluster.Status.Routers[i].InterfaceIDs[networkName] = ifaceUUID
			return
		}
	}
	// Router status entry doesn't exist yet; create it.
	clusterScope.CloudscaleCluster.Status.Routers = append(clusterScope.CloudscaleCluster.Status.Routers, infrastructurev1beta2.RouterStatus{
		Name:         routerName,
		InterfaceIDs: map[string]string{networkName: ifaceUUID},
	})
}

// routerInterfaceForNetwork returns the router's internal interface attached to the
// given network, or nil if none exists.
func routerInterfaceForNetwork(router *cloudscalesdk.Router, networkID string) *cloudscalesdk.RouterInterface {
	if router == nil {
		return nil
	}
	for i := range router.Interfaces {
		if router.Interfaces[i].Network.UUID == networkID {
			return &router.Interfaces[i]
		}
	}
	return nil
}

// routerInterfaceAddress returns the interface's address in the given subnet as a
// string, or "" if the interface is nil or has no address in that subnet.
func routerInterfaceAddress(iface *cloudscalesdk.RouterInterface, subnetID string) string {
	if iface == nil {
		return ""
	}
	for _, addr := range iface.Addresses {
		if addr.Subnet.UUID == subnetID {
			return addr.Address
		}
	}
	return ""
}
