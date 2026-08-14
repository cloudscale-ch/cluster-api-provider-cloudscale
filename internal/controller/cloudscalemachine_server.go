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
	"maps"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/observability"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// InterfaceTypePublic is the cloudscale.ch SDK value for a public network interface,
// used both as the Server.Interfaces[].Type and as InterfaceRequest.Network.
const InterfaceTypePublic = "public"

// ServerStatus represents the status of a cloudscale.ch server.
type ServerStatus string

// Server status constants from cloudscale.ch API.
const (
	// ServerStatusChanging indicates the server is being created or modified.
	ServerStatusChanging ServerStatus = "changing"

	// ServerStatusRunning indicates the server is powered on and ready.
	ServerStatusRunning ServerStatus = "running"

	// ServerStatusStopped indicates the server is powered off.
	ServerStatusStopped ServerStatus = "stopped"

	// ServerStatusPaused indicates the server has been paused.
	ServerStatusPaused ServerStatus = "paused"

	// ServerStatusRescueRunning indicates rescue mode while powered on.
	ServerStatusRescueRunning ServerStatus = "rescue_running"

	// ServerStatusRescueStopped indicates rescue mode while stopped.
	ServerStatusRescueStopped ServerStatus = "rescue_stopped"

	// ServerStatusError indicates an internal error.
	ServerStatusError ServerStatus = "error"

	// ServerStatusUnknown indicates an internal error.
	ServerStatusUnknown ServerStatus = "unknown"
)

func (r *CloudscaleMachineReconciler) reconcileServer(ctx context.Context, machineScope *scope.MachineScope) (_ ctrl.Result, reterr error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleMachineReconciler.reconcileServer")
	defer done()

	logger.Info("Reconciling server")

	var server *cloudscalesdk.Server
	defer func() {
		if reterr != nil {
			r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerErrorReason, reterr.Error())
		} else if server != nil {
			r.setServerStatusCondition(machineScope, server)
		}
	}()

	machineScope.Info("reconciling server", "name", machineScope.Machine.Name, "namespace", machineScope.Machine.Namespace, "server", machineScope.CloudscaleMachine.Status.ServerID)

	// 1. If we have a server ID in status, verify it still exists
	if machineScope.CloudscaleMachine.Status.ServerID != "" {
		var err error
		getCtx, cancel := context.WithTimeout(ctx, cloudscale.ReadTimeout)
		defer cancel()
		server, err = machineScope.CloudscaleClient.Servers.Get(getCtx, machineScope.CloudscaleMachine.Status.ServerID)
		if err == nil {
			machineScope.V(2).Info("Server already exists", "serverID", machineScope.CloudscaleMachine.Status.ServerID, "status", server.Status)
			r.updateMachineFromServer(machineScope, server)
			if ServerStatus(server.Status) == ServerStatusChanging {
				return ctrl.Result{RequeueAfter: ServerStatusPollInterval}, nil
			}
			return ctrl.Result{}, nil
		}
		// Server was deleted externally - set condition and return (no error)
		if cloudscale.IsNotFound(err) {
			machineScope.Error(err, "Server was deleted externally", "serverID", machineScope.CloudscaleMachine.Status.ServerID)
			r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerDeletedExternallyReason, fmt.Sprintf("Server %s was deleted outside of CAPI", machineScope.CloudscaleMachine.Status.ServerID))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting server: %w", err)
	}

	// 2. Search for existing server by tag (idempotency after crash)
	listCtx, cancelList := context.WithTimeout(ctx, cloudscale.ReadTimeout)
	defer cancelList()
	servers, err := machineScope.CloudscaleClient.Servers.List(listCtx,
		cloudscalesdk.WithTagFilter(r.machineLookupTag(machineScope)))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("listing servers: %w", err)
	}
	if len(servers) > 1 {
		return ctrl.Result{}, fmt.Errorf("found %d servers matching tag filter, expected at most 1", len(servers))
	}
	if len(servers) == 1 {
		server = &servers[0]
		machineScope.CloudscaleMachine.Status.ServerID = server.UUID
		machineScope.Info("Found existing server by tag", "serverID", server.UUID, "status", server.Status)
		r.updateMachineFromServer(machineScope, server)
		if ServerStatus(server.Status) == ServerStatusChanging {
			return ctrl.Result{RequeueAfter: ServerStatusPollInterval}, nil
		}
		return ctrl.Result{}, nil
	}

	// 3. Create new server
	zone := machineScope.CloudscaleCluster.Spec.Zone
	machineScope.Info("Creating server", "zone", zone, "flavor", machineScope.CloudscaleMachine.Spec.Flavor)

	// Get bootstrap data
	bootstrapData, err := machineScope.GetBootstrapData(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting bootstrap data: %w", err)
	}

	// Build network interfaces
	interfaces, ipFamily, err := r.buildInterfaceRequests(machineScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("building interface requests: %w", err)
	}

	// Build server request
	req := &cloudscalesdk.ServerRequest{
		Name:   machineScope.Name(),
		Flavor: machineScope.CloudscaleMachine.Spec.Flavor,
		Image:  machineScope.CloudscaleMachine.Spec.Image,
		Zone:   zone,
		TaggedResourceRequest: cloudscalesdk.TaggedResourceRequest{
			Tags: r.machineCreateTags(machineScope),
		},
		UserData:   bootstrapData,
		Interfaces: interfaces,
		UseIPV6:    ipFamilyToUseIPV6(ipFamily),
		// sending nil doesn't work, we need to explicitly send an empty slice
		SSHKeys: []string{},
	}

	if machineScope.CloudscaleMachine.Spec.RootVolumeSize > 0 {
		req.VolumeSizeGB = machineScope.CloudscaleMachine.Spec.RootVolumeSize
	}

	if machineScope.CloudscaleMachine.Status.ServerGroupID != "" {
		req.ServerGroups = []string{machineScope.CloudscaleMachine.Status.ServerGroupID}
	}

	createCtx, cancelCreate := context.WithTimeout(ctx, cloudscale.WriteTimeout)
	defer cancelCreate()
	server, err = machineScope.CloudscaleClient.Servers.Create(createCtx, req)
	if err != nil {
		if cloudscale.IsTimeoutError(err) {
			requeueAfter := 30 * time.Second
			machineScope.Info("Server creation timed out, waiting before retry", "requeueAfter", requeueAfter)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return ctrl.Result{}, fmt.Errorf("creating server: %w", err)
	}

	machineScope.CloudscaleMachine.Status.ServerID = server.UUID
	machineScope.Info("Created server", "serverID", server.UUID, "status", server.Status)
	r.recorder.Eventf(machineScope.CloudscaleMachine, nil, corev1.EventTypeNormal, "ServerCreated", "CreateServer", "Created server %s in zone %s", server.UUID, zone)

	r.updateMachineFromServer(machineScope, server)

	if ServerStatus(server.Status) == ServerStatusChanging {
		return ctrl.Result{RequeueAfter: ServerStatusPollInterval}, nil
	}
	return ctrl.Result{}, nil
}

