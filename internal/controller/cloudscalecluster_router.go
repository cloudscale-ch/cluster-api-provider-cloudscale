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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/observability"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// routerRequeueAfter is how long to wait before looking again at a router that is still
// settling: a create or attach that timed out, or a router that is not active yet.
const routerRequeueAfter = 5 * time.Second

// reconcileRouters orchestrates router provisioning for all routers
// defined in spec.routers. A single RouterReadyCondition covers all routers.
func (r *CloudscaleClusterReconciler) reconcileRouters(ctx context.Context, clusterScope *scope.ClusterScope) (result ctrl.Result, reterr error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.reconcileRouters")
	defer done()

	defer func() {
		switch {
		case reterr != nil:
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterErrorReason, reterr.Error())
		case !result.IsZero():
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterNotReadyReason, "Waiting for routers to become available")
		case len(clusterScope.CloudscaleCluster.Spec.Routers) == 0:
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.RouterDisabledReason, "")
		default:
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.RouterProvisionedReason, "")
		}
	}()

	logger.Info("Reconciling routers")

	// Process all routers in one reconcile call by returning the lowest requeueAfter time (or just a non-zero result).
	// This ensures routers get created in parallel and avoids longer waiting times if there is more than one router
	// specified.
	for _, routerSpec := range clusterScope.CloudscaleCluster.Spec.Routers {
		res, err := r.reconcileRouter(ctx, clusterScope, routerSpec)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("reconciling router %q: %w", routerSpec.Name, err)
		}
		noResultYet := result.IsZero()
		lowerRequeueAfter := res.RequeueAfter < result.RequeueAfter
		if !res.IsZero() && (noResultYet || lowerRequeueAfter) {
			result = res
		}
	}
	return result, nil
}

// reconcileRouter brings one router to the state its spec asks for: resolve or create it,
// record it in status, and attach its interfaces once it is active.
func (r *CloudscaleClusterReconciler) reconcileRouter(ctx context.Context, clusterScope *scope.ClusterScope, routerSpec infrastructurev1beta2.RouterSpec) (ctrl.Result, error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.reconcileRouter")
	defer done()

	var (
		router *cloudscalesdk.Router
		result ctrl.Result
		err    error
	)
	if routerSpec.UUID != "" {
		router, err = r.adoptRouter(ctx, clusterScope, routerSpec)
	} else {
		router, result, err = r.ensureManagedRouter(ctx, clusterScope, routerSpec)
	}
	if err != nil || !result.IsZero() {
		return result, err
	}

	r.setRouterStatus(clusterScope, routerSpec, router.UUID)

	if router.Status != cloudscalesdk.RouterActive {
		logger.Info("Router is not active yet, waiting before retry",
			"name", routerSpec.Name, "routerID", router.UUID, "status", router.Status, "requeueAfter", routerRequeueAfter)
		return ctrl.Result{RequeueAfter: routerRequeueAfter}, nil
	}

	return r.reconcileRouterInterfaces(ctx, clusterScope, routerSpec, router)
}

// adoptRouter returns the pre-existing router the spec entry references by UUID.
//
// An adopted router is one CAPCS attaches and detaches interfaces on, but never creates or
// deletes itself. Resolving it is a plain read, so it never has to requeue.
func (r *CloudscaleClusterReconciler) adoptRouter(ctx context.Context, clusterScope *scope.ClusterScope, routerSpec infrastructurev1beta2.RouterSpec) (*cloudscalesdk.Router, error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.adoptRouter")
	defer done()

	getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
	router, err := clusterScope.CloudscaleClient.Routers.Get(getCtx, routerSpec.UUID)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("getting pre-existing router %s: %w", routerSpec.UUID, err)
	}
	if router.Zone.Slug != clusterScope.CloudscaleCluster.Spec.Zone {
		return nil, fmt.Errorf("pre-existing router %s is in zone %q, expected zone %q", routerSpec.UUID, router.Zone.Slug, clusterScope.CloudscaleCluster.Spec.Zone)
	}
	// only log if we haven't recorded the status yet
	if clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerSpec.Name) == nil {
		logger.Info("Discovered pre-existing router", "name", routerSpec.Name, "routerID", router.UUID)
	}
	return router, nil
}

