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

const defaultSubnetCIDR = "172.18.0.0/24"

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CloudscaleCluster.
func (d *CloudscaleClusterCustomDefaulter) Default(_ context.Context, cluster *infrastructurev1beta2.CloudscaleCluster) error {
	cloudscaleclusterlog.Info("Defaulting for CloudscaleCluster", "name", cluster.GetName())

	// Zone and the default network name depend on the final object's identity
	// (region and metadata.Name respectively), so they're only defaulted here,
	// not in the shared template defaulter. ClusterClass-managed clusters get
	// their region from a topology patch and their name from the parent
	// Cluster — defaulting these at template-apply time would bake in values
	// tied to the template, which then get copied verbatim into every cluster.
	if cluster.Spec.Zone == "" {
		cluster.Spec.Zone = d.RegionInfo.GetDefaultZoneForRegion(cluster.Spec.Region)
	}
	if len(cluster.Spec.Networks) == 0 {
		cluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
			{
				Name: cluster.Name,
				CIDR: defaultSubnetCIDR,
			},
		}
	}

	clusterSpecDefault(&cluster.Spec)

	return nil
}

func clusterSpecDefault(spec *infrastructurev1beta2.CloudscaleClusterSpec) {
	// Default load balancer settings
	if spec.ControlPlaneLoadBalancer.Enabled == nil {
		spec.ControlPlaneLoadBalancer.Enabled = new(true)
	}
	if spec.ControlPlaneLoadBalancer.Algorithm == "" {
		spec.ControlPlaneLoadBalancer.Algorithm = "round_robin"
	}
	if spec.ControlPlaneLoadBalancer.Flavor == "" {
		spec.ControlPlaneLoadBalancer.Flavor = "lb-standard"
	}
	if spec.ControlPlaneLoadBalancer.APIServerPort == 0 {
		spec.ControlPlaneLoadBalancer.APIServerPort = 6443
	}

	if spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS == 0 {
		spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 5
	}
	if spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS == 0 {
		spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS = 3
	}
	if spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold == 0 {
		spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold = 2
	}
	if spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold == 0 {
		spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold = 3
	}

	// Default floating IP: if set but both fields empty, default to IPv4
	if spec.FloatingIP != nil && spec.FloatingIP.IPFamily == nil && spec.FloatingIP.Address == "" {
		spec.FloatingIP.IPFamily = new(infrastructurev1beta2.IPFamilyIPv4)
	}
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

	allErrs := clusterSpecValidateCreate(cluster.Spec, v.RegionInfo, field.NewPath("spec"))

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleCluster"},
			cluster.Name, allErrs)
	}

	return nil, nil
}

