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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/credentials"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// CloudscaleClusterReconciler reconciles a CloudscaleCluster object
type CloudscaleClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	recorder    events.EventRecorder
	WatchFilter string
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
	logger := logf.FromContext(ctx)

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

	cloudscaleClient := cloudscale.NewClient(token)

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
		if err := clusterScope.Close(ctx); err != nil && reterr == nil {
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
	clusterScope.Info("Reconciling CloudscaleCluster")
	// update ready conditions upon returning from this function based on updated clusterScope.
	defer r.setReadyCondition(clusterScope)

	if err := r.reconcileNetwork(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling network: %w", err)
	}

	result, err := r.reconcileLoadBalancer(ctx, clusterScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling load balancer: %w", err)
	}
	if !result.IsZero() {
		return result, nil
	}

	// Mark infrastructure as provisioned when all resources exist
	if clusterScope.CloudscaleCluster.Status.Initialization == nil {
		clusterScope.CloudscaleCluster.Status.Initialization = &infrastructurev1beta2.ClusterInitializationStatus{}
	}
	provisioned := r.isInfrastructureProvisioned(clusterScope)
	clusterScope.CloudscaleCluster.Status.Initialization.Provisioned = ptr.To(provisioned)

	// Set Ready condition based on all sub-conditions
	r.setReadyCondition(clusterScope)

	return ctrl.Result{}, nil
}

// reconcileDelete handles deletion of cloudscale.ch infrastructure.
// Resources are deleted in reverse order of creation: Load Balancer -> Network.
//
//nolint:unparam // Returns ctrl.Result for consistency with reconcile pattern
func (r *CloudscaleClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	clusterScope.Info("Reconciling CloudscaleCluster deletion")

	// Set Deleting condition
	r.setCondition(clusterScope, infrastructurev1beta2.DeletingCondition, metav1.ConditionTrue, infrastructurev1beta2.DeletingReason, "Deleting infrastructure resources")

	// Delete load balancer first (it depends on the subnet)
	if err := r.deleteLoadBalancer(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting load balancer: %w", err)
	}

	if err := r.deleteServerGroups(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting server groups: %w", err)
	}

	if err := r.deleteNetwork(ctx, clusterScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting network: %w", err)
	}

	// Set Ready to False since we're deleting
	r.setCondition(clusterScope, infrastructurev1beta2.ReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.DeletingReason, "Infrastructure has been deleted")

	controllerutil.RemoveFinalizer(clusterScope.CloudscaleCluster, infrastructurev1beta2.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// isInfrastructureProvisioned returns true if all cluster infrastructure is ready.
// This includes Network, Subnet, Load Balancer (with pool and listener) if enabled, and Control Plane Endpoint.
func (r *CloudscaleClusterReconciler) isInfrastructureProvisioned(clusterScope *scope.ClusterScope) bool {
	// Network and Subnet must exist
	if clusterScope.CloudscaleCluster.Status.NetworkID == "" ||
		clusterScope.CloudscaleCluster.Status.SubnetID == "" {
		return false
	}

	// Load balancer, pool, and listener must exist (if LB is enabled)
	if ptr.Deref(clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		if clusterScope.CloudscaleCluster.Status.LoadBalancerID == "" ||
			clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID == "" ||
			clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID == "" {
			return false
		}
	}

	// Control plane endpoint must be set (from LB VIP or externally)
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
		WithEventFilter(predicates.ResourceNotPaused(r.Scheme, logger)).
		WithEventFilter(predicates.ResourceHasFilterLabel(r.Scheme, logger, r.WatchFilter)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(r.Scheme, logger)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrastructurev1beta2.SchemeGroupVersion.WithKind("CloudscaleCluster"), mgr.GetClient(), &infrastructurev1beta2.CloudscaleCluster{})),
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
		if _, ok := machine.Labels[clusterv1.MachineControlPlaneNameLabel]; !ok {
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
