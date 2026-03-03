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

package v1beta2

import (
	"context"
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// cloudscaleclusterlog is for logging in this package.
var cloudscaleclusterlog = logf.Log.WithName("cloudscalecluster-resource")

// SetupCloudscaleClusterWebhookWithManager registers the webhook for CloudscaleCluster in the manager.
func SetupCloudscaleClusterWebhookWithManager(mgr ctrl.Manager, regionInfo *cloudscale.RegionInfo) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrastructurev1beta2.CloudscaleCluster{}).
		WithValidator(&CloudscaleClusterCustomValidator{
			RegionInfo: regionInfo,
		}).
		WithDefaulter(&CloudscaleClusterCustomDefaulter{
			RegionInfo: regionInfo,
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalecluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclusters,verbs=create;update,versions=v1beta2,name=mcloudscalecluster-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleClusterCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CloudscaleCluster when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CloudscaleClusterCustomDefaulter struct {
	RegionInfo *cloudscale.RegionInfo
}

const defaultSubnetCIDR = "10.0.0.0/24"

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CloudscaleCluster.
func (d *CloudscaleClusterCustomDefaulter) Default(_ context.Context, cluster *infrastructurev1beta2.CloudscaleCluster) error {
	cloudscaleclusterlog.Info("Defaulting for CloudscaleCluster", "name", cluster.GetName())

	// Default network zone to region's default zone if not set
	if cluster.Spec.Zone == "" {
		cluster.Spec.Zone = d.RegionInfo.GetDefaultZoneForRegion(cluster.Spec.Region)
	}

	// Default network CIDR if not set
	if cluster.Spec.Network.CIDR == "" {
		cluster.Spec.Network.CIDR = defaultSubnetCIDR
	}

	// Default gateway address to empty string (no gateway)
	// This ensures outbound internet traffic uses the public interface,
	// which is required for CCM to reach the cloudscale.ch API.
	if cluster.Spec.Network.GatewayAddress == nil {
		cluster.Spec.Network.GatewayAddress = ptr.To("")
	}

	// Default load balancer settings
	if cluster.Spec.ControlPlaneLoadBalancer.Enabled == nil {
		cluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
	}
	if cluster.Spec.ControlPlaneLoadBalancer.Algorithm == "" {
		cluster.Spec.ControlPlaneLoadBalancer.Algorithm = "round_robin"
	}
	if cluster.Spec.ControlPlaneLoadBalancer.Flavor == "" {
		cluster.Spec.ControlPlaneLoadBalancer.Flavor = "lb-standard"
	}
	if cluster.Spec.ControlPlaneLoadBalancer.APIServerPort == 0 {
		cluster.Spec.ControlPlaneLoadBalancer.APIServerPort = 6443
	}
	if cluster.Spec.ControlPlaneLoadBalancer.Algorithm == "" {
		cluster.Spec.ControlPlaneLoadBalancer.Algorithm = "round_robin"
	}

	if cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS == 0 {
		cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 5
	}
	if cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS == 0 {
		cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS = 3
	}
	if cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold == 0 {
		cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold = 2
	}
	if cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold == 0 {
		cluster.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold = 3
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-cloudscalecluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=cloudscaleclusters,verbs=create;update,versions=v1beta2,name=vcloudscalecluster-v1beta2.kb.io,admissionReviewVersions=v1

// CloudscaleClusterCustomValidator struct is responsible for validating the CloudscaleCluster resource
// when it is created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CloudscaleClusterCustomValidator struct {
	RegionInfo *cloudscale.RegionInfo
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleCluster.
func (v *CloudscaleClusterCustomValidator) ValidateCreate(_ context.Context, cluster *infrastructurev1beta2.CloudscaleCluster) (admission.Warnings, error) {
	cloudscaleclusterlog.Info("Validation for CloudscaleCluster upon creation", "name", cluster.GetName())

	var allErrs field.ErrorList

	// Validate zone belongs to region
	if cluster.Spec.Zone != "" {
		if !v.RegionInfo.ZoneBelongsToRegion(cluster.Spec.Zone, cluster.Spec.Region) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "zone"),
				cluster.Spec.Zone,
				fmt.Sprintf("zone must belong to region %q", cluster.Spec.Region)))
		}
	}

	// Validate gateway address is within CIDR if specified
	if cluster.Spec.Network.GatewayAddress != nil && *cluster.Spec.Network.GatewayAddress != "" {
		allErrs = append(allErrs, validateGatewayInCIDR(
			cluster.Spec.Network.CIDR,
			*cluster.Spec.Network.GatewayAddress,
			field.NewPath("spec", "network", "gatewayAddress"),
		)...)
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.GroupVersion.Group, Kind: "CloudscaleCluster"},
			cluster.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleCluster.
func (v *CloudscaleClusterCustomValidator) ValidateUpdate(_ context.Context, oldCluster, newCluster *infrastructurev1beta2.CloudscaleCluster) (admission.Warnings, error) {
	cloudscaleclusterlog.Info("Validation for CloudscaleCluster upon update", "name", newCluster.GetName())

	var allErrs field.ErrorList

	// Region is immutable
	if newCluster.Spec.Region != oldCluster.Spec.Region {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "region"),
			"field is immutable after cluster creation"))
	}

	// Network zone is immutable
	if newCluster.Spec.Zone != oldCluster.Spec.Zone {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "zone"),
			"field is immutable after cluster creation"))
	}

	// Network CIDR is immutable
	if newCluster.Spec.Network.CIDR != oldCluster.Spec.Network.CIDR {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "network", "cidr"),
			"field is immutable after cluster creation"))
	}

	// LoadBalancer Enabled is immutable
	if ptr.Deref(newCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) != ptr.Deref(oldCluster.Spec.ControlPlaneLoadBalancer.Enabled, true) {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "controlPlaneLoadBalancer", "enabled"),
			"field is immutable after cluster creation"))
	}

	// ControlPlaneEndpoint is immutable once set
	if oldCluster.Spec.ControlPlaneEndpoint.Host != "" {
		if newCluster.Spec.ControlPlaneEndpoint.Host != oldCluster.Spec.ControlPlaneEndpoint.Host {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlaneEndpoint", "host"),
				"field is immutable once set"))
		}
		if newCluster.Spec.ControlPlaneEndpoint.Port != oldCluster.Spec.ControlPlaneEndpoint.Port {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlaneEndpoint", "port"),
				"field is immutable once set"))
		}
	}

	// GatewayAddress is immutable
	oldGateway := ""
	newGateway := ""
	if oldCluster.Spec.Network.GatewayAddress != nil {
		oldGateway = *oldCluster.Spec.Network.GatewayAddress
	}
	if newCluster.Spec.Network.GatewayAddress != nil {
		newGateway = *newCluster.Spec.Network.GatewayAddress
	}
	if oldGateway != newGateway {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "network", "gatewayAddress"),
			"field is immutable after cluster creation"))
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.GroupVersion.Group, Kind: "CloudscaleCluster"},
			newCluster.Name, allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleCluster.
func (v *CloudscaleClusterCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1beta2.CloudscaleCluster) (admission.Warnings, error) {
	cloudscaleclusterlog.Info("Validation for CloudscaleCluster upon deletion", "name", obj.GetName())
	return nil, nil
}

// validateGatewayInCIDR validates that the gateway address is within the specified CIDR.
func validateGatewayInCIDR(cidr, gateway string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		// CIDR validation should have caught this earlier
		return allErrs
	}

	gatewayIP := net.ParseIP(gateway)
	if gatewayIP == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, gateway, "invalid IP address"))
		return allErrs
	}

	if !ipNet.Contains(gatewayIP) {
		allErrs = append(allErrs, field.Invalid(fldPath, gateway,
			fmt.Sprintf("gateway must be within CIDR %s", cidr)))
	}

	return allErrs
}
