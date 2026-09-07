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
	"testing"

	. "github.com/onsi/gomega"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

func newClusterTemplateWebhookTestObjects() (
	obj *infrastructurev1beta2.CloudscaleClusterTemplate,
	oldObj *infrastructurev1beta2.CloudscaleClusterTemplate,
	validator CloudscaleClusterTemplateCustomValidator,
	defaulter CloudscaleClusterTemplateCustomDefaulter, //nolint:unparam
) {
	obj = &infrastructurev1beta2.CloudscaleClusterTemplate{}
	oldObj = &infrastructurev1beta2.CloudscaleClusterTemplate{}
	validator = CloudscaleClusterTemplateCustomValidator{
		RegionInfo: testutils.NewTestRegionInfo(),
	}
	defaulter = CloudscaleClusterTemplateCustomDefaulter{}
	return
}

// ============================================================================
// Tests for CloudscaleClusterTemplate Defaulting Webhook
// ============================================================================

// TestClusterTemplateDefaulting_ZoneNotDefaulted verifies that the zone is not defaulted in CloudscaleClusterTemplate.
// ClusterClass patches can change the region on the generated CloudscaleCluster, but they don't reach into the
// stored template — so a zone defaulted here against the template's region
// would persist after the patch and end up mismatched against the new region.
// The CloudscaleCluster defaulter handles zone defaulting on the final patched
// object instead.
func TestClusterTemplateDefaulting_ZoneNotDefaulted(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.Zone).To(BeEmpty())
}

func TestClusterTemplateDefaulting_ExplicitZoneNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.Zone).To(Equal(ZoneRma1))
}

// Networks must not be defaulted on the template. The default name is sourced
// from the object's metadata.Name, but for a template that's the template's
// own name — which would then be copied verbatim into every CloudscaleCluster
// generated from this ClusterClass, causing all clusters to share a network
// name. The CloudscaleCluster defaulter handles network defaulting on the
// final generated object instead, where Name is the parent Cluster's name.
func TestClusterTemplateDefaulting_NetworksNotDefaulted(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Name = "my-cluster"
	obj.Spec.Template.Spec.Region = RegionRma

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.Networks).To(BeEmpty())
}

func TestClusterTemplateDefaulting_ExplicitNetworksNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "custom", CIDR: "10.1.0.0/16"},
	}

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.Networks).To(HaveLen(1))
	g.Expect(obj.Spec.Template.Spec.Networks[0].Name).To(Equal("custom"))
	g.Expect(obj.Spec.Template.Spec.Networks[0].CIDR).To(Equal("10.1.0.0/16"))
}

// Router interface addresses must not be defaulted on the template.
func TestClusterTemplateDefaulting_RouterAddressesNotDefaulted(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = routerNetworks("10.0.0.0/24")
	obj.Spec.Template.Spec.Routers = routerWith(routerIface(""))

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.Routers[0].Interfaces[0].Address).To(BeEmpty())
}

func TestClusterTemplateDefaulting_LBEnabledToTrue(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled = nil

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(new(true)))
}

func TestClusterTemplateDefaulting_LBEnabledFalseNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled = new(false)

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(new(false)))
}

func TestClusterTemplateDefaulting_LBAlgorithm(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Algorithm = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
}

func TestClusterTemplateDefaulting_LBFlavor(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Flavor = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
}

func TestClusterTemplateDefaulting_APIServerPort(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.APIServerPort = 0

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
}

func TestClusterTemplateDefaulting_HealthMonitorFields(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
}

func TestClusterTemplateDefaulting_ExplicitHealthMonitorNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 10
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS = 7
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold = 5
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold = 8

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(10))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(7))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(5))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(8))
}

func TestClusterTemplateDefaulting_FloatingIPDefaultsToIPv4(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.FloatingIP.IPFamily).To(Equal(new(infrastructurev1beta2.IPFamilyIPv4)))
}

func TestClusterTemplateDefaulting_FloatingIPExplicitNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Template.Spec.FloatingIP.IPFamily).To(BeNil())
	g.Expect(obj.Spec.Template.Spec.FloatingIP.Address).To(Equal("1.2.3.4"))
}

