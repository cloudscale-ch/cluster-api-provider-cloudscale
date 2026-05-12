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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

const (
	RegionRma = "rma"
	ZoneRma1  = "rma1"
)

func newTestRegionInfo() *cloudscale.RegionInfo {
	return cloudscale.NewRegionInfo([]cloudscalesdk.Region{
		{Slug: RegionRma, Zones: []cloudscalesdk.ZoneStub{{Slug: ZoneRma1}}},
		{Slug: "lpg", Zones: []cloudscalesdk.ZoneStub{{Slug: "lpg1"}}},
	})
}

func newClusterWebhookTestObjects() (
	obj *infrastructurev1beta2.CloudscaleCluster,
	oldObj *infrastructurev1beta2.CloudscaleCluster,
	validator CloudscaleClusterCustomValidator,
	defaulter CloudscaleClusterCustomDefaulter,
) {
	obj = &infrastructurev1beta2.CloudscaleCluster{}
	oldObj = &infrastructurev1beta2.CloudscaleCluster{}
	validator = CloudscaleClusterCustomValidator{
		RegionInfo: newTestRegionInfo(),
	}
	defaulter = CloudscaleClusterCustomDefaulter{
		RegionInfo: newTestRegionInfo(),
	}
	return
}

// ============================================================================
// Tests for CloudscaleCluster Defaulting Webhook
// ============================================================================

func TestClusterDefaulting_ZoneFromRegion(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Zone).To(Equal(ZoneRma1))
}

func TestClusterDefaulting_ExplicitZoneNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Zone).To(Equal(ZoneRma1))
}

func TestClusterDefaulting_NetworksDefaultToClusterName(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Name = "my-cluster"
	obj.Spec.Region = RegionRma

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Networks).To(HaveLen(1))
	g.Expect(obj.Spec.Networks[0].Name).To(Equal("my-cluster"))
	g.Expect(obj.Spec.Networks[0].CIDR).To(Equal(defaultSubnetCIDR))
}

func TestClusterDefaulting_ExplicitNetworksNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "custom", CIDR: "10.1.0.0/16"},
	}

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Networks).To(HaveLen(1))
	g.Expect(obj.Spec.Networks[0].Name).To(Equal("custom"))
	g.Expect(obj.Spec.Networks[0].CIDR).To(Equal("10.1.0.0/16"))
}

func TestClusterDefaulting_LBEnabledToTrue(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Enabled = nil

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(ptr.To(true)))
}

func TestClusterDefaulting_LBEnabledFalseNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(ptr.To(false)))
}

func TestClusterDefaulting_LBAlgorithm(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Algorithm = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
}

func TestClusterDefaulting_LBFlavor(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Flavor = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
}

func TestClusterDefaulting_APIServerPort(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.APIServerPort = 0

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
}

func TestClusterDefaulting_HealthMonitorFields(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
}

func TestClusterDefaulting_ExplicitHealthMonitorNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 10
	obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS = 7
	obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold = 5
	obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold = 8

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(10))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(7))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(5))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(8))
}

func TestClusterDefaulting_FloatingIPDefaultsToIPv4(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.FloatingIP.IPFamily).To(Equal(ptr.To(infrastructurev1beta2.IPFamilyIPv4)))
}

func TestClusterDefaulting_FloatingIPExplicitNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.FloatingIP.IPFamily).To(BeNil())
	g.Expect(obj.Spec.FloatingIP.Address).To(Equal("1.2.3.4"))
}

func TestClusterDefaulting_AllDefaultsApplied(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Name = "test-cluster"
	obj.Spec.Region = RegionRma

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())

	g.Expect(obj.Spec.Zone).To(Equal(ZoneRma1))
	g.Expect(obj.Spec.Networks).To(HaveLen(1))
	g.Expect(obj.Spec.Networks[0].Name).To(Equal("test-cluster"))
	g.Expect(obj.Spec.Networks[0].CIDR).To(Equal(defaultSubnetCIDR))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(ptr.To(true)))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
}

// ============================================================================
// Tests for CloudscaleCluster Validating Webhook - Create
// ============================================================================

func TestClusterValidateCreate_ValidCluster(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_ZoneNotInRegion(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = "lpg1"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
}

func TestClusterValidateCreate_UnknownZone(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = "xyz1"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
}

func TestClusterValidateCreate_EmptyZone(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ""

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_NetworkMustHaveUUIDOrCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "bad"}, // neither UUID nor CIDR
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exactly one of uuid or cidr"))
}

func TestClusterValidateCreate_NetworkBothUUIDAndCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "bad", UUID: "some-uuid", CIDR: "10.0.0.0/24"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exactly one of uuid or cidr"))
}

