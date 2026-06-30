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
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/credentials"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/observability"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// CloudscaleClusterReconciler reconciles a CloudscaleCluster object
type CloudscaleClusterReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	recorder                events.EventRecorder
	WatchFilter             string
	Transport               http.RoundTripper
	Version                 string
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachines,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile handles CloudscaleCluster reconciliation.
func (r *CloudscaleClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ctx, logger, done := observability.StartSpanWithLogger(ctx,
		"controllers.CloudscaleClusterReconciler.Reconcile",
		attribute.String("namespace", req.Namespace),
		attribute.String("name", req.Name),
	)
	defer done()

	cloudscaleCluster := &infrastructurev1beta2.CloudscaleCluster{}
	if err := r.Get(ctx, req.NamespacedName, cloudscaleCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, cloudscaleCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, cloudscaleCluster) {
		logger.Info("CloudscaleCluster or owning Cluster is paused, skipping reconciliation")
		conditions.Set(cloudscaleCluster, metav1.Condition{
			Type:   infrastructurev1beta2.PausedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrastructurev1beta2.PausedReason,
		})
		return ctrl.Result{}, nil
	}

	// Clear Paused condition if it was previously set
	conditions.Delete(cloudscaleCluster, infrastructurev1beta2.PausedCondition)

	token, err := credentials.GetToken(ctx, r.Client, cloudscaleCluster.CredentialsSecretRef(), cloudscaleCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get cloudscale.ch credentials: %w", err)
	}

	cloudscaleClient := cloudscale.NewClient(token, r.Version, r.Transport)

	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:            r.Client,
		Logger:            logger,
		Cluster:           cluster,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleClient:  cloudscaleClient,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create cluster scope: %w", err)
	}

	defer func() {
		// Use a separate context for the status patch so it succeeds even
		// when the reconcile context has timed out.
		patchCtx, patchCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer patchCancel()
		if err := clusterScope.Close(patchCtx); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !cloudscaleCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, clusterScope)
	}

	if !controllerutil.ContainsFinalizer(cloudscaleCluster, infrastructurev1beta2.ClusterFinalizer) {
		controllerutil.AddFinalizer(cloudscaleCluster, infrastructurev1beta2.ClusterFinalizer)
	}

	return r.reconcileNormal(ctx, clusterScope)
}

