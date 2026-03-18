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

func TestClusterDefaulting_CIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Network.CIDR = ""

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Network.CIDR).To(Equal(defaultSubnetCIDR))
}

func TestClusterDefaulting_ExplicitCIDRNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Network.CIDR = "10.1.0.0/16"

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Network.CIDR).To(Equal("10.1.0.0/16"))
}

func TestClusterDefaulting_GatewayToEmptyString(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Network.GatewayAddress = nil

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Network.GatewayAddress).To(Equal(ptr.To("")))
}

func TestClusterDefaulting_ExplicitGatewayNotOverridden(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	g.Expect(obj.Spec.Network.GatewayAddress).To(Equal(ptr.To("10.0.0.1")))
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

func TestClusterDefaulting_AllDefaultsApplied(t *testing.T) {
	g := NewWithT(t)
	obj, _, _, defaulter := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())

	g.Expect(obj.Spec.Zone).To(Equal(ZoneRma1))
	g.Expect(obj.Spec.Network.CIDR).To(Equal(defaultSubnetCIDR))
	g.Expect(obj.Spec.Network.GatewayAddress).To(Equal(ptr.To("")))
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

func TestClusterValidateCreate_GatewayWithinCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Network.CIDR = defaultSubnetCIDR
	obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_GatewayOutsideCIDR(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Network.CIDR = defaultSubnetCIDR
	obj.Spec.Network.GatewayAddress = ptr.To("192.168.1.1")

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
}

func TestClusterValidateCreate_InvalidGatewayIP(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Network.CIDR = defaultSubnetCIDR
	obj.Spec.Network.GatewayAddress = ptr.To("notanip")

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
}

func TestClusterValidateCreate_EmptyGatewayString(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Network.GatewayAddress = ptr.To("")

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateCreate_NilGateway(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()
	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Network.GatewayAddress = nil

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
	oldObj.Spec.Region = RegionRma
	oldObj.Spec.Zone = ZoneRma1
	oldObj.Spec.Network.CIDR = defaultSubnetCIDR
	oldObj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
	oldObj.Spec.Network.GatewayAddress = ptr.To("")

	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Network.CIDR = defaultSubnetCIDR
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
	obj.Spec.Network.GatewayAddress = ptr.To("")
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

func TestClusterValidateUpdate_CIDRChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Network.CIDR = "10.1.0.0/24"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.network.cidr"))
}

func TestClusterValidateUpdate_LBEnabledChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.controlPlaneLoadBalancer.enabled"))
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

func TestClusterValidateUpdate_GatewayChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.Network.GatewayAddress = ptr.To("")
	obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
}

func TestClusterValidateUpdate_GatewayNilToValue(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.Network.GatewayAddress = nil
	obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
}

func TestClusterValidateUpdate_GatewayNilToNil(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.Network.GatewayAddress = nil
	obj.Spec.Network.GatewayAddress = nil

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateUpdate_MutableFieldsChange(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.ControlPlaneLoadBalancer.Algorithm = "least_connections"
	obj.Spec.ControlPlaneLoadBalancer.Flavor = "lb-premium"
	obj.Spec.ControlPlaneLoadBalancer.APIServerPort = 8443
	obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 10

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterValidateUpdate_MultipleImmutableChanges(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	obj.Spec.Region = "lpg"
	obj.Spec.Zone = "lpg1"
	obj.Spec.Network.CIDR = "10.1.0.0/24"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.region"))
	g.Expect(err.Error()).To(ContainSubstring("spec.zone"))
	g.Expect(err.Error()).To(ContainSubstring("spec.network.cidr"))
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
	errs := validateGatewayInCIDR(defaultSubnetCIDR, "10.0.0.1", field.NewPath("spec", "network", "gatewayAddress"))
	g.Expect(errs).To(BeEmpty())
}

func TestValidateGatewayInCIDR_OutsideCIDR(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR(defaultSubnetCIDR, "192.168.1.1", field.NewPath("spec", "network", "gatewayAddress"))
	g.Expect(errs).To(HaveLen(1))
}

func TestValidateGatewayInCIDR_InvalidIP(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR(defaultSubnetCIDR, "invalid", field.NewPath("spec", "network", "gatewayAddress"))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0].Detail).To(ContainSubstring("invalid IP"))
}

func TestValidateGatewayInCIDR_InvalidCIDR(t *testing.T) {
	g := NewWithT(t)
	errs := validateGatewayInCIDR("notacidr", "10.0.0.1", field.NewPath("spec", "network", "gatewayAddress"))
	g.Expect(errs).To(BeEmpty())
}