func TestClusterValidateCreate_DuplicateNetworkNames(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "dup", CIDR: "10.0.0.0/24"},
		{Name: "dup", CIDR: "10.1.0.0/24"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("Duplicate"))
}

func TestClusterValidateCreate_GatewayWithinCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "172.18.0.1"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_GatewayOutsideCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "192.168.1.1"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("gatewayAddress"))
}

func TestClusterValidateCreate_InvalidGatewayIP(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "notanip"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("gatewayAddress"))
}

func TestClusterValidateCreate_GatewayOnPreExistingNetworkRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "pre-existing", UUID: "some-uuid", GatewayAddress: "10.0.0.1"},
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("gatewayAddress"))
}

func TestClusterValidateCreate_LBNetworkReference(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.ControlPlaneLoadBalancer.Network = "main"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_PublicLBWithMultipleNetworksRequiresExplicitNetwork(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
		{Name: "aux", CIDR: "10.1.0.0/24"},
	}
	// Public LB (Network == ""), multiple networks → ambiguous which subnet
	// pool members should attach to.
	obj.Spec.ControlPlaneLoadBalancer.Network = ""

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("controlPlaneLoadBalancer.network"))
}

func TestClusterValidateCreate_LBNetworkReferenceInvalid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.ControlPlaneLoadBalancer.Network = "nonexistent"

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("controlPlaneLoadBalancer.network"))
}

func TestClusterValidateCreate_FloatingIPValid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: ptr.To(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_FloatingIPWithPrivateLBRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.ControlPlaneLoadBalancer.Network = "main"
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: ptr.To(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
	g.Expect(err.Error()).To(ContainSubstring("private"))
}

func TestClusterValidateCreate_FloatingIPBothFieldsInvalid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: ptr.To(infrastructurev1beta2.IPFamilyIPv4),
		Address:  "1.2.3.4",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
	g.Expect(err.Error()).To(ContainSubstring("ipFamily"))
	g.Expect(err.Error()).To(ContainSubstring("ip"))
}

func TestClusterValidateCreate_PreExistingFloatingIPInvalidIP(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "not-an-ip",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP.ip"))
}

func TestClusterValidateCreate_FloatingIPNeitherFieldInvalid(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
}

func TestClusterValidateCreate_ManagedFloatingIPWithoutLBRejected(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: ptr.To(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("managed floating IP"))
}

func TestClusterValidateCreate_PreExistingFloatingIPWithoutLBAllowed(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// ============================================================================
// Tests for CloudscaleCluster Validating Webhook - Update
// ============================================================================

func setupUpdateTestObjects() (
	obj *infrastructurev1beta2.CloudscaleCluster,
	oldObj *infrastructurev1beta2.CloudscaleCluster,
	validator CloudscaleClusterCustomValidator,
) {
	obj, oldObj, validator, _ = newClusterWebhookTestObjects()
	networks := []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}

	oldObj.Spec.Region = RegionRma
	oldObj.Spec.Zone = ZoneRma1
	oldObj.Spec.Networks = networks
	oldObj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)

	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: defaultSubnetCIDR},
	}
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
	return
}

func TestClusterValidateUpdate_NoChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateUpdate_RegionChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Region = "lpg"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.region"))
}

func TestClusterValidateUpdate_ZoneChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Zone = "rma2"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
}

func TestClusterValidateUpdate_NetworkCIDRChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Networks[0].CIDR = "10.1.0.0/24"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cidr"))
}

func TestClusterValidateUpdate_NetworkReorderedCIDRChangedReportsNewIndex(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "net-a", CIDR: "10.1.0.0/24"},
		{Name: "net-b", CIDR: "10.2.0.0/24"},
	}
	// Swap order in new object and change net-a's CIDR.
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "net-b", CIDR: "10.2.0.0/24"},
		{Name: "net-a", CIDR: "10.9.0.0/24"},
	}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.networks[1].cidr"),
		"error message must point at net-a's new index (1), not its old index (0)")
}

func TestClusterValidateUpdate_NetworkRemoved(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Networks = nil

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("removing network"))
}

func TestClusterValidateUpdate_NetworkAdded(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Networks = append(obj.Spec.Networks, infrastructurev1beta2.NetworkSpec{
		Name: "extra", CIDR: "10.1.0.0/24",
	})
	// Multiple networks now require an explicit LB pool-member network. Pin to "main" (matches old).
	oldObj.Spec.ControlPlaneLoadBalancer.Network = "main"
	obj.Spec.ControlPlaneLoadBalancer.Network = "main"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateUpdate_LBEnabledChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.controlPlaneLoadBalancer.enabled"))
}