// setServerStatusCondition sets the ServerReadyCondition based on the server's current status.
func (r *CloudscaleMachineReconciler) setServerStatusCondition(machineScope *scope.MachineScope, server *cloudscalesdk.Server) {
	status := ServerStatus(server.Status)
	isAlreadyProvisioned := machineScope.CloudscaleMachine.Status.Initialization != nil &&
		ptr.Deref(machineScope.CloudscaleMachine.Status.Initialization.Provisioned, false)

	switch status {
	case ServerStatusRunning:
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.ServerRunningReason, "")

	case ServerStatusChanging:
		reason := infrastructurev1beta2.ServerStartingReason
		message := "Server is being created"
		if isAlreadyProvisioned {
			reason = infrastructurev1beta2.ServerChangingReason
			message = "Server is transitioning"
		}
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, reason, message)

	case ServerStatusStopped:
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerStoppedReason, "Server is stopped")

	case ServerStatusRescueRunning, ServerStatusRescueStopped:
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerInRescueModeReason, fmt.Sprintf("Server is in rescue mode (status: %s)", status))

	case ServerStatusPaused, ServerStatusError, ServerStatusUnknown:
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerInternalErrorReason, fmt.Sprintf("Server has internal issue (status: %s), contact cloudscale.ch support", status))

	default:
		machineScope.Info("Unknown server status", "status", status)
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerErrorReason, fmt.Sprintf("Unknown server status: %s", status))
	}
}

func (r *CloudscaleMachineReconciler) updateMachineFromServer(machineScope *scope.MachineScope, server *cloudscalesdk.Server) {
	// Set provider ID
	machineScope.SetProviderID(server.UUID)

	// Set addresses - preallocate to number of interfaces times two (ipv4 and ipv6 address per interface)
	addresses := make([]clusterv1.MachineAddress, 0, len(server.Interfaces)*2)
	for _, iface := range server.Interfaces {
		addressType := clusterv1.MachineInternalIP
		if iface.Type == InterfaceTypePublic {
			addressType = clusterv1.MachineExternalIP
		}
		for _, addr := range iface.Addresses {
			addresses = append(addresses, clusterv1.MachineAddress{
				Type:    addressType,
				Address: addr.Address,
			})
		}
	}
	machineScope.CloudscaleMachine.Status.Addresses = addresses
}

