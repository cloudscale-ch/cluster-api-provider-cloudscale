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

const createFloatingIPTimeoutRequeueAfter = 5 * time.Second

// reconcileFloatingIP ensures the floating IP exists and is assigned to the correct target.
// When no floating IP is configured, this sets the condition to true and returns.
func (r *CloudscaleClusterReconciler) reconcileFloatingIP(ctx context.Context, clusterScope *scope.ClusterScope) (_ ctrl.Result, reterr error) {
	fipSpec := clusterScope.CloudscaleCluster.Spec.FloatingIP
	if fipSpec == nil {
		r.setCondition(clusterScope, infrastructurev1beta2.FloatingIPReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.FloatingIPDisabledReason, "")
		return ctrl.Result{}, nil
	}

	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.FloatingIPReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.FloatingIPErrorReason, reterr.Error())
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.FloatingIPReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.FloatingIPProvisionedReason, "")
		}
	}()

	// Pre-existing floating IP: just look it up and use its address
	if fipSpec.Address != "" {
		err := r.reconcilePreExistingFloatingIP(ctx, clusterScope, fipSpec.Address)
		return ctrl.Result{}, err
	}

	// Managed floating IP: create if needed, then assign
	return r.reconcileManagedFloatingIP(ctx, clusterScope, fipSpec)
}

func (r *CloudscaleClusterReconciler) reconcilePreExistingFloatingIP(ctx context.Context, clusterScope *scope.ClusterScope, ip string) error {
	getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
	defer cancel()
	fip, err := clusterScope.CloudscaleClient.FloatingIPs.Get(getCtx, ip)
	if err != nil {
		return fmt.Errorf("getting pre-existing floating IP %s: %w", ip, err)
	}

	// fip.Region is nil for global FIPs, which are valid for any cluster region.
	if fip.Region != nil && fip.Region.Slug != clusterScope.CloudscaleCluster.Spec.Region {
		return fmt.Errorf("pre-existing floating IP %s is in region %q, expected region %q", ip, fip.Region.Slug, clusterScope.CloudscaleCluster.Spec.Region)
	}

	clusterScope.CloudscaleCluster.Status.FloatingIP = fip.IP()
	r.setControlPlaneEndpointFromFIP(clusterScope, fip)
	return r.ensureFloatingIPAssignment(ctx, clusterScope, fip)
}

func (r *CloudscaleClusterReconciler) reconcileManagedFloatingIP(ctx context.Context, clusterScope *scope.ClusterScope, fipSpec *infrastructurev1beta2.FloatingIPSpec) (ctrl.Result, error) {
	tags := clusterOwnershipTags(clusterScope.CloudscaleCluster)

	clusterScope.Info("reconcile managed floating IP")

	// Check if the floating IP already exists (by status ID or by tags)
	fip, id, err := ensureResource(ctx, clusterScope,
		clusterScope.CloudscaleCluster.Status.FloatingIP,
		"floating IP",
		clusterScope.CloudscaleClient.FloatingIPs,
		func(fip cloudscalesdk.FloatingIP) string { return fip.IP() },
		tags,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	clusterScope.CloudscaleCluster.Status.FloatingIP = id

	if id != "" {
		// Existing floating IP: ensure it's assigned to the right target and set endpoint
		if err := r.ensureFloatingIPAssignment(ctx, clusterScope, fip); err != nil {
			return ctrl.Result{}, err
		}
		r.setControlPlaneEndpointFromFIP(clusterScope, fip)
		return ctrl.Result{}, nil
	}

	// Create new floating IP
	ipVersion := 4
	if fipSpec.IPFamily != nil && *fipSpec.IPFamily == infrastructurev1beta2.IPFamilyIPv6 {
		ipVersion = 6
	}

	req := &cloudscalesdk.FloatingIPCreateRequest{
		IPVersion: ipVersion,
		RegionalResourceRequest: cloudscalesdk.RegionalResourceRequest{
			Region: clusterScope.CloudscaleCluster.Spec.Region,
		},
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: ptr.To(tags),
		},
	}

	// Assign to LB or CP server.
	// If the target is not ready yet, create the FIP unassigned anyway.
	// On the next reconcile, ensureResource finds it by tag and
	// ensureFloatingIPAssignment attaches it to the correct target.
	target, err := r.getFloatingIPTarget(ctx, clusterScope)
	if err != nil {
		clusterScope.Info("no target available yet", "err", err)
	}
	if target.lbUUID != "" {
		req.LoadBalancer = target.lbUUID
	} else if target.serverUUID != "" {
		req.Server = target.serverUUID
	}

	clusterScope.Info("Creating floating IP", "ipVersion", ipVersion, "target", target)
	createCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
	defer cancel()
	fip, err = clusterScope.CloudscaleClient.FloatingIPs.Create(createCtx, req)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			clusterScope.Info("Floating IP creation timed out, waiting before retry", "requeueAfter", createFloatingIPTimeoutRequeueAfter)
			return ctrl.Result{RequeueAfter: createFloatingIPTimeoutRequeueAfter}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating floating IP: %w", err)
	}

	ip := fip.IP()
	clusterScope.CloudscaleCluster.Status.FloatingIP = ip
	clusterScope.Info("Created floating IP", "network", fip.Network, "ip", ip)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "FloatingIPCreated", "CreateFloatingIP",
		"Created floating IP %s", ip)

	r.setControlPlaneEndpointFromFIP(clusterScope, fip)
	return ctrl.Result{}, nil
}

