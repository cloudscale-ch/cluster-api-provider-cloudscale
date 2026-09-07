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
	"maps"
	"net/netip"
	"slices"

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

	// Router interface addresses are derived from the networks' CIDRs and are immutable
	// once set, so they are defaulted here rather than in the shared defaulter.
	defaultRouterInterfaceAddresses(&cluster.Spec)

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

// defaultRouterInterfaceAddresses assigns an address to every router interface that
// does not declare one.
// Interfaces on a network referenced by uuid are left untouched: that subnet is only
// known at runtime, so the address cannot be derived here.
func defaultRouterInterfaceAddresses(spec *infrastructurev1beta2.CloudscaleClusterSpec) {
	networks := indexNetworks(spec.Networks)

	usedByNetwork := make(map[string]map[string]struct{})
	for ri := range spec.Routers {
		for ii := range spec.Routers[ri].Interfaces {
			ifaceSpec := &spec.Routers[ri].Interfaces[ii]
			if usedByNetwork[ifaceSpec.Network] == nil {
				usedByNetwork[ifaceSpec.Network] = make(map[string]struct{})
			}
			if ifaceSpec.Address != "" {
				usedByNetwork[ifaceSpec.Network][ifaceSpec.Address] = struct{}{}
			}
		}
	}

	for ri := range spec.Routers {
		for ii := range spec.Routers[ri].Interfaces {
			ifaceSpec := &spec.Routers[ri].Interfaces[ii]

			// Normalise here rather than relying on the CRD default, so the value is
			// set no matter which path reached this defaulter.
			if ifaceSpec.ConfigureSubnetGateway == nil {
				ifaceSpec.ConfigureSubnetGateway = new(true)
			}
			// An adopted interface keeps the address it already holds, which is only
			// known once the controller reads the router.
			if ifaceSpec.Address != "" || ifaceSpec.UUID != "" {
				continue
			}

			used := usedByNetwork[ifaceSpec.Network]

			// The gateway owner takes the address the subnet already advertises. If
			// another interface has claimed it, fall through and allocate instead.
			gateway := networks[ifaceSpec.Network].GatewayAddress
			if gateway != "" && *ifaceSpec.ConfigureSubnetGateway {
				if _, taken := used[gateway]; !taken {
					ifaceSpec.Address = gateway
					used[gateway] = struct{}{}
					continue
				}
			}

			// err is ignored: an unparseable CIDR leaves the zero Prefix, which
			// nextFreeAddress skips. validateNetworks reports the malformed value.
			prefix, _ := netip.ParsePrefix(networks[ifaceSpec.Network].CIDR)
			if addr, ok := nextFreeAddress(prefix, used); ok {
				ifaceSpec.Address = addr
				used[addr] = struct{}{}
			}
		}
	}
}

// .101-.254 of every subnet is DHCP.
const (
	dhcpPoolFirstOctet = 101
	dhcpPoolLastOctet  = 254
)

// unassignableReason says why addr cannot be given to a router interface inside prefix, or
// "" when it can. An address in the DHCP pool may later be handed to a node, and the
// network and broadcast addresses cannot be held by an interface at all.
func unassignableReason(prefix netip.Prefix, addr netip.Addr) string {
	if !addr.Is4() {
		return ""
	}
	switch lastOctet := addr.As4()[3]; {
	case addr == prefix.Masked().Addr():
		return "it is the network address"
	case !prefix.Contains(addr.Next()):
		return "it is the broadcast address"
	case lastOctet >= dhcpPoolFirstOctet && lastOctet <= dhcpPoolLastOctet:
		return fmt.Sprintf("it is inside the DHCP pool .%d-.%d", dhcpPoolFirstOctet, dhcpPoolLastOctet)
	}
	return ""
}