func (r *CloudscaleMachineReconciler) deleteServer(ctx context.Context, machineScope *scope.MachineScope) (reterr error) {
	defer func() {
		if reterr != nil {
			r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerErrorReason, reterr.Error())
		} else {
			r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.ServerDeletingReason, "Server has been deleted")
		}
	}()

	if machineScope.CloudscaleMachine.Status.ServerID == "" {
		return nil
	}

	serverID := machineScope.CloudscaleMachine.Status.ServerID
	machineScope.Info("Deleting server", "serverID", serverID)

	deleteCtx, cancel := context.WithTimeout(ctx, cloudscale.DeleteTimeout)
	defer cancel()
	if err := machineScope.CloudscaleClient.Servers.Delete(deleteCtx, serverID); err != nil {
		// Ignore 404 - server was already deleted externally
		if !cloudscale.IsNotFound(err) {
			return fmt.Errorf("deleting server: %w", err)
		}
		machineScope.Info("Server already deleted", "serverID", serverID)
	}

	r.recorder.Eventf(machineScope.CloudscaleMachine, nil, corev1.EventTypeNormal, "ServerDeleted", "DeleteServer",
		"Deleted server %s", serverID)
	machineScope.CloudscaleMachine.Status.ServerID = ""
	return nil
}

// machineCreateTags returns the full tag set for creating machine resources
// (lookup tag + user-specified tags).
func (r *CloudscaleMachineReconciler) machineCreateTags(machineScope *scope.MachineScope) *cloudscalesdk.TagMap {
	tags := r.machineLookupTag(machineScope)

	// Merge user-specified tags
	maps.Copy(tags, machineScope.CloudscaleMachine.Spec.Tags)

	return &tags
}

// machineLookupTag returns the ownership tag used to find a machine's server by tag filter.
func (r *CloudscaleMachineReconciler) machineLookupTag(machineScope *scope.MachineScope) cloudscalesdk.TagMap {
	tags := cloudscalesdk.TagMap{
		machineScope.CloudscaleMachine.MachineTagKey(machineScope.Cluster.Name): fmt.Sprintf("%s/%s/%s", machineScope.Cluster.Namespace, machineScope.Cluster.Name, machineScope.CloudscaleMachine.Name),
	}
	return tags
}

// buildInterfaceRequests constructs the network interfaces for server creation.
// If spec.interfaces is empty, defaults to the first cluster network + a public interface
// (runtime cross-resource resolution that the webhook cannot do).
// Returns the interface requests and the IPFamily from the public interface (if any).
func (r *CloudscaleMachineReconciler) buildInterfaceRequests(machineScope *scope.MachineScope) (*[]cloudscalesdk.ServerInterfaceRequest, *infrastructurev1beta2.IPFamily, error) {
	ifaceSpecs := machineScope.CloudscaleMachine.Spec.Interfaces

	// Runtime default: first cluster network + public DualStack interface
	if len(ifaceSpecs) == 0 {
		if len(machineScope.CloudscaleCluster.Status.Networks) == 0 {
			return nil, nil, fmt.Errorf("cluster has no networks provisioned yet")
		}
		firstNetwork := machineScope.CloudscaleCluster.Status.Networks[0]
		return &[]cloudscalesdk.ServerInterfaceRequest{
			{Network: InterfaceTypePublic},
			{Network: firstNetwork.NetworkID},
		}, nil, nil
	}

	// Build from spec
	reqs := make([]cloudscalesdk.ServerInterfaceRequest, 0, len(ifaceSpecs))
	var ipFamily *infrastructurev1beta2.IPFamily
	for _, iface := range ifaceSpecs {
		switch {
		case iface.Type == InterfaceTypePublic:
			reqs = append(reqs, cloudscalesdk.ServerInterfaceRequest{Network: InterfaceTypePublic})
			ipFamily = iface.IPFamily
		case iface.Network != "":
			ns := machineScope.CloudscaleCluster.Status.GetNetworkStatus(iface.Network)
			if ns == nil {
				return nil, nil, fmt.Errorf("network %q not found in cluster status", iface.Network)
			}
			if ns.NetworkID == "" {
				return nil, nil, fmt.Errorf("network %q not yet provisioned", iface.Network)
			}
			reqs = append(reqs, cloudscalesdk.ServerInterfaceRequest{Network: ns.NetworkID})
		default:
			return nil, nil, fmt.Errorf("interface must have either type or network set")
		}
	}

	return &reqs, ipFamily, nil
}

// ipFamilyToUseIPV6 maps an IPFamily value to the cloudscale API's use_ipv6 server-level setting.
func ipFamilyToUseIPV6(ipFamily *infrastructurev1beta2.IPFamily) *bool {
	if ipFamily == nil {
		return nil
	}
	switch *ipFamily {
	case infrastructurev1beta2.IPFamilyDualStack, infrastructurev1beta2.IPFamilyIPv6:
		return new(true)
	case infrastructurev1beta2.IPFamilyIPv4:
		return new(false)
	default:
		return nil
	}
}