type floatingIPTarget struct {
	lbUUID     string
	serverUUID string
}

func (t floatingIPTarget) String() string {
	if t.lbUUID != "" {
		return "lb:" + t.lbUUID
	}
	return "server:" + t.serverUUID
}

// getFloatingIPTarget returns the target to assign the floating IP to.
// If LB is enabled, targets the LB. Otherwise, targets the first ready CP server.
func (r *CloudscaleClusterReconciler) getFloatingIPTarget(ctx context.Context, clusterScope *scope.ClusterScope) (floatingIPTarget, error) {
	if ptr.Deref(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		lbID := clusterScope.CloudscaleCluster.Status.LoadBalancerID
		if lbID == "" {
			return floatingIPTarget{}, fmt.Errorf("waiting for load balancer to be provisioned")
		}
		return floatingIPTarget{lbUUID: lbID}, nil
	}

	// LB disabled (Pre-existing FIP without LB): find the first ready CP server.
	// The user is responsible for configuring a dummy interface with the FIP address
	// on their control plane servers (see cloudscale.ch docs).
	machineList := &infrastructurev1beta2.CloudscaleMachineList{}
	if err := r.List(ctx, machineList,
		client.InNamespace(clusterScope.CloudscaleCluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:         clusterScope.Cluster.Name,
			clusterv1.MachineControlPlaneLabel: "",
		},
	); err != nil {
		return floatingIPTarget{}, fmt.Errorf("listing CP machines: %w", err)
	}

	for _, machine := range machineList.Items {
		if machine.Status.ServerID != "" {
			return floatingIPTarget{serverUUID: machine.Status.ServerID}, nil
		}
	}

	return floatingIPTarget{}, fmt.Errorf("waiting for a control plane server to be provisioned")
}

