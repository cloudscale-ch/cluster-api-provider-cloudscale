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
	"net"
	"slices"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

const (
	// LoadBalancerRunningStatus indicates the load balancer is ready.
	LoadBalancerRunningStatus = "running"
)

// reconcileLoadBalancer ensures the load balancer, pool, and listener exist for the control plane.
// It also sets the control plane endpoint from the load balancer's VIP address.
// When the load balancer is disabled (external control plane), this function returns immediately.
func (r *CloudscaleClusterReconciler) reconcileLoadBalancer(ctx context.Context, clusterScope *scope.ClusterScope) (result ctrl.Result, reterr error) {
	// LB disabled: set condition and return before defer is registered
	if !ptr.Deref(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		clusterScope.Info("Load balancer is disabled, skipping reconciliation (external control plane)")
		r.setCondition(clusterScope, infrastructurev1beta2.LoadBalancerReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.LoadBalancerDisabledReason, "")
		return ctrl.Result{}, nil
	}

	var lbPending bool

	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.LoadBalancerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.LoadBalancerErrorReason, reterr.Error())
		} else if lbPending {
			r.setCondition(clusterScope, infrastructurev1beta2.LoadBalancerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.LoadBalancerNotReadyReason, "Load balancer is not yet running")
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.LoadBalancerReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.LoadBalancerRunningReason, "")
		}
	}()

	// 1. Reconcile the load balancer itself
	if result, err := r.reconcileLB(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// Wait for LB to be running before creating pool/listener
	if clusterScope.CloudscaleCluster.Status.LoadBalancerID == "" {
		lbPending = true
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if LB is running
	lb, err := clusterScope.CloudscaleClient.LoadBalancers.Get(ctx, clusterScope.CloudscaleCluster.Status.LoadBalancerID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting load balancer status: %w", err)
	}
	if lb.Status != LoadBalancerRunningStatus {
		clusterScope.Info("Waiting for load balancer to be running", "status", lb.Status)
		lbPending = true
		requeueAfter := 5 * time.Second
		if lb.Status == "error" || lb.Status == "degraded" {
			// During bootstrap, error/degraded is expected because the health
			// monitor checks an empty pool (no CP machines ready yet).
			requeueAfter = 30 * time.Second
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// 2. Reconcile the pool
	if result, err := r.reconcileLBPool(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer pool: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// 3. Reconcile the listener
	if result, err := r.reconcileLBListener(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer listener: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// 4. Reconcile the health monitor
	if result, err := r.reconcileLBHealthMonitor(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer health monitor: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// 5. Reconcile the members
	if result, err := r.reconcileLBMembers(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer members: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// 6. Set the control plane endpoint from the VIP
	if clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host == "" {
		if clusterScope.CloudscaleCluster.Spec.FloatingIP != nil {
			// Floating IP is configured — the FIP reconciler will set the endpoint.
			// The FIP provides a stable IP that survives LB recreation.
			clusterScope.Info("Skipping control plane endpoint from LB VIP (floating IP will provide it)")
		} else if len(lb.VIPAddresses) > 0 {
			apiServerPort := clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.APIServerPort
			clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = lb.VIPAddresses[0].Address
			clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port = apiServerPort
			clusterScope.Info("Set control plane endpoint from load balancer VIP",
				"endpoint", lb.VIPAddresses[0].Address, "port", apiServerPort)
			r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "ControlPlaneSet", "SetControlPlaneEndpoint", "Control plane endpoint set to %s:%d", lb.VIPAddresses[0].Address, apiServerPort)
		}
	}

	return ctrl.Result{}, nil
}

// reconcileLB ensures the load balancer exists.
func (r *CloudscaleClusterReconciler) reconcileLB(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	_, id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.LoadBalancerID,
		"load balancer",
		clusterScope.CloudscaleClient.LoadBalancers,
		func(lb cloudscalesdk.LoadBalancer) string { return lb.UUID },
		clusterOwnershipTags(clusterScope.CloudscaleCluster),
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = id
	if id != "" {
		return ctrl.Result{}, nil
	}

	// Create new load balancer
	zone := clusterScope.CloudscaleCluster.Spec.Zone
	lbSpec := clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer

	req := &cloudscalesdk.LoadBalancerRequest{
		Name:   fmt.Sprintf("%s-cp-lb", clusterScope.CloudscaleCluster.Name),
		Flavor: lbSpec.Flavor,
		ZonalResourceRequest: cloudscalesdk.ZonalResourceRequest{
			Zone: zone,
		},
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: ptr.To(clusterOwnershipTags(clusterScope.CloudscaleCluster)),
		},
	}

	// Place LB on a private network if specified, otherwise public VIP
	if lbSpec.Network != "" {
		subnetID, err := lbPrivateNetworkSubnetID(clusterScope)
		if err != nil {
			return ctrl.Result{}, err
		}
		req.VIPAddresses = &[]cloudscalesdk.VIPAddressRequest{
			{Subnet: subnetID},
		}
		clusterScope.Info("Creating load balancer with private VIP", "network", lbSpec.Network, "subnet", subnetID)
	}

	clusterScope.Info("Creating load balancer", "zone", zone, "flavor", lbSpec.Flavor)
	lb, err := clusterScope.CloudscaleClient.LoadBalancers.Create(ctx, req)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			clusterScope.Info("Load balancer creation timed out, waiting before retry", "requeueAfter", CreateTimeoutRequeueInterval)
			return ctrl.Result{RequeueAfter: CreateTimeoutRequeueInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating load balancer: %w", err)
	}

	clusterScope.CloudscaleCluster.Status.LoadBalancerID = lb.UUID
	clusterScope.Info("Created load balancer", "loadBalancerID", lb.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "LoadBalancerCreated", "CreateLoadBalancer", "Created load balancer %s in zone %s", lb.UUID, zone)

	return ctrl.Result{}, nil
}

// reconcileLBPool ensures the load balancer pool exists.
func (r *CloudscaleClusterReconciler) reconcileLBPool(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	_, id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID,
		"load balancer pool",
		clusterScope.CloudscaleClient.LoadBalancerPools,
		func(p cloudscalesdk.LoadBalancerPool) string { return p.UUID },
		clusterOwnershipTags(clusterScope.CloudscaleCluster),
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = id
	if id != "" {
		return ctrl.Result{}, nil
	}

	// Create new pool
	algorithm := clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Algorithm

	req := &cloudscalesdk.LoadBalancerPoolRequest{
		Name:         fmt.Sprintf("%s-cp-pool", clusterScope.CloudscaleCluster.Name),
		LoadBalancer: clusterScope.CloudscaleCluster.Status.LoadBalancerID,
		Algorithm:    algorithm,
		Protocol:     "tcp",
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: ptr.To(clusterOwnershipTags(clusterScope.CloudscaleCluster)),
		},
	}

	clusterScope.Info("Creating load balancer pool", "algorithm", algorithm)
	pool, err := clusterScope.CloudscaleClient.LoadBalancerPools.Create(ctx, req)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			clusterScope.Info("Load balancer pool creation timed out, waiting before retry", "requeueAfter", CreateTimeoutRequeueInterval)
			return ctrl.Result{RequeueAfter: CreateTimeoutRequeueInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating load balancer pool: %w", err)
	}

	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = pool.UUID
	clusterScope.Info("Created load balancer pool", "poolID", pool.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "PoolCreated", "CreateLoadBalancerPool", "Created load balancer pool %s", pool.UUID)

	return ctrl.Result{}, nil
}

// reconcileLBListener ensures the load balancer listener exists.
func (r *CloudscaleClusterReconciler) reconcileLBListener(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	_, id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID,
		"load balancer listener",
		clusterScope.CloudscaleClient.LoadBalancerListeners,
		func(l cloudscalesdk.LoadBalancerListener) string { return l.UUID },
		clusterOwnershipTags(clusterScope.CloudscaleCluster),
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = id
	if id != "" {
		return ctrl.Result{}, nil
	}

	apiServerPort := int(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.APIServerPort)
	req := &cloudscalesdk.LoadBalancerListenerRequest{
		Name:         fmt.Sprintf("%s-cp-listener", clusterScope.CloudscaleCluster.Name),
		Pool:         clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID,
		Protocol:     "tcp",
		ProtocolPort: apiServerPort,
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: ptr.To(clusterOwnershipTags(clusterScope.CloudscaleCluster)),
		},
	}

	clusterScope.Info("Creating load balancer listener", "port", apiServerPort)
	listener, err := clusterScope.CloudscaleClient.LoadBalancerListeners.Create(ctx, req)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			clusterScope.Info("Load balancer listener creation timed out, waiting before retry", "requeueAfter", CreateTimeoutRequeueInterval)
			return ctrl.Result{RequeueAfter: CreateTimeoutRequeueInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating load balancer listener: %w", err)
	}

	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = listener.UUID
	clusterScope.Info("Created load balancer listener", "listenerID", listener.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "ListenerCreated", "CreateLoadBalancerListener",
		"Created load balancer listener %s on port %d", listener.UUID, apiServerPort)

	return ctrl.Result{}, nil
}

// reconcileLBHealthMonitor ensures the load balancer health monitor exists.
// The health monitor performs TCP health checks on the API server port.
func (r *CloudscaleClusterReconciler) reconcileLBHealthMonitor(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	_, id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID,
		"load balancer health monitor",
		clusterScope.CloudscaleClient.LoadBalancerHealthMonitors,
		func(m cloudscalesdk.LoadBalancerHealthMonitor) string { return m.UUID },
		clusterOwnershipTags(clusterScope.CloudscaleCluster),
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = id
	if id != "" {
		return ctrl.Result{}, nil
	}

	healthMonitorSpec := clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.HealthMonitor

	req := &cloudscalesdk.LoadBalancerHealthMonitorRequest{
		Pool:          clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID,
		Type:          "tcp",
		DelayS:        healthMonitorSpec.DelayS,
		TimeoutS:      healthMonitorSpec.TimeoutS,
		UpThreshold:   healthMonitorSpec.UpThreshold,
		DownThreshold: healthMonitorSpec.DownThreshold,
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: ptr.To(clusterOwnershipTags(clusterScope.CloudscaleCluster)),
		},
	}

	clusterScope.Info("Creating load balancer health monitor", "type", "tcp", "pool", clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID, "spec", healthMonitorSpec)
	monitor, err := clusterScope.CloudscaleClient.LoadBalancerHealthMonitors.Create(ctx, req)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			clusterScope.Info("Load balancer health monitor creation timed out, waiting before retry", "requeueAfter", CreateTimeoutRequeueInterval)
			return ctrl.Result{RequeueAfter: CreateTimeoutRequeueInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating load balancer health monitor: %w", err)
	}

	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = monitor.UUID
	clusterScope.Info("Created load balancer health monitor", "healthMonitorID", monitor.UUID)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "HealthMonitorCreated", "CreateLoadBalancerHealthMonitor",
		"Created load balancer health monitor %s", monitor.UUID)

	return ctrl.Result{}, nil
}

func (r *CloudscaleClusterReconciler) reconcileLBMembers(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	// Fetch current members from the load balancer
	currentMembers, err := clusterScope.CloudscaleClient.LoadBalancerPoolMembers.List(ctx, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get current load balancer members: %w", err)
	}

	// Fetch control plane machines as desired members
	desiredMembers, err := r.getDesiredLoadBalancerMembers(ctx, clusterScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get desired load balancer members: %w", err)
	}

	clusterScope.V(2).Info("reconcileLBMembers", "currentMembers", currentMembers, "desiredMembers", desiredMembers)

	// Build maps keyed by member name for comparison
	currentByName := make(map[string]cloudscalesdk.LoadBalancerPoolMember, len(currentMembers))
	for _, member := range currentMembers {
		currentByName[member.Name] = member
	}

	desiredByName := make(map[string]cloudscalesdk.LoadBalancerPoolMemberRequest, len(desiredMembers))
	for _, member := range desiredMembers {
		desiredByName[member.Name] = member
	}

	// Add missing members and update existing members whose address has changed
	for _, desired := range desiredMembers {
		current, exists := currentByName[desired.Name]
		if !exists {
			if result, err := r.createLoadBalancerMember(ctx, clusterScope, desired); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create load balancer member: %w", err)
			} else if !result.IsZero() {
				return result, nil
			}
		} else if current.Address != desired.Address {
			if err := r.updateLoadBalancerMember(ctx, clusterScope, current.UUID, desired.Address); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update load balancer member: %w", err)
			}
		}
	}

	// Remove extra members
	for _, member := range currentMembers {
		if _, exists := desiredByName[member.Name]; !exists {
			if err := r.deleteLoadBalancerMember(ctx, clusterScope, member); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete load balancer member: %w", err)
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *CloudscaleClusterReconciler) getDesiredLoadBalancerMembers(ctx context.Context, clusterScope *scope.ClusterScope) ([]cloudscalesdk.LoadBalancerPoolMemberRequest, error) {
	machineList := &infrastructurev1beta2.CloudscaleMachineList{}
	if err := r.List(ctx, machineList,
		client.InNamespace(clusterScope.CloudscaleCluster.Namespace),
		client.MatchingLabels{
			// must be machines of the current cluster
			clusterv1.ClusterNameLabel: clusterScope.Cluster.Name,
			// must be control-plane machines
			clusterv1.MachineControlPlaneLabel: "",
		},
	); err != nil {
		return nil, fmt.Errorf("listing CloudscaleMachines: %w", err)
	}

	// Determine which subnet to use for pool members
	memberSubnetID, err := r.getPoolMemberSubnetID(clusterScope)
	if err != nil {
		return nil, err
	}

	memberSubnetCIDR, err := r.getMemberSubnetCIDR(clusterScope, memberSubnetID)
	if err != nil {
		return nil, fmt.Errorf("resolving pool member subnet CIDR: %w", err)
	}

	_, memberIPNet, err := net.ParseCIDR(memberSubnetCIDR)
	if err != nil {
		return nil, fmt.Errorf("parsing pool member subnet CIDR %q: %w", memberSubnetCIDR, err)
	}

	desiredList := make([]cloudscalesdk.LoadBalancerPoolMemberRequest, 0)

	// Build desired pool members from machines with an internal IP on the member subnet.
	// With multi-NIC machines, the first MachineInternalIP may belong to a different
	// network than the pool's subnet, so we filter by CIDR containment.
	for _, machine := range machineList.Items {
		member := cloudscalesdk.LoadBalancerPoolMemberRequest{
			Name:         machine.Name,
			Subnet:       memberSubnetID,
			ProtocolPort: int(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.APIServerPort),
			TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
				Tags: ptr.To(clusterOwnershipTags(clusterScope.CloudscaleCluster)),
			},
		}
		hasAddr := false
		for _, addr := range machine.Status.Addresses {
			if addr.Type == clusterv1.MachineInternalIP && addr.Address != "" {
				ip := net.ParseIP(addr.Address)
				if ip != nil && memberIPNet.Contains(ip) {
					hasAddr = true
					member.Address = addr.Address
					break
				}
			}
		}
		// can't add a member without an address
		if hasAddr {
			desiredList = append(desiredList, member)
		}
	}
	return desiredList, nil
}