// ensureManagedRouter returns the router CAPCS owns for this spec entry, creating it when it
// does not exist yet. A non-zero result means the router is not available yet and the caller
// should requeue.
func (r *CloudscaleClusterReconciler) ensureManagedRouter(ctx context.Context, clusterScope *scope.ClusterScope, routerSpec infrastructurev1beta2.RouterSpec) (*cloudscalesdk.Router, ctrl.Result, error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.ensureManagedRouter")
	defer done()

	var routerID string
	if rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerSpec.Name); rs != nil {
		routerID = rs.RouterID
	}

	tags := r.routerTags(clusterScope, routerSpec.Name)
	router, resolvedRouterID, err := ensureResource(ctx, clusterScope,
		routerID,
		fmt.Sprintf("router/%s", routerSpec.Name),
		clusterScope.CloudscaleClient.Routers,
		func(r cloudscalesdk.Router) string { return r.UUID },
		tags,
	)
	if err != nil {
		return nil, ctrl.Result{}, fmt.Errorf("ensuring router: %w", err)
	}
	if resolvedRouterID != "" {
		return router, ctrl.Result{}, nil
	}

	logger.Info("Creating router", "name", routerSpec.Name)
	createCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
	router, err = clusterScope.CloudscaleClient.Routers.Create(createCtx, &cloudscalesdk.RouterCreateRequest{
		Name:            routerSpec.Name,
		InternetGateway: routerSpec.InternetGateway,
		Zone:            clusterScope.CloudscaleCluster.Spec.Zone,
		Tags:            new(tags),
	})
	cancel()
	if err != nil {
		if result, timedOut := r.requeueOnTimeout(clusterScope, err, "Router creation"); timedOut {
			return nil, result, nil
		}
		return nil, ctrl.Result{}, fmt.Errorf("creating router: %w", err)
	}
	logger.Info("Created router", "name", routerSpec.Name, "routerID", router.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "RouterCreated", "CreateRouter",
		"Created router %s (%s) in zone %s", routerSpec.Name, router.UUID, router.Zone.Slug)

	return router, ctrl.Result{}, nil
}

// requeueOnTimeout turns a timed-out write into a requeue instead of an error: the call
// may well have landed, so the next reconcile re-reads rather than retrying blindly.
func (r *CloudscaleClusterReconciler) requeueOnTimeout(clusterScope *scope.ClusterScope, err error, what string) (ctrl.Result, bool) {
	if !cloudscale.IsTimeoutError(err) {
		return ctrl.Result{}, false
	}
	clusterScope.Info(what+" timed out, waiting before retry", "requeueAfter", routerRequeueAfter)
	return ctrl.Result{RequeueAfter: routerRequeueAfter}, true
}