func TestClusterTemplateDefaulting_AllDefaultsApplied(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Name = "test-cluster"
	obj.Spec.Template.Spec.Region = RegionRma

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())

	g.Expect(obj.Spec.Template.Spec.Zone).To(BeEmpty())
	g.Expect(obj.Spec.Template.Spec.Networks).To(BeEmpty())
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(new(true)))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
	g.Expect(obj.Spec.Template.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
}

// ============================================================================
// Tests for CloudscaleCluster Validating Webhook - Create
// ============================================================================

func TestClusterTemplateValidateCreate_ValidCluster(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterTemplateValidateCreate_ZoneNotInRegion(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = "lpg1"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
}

func TestClusterTemplateValidateCreate_UnknownZone(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = "xyz1"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
}

func TestClusterTemplateValidateCreate_EmptyZone(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ""

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterTemplateValidateCreate_NetworkMustHaveUUIDOrCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "bad"}, // neither UUID nor CIDR
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exactly one of uuid or cidr"))
}

func TestClusterTemplateValidateCreate_NetworkBothUUIDAndCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "bad", UUID: "some-uuid", CIDR: "10.0.0.0/24"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exactly one of uuid or cidr"))
}

func TestClusterTemplateValidateCreate_DuplicateNetworkNames(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "dup", CIDR: "10.0.0.0/24"},
		{Name: "dup", CIDR: "10.1.0.0/24"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("Duplicate"))
}

func TestClusterTemplateValidateCreate_GatewayWithinCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "172.18.0.1"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterTemplateValidateCreate_GatewayOutsideCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "192.168.1.1"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("gatewayAddress"))
}

func TestClusterTemplateValidateCreate_InvalidGatewayIP(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "notanip"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("gatewayAddress"))
}

func TestClusterTemplateValidateCreate_GatewayOnPreExistingNetworkRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "pre-existing", UUID: "some-uuid", GatewayAddress: "10.0.0.1"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("gatewayAddress"))
}

func TestClusterTemplateValidateCreate_LBNetworkReference(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Network = "main"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterTemplateValidateCreate_PublicLBWithMultipleNetworksRequiresExplicitNetwork(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
		{Name: "aux", CIDR: "10.1.0.0/24"},
	}
	// Public LB (Network == ""), multiple networks → ambiguous which subnet
	// pool members should attach to.
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Network = ""

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("controlPlaneLoadBalancer"))
}

func TestClusterTemplateValidateCreate_LBNetworkReferenceInvalid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Network = "nonexistent"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("controlPlaneLoadBalancer.network"))
}

func TestClusterTemplateValidateCreate_FloatingIPValid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterTemplateValidateCreate_FloatingIPWithPrivateLBRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Network = "main"
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
	g.Expect(err.Error()).To(ContainSubstring("private"))
}

func TestClusterTemplateValidateCreate_FloatingIPBothFieldsInvalid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
		Address:  "1.2.3.4",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
	g.Expect(err.Error()).To(ContainSubstring("ipFamily"))
	g.Expect(err.Error()).To(ContainSubstring("ip"))
}

func TestClusterTemplateValidateCreate_PreExistingFloatingIPInvalidIP(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "not-an-ip",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP.ip"))
}

func TestClusterTemplateValidateCreate_FloatingIPNeitherFieldInvalid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
}

func TestClusterTemplateValidateCreate_ManagedFloatingIPWithoutLBRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("managed floating IP"))
}