// ensureFloatingIPAssignment verifies the FIP is assigned to the correct target and reassigns if needed.
func (r *CloudscaleClusterReconciler) ensureFloatingIPAssignment(ctx context.Context, clusterScope *scope.ClusterScope, fip *cloudscalesdk.FloatingIP) error {
	target, err := r.getFloatingIPTarget(ctx, clusterScope)
	if err != nil {
		// Target not ready yet, leave current assignment
		return nil
	}

	needsUpdate := false
	updateReq := &cloudscalesdk.FloatingIPUpdateRequest{}

	if target.lbUUID != "" && (fip.LoadBalancer == nil || fip.LoadBalancer.UUID != target.lbUUID) {
		updateReq.LoadBalancer = target.lbUUID
		needsUpdate = true
	} else if target.serverUUID != "" && (fip.Server == nil || fip.Server.UUID != target.serverUUID) {
		updateReq.Server = target.serverUUID
		needsUpdate = true
	}

	if needsUpdate {
		floatingIP := clusterScope.CloudscaleCluster.Status.FloatingIP
		clusterScope.Info("Reassigning floating IP", "ip", floatingIP, "target", target)
		updateCtx, cancel := context.WithTimeout(ctx, cloudscale.WriteTimeout)
		defer cancel()
		if err := clusterScope.CloudscaleClient.FloatingIPs.Update(updateCtx, floatingIP, updateReq); err != nil {
			if cloudscale.IsFloatingIPNoPublicInterface(err) {
				return fmt.Errorf("floating IP cannot be assigned to control plane server: server must have a public interface when the load balancer is disabled; add {type: public} to the control-plane machine template interfaces")
			}
			return fmt.Errorf("updating floating IP assignment: %w", err)
		}
		r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "FloatingIPReassigned", "UpdateFloatingIP",
			"Reassigned floating IP %s to %s", floatingIP, target)
	}

	return nil
}

func (r *CloudscaleClusterReconciler) setControlPlaneEndpointFromFIP(clusterScope *scope.ClusterScope, fip *cloudscalesdk.FloatingIP) {
	if clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host != "" {
		return
	}

	floatingIP := fip.IP()

	apiServerPort := clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.APIServerPort
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = floatingIP
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port = apiServerPort
	clusterScope.Info("Set control plane endpoint from floating IP",
		"endpoint", floatingIP, "port", apiServerPort)
	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "ControlPlaneSet", "SetControlPlaneEndpoint",
		"Control plane endpoint set to %s:%d (floating IP)", floatingIP, apiServerPort)
}

// deleteFloatingIP deletes the floating IP if it's managed.
// Pre-existing floating IPs are left untouched.
func (r *CloudscaleClusterReconciler) deleteFloatingIP(ctx context.Context, clusterScope *scope.ClusterScope) (reterr error) {
	fipSpec := clusterScope.CloudscaleCluster.Spec.FloatingIP
	if fipSpec == nil {
		return nil
	}

	// Pre-existing floating IPs are not deleted; skip before registering the defer
	// so the condition is not set to "Deleting" for an untouched resource.
	if fipSpec.Address != "" {
		clusterScope.Info("Skipping pre-existing floating IP deletion", "address", fipSpec.Address)
		return nil
	}

	defer func() {
		if reterr != nil {
			r.setCondition(clusterScope, infrastructurev1beta2.FloatingIPReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.FloatingIPErrorReason, fmt.Sprintf("Failed to delete floating IP: %v", reterr))
		} else {
			r.setCondition(clusterScope, infrastructurev1beta2.FloatingIPReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.FloatingIPDeletingReason, "Floating IP has been deleted")
		}
	}()

	floatingIP := clusterScope.CloudscaleCluster.Status.FloatingIP
	if floatingIP == "" {
		return nil
	}

	clusterScope.Info("Deleting floating IP", "id", floatingIP)
	deleteCtx, cancel := context.WithTimeout(ctx, cloudscale.DeleteTimeout)
	defer cancel()
	if err := clusterScope.CloudscaleClient.FloatingIPs.Delete(deleteCtx, floatingIP); err != nil {
		if !cloudscale.IsNotFound(err) {
			return fmt.Errorf("deleting floating IP: %w", err)
		}
		clusterScope.Info("Floating IP already deleted", "id", floatingIP)
	}

	r.recorder.Eventf(clusterScope.CloudscaleCluster, nil, corev1.EventTypeNormal, "FloatingIPDeleted", "DeleteFloatingIP",
		"Deleted floating IP %s", floatingIP)
	clusterScope.CloudscaleCluster.Status.FloatingIP = ""

	return nil
}
