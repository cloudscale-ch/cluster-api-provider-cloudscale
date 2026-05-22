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
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/credentials"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

const (
	// ServerStatusPollInterval is the requeue interval when waiting for server status.
	ServerStatusPollInterval = 5 * time.Second
)

// CloudscaleMachineReconciler reconciles a CloudscaleMachine object
type CloudscaleMachineReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	recorder                events.EventRecorder
	WatchFilter             string
	Transport               *http.Transport
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=list;watch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile handles CloudscaleMachine reconciliation.
func (r *CloudscaleMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	logger := logf.FromContext(ctx)

	cloudscaleMachine := &infrastructurev1beta2.CloudscaleMachine{}
	if err := r.Get(ctx, req.NamespacedName, cloudscaleMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, cloudscaleMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("machine", machine.Name)

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, cloudscaleMachine) {
		logger.Info("CloudscaleMachine or owning Cluster is paused, skipping reconciliation")
		r.setCondition(cloudscaleMachine, infrastructurev1beta2.PausedCondition, metav1.ConditionTrue, infrastructurev1beta2.PausedReason, "")
		return ctrl.Result{}, nil
	}

	// Clear Paused condition if it was previously set
	conditions.Delete(cloudscaleMachine, infrastructurev1beta2.PausedCondition)

	cloudscaleCluster := &infrastructurev1beta2.CloudscaleCluster{}
	cloudscaleClusterKey := client.ObjectKey{
		Namespace: cloudscaleMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Get(ctx, cloudscaleClusterKey, cloudscaleCluster); err != nil {
		logger.Info("CloudscaleCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Wait for infrastructure to be ready
	if cloudscaleCluster.Status.Initialization == nil ||
		cloudscaleCluster.Status.Initialization.Provisioned == nil ||
		!*cloudscaleCluster.Status.Initialization.Provisioned {
		logger.Info("CloudscaleCluster infrastructure is not ready yet")
		r.setCondition(cloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.WaitingForClusterInfrastructureReason, "Waiting for cluster infrastructure to be provisioned")
		r.setReadyCondition(cloudscaleMachine)
		return ctrl.Result{}, nil
	}

	token, err := credentials.GetToken(ctx, r.Client, cloudscaleCluster.CredentialsSecretRef(), cloudscaleCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get cloudscale.ch credentials: %w", err)
	}

	cloudscaleClient := cloudscale.NewClient(token, r.Transport)

	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client:            r.Client,
		Logger:            logger,
		Cluster:           cluster,
		Machine:           machine,
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient:  cloudscaleClient,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create machine scope: %w", err)
	}

	defer func() {
		// Use a separate context for the status patch so it succeeds even
		// when the reconcile context has timed out.
		patchCtx, patchCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer patchCancel()
		if err := machineScope.Close(patchCtx); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !cloudscaleMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machineScope)
	}

	if !controllerutil.ContainsFinalizer(cloudscaleMachine, infrastructurev1beta2.MachineFinalizer) {
		controllerutil.AddFinalizer(cloudscaleMachine, infrastructurev1beta2.MachineFinalizer)
	}

	return r.reconcileNormal(ctx, machineScope)
}

// reconcileNormal handles normal reconciliation of CloudscaleMachine.
func (r *CloudscaleMachineReconciler) reconcileNormal(ctx context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) {
	machineScope.Info("Reconciling CloudscaleMachine")

	defer r.setReadyCondition(machineScope.CloudscaleMachine)

	// Check if bootstrap data is ready
	if machineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		machineScope.Info("Bootstrap data is not ready yet")
		r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.WaitingForBootstrapDataReason, "Waiting for bootstrap data to be available")
		return ctrl.Result{}, nil
	}

	if machineScope.CloudscaleMachine.Spec.ServerGroup != nil {
		if result, err := r.reconcileServerGroup(ctx, machineScope); err != nil {
			return ctrl.Result{}, err
		} else if !result.IsZero() {
			return result, nil
		}
	}

	result, err := r.reconcileServer(ctx, machineScope)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling server: %w", err)
	}

	// Set Provisioned=true when server is ready (only once, never reverted)
	if conditions.IsTrue(machineScope.CloudscaleMachine, infrastructurev1beta2.ServerReadyCondition) {
		if machineScope.CloudscaleMachine.Status.Initialization == nil {
			machineScope.CloudscaleMachine.Status.Initialization = &infrastructurev1beta2.MachineInitializationStatus{}
		}
		machineScope.CloudscaleMachine.Status.Initialization.Provisioned = new(true)
	}

	return result, nil
}