// reconcileRouterInterfaces attaches the router to every network in its spec it is not
// already attached to, and points those subnets' gateways at the interface where the spec
// asks for it.
func (r *CloudscaleClusterReconciler) reconcileRouterInterfaces(
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	routerSpec infrastructurev1beta2.RouterSpec,
	router *cloudscalesdk.Router,
) (ctrl.Result, error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.reconcileRouterInterfaces")
	defer done()

	// A router attaches to any given network at most once, so indexing by network is
	// unambiguous. The index by UUID is what an adopted interface is looked up through.
	liveByNetwork := make(map[string]cloudscalesdk.RouterInterface, len(router.Interfaces))
	liveByUUID := make(map[string]cloudscalesdk.RouterInterface, len(router.Interfaces))
	for _, iface := range router.Interfaces {
		liveByUUID[iface.UUID] = iface
		liveByNetwork[iface.Network.UUID] = iface
	}

	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerSpec.Name)
	if rs == nil {
		return ctrl.Result{}, fmt.Errorf("router %q not found in status", routerSpec.Name)
	}

	for _, ifaceSpec := range routerSpec.Interfaces {
		logger := logger.WithValues("name", routerSpec.Name, "routerID", router.UUID, "network", ifaceSpec.Network, "specAddress", ifaceSpec.Address)

		ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(ifaceSpec.Network)
		if ns == nil {
			return ctrl.Result{}, fmt.Errorf("network %q not found in status for router %q interface", ifaceSpec.Network, routerSpec.Name)
		}

		// We always attach at the address the spec asks for, unless the router turns out
		// to already be attached to this network, in which case the interface keeps the
		// address it has: an attachment cannot be moved.
		address := ifaceSpec.Address
		live, attached := liveByNetwork[ns.NetworkID]

		switch {
		case ifaceSpec.UUID != "":
			adopted, err := r.adoptRouterInterface(clusterScope, logger, routerSpec, ifaceSpec, ns, liveByUUID)
			if err != nil {
				return ctrl.Result{}, err
			}
			address = adopted
		case attached:
			claimed, err := r.claimRouterInterface(clusterScope, logger, routerSpec, rs, ifaceSpec, ns, live)
			if err != nil {
				return ctrl.Result{}, err
			}
			address = claimed
		default:
			result, err := r.createRouterInterface(ctx, clusterScope, logger, routerSpec, router, ifaceSpec, ns)
			if err != nil || !result.IsZero() {
				return result, err
			}
		}

		if result, err := r.reconcileSubnetGateway(ctx, clusterScope, logger, routerSpec, ifaceSpec, ns, address); err != nil || !result.IsZero() {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

// adoptRouterInterface records the pre-existing interface the spec entry references by
// UUID, and returns the address it holds.
func (r *CloudscaleClusterReconciler) adoptRouterInterface(
	clusterScope *scope.ClusterScope,
	logger logr.Logger,
	routerSpec infrastructurev1beta2.RouterSpec,
	ifaceSpec infrastructurev1beta2.RouterInterfaceSpec,
	ns *infrastructurev1beta2.NetworkStatus,
	liveByUUID map[string]cloudscalesdk.RouterInterface,
) (string, error) {
	live, exists := liveByUUID[ifaceSpec.UUID]
	if !exists {
		return "", fmt.Errorf("router %q has no interface %s to adopt, it may have been detached outside of CAPCS", routerSpec.Name, ifaceSpec.UUID)
	}
	if live.Network.UUID != ns.NetworkID {
		return "", fmt.Errorf("router %q interface %s is attached to network %s, not to network %q (%s)",
			routerSpec.Name, ifaceSpec.UUID, live.Network.UUID, ifaceSpec.Network, ns.NetworkID)
	}

	address := interfaceAddress(live, ns.SubnetID)
	if changed := r.setRouterInterfaceStatus(clusterScope, routerSpec.Name, ns.Name, live.UUID, false); changed {
		logger.Info("Adopted pre-existing router interface", "interfaceID", live.UUID, "address", address)
	}
	return address, nil
}

// claimRouterInterface records an already existing interface in the clusters' status.
func (r *CloudscaleClusterReconciler) claimRouterInterface(
	clusterScope *scope.ClusterScope,
	logger logr.Logger,
	routerSpec infrastructurev1beta2.RouterSpec,
	rs *infrastructurev1beta2.RouterStatus,
	ifaceSpec infrastructurev1beta2.RouterInterfaceSpec,
	ns *infrastructurev1beta2.NetworkStatus,
	live cloudscalesdk.RouterInterface,
) (string, error) {
	// if the interface is already in the API, but not yet in our status and the router is not managed, this means
	// that we adopted a router with a pre-existing interface, but that interface was not mentioned in the list with a UUID.
	if prev := rs.GetInterfaceStatus(ns.Name); prev == nil && !rs.Managed {
		return "", fmt.Errorf("pre-existing router %q is already attached to network %q by interface %s: set spec.routers[].interfaces[].uuid to adopt that interface, or detach it",
			routerSpec.Name, ifaceSpec.Network, live.UUID)
	}

	address := interfaceAddress(live, ns.SubnetID)

	changed := r.setRouterInterfaceStatus(clusterScope, routerSpec.Name, ns.Name, live.UUID, true)
	if changed && address != ifaceSpec.Address {
		logger.Info("Router is already attached to network at a different address", "address", address)
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeWarning, "RouterInterfaceAddressMismatch", "ReconcileRouterInterface",
			"Router %s is attached to network %s at %q, not at the requested %q, which therefore has no effect",
			routerSpec.Name, ifaceSpec.Network, address, ifaceSpec.Address)
	}
	return address, nil
}

// createRouterInterface attaches the router to the network at the address the spec asks
// for.
func (r *CloudscaleClusterReconciler) createRouterInterface(
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	logger logr.Logger,
	routerSpec infrastructurev1beta2.RouterSpec,
	router *cloudscalesdk.Router,
	ifaceSpec infrastructurev1beta2.RouterInterfaceSpec,
	ns *infrastructurev1beta2.NetworkStatus,
) (ctrl.Result, error) {
	// Record the attachment before it exists. The response can be lost to a timeout while
	// the interface gets created anyway, and this entry is what tells the next reconcile the
	// interface is one we created.
	r.setRouterInterfaceStatus(clusterScope, routerSpec.Name, ns.Name, "", true)

	logger.Info("Creating interface for router")
	createCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
	createdIface, err := clusterScope.CloudscaleClient.Routers.CreateInterface(createCtx, router.UUID, cloudscalesdk.CreateInterfaceRequest{
		Network: ns.NetworkID,
		Addresses: []cloudscalesdk.CreateAddressRequest{
			{
				Subnet:  ns.SubnetID,
				Address: ifaceSpec.Address,
			},
		},
	})
	cancel()
	if err != nil {
		if result, timedOut := r.requeueOnTimeout(clusterScope, err, "Router interface creation"); timedOut {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating interface for router %q network %q: %w", routerSpec.Name, ifaceSpec.Network, err)
	}

	r.setRouterInterfaceStatus(clusterScope, routerSpec.Name, ns.Name, createdIface.UUID, true)
	logger.Info("Created interface for router", "interfaceID", createdIface.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "RouterInterfaceCreated", "CreateRouterInterface",
		"Attached router %s to network %s with address %s", routerSpec.Name, ifaceSpec.Network, ifaceSpec.Address)
	return ctrl.Result{}, nil
}

// reconcileSubnetGateway points the subnet's gateway at the address the router interface
// holds, making the router the default route for servers on that subnet.
func (r *CloudscaleClusterReconciler) reconcileSubnetGateway(
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	logger logr.Logger,
	routerSpec infrastructurev1beta2.RouterSpec,
	ifaceSpec infrastructurev1beta2.RouterInterfaceSpec,
	ns *infrastructurev1beta2.NetworkStatus,
	address string,
) (ctrl.Result, error) {
	// no configuration of the subnet gateway requested
	if !ptr.Deref(ifaceSpec.ConfigureSubnetGateway, true) {
		return ctrl.Result{}, nil
	}
	// no address set
	if address == "" {
		// The API always returns the interface's address, so this cannot normally
		// happen. Guard anyway: writing the empty address would strip the subnet's
		// default route from every server on it.
		logger.Info("Router interface holds no address on the tracked subnet, leaving the subnet gateway untouched", "subnetID", ns.SubnetID)
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeWarning, "RouterInterfaceAddressUnknown", "ReconcileRouterInterface",
			"Router %s is attached to network %s but holds no address on its subnet; the subnet gateway was left unchanged",
			routerSpec.Name, ifaceSpec.Network)
		return ctrl.Result{}, nil
	}
	// gateway address already correct
	if ns.GatewayAddress == address {
		return ctrl.Result{}, nil
	}

	logger.Info("Configuring subnet gateway", "address", address)
	updateCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
	err := clusterScope.CloudscaleClient.Subnets.Update(updateCtx, ns.SubnetID, &cloudscalesdk.SubnetUpdateRequest{
		GatewayAddress: address,
	})
	cancel()
	if err != nil {
		if result, timedOut := r.requeueOnTimeout(clusterScope, err, "Subnet gateway update"); timedOut {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("updating subnet gateway for router %q network %q: %w", routerSpec.Name, ifaceSpec.Network, err)
	}
	r.setNetworkGatewayAddress(clusterScope, ifaceSpec.Network, address)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "SubnetGatewayConfigured", "ConfigureSubnetGateway",
		"Configured subnet %s gateway to router interface address %s", ifaceSpec.Network, address)
	return ctrl.Result{}, nil
}

// interfaceAddress returns the address the interface holds on the given subnet, or "" when
// it holds none: a network can carry more than one subnet, and the router may be attached
// on a different one than CAPCS tracks.
func interfaceAddress(iface cloudscalesdk.RouterInterface, subnetID string) string {
	for _, addr := range iface.Addresses {
		if addr.Subnet.UUID == subnetID {
			return addr.Address
		}
	}
	return ""
}

// deleteRouters removes the interfaces we created, then the routers it created.
// Pre-existing routers are left in place: only their managed interfaces go.
// On partial failure the affected routers stay in status so the next attempt retries
// exactly those.
func (r *CloudscaleClusterReconciler) deleteRouters(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.deleteRouters")
	defer done()

	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterErrorReason, fmt.Sprintf("Failed to delete routers: %v", reterr))
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.RouterReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.RouterDeletingReason, "Routers have been deleted")
		}
	}()

	var remaining []infrastructurev1beta2.RouterStatus
	var errs []error

	for ri := range clusterScope.CloudscaleCluster.Status.Routers {
		router := &clusterScope.CloudscaleCluster.Status.Routers[ri]
		logger := logger.WithValues("router", router.Name, "routerID", router.RouterID)

		// Never provisioned, so there is nothing to clean up.
		if router.RouterID == "" {
			continue
		}

		// Routers can only be deleted if it's private interfaces are deleted first.
		if ifaceErrs := r.deleteRouterInterfaces(ctx, clusterScope, router); len(ifaceErrs) > 0 {
			errs = append(errs, ifaceErrs...)
			remaining = append(remaining, *router)
			continue
		}

		if !router.Managed {
			logger.Info("Skipping pre-existing router deletion")
			remaining = append(remaining, *router)
			continue
		}

		logger.Info("Deleting router")
		deleteCtx, cancel := context.WithTimeout(ctx, cloudscale.DeleteTimeout)
		err := clusterScope.CloudscaleClient.Routers.Delete(deleteCtx, router.RouterID)
		cancel()

		isNotFound := cloudscale.IsNotFound(err)

		if err != nil && !isNotFound {
			remaining = append(remaining, *router)
			errs = append(errs, fmt.Errorf("deleting router %q: %w", router.Name, err))
			continue
		}

		// Idempotent not-found: log and fall through to the event.
		if isNotFound {
			logger.Info("Router already deleted")
		}
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "RouterDeleted", "DeleteRouter",
			"Deleted router %s (%s)", router.Name, router.RouterID)
	}
	clusterScope.CloudscaleCluster.Status.Routers = remaining
	return errors.Join(errs...)
}