// getMemberSubnetCIDR resolves the subnet UUID used for LB pool members to its CIDR.
// The CIDR is read from status (set during network reconciliation) so that pre-existing subnets
// are discovered once and cached, avoiding repeated API calls.
func (r *CloudscaleClusterReconciler) getMemberSubnetCIDR(clusterScope *scope.ClusterScope, subnetID string) (string, error) {
	for _, ns := range clusterScope.CloudscaleCluster.Status.Networks {
		if ns.SubnetID == subnetID && ns.CIDR != "" {
			return ns.CIDR, nil
		}
	}
	return "", fmt.Errorf("subnet %s has no cached CIDR in status; ensure networks are reconciled first", subnetID)
}

// getPoolMemberSubnetID determines the subnet UUID for LB pool members.
// If the LB is on a private network, use that network's subnet.
// Otherwise (public LB), use the first network's subnet.
func (r *CloudscaleClusterReconciler) getPoolMemberSubnetID(clusterScope *scope.ClusterScope) (string, error) {
	if clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Network != "" {
		return lbPrivateNetworkSubnetID(clusterScope)
	}

	networks := clusterScope.CloudscaleCluster.Status.Networks
	if len(networks) == 0 {
		return "", fmt.Errorf("no networks in cluster status")
	}
	if networks[0].SubnetID == "" {
		return "", fmt.Errorf("first network %q has no subnet ID", networks[0].Name)
	}
	return networks[0].SubnetID, nil
}