func TestClusterTemplateValidateCreate_PreExistingFloatingIPWithoutLBAllowed(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Zone = ZoneRma1
	obj.Spec.Template.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
	obj.Spec.Template.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// A template's router interface addresses are derived per cluster, not here, so leaving
// one empty is what the ClusterClass flow relies on: a topology patch may replace the
// network's CIDR and the address follows it.
func TestClusterTemplateValidateCreate_RouterInterfaceAddressDeferred(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, defaulter := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = routerNetworks("10.10.0.0/24")
	obj.Spec.Template.Spec.Routers = routerWith(routerIface(""))

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// A uuid network never gains a CIDR, so no later defaulting could derive an address for it
// either.
func TestClusterTemplateValidateCreate_RouterInterfaceOnUUIDNetworkStillRequiresAddress(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", UUID: "net-uuid"}}
	obj.Spec.Template.Spec.Routers = routerWith(routerIface(""))

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("must be set explicitly"))
	g.Expect(err.Error()).To(ContainSubstring("referenced by uuid"))
}

func TestClusterTemplateValidateCreate_RouterInterfaceAddressOutsideCIDRRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = routerNetworks("10.10.0.0/24")
	obj.Spec.Template.Spec.Routers = routerWith(routerIface("10.20.0.1"))

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("must be within CIDR 10.10.0.0/24"))
}

func TestClusterTemplateValidateCreate_RouterInterfaceAddressInDHCPPoolRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = routerNetworks("10.10.0.0/24")
	obj.Spec.Template.Spec.Routers = routerWith(routerIface("10.10.0.150"))

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("DHCP pool"))
}

func TestClusterTemplateValidateCreate_RouterInterfaceUnknownNetworkRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = RegionRma
	obj.Spec.Template.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "other", CIDR: "10.10.0.0/24"}}
	obj.Spec.Template.Spec.Routers = routerWith(routerIface(""))

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("Not found"))
}

func TestClusterTemplateThenClusterDefaulting_DerivesRouterAddress(t *testing.T) {
	for _, tc := range []struct {
		name        string
		patchedCIDR string
		wantAddress string
	}{
		{name: "template CIDR untouched", patchedCIDR: "10.10.0.0/24", wantAddress: "10.10.0.1"},
		{name: "CIDR replaced by a topology patch", patchedCIDR: "10.20.0.0/24", wantAddress: "10.20.0.1"},
		{name: "CIDR narrowed by a topology patch", patchedCIDR: "172.16.5.0/25", wantAddress: "172.16.5.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			tmpl, _, tmplValidator, tmplDefaulter := newClusterTemplateWebhookTestObjects()
			tmpl.Spec.Template.Spec.Region = RegionRma
			tmpl.Spec.Template.Spec.Networks = routerNetworks("10.10.0.0/24")
			tmpl.Spec.Template.Spec.Routers = routerWith(routerIface(""))

			g.Expect(tmplDefaulter.Default(ctx, tmpl)).To(Succeed())
			_, err := tmplValidator.ValidateCreate(ctx, tmpl)
			g.Expect(err).NotTo(HaveOccurred())

			// What the topology controller does: copy the template spec, apply the
			// patches, then let the cluster webhooks run on the result.
			cluster, _, validator, defaulter := newClusterWebhookTestObjects()
			cluster.Name = "my-cluster-abcde"
			cluster.Spec = *tmpl.Spec.Template.Spec.DeepCopy()
			cluster.Spec.Networks[0].CIDR = tc.patchedCIDR

			g.Expect(defaulter.Default(ctx, cluster)).To(Succeed())
			_, err = validator.ValidateCreate(ctx, cluster)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(cluster.Spec.Routers[0].Interfaces[0].Address).To(Equal(tc.wantAddress))
		})
	}
}

// ============================================================================
// Tests for CloudscaleClusterTemplate Validating Webhook - Update
// ============================================================================

func TestClusterTemplateValidateUpdate_NoChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator, _ := newClusterTemplateWebhookTestObjects()

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterTemplateValidateUpdate_AnyChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator, _ := newClusterTemplateWebhookTestObjects()
	obj.Spec.Template.Spec.Region = "lpg"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec"))
	g.Expect(err.Error()).To(ContainSubstring("Forbidden"))
	g.Expect(err.Error()).To(ContainSubstring("immutable"))
}

// ============================================================================
// Tests for CloudscaleClusterTemplate Validating Webhook - Delete
// ============================================================================

func TestClusterTemplateValidateDelete_AlwaysSucceeds(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterTemplateWebhookTestObjects()

	_, err := validator.ValidateDelete(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}