// deleteRouterInterfaces deletes the interfaces CAPCS created on a router.
//
// The router is read once so the recorded interfaces can be reconciled against the live
// ones.
func (r *CloudscaleClusterReconciler) deleteRouterInterfaces(
	ctx context.Context,
	clusterScope *scope.ClusterScope,
	rs *infrastructurev1beta2.RouterStatus,
) (errs []error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.deleteRouterInterfaces")
	defer done()

	logger = logger.WithValues("router", rs.Name, "routerID", rs.RouterID)

	if !hasManagedInterface(rs) {
		// Nothing of ours to detach, so the router does not have to be read at all.
		return nil
	}

	getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
	router, err := clusterScope.CloudscaleClient.Routers.Get(getCtx, rs.RouterID)
	cancel()
	if cloudscale.IsNotFound(err) {
		// The router is gone, and its interfaces went with it.
		logger.Info("Router already deleted, no interfaces left to detach")
		rs.Interfaces = nil
		return nil
	}
	if err != nil {
		return []error{fmt.Errorf("getting router %q to detach its interfaces: %w", rs.Name, err)}
	}

	liveUUIDs := make(map[string]struct{}, len(router.Interfaces))
	liveByNetwork := make(map[string]string, len(router.Interfaces))
	for _, iface := range router.Interfaces {
		liveUUIDs[iface.UUID] = struct{}{}
		liveByNetwork[iface.Network.UUID] = iface.UUID
	}

	var remaining []infrastructurev1beta2.RouterInterfaceStatus
	for _, iface := range rs.Interfaces {
		if !iface.Managed {
			remaining = append(remaining, iface)
			continue
		}

		interfaceID := iface.InterfaceID
		if interfaceID == "" {
			// An attach whose response never arrived: the network is all we recorded.
			if ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(iface.Network); ns != nil {
				interfaceID = liveByNetwork[ns.NetworkID]
			}
		}
		if _, live := liveUUIDs[interfaceID]; !live {
			// Never attached, or already detached: either way there is nothing to do.
			logger.Info("Router interface already detached", "network", iface.Network, "interfaceID", iface.InterfaceID)
			continue
		}

		logger.Info("Deleting router interface", "network", iface.Network, "interfaceID", interfaceID)

		delCtx, cancel := context.WithTimeout(ctx, cloudscale.DeleteTimeout)
		err := clusterScope.CloudscaleClient.Routers.DeleteInterface(delCtx, rs.RouterID, interfaceID)
		cancel()
		if err != nil && !cloudscale.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting router interface for %s/%s: %w", rs.Name, iface.Network, err))
			// Keep the resolved UUID, so a retry does not have to resolve it again.
			iface.InterfaceID = interfaceID
			remaining = append(remaining, iface)
			continue
		}
		logger.Info("Deleted router interface", "network", iface.Network, "interfaceID", interfaceID)
	}
	rs.Interfaces = remaining
	return errs
}