// lbPrivateNetworkSubnetID returns the subnet UUID of the private network that the LB
// VIP is placed on (spec.controlPlaneLoadBalancer.network). Caller must verify that
// spec.controlPlaneLoadBalancer.network is non-empty before calling.
func lbPrivateNetworkSubnetID(clusterScope *scope.ClusterScope) (string, error) {
	name := clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Network
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus(name)
	if ns == nil || ns.SubnetID == "" {
		return "", fmt.Errorf("network %q not yet provisioned for LB VIP placement", name)
	}
	return ns.SubnetID, nil
}

func (r *CloudscaleClusterReconciler) createLoadBalancerMember(ctx context.Context, clusterScope *scope.ClusterScope, member cloudscalesdk.LoadBalancerPoolMemberRequest) (ctrl.Result, error) {
	clusterScope.V(2).Info("Creating load balancer member", "member", member)
	cm, err := clusterScope.CloudscaleClient.LoadBalancerPoolMembers.Create(ctx, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID, &member)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			clusterScope.Info("Load balancer member creation timed out, waiting before retry", "requeueAfter", CreateTimeoutRequeueInterval)
			return ctrl.Result{RequeueAfter: CreateTimeoutRequeueInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating load balancer member: %w", err)
	}

	clusterScope.V(2).Info("Created load balancer member", "member", member)
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = append(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs, cm.UUID)
	return ctrl.Result{}, nil
}