func clusterSpecValidateCreate(spec infrastructurev1beta2.CloudscaleClusterSpec, regionInfo *cloudscale.RegionInfo, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Validate zone belongs to region
	if spec.Zone != "" {
		if !regionInfo.ZoneBelongsToRegion(spec.Zone, spec.Region) {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("zone"),
				spec.Zone,
				fmt.Sprintf("zone must belong to region %q", spec.Region)))
		}
	}

	// Validate networks
	allErrs = append(allErrs, validateNetworks(spec.Networks, fldPath.Child("networks"))...)

	// Validate LB network reference
	if spec.ControlPlaneLoadBalancer.Network != "" {
		allErrs = append(allErrs, validateNetworkReference(
			spec.ControlPlaneLoadBalancer.Network,
			spec.Networks,
			fldPath.Child("controlPlaneLoadBalancer", "network"),
		)...)
	}

	// Validate floating IP
	if spec.FloatingIP != nil {
		allErrs = append(allErrs, validateFloatingIP(spec.FloatingIP, fldPath.Child("floatingIP"))...)
	}

	allErrs = append(allErrs, validateFloatingIPRequiresLBOrPreExisting(spec, fldPath)...)
	allErrs = append(allErrs, validateFloatingIPRequiresPublicLB(spec, fldPath)...)
	allErrs = append(allErrs, validateLBPoolMemberNetworkResolvable(spec, fldPath)...)
	return allErrs
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleCluster.
func (v *CloudscaleClusterCustomValidator) ValidateUpdate(_ context.Context, oldCluster, newCluster *infrastructurev1beta2.CloudscaleCluster) (admission.Warnings, error) {
	cloudscaleclusterlog.Info("Validation for CloudscaleCluster upon update", "name", newCluster.GetName())

	var allErrs field.ErrorList

	newClusterSpec := newCluster.Spec
	oldClusterSpec := oldCluster.Spec

	// Region is immutable
	if newClusterSpec.Region != oldClusterSpec.Region {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "region"),
			"field is immutable after cluster creation"))
	}

	// Zone is immutable
	if newClusterSpec.Zone != oldClusterSpec.Zone {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "zone"),
			"field is immutable after cluster creation"))
	}

	// Network immutability: existing networks cannot be modified or removed
	allErrs = append(allErrs, validateNetworkImmutability(oldClusterSpec.Networks, newClusterSpec.Networks, field.NewPath("spec", "networks"))...)

	// Validate new networks (new entries must still pass creation validation)
	allErrs = append(allErrs, validateNetworks(newClusterSpec.Networks, field.NewPath("spec", "networks"))...)

	// LoadBalancer Enabled is immutable
	if ptr.Deref(newClusterSpec.ControlPlaneLoadBalancer.Enabled, true) != ptr.Deref(oldClusterSpec.ControlPlaneLoadBalancer.Enabled, true) {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "controlPlaneLoadBalancer", "enabled"),
			"field is immutable after cluster creation"))
	}

	// LB network is immutable once set
	if oldClusterSpec.ControlPlaneLoadBalancer.Network != "" &&
		newClusterSpec.ControlPlaneLoadBalancer.Network != oldClusterSpec.ControlPlaneLoadBalancer.Network {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "controlPlaneLoadBalancer", "network"),
			"field is immutable once set"))
	}

	// Other LB fields are immutable: they're baked into the LB at creation
	// and changing them post-create would silently diverge from the live LB.
	allErrs = append(allErrs, validateLBImmutability(
		&oldClusterSpec.ControlPlaneLoadBalancer,
		&newClusterSpec.ControlPlaneLoadBalancer,
		field.NewPath("spec", "controlPlaneLoadBalancer"),
	)...)

	// Validate LB network reference (for new or existing)
	if newClusterSpec.ControlPlaneLoadBalancer.Network != "" {
		allErrs = append(allErrs, validateNetworkReference(
			newClusterSpec.ControlPlaneLoadBalancer.Network,
			newClusterSpec.Networks,
			field.NewPath("spec", "controlPlaneLoadBalancer", "network"),
		)...)
	}

	// ControlPlaneEndpoint is immutable once set
	if oldClusterSpec.ControlPlaneEndpoint.Host != "" {
		if newClusterSpec.ControlPlaneEndpoint.Host != oldClusterSpec.ControlPlaneEndpoint.Host {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlaneEndpoint", "host"),
				"field is immutable once set"))
		}
		if newClusterSpec.ControlPlaneEndpoint.Port != oldClusterSpec.ControlPlaneEndpoint.Port {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "controlPlaneEndpoint", "port"),
				"field is immutable once set"))
		}
	}

	// FloatingIP is immutable once set
	allErrs = append(allErrs, validateFloatingIPImmutability(oldClusterSpec.FloatingIP, newClusterSpec.FloatingIP, field.NewPath("spec", "floatingIP"))...)

	// Validate floating IP if set
	if newClusterSpec.FloatingIP != nil {
		allErrs = append(allErrs, validateFloatingIP(newClusterSpec.FloatingIP, field.NewPath("spec", "floatingIP"))...)
	}

	allErrs = append(allErrs, validateFloatingIPRequiresLBOrPreExisting(newClusterSpec, field.NewPath("spec"))...)
	allErrs = append(allErrs, validateFloatingIPRequiresPublicLB(newClusterSpec, field.NewPath("spec"))...)
	allErrs = append(allErrs, validateLBPoolMemberNetworkResolvable(newClusterSpec, field.NewPath("spec"))...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleCluster"},
			newCluster.Name, allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CloudscaleCluster.
func (v *CloudscaleClusterCustomValidator) ValidateDelete(_ context.Context, obj *infrastructurev1beta2.CloudscaleCluster) (admission.Warnings, error) {
	cloudscaleclusterlog.Info("Validation for CloudscaleCluster upon deletion", "name", obj.GetName())
	return nil, nil
}

// validateNetworks validates the network list.
func validateNetworks(networks []infrastructurev1beta2.NetworkSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	names := make(map[string]bool)
	for i, netSpec := range networks {
		netPath := fldPath.Index(i)

		// Unique names
		if names[netSpec.Name] {
			allErrs = append(allErrs, field.Duplicate(netPath.Child("name"), netSpec.Name))
		}
		names[netSpec.Name] = true

		// Exactly one of UUID or CIDR
		hasUUID := netSpec.UUID != ""
		hasCIDR := netSpec.CIDR != ""
		if hasUUID == hasCIDR {
			allErrs = append(allErrs, field.Invalid(netPath, netSpec.Name,
				"exactly one of uuid or cidr must be specified"))
		}

		// Validate CIDR format
		if hasCIDR {
			if _, _, err := net.ParseCIDR(netSpec.CIDR); err != nil {
				allErrs = append(allErrs, field.Invalid(netPath.Child("cidr"), netSpec.CIDR,
					fmt.Sprintf("invalid CIDR: %v", err)))
			}
		}

		// GatewayAddress only valid with CIDR
		if netSpec.GatewayAddress != "" && !hasCIDR {
			allErrs = append(allErrs, field.Invalid(netPath.Child("gatewayAddress"), netSpec.GatewayAddress,
				"gatewayAddress can only be set when cidr is specified"))
		}

		// Validate gateway is within CIDR
		if netSpec.GatewayAddress != "" && hasCIDR {
			allErrs = append(allErrs, validateGatewayInCIDR(
				netSpec.CIDR,
				netSpec.GatewayAddress,
				netPath.Child("gatewayAddress"),
			)...)
		}
	}

	return allErrs
}

// validateNetworkReference validates that a network name references a defined network.
func validateNetworkReference(networkName string, networks []infrastructurev1beta2.NetworkSpec, fldPath *field.Path) field.ErrorList {
	for _, n := range networks {
		if n.Name == networkName {
			return nil
		}
	}
	return field.ErrorList{
		field.NotFound(fldPath, networkName),
	}
}

// validateNetworkImmutability checks that existing networks are not modified or removed.
// Errors reference the network's index in the *new* list so users can locate the offending
// entry in their updated manifest, even after a reorder.
func validateNetworkImmutability(oldNetworks, newNetworks []infrastructurev1beta2.NetworkSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	newByName := make(map[string]*infrastructurev1beta2.NetworkSpec, len(newNetworks))
	newIndexByName := make(map[string]int, len(newNetworks))
	for i := range newNetworks {
		newByName[newNetworks[i].Name] = &newNetworks[i]
		newIndexByName[newNetworks[i].Name] = i
	}

	for _, oldNet := range oldNetworks {
		newNet, exists := newByName[oldNet.Name]
		if !exists {
			allErrs = append(allErrs, field.Forbidden(
				fldPath,
				fmt.Sprintf("removing network %q is not allowed", oldNet.Name)))
			continue
		}

		newIdx := newIndexByName[oldNet.Name]
		newPath := fldPath.Index(newIdx)

		if newNet.CIDR != oldNet.CIDR {
			allErrs = append(allErrs, field.Forbidden(
				newPath.Child("cidr"),
				"field is immutable after cluster creation"))
		}

		if newNet.UUID != oldNet.UUID {
			allErrs = append(allErrs, field.Forbidden(
				newPath.Child("uuid"),
				"field is immutable after cluster creation"))
		}

		if newNet.GatewayAddress != oldNet.GatewayAddress {
			allErrs = append(allErrs, field.Forbidden(
				newPath.Child("gatewayAddress"),
				"field is immutable after cluster creation"))
		}
	}

	return allErrs
}

// validateFloatingIPRequiresLBOrPreExisting rejects managed floating IPs when the load balancer is disabled.
// cloudscale.ch floating IPs require a dummy interface on the target server.
// With a pre-existing FIP, the user knows the address upfront and can configure
// the dummy interface in KubeadmControlPlane preKubeadmCommands.
// With a managed FIP, the address isn't known until creation,
// so the dummy interface can't be pre-configured.
func validateFloatingIPRequiresLBOrPreExisting(spec infrastructurev1beta2.CloudscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.FloatingIP != nil &&
		spec.FloatingIP.Address == "" &&
		!ptr.Deref(spec.ControlPlaneLoadBalancer.Enabled, true) {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("floatingIP"),
			"",
			"managed floating IP requires the load balancer to be enabled; use a pre-existing floating IP if you need a floating IP without a load balancer"))
	}

	return allErrs
}

