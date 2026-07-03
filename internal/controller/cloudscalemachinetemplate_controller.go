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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// CloudscaleMachineTemplateReconciler reconciles a CloudscaleMachineTemplate object
type CloudscaleMachineTemplateReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	FlavorInfo *cloudscale.FlavorInfo
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachinetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=cloudscalemachinetemplates/status,verbs=get;update;patch

// Reconcile populates the status.capacity and status.nodeInfo fields.
func (r *CloudscaleMachineTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Fetch the CloudscaleMachineTemplate
	template := &infrastructurev1beta2.CloudscaleMachineTemplate{}
	if err := r.Get(ctx, req.NamespacedName, template); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Skip if FlavorInfo is not available
	if r.FlavorInfo == nil {
		logger.Info("FlavorInfo not available, skipping status update")
		return ctrl.Result{}, nil
	}

	// Create patch helper (snapshots current state)
	patchHelper, err := patch.NewHelper(template, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get the flavor from the spec
	flavor := template.Spec.Template.Spec.Flavor
	// flavor is already validated in webhook - condition only here for defensive coding reasons
	if flavor == "" {
		return ctrl.Result{}, nil
	}

	// Get capacity for the flavor
	capacity, err := r.FlavorInfo.GetCapacity(flavor)
	if err != nil {
		// Unknown flavor - don't populate capacity
		// flavor is already validated in webhook - this branch is only here for defensive coding reasons
		logger.Info("Unknown flavor, skipping status update", "flavor", flavor, "error", err)
		return ctrl.Result{}, nil
	}

	template.Status.Capacity = capacity

	// This patch is only reached on the success path, so advance
	// status.observedGeneration to the reconciled generation.
	if err := patchHelper.Patch(ctx, template, patch.WithStatusObservedGeneration{}); err != nil {
		logger.Error(err, "Failed to patch CloudscaleMachineTemplate status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudscaleMachineTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.CloudscaleMachineTemplate{}).
		Named("cloudscalemachinetemplate").
		Complete(r)
}