func (r *CloudscaleClusterReconciler) updateLoadBalancerMember(ctx context.Context, clusterScope *scope.ClusterScope, memberUUID, newAddress string) error {
	clusterScope.V(2).Info("Updating load balancer member address", "memberUUID", memberUUID, "newAddress", newAddress)
	err := clusterScope.CloudscaleClient.LoadBalancerPoolMembers.Update(ctx, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID, memberUUID, &cloudscalesdk.LoadBalancerPoolMemberRequest{
		Address: newAddress,
	})
	if err != nil {
		return fmt.Errorf("updating load balancer member: %w", err)
	}
	clusterScope.V(2).Info("Updated load balancer member address", "memberUUID", memberUUID)
	return nil
}

func (r *CloudscaleClusterReconciler) deleteLoadBalancerMember(ctx context.Context, clusterScope *scope.ClusterScope, member cloudscalesdk.LoadBalancerPoolMember) error {
	clusterScope.V(2).Info("Deleting load balancer member", "member", member)
	err := clusterScope.CloudscaleClient.LoadBalancerPoolMembers.Delete(ctx, clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID, member.UUID)
	if err != nil {
		return fmt.Errorf("deleting load balancer member: %w", err)
	}

	clusterScope.V(2).Info("Deleted load balancer member", "member", member)
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = slices.DeleteFunc(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs, func(id string) bool { return id == member.UUID })
	return nil
}