// hasManagedInterface reports whether the router has any interface CAPCS has to detach.
func hasManagedInterface(rs *infrastructurev1beta2.RouterStatus) bool {
	for _, iface := range rs.Interfaces {
		if iface.Managed {
			return true
		}
	}
	return false
}

// routerTags returns the ownership tags for a specific named router.
func (r *CloudscaleClusterReconciler) routerTags(clusterScope *scope.ClusterScope, routerName string) cloudscalesdk.TagMap {
	return cloudscalesdk.TagMap{
		infrastructurev1beta2.NameCloudscaleProviderOwned + clusterScope.Cluster.Name: routerName,
	}
}

// setRouterStatus updates or appends the router status entry for the given spec entry.
// .Interfaces is only written by setRouterInterfaceStatus and pruned by deleteRouterInterfaces.
func (r *CloudscaleClusterReconciler) setRouterStatus(clusterScope *scope.ClusterScope, routerSpec infrastructurev1beta2.RouterSpec, routerID string) {
	managed := routerSpec.UUID == ""
	if rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerSpec.Name); rs != nil {
		rs.RouterID = routerID
		rs.Managed = managed
		return
	}
	clusterScope.CloudscaleCluster.Status.Routers = append(clusterScope.CloudscaleCluster.Status.Routers, infrastructurev1beta2.RouterStatus{
		Name:     routerSpec.Name,
		RouterID: routerID,
		Managed:  managed,
	})
}

// setRouterInterfaceStatus upserts an interface entry on the named router's status.
func (r *CloudscaleClusterReconciler) setRouterInterfaceStatus(clusterScope *scope.ClusterScope, routerName, networkName, interfaceID string, managed bool) bool {
	rs := clusterScope.CloudscaleCluster.Status.GetRouterStatus(routerName)
	if rs == nil {
		return false
	}
	if iface := rs.GetInterfaceStatus(networkName); iface != nil {
		if iface.InterfaceID == interfaceID && iface.Managed == managed {
			return false
		}
		iface.InterfaceID = interfaceID
		iface.Managed = managed
		return true
	}
	rs.Interfaces = append(rs.Interfaces, infrastructurev1beta2.RouterInterfaceStatus{
		Network:     networkName,
		InterfaceID: interfaceID,
		Managed:     managed,
	})
	return true
}