// nextFreeAddress returns the first assignable address inside prefix that is not already
// used. It reports false for an invalid prefix or an exhausted range.
func nextFreeAddress(prefix netip.Prefix, used map[string]struct{}) (string, bool) {
	if !prefix.IsValid() {
		return "", false
	}
	for addr := prefix.Masked().Addr().Next(); prefix.Contains(addr); addr = addr.Next() {
		// Step over rather than stop: on a prefix wider than a /24 the DHCP pool recurs
		// inside the range, so there can be assignable addresses above it.
		if unassignableReason(prefix, addr) != "" {
			continue
		}
		if _, taken := used[addr.String()]; !taken {
			return addr.String(), true
		}
	}
	return "", false
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

	allErrs := clusterSpecValidateCreate(cluster.Spec, v.RegionInfo, modeCluster, field.NewPath("spec"))

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: infrastructurev1beta2.SchemeGroupVersion.Group, Kind: "CloudscaleCluster"},
			cluster.Name, allErrs)
	}

	return nil, nil
}

// validationMode captures what a spec has already been through by the time it reaches the
// validator. A CloudscaleCluster arrives defaulted; a CloudscaleClusterTemplate does not,
// because defaultRouterInterfaceAddresses only runs on the cluster (see Default).
type validationMode struct {
	// routerAddressesDefaulted is true when defaultRouterInterfaceAddresses has already run,
	// so an empty router interface address means "could not be derived" rather than "will be
	// derived when a cluster is stamped from this template".
	routerAddressesDefaulted bool
}

var (
	// modeCluster validates a CloudscaleCluster, i.e. a fully defaulted spec.
	modeCluster = validationMode{routerAddressesDefaulted: true}
	// modeTemplate validates a CloudscaleClusterTemplate, whose router interface addresses
	// are only derived once the topology controller stamps out a CloudscaleCluster.
	modeTemplate = validationMode{}
)

