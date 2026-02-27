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
	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v6"
	. "github.com/onsi/ginkgo/v2"
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
		{Slug: RegionRma, Zones: []cloudscalesdk.Zone{{Slug: ZoneRma1}}},
		{Slug: "lpg", Zones: []cloudscalesdk.Zone{{Slug: "lpg1"}}},
	})
}

var _ = Describe("CloudscaleCluster Webhook", func() {
	var (
		obj       *infrastructurev1beta2.CloudscaleCluster
		oldObj    *infrastructurev1beta2.CloudscaleCluster
		validator CloudscaleClusterCustomValidator
		defaulter CloudscaleClusterCustomDefaulter
	)

	BeforeEach(func() {
		obj = &infrastructurev1beta2.CloudscaleCluster{}
		oldObj = &infrastructurev1beta2.CloudscaleCluster{}
		validator = CloudscaleClusterCustomValidator{
			RegionInfo: newTestRegionInfo(),
		}
		defaulter = CloudscaleClusterCustomDefaulter{
			RegionInfo: newTestRegionInfo(),
		}
	})

	Context("When creating CloudscaleCluster under Defaulting Webhook", func() {
		It("Should default network zone from region", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ""

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Network.Zone).To(Equal(ZoneRma1))
		})

		It("Should not override explicit zone", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Network.Zone).To(Equal(ZoneRma1))
		})

		It("Should default CIDR", func() {
			obj.Spec.Network.CIDR = ""

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Network.CIDR).To(Equal(defaultSubnetCIDR))
		})

		It("Should not override explicit CIDR", func() {
			obj.Spec.Network.CIDR = "10.1.0.0/16"

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Network.CIDR).To(Equal("10.1.0.0/16"))
		})

		It("Should default gateway to empty string", func() {
			obj.Spec.Network.GatewayAddress = nil

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Network.GatewayAddress).To(Equal(ptr.To("")))
		})

		It("Should not override explicit gateway", func() {
			obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Network.GatewayAddress).To(Equal(ptr.To("10.0.0.1")))
		})

		It("Should default LB enabled to true", func() {
			obj.Spec.ControlPlaneLoadBalancer.Enabled = nil

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(ptr.To(true)))
		})

		It("Should not override LB enabled=false", func() {
			obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(ptr.To(false)))
		})

		It("Should default LB algorithm", func() {
			obj.Spec.ControlPlaneLoadBalancer.Algorithm = ""

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
		})

		It("Should default LB flavor", func() {
			obj.Spec.ControlPlaneLoadBalancer.Flavor = ""

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
		})

		It("Should default API server port", func() {
			obj.Spec.ControlPlaneLoadBalancer.APIServerPort = 0

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
		})

		It("Should default all health monitor fields", func() {
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
		})

		It("Should not override explicit health monitor values", func() {
			obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 10
			obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS = 7
			obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold = 5
			obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold = 8

			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(10))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(7))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(5))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(8))
		})

		It("Should apply all defaults to empty spec", func() {
			obj.Spec.Region = RegionRma

			Expect(defaulter.Default(ctx, obj)).To(Succeed())

			Expect(obj.Spec.Network.Zone).To(Equal(ZoneRma1))
			Expect(obj.Spec.Network.CIDR).To(Equal(defaultSubnetCIDR))
			Expect(obj.Spec.Network.GatewayAddress).To(Equal(ptr.To("")))
			Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(ptr.To(true)))
			Expect(obj.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
			Expect(obj.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
			Expect(obj.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
			Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
		})
	})

	Context("When creating CloudscaleCluster under Validating Webhook", func() {
		It("Should accept a valid cluster", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject zone not belonging to region", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = "lpg1"

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.zone"))
		})

		It("Should reject unknown zone", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = "xyz1"

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.zone"))
		})

		It("Should accept empty zone (defaulted before validation)", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ""

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should accept gateway within CIDR", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1
			obj.Spec.Network.CIDR = defaultSubnetCIDR
			obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject gateway outside CIDR", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1
			obj.Spec.Network.CIDR = defaultSubnetCIDR
			obj.Spec.Network.GatewayAddress = ptr.To("192.168.1.1")

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
		})

		It("Should reject invalid gateway IP", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1
			obj.Spec.Network.CIDR = defaultSubnetCIDR
			obj.Spec.Network.GatewayAddress = ptr.To("notanip")

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
		})

		It("Should accept empty gateway string", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1
			obj.Spec.Network.GatewayAddress = ptr.To("")

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should accept nil gateway", func() {
			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1
			obj.Spec.Network.GatewayAddress = nil

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When updating CloudscaleCluster under Validating Webhook", func() {
		BeforeEach(func() {
			oldObj.Spec.Region = RegionRma
			oldObj.Spec.Network.Zone = ZoneRma1
			oldObj.Spec.Network.CIDR = defaultSubnetCIDR
			oldObj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
			oldObj.Spec.Network.GatewayAddress = ptr.To("")

			obj.Spec.Region = RegionRma
			obj.Spec.Network.Zone = ZoneRma1
			obj.Spec.Network.CIDR = defaultSubnetCIDR
			obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
			obj.Spec.Network.GatewayAddress = ptr.To("")
		})

		It("Should accept no changes", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject region change", func() {
			obj.Spec.Region = "lpg"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.region"))
		})

		It("Should reject network zone change", func() {
			obj.Spec.Network.Zone = "rma2"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.zone"))
		})

		It("Should reject network CIDR change", func() {
			obj.Spec.Network.CIDR = "10.1.0.0/24"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.cidr"))
		})

		It("Should reject LB enabled change", func() {
			obj.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.controlPlaneLoadBalancer.enabled"))
		})

		It("Should reject ControlPlaneEndpoint host change once set", func() {
			oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
			obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "5.6.7.8", Port: 6443}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.controlPlaneEndpoint.host"))
		})

		It("Should reject ControlPlaneEndpoint port change once set", func() {
			oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
			obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 8443}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.controlPlaneEndpoint.port"))
		})

		It("Should allow setting ControlPlaneEndpoint when previously empty", func() {
			oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "", Port: 0}
			obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject gateway change", func() {
			oldObj.Spec.Network.GatewayAddress = ptr.To("")
			obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
		})

		It("Should reject gateway nil to value change", func() {
			oldObj.Spec.Network.GatewayAddress = nil
			obj.Spec.Network.GatewayAddress = ptr.To("10.0.0.1")

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.network.gatewayAddress"))
		})

		It("Should accept gateway nil to nil", func() {
			oldObj.Spec.Network.GatewayAddress = nil
			obj.Spec.Network.GatewayAddress = nil

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should allow mutable fields to change", func() {
			obj.Spec.ControlPlaneLoadBalancer.Algorithm = "least_connections"
			obj.Spec.ControlPlaneLoadBalancer.Flavor = "lb-premium"
			obj.Spec.ControlPlaneLoadBalancer.APIServerPort = 8443
			obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS = 10

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should report multiple immutable field changes", func() {
			obj.Spec.Region = "lpg"
			obj.Spec.Network.Zone = "lpg1"
			obj.Spec.Network.CIDR = "10.1.0.0/24"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.region"))
			Expect(err.Error()).To(ContainSubstring("spec.network.zone"))
			Expect(err.Error()).To(ContainSubstring("spec.network.cidr"))
		})
	})

	Context("When deleting CloudscaleCluster under Validating Webhook", func() {
		It("Should always succeed", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("validateGatewayInCIDR", func() {
		It("Should accept valid gateway in CIDR", func() {
			errs := validateGatewayInCIDR(defaultSubnetCIDR, "10.0.0.1", field.NewPath("spec", "network", "gatewayAddress"))
			Expect(errs).To(BeEmpty())
		})

		It("Should reject gateway outside CIDR", func() {
			errs := validateGatewayInCIDR(defaultSubnetCIDR, "192.168.1.1", field.NewPath("spec", "network", "gatewayAddress"))
			Expect(errs).To(HaveLen(1))
		})

		It("Should reject invalid gateway IP", func() {
			errs := validateGatewayInCIDR(defaultSubnetCIDR, "invalid", field.NewPath("spec", "network", "gatewayAddress"))
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Detail).To(ContainSubstring("invalid IP"))
		})

		It("Should return no errors for invalid CIDR", func() {
			errs := validateGatewayInCIDR("notacidr", "10.0.0.1", field.NewPath("spec", "network", "gatewayAddress"))
			Expect(errs).To(BeEmpty())
		})
	})
})