// deleteLoadBalancer deletes the load balancer (the API cascade-deletes child resources).
// When the load balancer is disabled (external control plane), this function returns immediately.
func (r *CloudscaleClusterReconciler) deleteLoadBalancer(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	// Skip LB deletion if disabled (external control plane, e.g., hosted control plane)
	if !ptr.Deref(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		clusterScope.Info("Load balancer is disabled, skipping deletion (external control plane)")
		return nil
	}

	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.LoadBalancerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.LoadBalancerErrorReason, fmt.Sprintf("Failed to delete load balancer: %v", reterr))
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.LoadBalancerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.LoadBalancerDeletingReason, "Load balancer has been deleted")
		}
	}()

	lbID := clusterScope.CloudscaleCluster.Status.LoadBalancerID
	if lbID != "" {
		clusterScope.Info("Deleting load balancer", "id", lbID)
		if err := clusterScope.CloudscaleClient.LoadBalancers.Delete(ctx, lbID); err != nil {
			if !cloudscale.IsNotFound(err) {
				return fmt.Errorf("deleting load balancer: %w", err)
			}
		}
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "LoadBalancerDeleted", "DeleteLoadBalancer",
			"Deleted load balancer %s", lbID)
	}

	// Clear all status IDs (the API cascade-deletes child resources)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = ""
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = ""
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = ""
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = ""
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = nil

	return nil
}