func clusterSpecValidateCreate(spec infrastructurev1beta2.CloudscaleClusterSpec, regionInfo *cloudscale.RegionInfo, mode validationMode, fldPath *field.Path) field.ErrorList {
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

	// Validate routers
	allErrs = append(allErrs, validateRouters(spec.Routers, spec.Networks, mode, fldPath.Child("routers"))...)

	// Validate LB network reference
	if spec.ControlPlaneLoadBalancer.Network != "" {
		allErrs = append(allErrs, validateNetworkReference(
			spec.ControlPlaneLoadBalancer.Network,
			spec.Networks,
			fldPath.Child("controlPlaneLoadBalancer", "network"),
		)...)
	}

	// Validate LB pool member network reference
	if spec.ControlPlaneLoadBalancer.PoolMemberNetwork != "" {
		allErrs = append(allErrs, validateNetworkReference(
			spec.ControlPlaneLoadBalancer.PoolMemberNetwork,
			spec.Networks,
			fldPath.Child("controlPlaneLoadBalancer", "poolMemberNetwork"),
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

	// The updated router set must satisfy the creation rules
	routersPath := field.NewPath("spec", "routers")
	allErrs = append(allErrs, validateRouters(newClusterSpec.Routers, newClusterSpec.Networks, modeCluster, routersPath)...)
	// ...and on top of that, nothing already there may change or disappear.
	allErrs = append(allErrs, validateRouterDiff(oldClusterSpec.Routers, newClusterSpec.Routers, routersPath)...)

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

	// Validate LB pool member network reference (for new or existing)
	if newClusterSpec.ControlPlaneLoadBalancer.PoolMemberNetwork != "" {
		allErrs = append(allErrs, validateNetworkReference(
			newClusterSpec.ControlPlaneLoadBalancer.PoolMemberNetwork,
			newClusterSpec.Networks,
			field.NewPath("spec", "controlPlaneLoadBalancer", "poolMemberNetwork"),
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
			if _, err := netip.ParsePrefix(netSpec.CIDR); err != nil {
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
			allErrs = append(allErrs, validateAddressInCIDR(
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

// indexNetworks maps each network name to its spec, so router validation and defaulting
// can resolve an interface's network reference in one place.
func indexNetworks(networks []infrastructurev1beta2.NetworkSpec) map[string]infrastructurev1beta2.NetworkSpec {
	byName := make(map[string]infrastructurev1beta2.NetworkSpec, len(networks))
	for _, n := range networks {
		byName[n.Name] = n
	}
	return byName
}

// validateRouters runs every rule that applies to a router set, whether it was declared
// at creation or added later. ValidateUpdate calls it on the new set too, so additions
// are held to the same standard as originals without restating the rules.
func validateRouters(routers []infrastructurev1beta2.RouterSpec, networks []infrastructurev1beta2.NetworkSpec, mode validationMode, fldPath *field.Path) field.ErrorList {
	byName := indexNetworks(networks)
	var allErrs field.ErrorList
	for i, routerSpec := range routers {
		allErrs = append(allErrs, validateSingleRouter(routerSpec, byName, mode, fldPath.Index(i))...)
	}
	allErrs = append(allErrs, validateRouterNetworkAddressing(routers, byName, fldPath)...)
	return allErrs
}

// validateSingleRouter validates one router spec against the network index, using
// routerPath as the base field.Path for errors.
//
// Router names are not checked for uniqueness: spec.routers is a list-map keyed by name, so
// the API server rejects duplicates during schema validation, which runs before validating
// webhooks.
func validateSingleRouter(
	routerSpec infrastructurev1beta2.RouterSpec,
	networks map[string]infrastructurev1beta2.NetworkSpec,
	mode validationMode,
	routerPath *field.Path,
) field.ErrorList {
	var allErrs field.ErrorList

	if routerSpec.UUID != "" && routerSpec.InternetGateway {
		allErrs = append(allErrs, field.Invalid(routerPath, routerSpec.Name,
			"uuid and internetGateway are mutually exclusive"))
	}

	for j, ifaceSpec := range routerSpec.Interfaces {
		allErrs = append(allErrs, validateRouterInterface(ifaceSpec, networks, routerSpec.UUID != "", mode, routerPath.Child("interfaces").Index(j))...)
	}

	return allErrs
}

// validateRouterInterface validates one router interface against the network index. On a
// CloudscaleCluster the defaulter has already filled in every address it could derive, so an
// empty address means it could not — either because the network is referenced by uuid (no
// CIDR at admission time) or because the CIDR is exhausted. Both need the user to be
// explicit. An interface adopted by uuid is the exception: its address is whatever the
// pre-existing interface already holds.
//
// On a CloudscaleClusterTemplate (mode.routerAddressesDefaulted false) an empty address on a
// network that has a CIDR is fine: it is derived per cluster, so a topology patch may change
// the CIDR and have the address follow. Only the uuid-network case still errors, since such
// a network never gains a CIDR and so could not be derived later either.
func validateRouterInterface(
	ifaceSpec infrastructurev1beta2.RouterInterfaceSpec,
	networks map[string]infrastructurev1beta2.NetworkSpec,
	routerAdopted bool,
	mode validationMode,
	ifacePath *field.Path,
) field.ErrorList {
	network, netExists := networks[ifaceSpec.Network]
	if !netExists {
		return field.ErrorList{field.NotFound(ifacePath.Child("network"), ifaceSpec.Network)}
	}

	if ifaceSpec.UUID != "" {
		var allErrs field.ErrorList
		if !routerAdopted {
			allErrs = append(allErrs, field.Forbidden(ifacePath.Child("uuid"),
				"only a router referenced by uuid carries pre-existing interfaces to adopt"))
		}
		if ifaceSpec.Address != "" {
			allErrs = append(allErrs, field.Forbidden(ifacePath.Child("address"),
				"the address of an adopted interface is read from the interface itself, so it cannot be requested"))
		}
		return allErrs
	}

	if ifaceSpec.Address == "" {
		if network.CIDR == "" {
			return field.ErrorList{field.Required(ifacePath.Child("address"), fmt.Sprintf(
				"must be set explicitly: network %q is referenced by uuid, so its subnet is not known yet",
				ifaceSpec.Network))}
		}
		if !mode.routerAddressesDefaulted {
			// Derived from the network's CIDR when a cluster is stamped out of this template.
			return nil
		}
		return field.ErrorList{field.Required(ifacePath.Child("address"), fmt.Sprintf(
			"no free address left in %s for network %q", network.CIDR, ifaceSpec.Network))}
	}

	// Containment can only be checked for a managed network; an uuid network's subnet
	// is resolved by the controller at runtime.
	if network.CIDR == "" {
		return nil
	}

	addrPath := ifacePath.Child("address")
	if errs := validateAddressInCIDR(network.CIDR, ifaceSpec.Address, addrPath); len(errs) > 0 {
		return errs
	}

	prefix, _ := netip.ParsePrefix(network.CIDR)
	ip, _ := netip.ParseAddr(ifaceSpec.Address)
	if reason := unassignableReason(prefix, ip); reason != "" {
		return field.ErrorList{field.Invalid(addrPath, ifaceSpec.Address,
			fmt.Sprintf("cannot be assigned to a router interface: %s of %s", reason, network.CIDR))}
	}
	return nil
}

// validateRouterNetworkAddressing enforces the rules that only make sense when all
// interfaces on a network are considered together: at most one interface may set the
// subnet gateway, explicit addresses must be unique per network, and the gateway owner's
// address must equal the network's declared gatewayAddress.
func validateRouterNetworkAddressing(routers []infrastructurev1beta2.RouterSpec, networks map[string]infrastructurev1beta2.NetworkSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	subnetGateways := make(map[string][]*field.Path)
	addrSeen := make(map[string]map[string]bool)

	for i, router := range routers {
		for j, iface := range router.Interfaces {
			ifacePath := fldPath.Index(i).Child("interfaces").Index(j)

			if ptr.Deref(iface.ConfigureSubnetGateway, true) {
				subnetGateways[iface.Network] = append(subnetGateways[iface.Network], ifacePath.Child("configureSubnetGateway"))

				// The controller points the subnet's gateway at this interface's
				// address, so a differing gatewayAddress would silently never take
				// effect. An address is only empty on an adopted interface, whose
				// address is not known until the controller reads it.
				if gateway := networks[iface.Network].GatewayAddress; gateway != "" && iface.Address != "" && iface.Address != gateway {
					allErrs = append(allErrs, field.Invalid(ifacePath.Child("address"), iface.Address,
						fmt.Sprintf("interface configures the subnet gateway of network %q, so its address must equal the network's gatewayAddress %q", iface.Network, gateway)))
				}
			}
			if iface.Address != "" {
				if addrSeen[iface.Network] == nil {
					addrSeen[iface.Network] = make(map[string]bool)
				}
				if addrSeen[iface.Network][iface.Address] {
					allErrs = append(allErrs, field.Duplicate(ifacePath.Child("address"), iface.Address))
				} else {
					addrSeen[iface.Network][iface.Address] = true
				}
			}
		}
	}

	// Report networks with more than one gateway, in a stable order.
	for _, network := range slices.Sorted(maps.Keys(subnetGateways)) {
		paths := subnetGateways[network]
		if len(paths) <= 1 {
			continue
		}
		for _, p := range paths {
			allErrs = append(allErrs, field.Invalid(p, true,
				fmt.Sprintf("network %q has multiple router interfaces configuring the subnet gateway; only one interface may set configureSubnetGateway=true", network)))
		}
	}

	return allErrs
}

// validateRouterDiff enforces immutability for spec.routers on update: routers and their
// interfaces may be added, but never removed or altered. Whether the additions themselves
// are valid is validateRouters' job, which ValidateUpdate runs over the new set.
func validateRouterDiff(oldRouters, newRouters []infrastructurev1beta2.RouterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	newByName := make(map[string]infrastructurev1beta2.RouterSpec, len(newRouters))
	newIndexByName := make(map[string]int, len(newRouters))
	for i, newRouter := range newRouters {
		newByName[newRouter.Name] = newRouter
		newIndexByName[newRouter.Name] = i
	}

	for _, oldRouter := range oldRouters {
		newRouter, exists := newByName[oldRouter.Name]
		if !exists {
			allErrs = append(allErrs, field.Forbidden(fldPath,
				fmt.Sprintf("removing router %q is not allowed", oldRouter.Name)))
			continue
		}

		newPath := fldPath.Index(newIndexByName[oldRouter.Name])

		if newRouter.UUID != oldRouter.UUID {
			allErrs = append(allErrs, field.Forbidden(newPath.Child("uuid"),
				"field is immutable after cluster creation"))
		}
		if newRouter.InternetGateway != oldRouter.InternetGateway {
			allErrs = append(allErrs, field.Forbidden(newPath.Child("internetGateway"),
				"field is immutable after cluster creation"))
		}

		newIfaceByNetwork := make(map[string]infrastructurev1beta2.RouterInterfaceSpec, len(newRouter.Interfaces))
		for _, iface := range newRouter.Interfaces {
			newIfaceByNetwork[iface.Network] = iface
		}

		for _, oldIface := range oldRouter.Interfaces {
			newIface, kept := newIfaceByNetwork[oldIface.Network]
			if !kept {
				allErrs = append(allErrs, field.Forbidden(newPath.Child("interfaces"),
					fmt.Sprintf("removing interface for network %q is not allowed", oldIface.Network)))
				continue
			}
			if ptr.Deref(newIface.ConfigureSubnetGateway, true) != ptr.Deref(oldIface.ConfigureSubnetGateway, true) {
				allErrs = append(allErrs, field.Forbidden(newPath.Child("interfaces"),
					fmt.Sprintf("configureSubnetGateway for network %q is immutable after cluster creation", newIface.Network)))
			}
			if newIface.Address != oldIface.Address {
				allErrs = append(allErrs, field.Forbidden(newPath.Child("interfaces"),
					fmt.Sprintf("address for network %q is immutable after cluster creation", newIface.Network)))
			}
			// Flipping uuid would hand ownership of a live interface over or away:
			// CAPCS detaches what it created and leaves adopted interfaces in place.
			if newIface.UUID != oldIface.UUID {
				allErrs = append(allErrs, field.Forbidden(newPath.Child("interfaces"),
					fmt.Sprintf("uuid for network %q is immutable after cluster creation", newIface.Network)))
			}
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
// PoolMemberNetwork is stricter than the sibling Network field (immutable *once set*):
// even setting it on a cluster that left it empty is forbidden, because the controller
// does not migrate live pool members from one subnet to another.
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
	forbidIfChanged("poolMemberNetwork", oldLB.PoolMemberNetwork, newLB.PoolMemberNetwork)

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

// validateLBPoolMemberNetworkResolvable requires either controlPlaneLoadBalancer.network
// or controlPlaneLoadBalancer.poolMemberNetwork to be set when there are multiple networks
// and the LB is public. With neither, the controller falls back to networks[0] for the pool
// members' subnet, which silently breaks clusters whose machines join a different network.
func validateLBPoolMemberNetworkResolvable(spec infrastructurev1beta2.CloudscaleClusterSpec, fldPath *field.Path) field.ErrorList {
	var allErrs = make(field.ErrorList, 0, 1)

	if !ptr.Deref(spec.ControlPlaneLoadBalancer.Enabled, true) {
		return nil
	}
	if spec.ControlPlaneLoadBalancer.Network != "" || spec.ControlPlaneLoadBalancer.PoolMemberNetwork != "" {
		return nil
	}
	if len(spec.Networks) <= 1 {
		return nil
	}

	// Reported on the parent: either child field satisfies the requirement, so pinning
	// the error on one of them would point the user at an arbitrary half of the fix.
	allErrs = append(allErrs, field.Required(
		fldPath.Child("controlPlaneLoadBalancer"),
		"controlPlaneLoadBalancer.network or controlPlaneLoadBalancer.poolMemberNetwork must be set to one of spec.networks[].name when multiple networks are defined; the load balancer pool members need an explicit subnet to attach to"))

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

	if _, err := netip.ParseAddr(fip.Address); hasIP && err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("ip"), fip.Address,
			fmt.Sprintf("must be a valid IP address: %s", err)))
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

// validateAddressInCIDR validates that an address is within the specified CIDR. Used
// for both subnet gateways and router interface addresses.
func validateAddressInCIDR(cidr, address string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		// CIDR validation should have caught this earlier
		return allErrs
	}

	ip, err := netip.ParseAddr(address)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, address, fmt.Sprintf("invalid IP address: %s", err)))
		return allErrs
	}

	if !prefix.Contains(ip) {
		allErrs = append(allErrs, field.Invalid(fldPath, address,
			fmt.Sprintf("must be within CIDR %s", cidr)))
	}

	return allErrs
}