// validateLBImmutability forbids changes to LB fields that are baked into the LB at creation.
// Algorithm, Flavor, APIServerPort and the HealthMonitor settings cannot be reissued
// to an existing cloudscale.ch LB, so changing them in spec would silently lie to the user.
func validateLBImmutability(oldLB, newLB *infrastructurev1beta2.LoadBalancerSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	forbidIfChanged := func(child string, oldV, newV any) {
		if oldV != newV {
			allErrs = append(allErrs, field.Forbidden(
				fldPath.Child(child),
				"field is immutable after cluster creation"))
		}
	}

	forbidIfChanged("algorithm", oldLB.Algorithm, newLB.Algorithm)
	forbidIfChanged("flavor", oldLB.Flavor, newLB.Flavor)
	forbidIfChanged("apiServerPort", oldLB.APIServerPort, newLB.APIServerPort)

	hmPath := fldPath.Child("healthMonitor")
	hmForbid := func(child string, oldV, newV int) {
		if oldV != newV {
			allErrs = append(allErrs, field.Forbidden(
				hmPath.Child(child),
				"field is immutable after cluster creation"))
		}
	}
	hmForbid("delayS", oldLB.HealthMonitor.DelayS, newLB.HealthMonitor.DelayS)
	hmForbid("timeoutS", oldLB.HealthMonitor.TimeoutS, newLB.HealthMonitor.TimeoutS)
	hmForbid("upThreshold", oldLB.HealthMonitor.UpThreshold, newLB.HealthMonitor.UpThreshold)
	hmForbid("downThreshold", oldLB.HealthMonitor.DownThreshold, newLB.HealthMonitor.DownThreshold)

	return allErrs
}