func TestClusterValidateUpdate_LBNetworkImmutable(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.ControlPlaneLoadBalancer.Network = "main"
	obj.Spec.ControlPlaneLoadBalancer.Network = "other"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("controlPlaneLoadBalancer.network"))
}

func TestClusterValidateUpdate_EndpointHostChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
	obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "5.6.7.8", Port: 6443}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.controlPlaneEndpoint.host"))
}

func TestClusterValidateUpdate_EndpointPortChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
	obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 8443}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.controlPlaneEndpoint.port"))
}

func TestClusterValidateUpdate_EndpointSetWhenEmpty(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "", Port: 0}
	obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateUpdate_FloatingIPCannotBeAdded(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: ptr.To(infrastructurev1beta2.IPFamilyIPv4),
	}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
}

func TestClusterValidateUpdate_FloatingIPCannotBeRemoved(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: ptr.To(infrastructurev1beta2.IPFamilyIPv4),
	}
	obj.Spec.FloatingIP = nil

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("floatingIP"))
}

func TestClusterValidateUpdate_LBFieldsImmutable(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(c *infrastructurev1beta2.CloudscaleCluster)
		errPath string
	}{
		{
			name: "Algorithm",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.Algorithm = "least_connections"
			},
			errPath: "controlPlaneLoadBalancer.algorithm",
		},
		{
			name: "Flavor",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.Flavor = "lb-premium"
			},
			errPath: "controlPlaneLoadBalancer.flavor",
		},
		{
			name:    "APIServerPort",
			mutate:  func(c *infrastructurev1beta2.CloudscaleCluster) { c.Spec.ControlPlaneLoadBalancer.APIServerPort = 8443 },
			errPath: "controlPlaneLoadBalancer.apiServerPort",
		},
		{
			name: "HealthMonitor.DelayS",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 10
			},
			errPath: "controlPlaneLoadBalancer.healthMonitor.delayS",
		},
		{
			name: "HealthMonitor.TimeoutS",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS = 10
			},
			errPath: "controlPlaneLoadBalancer.healthMonitor.timeoutS",
		},
		{
			name: "HealthMonitor.UpThreshold",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold = 9
			},
			errPath: "controlPlaneLoadBalancer.healthMonitor.upThreshold",
		},
		{
			name: "HealthMonitor.DownThreshold",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold = 9
			},
			errPath: "controlPlaneLoadBalancer.healthMonitor.downThreshold",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			obj, oldObj, validator := setupUpdateTestObjects()
			// Seed health monitor on old so changes are visible.
			oldObj.Spec.ControlPlaneLoadBalancer.Algorithm = "round_robin"
			oldObj.Spec.ControlPlaneLoadBalancer.Flavor = "lb-standard"
			oldObj.Spec.ControlPlaneLoadBalancer.APIServerPort = 6443
			oldObj.Spec.ControlPlaneLoadBalancer.HealthMonitor = infrastructurev1beta2.HealthMonitorSpec{
				DelayS: 5, TimeoutS: 3, UpThreshold: 2, DownThreshold: 3,
			}
			obj.Spec.ControlPlaneLoadBalancer = *oldObj.Spec.ControlPlaneLoadBalancer.DeepCopy()
			tc.mutate(obj)

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(tc.errPath))
		})
	}
}

func TestClusterValidateUpdate_MultipleImmutableChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Region = "lpg"
	obj.Spec.Zone = "lpg1"
	obj.Spec.Networks[0].CIDR = "10.1.0.0/24"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.region"))
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
	g.Expect(err.Error()).To(ContainSubstring("cidr"))
}

// ============================================================================
// Tests for CloudscaleCluster Validating Webhook - Delete
// ============================================================================

func TestClusterValidateDelete_AlwaysSucceeds(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()

	_, err := validator.ValidateDelete(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// ============================================================================
// Tests for validateGatewayInCIDR
// ============================================================================

func TestValidateGatewayInCIDR_ValidGateway(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR(defaultSubnetCIDR, "172.18.0.1", field.NewPath("spec", "networks", "gatewayAddress"))
	g.Expect(errs).To(BeEmpty())
}

func TestValidateGatewayInCIDR_OutsideCIDR(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR(defaultSubnetCIDR, "192.168.1.1", field.NewPath("spec", "networks", "gatewayAddress"))
	g.Expect(errs).To(HaveLen(1))
}

func TestValidateGatewayInCIDR_InvalidIP(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR(defaultSubnetCIDR, "invalid", field.NewPath("spec", "networks", "gatewayAddress"))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0].Detail).To(ContainSubstring("invalid IP"))
}

func TestValidateGatewayInCIDR_InvalidCIDR(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR("notacidr", "10.0.0.1", field.NewPath("spec", "networks", "gatewayAddress"))
	g.Expect(errs).To(BeEmpty())
}