// setCondition sets a condition on the CloudscaleMachine.
func (r *CloudscaleMachineReconciler) setCondition(machine *infrastructurev1beta2.CloudscaleMachine, condType string, status metav1.ConditionStatus, reason, message string) {
	conditions.Set(machine, metav1.Condition{
		Type:    condType,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// setReadyCondition derives the ReadyCondition from sub-conditions.
func (r *CloudscaleMachineReconciler) setReadyCondition(machine *infrastructurev1beta2.CloudscaleMachine) {
	subConditions := []string{
		infrastructurev1beta2.ServerReadyCondition,
	}
	for _, condType := range subConditions {
		if !conditions.IsTrue(machine, condType) {
			cond := conditions.Get(machine, condType)
			reason := infrastructurev1beta2.NotReadyReason
			message := "Waiting for " + condType
			if cond != nil {
				reason = cond.Reason
				message = cond.Message
			}
			r.setCondition(machine, infrastructurev1beta2.ReadyCondition, metav1.ConditionFalse, reason, message)
			return
		}
	}
	r.setCondition(machine, infrastructurev1beta2.ReadyCondition, metav1.ConditionTrue, infrastructurev1beta2.ReadyReason, "")
}

// reconcileDelete handles deletion of CloudscaleMachine.
//
//nolint:unparam // Returns ctrl.Result for consistency with reconcile pattern
func (r *CloudscaleMachineReconciler) reconcileDelete(ctx context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) {
	machineScope.Info("Reconciling CloudscaleMachine deletion")

	// Set Deleting condition
	r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.DeletingCondition, metav1.ConditionTrue, infrastructurev1beta2.DeletingReason, "Deleting server")

	if err := r.deleteServer(ctx, machineScope); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting server: %w", err)
	}

	r.deleteServerGroup(ctx, machineScope)

	r.setCondition(machineScope.CloudscaleMachine, infrastructurev1beta2.ReadyCondition, metav1.ConditionFalse, infrastructurev1beta2.DeletingReason, "Machine infrastructure has been deleted")

	controllerutil.RemoveFinalizer(machineScope.CloudscaleMachine, infrastructurev1beta2.MachineFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudscaleMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	logger := ctrl.LoggerFrom(ctx)

	r.recorder = mgr.GetEventRecorder("cloudscalemachine-controller")

	clusterToMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrastructurev1beta2.CloudscaleMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.CloudscaleMachine{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		WithEventFilter(predicates.ResourceNotPaused(r.Scheme, logger)).
		WithEventFilter(predicates.ResourceHasFilterLabel(r.Scheme, logger, r.WatchFilter)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(r.Scheme, logger)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrastructurev1beta2.SchemeGroupVersion.WithKind("CloudscaleMachine"))),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToMachines),
			builder.WithPredicates(
				predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(r.Scheme, logger),
				predicates.ResourceHasFilterLabel(r.Scheme, logger, r.WatchFilter),
			),
		).
		Watches(
			&infrastructurev1beta2.CloudscaleCluster{},
			handler.EnqueueRequestsFromMapFunc(r.cloudscaleClusterToCloudscaleMachines(ctx)),
		).
		Named("cloudscalemachine").
		Complete(r)
}

// cloudscaleClusterToCloudscaleMachines maps CloudscaleCluster events to CloudscaleMachines.
func (r *CloudscaleMachineReconciler) cloudscaleClusterToCloudscaleMachines(ctx context.Context) handler.MapFunc {
	logger := ctrl.LoggerFrom(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		cloudscaleCluster, ok := o.(*infrastructurev1beta2.CloudscaleCluster)
		if !ok {
			logger.Error(fmt.Errorf("expected a CloudscaleCluster but got a %T", o), "failed to get CloudscaleCluster")
			return nil
		}

		logger := logger.WithValues("CloudscaleCluster", cloudscaleCluster.Name, "Namespace", cloudscaleCluster.Namespace)

		// Don't handle deleted CloudscaleClusters
		if !cloudscaleCluster.DeletionTimestamp.IsZero() {
			logger.V(4).Info("CloudscaleCluster has a deletion timestamp, skipping mapping.")
			return nil
		}

		cluster, err := util.GetOwnerCluster(ctx, r.Client, cloudscaleCluster.ObjectMeta)
		if err != nil || cluster == nil {
			logger.Info("failed to get owner cluster for CloudscaleCluster", "error", err)
			return nil
		}

		machineList := &clusterv1.MachineList{}
		if err := r.List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{clusterv1.ClusterNameLabel: cluster.Name}); err != nil {
			return nil
		}

		var requests []ctrl.Request
		for _, machine := range machineList.Items {
			if machine.Spec.InfrastructureRef.Kind != "CloudscaleMachine" {
				continue
			}
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: machine.Namespace,
					Name:      machine.Spec.InfrastructureRef.Name,
				},
			})
		}
		return requests
	}
}