// validateFloatingIPRequiresPublicLB rejects a floating IP attached to a load balancer
// that uses an internal-network VIP. cloudscale.ch floating IPs only attach to public LBs.
func validateFloatingIPRequiresPublicLB(spec infrastructurev1beta2.CloudscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.FloatingIP != nil &&
		ptr.Deref(spec.ControlPlaneLoadBalancer.Enabled, true) &&
		spec.ControlPlaneLoadBalancer.Network != "" {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("floatingIP"),
			"",
			"floating IPs cannot be attached to a load balancer with a private VIP; use a public load balancer or remove the floating IP"))
	}

	return allErrs
}

// validateLBPoolMemberNetworkResolvable requires controlPlaneLoadBalancer.network to be set
// when there are multiple networks and the LB is public. Without an explicit network the
// controller would default the LB pool members' subnet to networks[0], which silently
// breaks clusters whose machines join a different network.
func validateLBPoolMemberNetworkResolvable(spec infrastructurev1beta2.CloudscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs = make(field.ErrorList, 0, 1)

	if !ptr.Deref(spec.ControlPlaneLoadBalancer.Enabled, true) {
		return nil
	}
	if spec.ControlPlaneLoadBalancer.Network != "" {
		return nil
	}
	if len(spec.Networks) <= 1 {
		return nil
	}

	allErrs = append(allErrs, field.Required(
		fldPath.Child("controlPlaneLoadBalancer", "network"),
		"must be set to one of spec.networks[].name when multiple networks are defined; the load balancer pool members need an explicit subnet to attach to"))

	return allErrs
}

// validateFloatingIP validates the floating IP spec.
func validateFloatingIP(fip *infrastructurev1beta2.FloatingIPSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	hasIPFamily := fip.IPFamily != nil
	hasIP := fip.Address != ""

	if hasIPFamily == hasIP {
		allErrs = append(allErrs, field.Invalid(fldPath, "",
			"exactly one of ipFamily or ip must be specified"))
	}

	if hasIP && net.ParseIP(fip.Address) == nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("ip"), fip.Address,
			"must be a valid IP address"))
	}

	return allErrs
}

// validateFloatingIPImmutability checks that the floating IP config is immutable once set.
func validateFloatingIPImmutability(oldFIP, newFIP *infrastructurev1beta2.FloatingIPSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if oldFIP == nil && newFIP == nil {
		return nil
	}

	// Cannot add or remove floating IP after creation.
	// After the both-nil early return above, at least one is non-nil.
	if oldFIP == nil {
		allErrs = append(allErrs, field.Forbidden(fldPath,
			"floating IP cannot be added after cluster creation"))
		return allErrs
	}
	if newFIP == nil {
		allErrs = append(allErrs, field.Forbidden(fldPath,
			"floating IP cannot be removed after cluster creation"))
		return allErrs
	}

	// Cannot switch between managed and pre-existing
	if oldFIP.Address != newFIP.Address {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("ip"),
			"field is immutable once set"))
	}
	if ptr.Deref(oldFIP.IPFamily, "") != ptr.Deref(newFIP.IPFamily, "") {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("ipFamily"),
			"field is immutable once set"))
	}

	return allErrs
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