// reconcileNormal handles normal reconciliation of cloudscale infrastructure.
func (r *CloudscaleClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.reconcileNormal")
	defer done()

	logger.Info("Reconciling CloudscaleCluster")
	// update ready conditions upon returning from this function based on updated clusterScope.
	defer r.setReadyCondition(clusterScope)

	result, err := r.reconcileNetwork(ctx, clusterScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling network: %w", err)
	}
	if !result.IsZero() {
		return result, nil
	}

	lb, result, err := r.reconcileLoadBalancer(ctx, clusterScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer: %w", err)
	}
	if !result.IsZero() {
		return result, nil
	}

	result, err = r.reconcileFloatingIP(ctx, clusterScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling floating IP: %w", err)
	}
	if !result.IsZero() {
		return result, nil
	}

	// Reconcile the control plane endpoint after the load balancer and floating IP have been reconciled.
	r.reconcileControlPlaneEndpoint(clusterScope, lb.ipAddress)

	// Mark infrastructure as provisioned when all resources exist
	if clusterScope.CloudscaleCluster.Status.Initialization == nil {
		clusterScope.CloudscaleCluster.Status.Initialization = &infrastructurev1beta2.ClusterInitializationStatus{}
	}
	provisioned := r.isInfrastructureProvisioned(clusterScope)
	clusterScope.CloudscaleCluster.Status.Initialization.Provisioned = new(provisioned)

	// ensure Cluster is re-queued, if the load-balancer is not ready or the control plane is not initialized yet.
	// This re-queuing needs to be done after controlPlaneEndpoint and provisioning status have been determined, because
	// they individually also contribute to the progression of a cluster provisioning.
	if !lb.ready || !ptr.Deref(clusterScope.Cluster.Status.Initialization.ControlPlaneInitialized, false) {
		clusterScope.Info("Waiting for control plane initialization", "lbReady", lb.ready, "controlPlaneInitialized", ptr.Deref(clusterScope.Cluster.Status.Initialization.ControlPlaneInitialized, false))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles deletion of cloudscale.ch infrastructure.
// Resources are deleted in reverse order of creation: Load Balancer -> Network.
//
//nolint:unparam // Returns ctrl.Result for consistency with reconcile pattern
func (r *CloudscaleClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	ctx, logger, done := observability.StartSpanWithLogger(ctx, "controllers.CloudscaleClusterReconciler.reconcileDelete")
	defer done()

	logger.Info("Reconciling CloudscaleCluster deletion")

	// Set Deleting condition
	r.setCondition(clusterScope, infrastructurev1beta2.DeletingCondition, metav1.ConditionTrue, infrastructurev1beta2.DeletingReason, "Deleting infrastructure resources")

	// Delete floating IP first (it may be assigned to the LB or a server)
	if err := r.deleteFloatingIP(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting floating IP: %w", err)
	}

	// Delete load balancer (it depends on the subnet)
	if err := r.deleteLoadBalancer(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting load balancer: %w", err)
	}

	if err := r.deleteServerGroups(ctx, clusterScope); err != nil {
		if errors.Is(err, errServerGroupNotEmpty) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("deleting server groups: %w", err)
	}

	if err := r.deleteNetwork(ctx, clusterScope); err != nil {
		if errors.Is(err, errNetworkNotReady) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("deleting network: %w", err)
	}

	// Set Ready to False since we're deleting
	r.setCondition(clusterScope, infrastructurev1beta2.ReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.DeletingReason, "Infrastructure has been deleted")

	controllerutil.RemoveFinalizer(clusterScope.CloudscaleCluster, infrastructurev1beta2.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// reconcileControlPlaneEndpoint ensures spec.ControlPlaneEndpoint is set to the right host/port combination
// at the end of the reconcile process when all dependant reconciliations have finished (lb and floatingIP).
// How it works:
//  1. a controlPlaneEndpoint once set, is never changed. The reconciler will be a no-op after setting it for the first time.
//  2. floatingIP has precedence over LB
//  3. host/port is only set once floatingIP or LB have finished provisioning
func (r *CloudscaleClusterReconciler) reconcileControlPlaneEndpoint(clusterScope *scope.ClusterScope, lbIPAddress string) {
	cluster := clusterScope.CloudscaleCluster

	// Already resolved (set explicitly by the user or on a previous reconcile).
	if cluster.Spec.ControlPlaneEndpoint.Host != "" {
		return
	}

	var host, source string
	switch {
	case cluster.Spec.FloatingIP != nil:
		host, source = cluster.Status.FloatingIP, "floating IP"
	case ptr.Deref(cluster.Spec.ControlPlaneLoadBalancer.Enabled, true):
		host, source = lbIPAddress, "load balancer"
	default:
		// External control plane: the endpoint must be provided by the user.
		return
	}

	if host == "" {
		// source is not provisioned yet; retry on the next reconcile.
		return
	}

	port := cluster.Spec.ControlPlaneLoadBalancer.APIServerPort
	cluster.Spec.ControlPlaneEndpoint.Host = host
	cluster.Spec.ControlPlaneEndpoint.Port = port

	clusterScope.Info("Set control plane endpoint", "endpoint", host, "port", port, "source", source)
	r.recorder.Eventf(cluster, nil, corev1.EventTypeNormal, "ControlPlaneSet", "SetControlPlaneEndpoint",
		"Control plane endpoint set to %s:%d (%s)", host, port, source)
}

// isInfrastructureProvisioned returns true if all cluster infrastructure is ready.
// This includes all Networks+Subnets, Load Balancer (with pool and listener) if enabled,
// Floating IP if configured, and Control Plane Endpoint.
func (r *CloudscaleClusterReconciler) isInfrastructureProvisioned(clusterScope *scope.ClusterScope) bool {
	// All networks must have both network and subnet IDs
	if len(clusterScope.CloudscaleCluster.Status.Networks) == 0 {
		return false
	}
	for _, ns := range clusterScope.CloudscaleCluster.Status.Networks {
		if ns.NetworkID == "" || ns.SubnetID == "" {
			return false
		}
	}

	// Load balancer, pool, and listener must exist (if LB is enabled)
	if ptr.Deref(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		if clusterScope.CloudscaleCluster.Status.LoadBalancerID == "" ||
			clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID == "" ||
			clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID == "" {
			return false
		}
	}

	// Floating IP must be provisioned if configured
	if clusterScope.CloudscaleCluster.Spec.FloatingIP != nil {
		if clusterScope.CloudscaleCluster.Status.FloatingIP == "" {
			return false
		}
	}

	// Control plane endpoint must be set (from LB VIP, floating IP, or externally)
	if clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host == "" {
		return false
	}

	return true
}

// setReadyCondition sets the Ready condition based on the sub-conditions.
func (r *CloudscaleClusterReconciler) setReadyCondition(clusterScope *scope.ClusterScope) {
	// Check all sub-conditions
	subConditions := []string{
		infrastructurev1beta2.NetworkReadyCondition,
		infrastructurev1beta2.LoadBalancerReadyCondition,
		infrastructurev1beta2.FloatingIPReadyCondition,
	}

	for _, condType := range subConditions {
		if !conditions.IsTrue(clusterScope.CloudscaleCluster, condType) {
			// Get the failing condition to use its reason/message
			cond := conditions.Get(clusterScope.CloudscaleCluster, condType)
			reason := infrastructurev1beta2.NotReadyReason
			message := "Waiting for " + condType
			if cond != nil {
				reason = cond.Reason
				message = cond.Message
			}
			r.setCondition(clusterScope, infrastructurev1beta2.ReadyCondition, metav1.ConditionFalse, reason, message)
			return
		}
	}

	// All sub-conditions are True
	r.setCondition(clusterScope, infrastructurev1beta2.ReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.ReadyReason, "")
}

// setCondition sets a condition on the CloudscaleCluster.
func (r *CloudscaleClusterReconciler) setCondition(clusterScope *scope.ClusterScope, condType string, status metav1.ConditionStatus, reason, message string) {
	conditions.Set(clusterScope.CloudscaleCluster, metav1.Condition{
		Type:    condType,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudscaleClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	logger := ctrl.LoggerFrom(ctx)

	r.recorder = mgr.GetEventRecorder("cloudscalecluster-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.CloudscaleCluster{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		WithEventFilter(predicates.ResourceNotPaused(r.Scheme, logger)).
		WithEventFilter(predicates.ResourceHasFilterLabel(r.Scheme, logger, r.WatchFilter)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(r.Scheme, logger)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrastructurev1beta2.SchemeGroupVersion.WithKind("CloudscaleCluster"), mgr.GetClient(), &infrastructurev1beta2.CloudscaleCluster{})),
			builder.WithPredicates(predicates.Any(r.Scheme, logger,
				// Resume reconciliation when the Cluster is paused/unpaused, and when
				// the control plane initializes so the LB health monitor gets added.
				predicates.ClusterPausedTransitions(r.Scheme, logger),
				predicates.ClusterControlPlaneInitialized(r.Scheme, logger),
			)),
		).
		Watches(
			&infrastructurev1beta2.CloudscaleMachine{},
			handler.EnqueueRequestsFromMapFunc(r.cloudscaleMachineToCluster(ctx, mgr.GetClient())),
		).
		Named("cloudscalecluster").
		Complete(r)
}

// cloudscaleMachineToCluster maps a CloudscaleMachine to the owning CloudscaleCluster.
// This is used to trigger cluster reconciliation when control plane machines change,
// so the control plane endpoint can be discovered from the first CP machine's public IP.
func (r *CloudscaleClusterReconciler) cloudscaleMachineToCluster(ctx context.Context, c client.Client) handler.MapFunc {
	return func(_ context.Context, o client.Object) []ctrl.Request {
		machine, ok := o.(*infrastructurev1beta2.CloudscaleMachine)
		if !ok {
			return nil
		}

		// Only care about control plane machines
		if _, ok := machine.Labels[clusterv1.MachineControlPlaneLabel]; !ok {
			return nil
		}

		// Get the cluster name from the machine's labels
		clusterName, ok := machine.Labels[clusterv1.ClusterNameLabel]
		if !ok {
			return nil
		}

		// Find the CloudscaleCluster in the same namespace
		clusterList := &infrastructurev1beta2.CloudscaleClusterList{}
		if err := c.List(ctx, clusterList,
			client.InNamespace(machine.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel: clusterName,
			},
		); err != nil {
			return nil
		}

		var requests []ctrl.Request
		for _, cluster := range clusterList.Items {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(&cluster),
			})
		}
		return requests
	}
}
