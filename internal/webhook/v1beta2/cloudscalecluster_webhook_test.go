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
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const (
	RegionRma = "rma"
	ZoneRma1  = "rma1"
)

func newClusterWebhookTestObjects() (
	obj *infrastructurev1beta2.CloudscaleCluster,
	oldObj *infrastructurev1beta2.CloudscaleCluster,
	validator CloudscaleClusterCustomValidator,
	defaulter CloudscaleClusterCustomDefaulter,
) {
	obj = &infrastructurev1beta2.CloudscaleCluster{}
	oldObj = &infrastructurev1beta2.CloudscaleCluster{}
	validator = CloudscaleClusterCustomValidator{
		RegionInfo: testutils.NewTestRegionInfo(),
	}
	defaulter = CloudscaleClusterCustomDefaulter{
		RegionInfo: testutils.NewTestRegionInfo(),
	}
	return
}

// ============================================================================
// CloudscaleCluster Defaulting Webhook
// ============================================================================

func TestClusterDefaulting(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(c *infrastructurev1beta2.CloudscaleCluster)
		assert func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster)
	}{
		{
			name: "zone derived from region when empty",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ""
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Zone).To(Equal(ZoneRma1))
			},
		},
		{
			name: "explicit zone preserved",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Zone).To(Equal(ZoneRma1))
			},
		},
		{
			name: "networks default to cluster name + default CIDR",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Name = "my-cluster"
				c.Spec.Region = RegionRma
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Networks).To(HaveLen(1))
				g.Expect(c.Spec.Networks[0].Name).To(Equal("my-cluster"))
				g.Expect(c.Spec.Networks[0].CIDR).To(Equal(defaultSubnetCIDR))
			},
		},
		{
			name: "explicit networks preserved",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "custom", CIDR: "10.1.0.0/16"},
				}
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Networks).To(HaveLen(1))
				g.Expect(c.Spec.Networks[0].Name).To(Equal("custom"))
				g.Expect(c.Spec.Networks[0].CIDR).To(Equal("10.1.0.0/16"))
			},
		},
		{
			name:   "LB.Enabled defaults to true",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) { c.Spec.ControlPlaneLoadBalancer.Enabled = nil },
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(new(true)))
			},
		},
		{
			name: "LB.Enabled=false preserved",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(new(false)))
			},
		},
		{
			name:   "LB.Algorithm defaults to round_robin",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) { c.Spec.ControlPlaneLoadBalancer.Algorithm = "" },
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
			},
		},
		{
			name:   "LB.Flavor defaults to lb-standard",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) { c.Spec.ControlPlaneLoadBalancer.Flavor = "" },
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
			},
		},
		{
			name: "LB.APIServerPort defaults to 6443",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.APIServerPort = 0
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
			},
		},
		{
			name:   "LB.HealthMonitor zero-values default to canonical numbers",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
			},
		},
		{
			name: "explicit LB.HealthMonitor preserved",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.HealthMonitor = infrastructurev1beta2.HealthMonitorSpec{
					DelayS: 10, TimeoutS: 7, UpThreshold: 5, DownThreshold: 8,
				}
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(10))
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(7))
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(5))
				g.Expect(c.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(8))
			},
		},
		{
			name: "FloatingIP managed defaults IPFamily to IPv4",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.FloatingIP.IPFamily).To(Equal(new(infrastructurev1beta2.IPFamilyIPv4)))
			},
		},
		{
			name: "FloatingIP with explicit Address keeps IPFamily nil",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{Address: "1.2.3.4"}
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.FloatingIP.IPFamily).To(BeNil())
				g.Expect(c.Spec.FloatingIP.Address).To(Equal("1.2.3.4"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			obj, _, _, defaulter := newClusterWebhookTestObjects()
			tc.mutate(obj)
			g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
			tc.assert(g, obj)
		})
	}
}

// TestClusterDefaulting_AllDefaultsApplied is the end-to-end smoke test for
// the defaulter: a minimal spec should come out fully populated.
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
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Enabled).To(Equal(new(true)))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Algorithm).To(Equal("round_robin"))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.Flavor).To(Equal("lb-standard"))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.APIServerPort).To(Equal(int32(6443)))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DelayS).To(Equal(5))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.TimeoutS).To(Equal(3))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.UpThreshold).To(Equal(2))
	g.Expect(obj.Spec.ControlPlaneLoadBalancer.HealthMonitor.DownThreshold).To(Equal(3))
}

// ============================================================================
// CloudscaleCluster Validating Webhook - Create
// ============================================================================

func TestClusterValidateCreate(t *testing.T) {
	cases := []struct {
		name           string
		mutate         func(c *infrastructurev1beta2.CloudscaleCluster)
		wantErr        bool
		wantSubstrings []string
	}{
		{
			name: "minimal valid cluster",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}
			},
		},
		{
			name: "zone not in region",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = "lpg1"
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.zone"},
		},
		{
			name: "unknown zone",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = "xyz1"
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.zone"},
		},
		{
			name: "empty zone is fine (defaulter would fill it)",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ""
			},
		},
		{
			name: "network missing both UUID and CIDR",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "bad"}}
			},
			wantErr:        true,
			wantSubstrings: []string{"exactly one of uuid or cidr"},
		},
		{
			name: "network with both UUID and CIDR",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "bad", UUID: "some-uuid", CIDR: "10.0.0.0/24"},
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"exactly one of uuid or cidr"},
		},
		{
			name: "duplicate network names",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "dup", CIDR: "10.0.0.0/24"},
					{Name: "dup", CIDR: "10.1.0.0/24"},
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"Duplicate"},
		},
		{
			name: "gateway within CIDR is valid",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "172.18.0.1"},
				}
			},
		},
		{
			name: "gateway outside CIDR is invalid",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "192.168.1.1"},
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"gatewayAddress"},
		},
		{
			name: "invalid gateway IP",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: defaultSubnetCIDR, GatewayAddress: "notanip"},
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"gatewayAddress"},
		},
		{
			name: "gateway on pre-existing (UUID-only) network rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "pre-existing", UUID: "some-uuid", GatewayAddress: "10.0.0.1"},
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"gatewayAddress"},
		},
		{
			name: "LB.Network refers to existing network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}
				c.Spec.ControlPlaneLoadBalancer.Network = "main"
			},
		},
		{
			name: "public LB with multiple networks requires explicit network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: defaultSubnetCIDR},
					{Name: "aux", CIDR: "10.1.0.0/24"},
				}
				c.Spec.ControlPlaneLoadBalancer.Network = ""
			},
			wantErr:        true,
			wantSubstrings: []string{"controlPlaneLoadBalancer.network"},
		},
		{
			name: "LB.Network references unknown network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}
				c.Spec.ControlPlaneLoadBalancer.Network = "nonexistent"
			},
			wantErr:        true,
			wantSubstrings: []string{"controlPlaneLoadBalancer.network"},
		},
		{
			name: "managed FloatingIP with IPFamily is valid",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
					IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
				}
			},
		},
		{
			name: "FloatingIP with private LB rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}
				c.Spec.ControlPlaneLoadBalancer.Network = "main"
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
					IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"floatingIP", "private"},
		},
		{
			name: "FloatingIP with both IPFamily and Address rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
					IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
					Address:  "1.2.3.4",
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.floatingIP", "exactly one of ipFamily or ip must be specified"},
		},
		{
			name: "pre-existing FloatingIP with invalid IP",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{Address: "not-an-ip"}
			},
			wantErr:        true,
			wantSubstrings: []string{"floatingIP.ip"},
		},
		{
			name: "FloatingIP with neither IPFamily nor Address rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
			},
			wantErr:        true,
			wantSubstrings: []string{"floatingIP"},
		},
		{
			name: "managed FloatingIP without LB rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
					IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"managed floating IP"},
		},
		{
			name: "pre-existing FloatingIP without LB allowed",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
				c.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{Address: "1.2.3.4"}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			obj, _, validator, _ := newClusterWebhookTestObjects()
			tc.mutate(obj)

			_, err := validator.ValidateCreate(ctx, obj)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				for _, s := range tc.wantSubstrings {
					g.Expect(err.Error()).To(ContainSubstring(s))
				}
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

// ============================================================================
// CloudscaleCluster Validating Webhook - Update
// ============================================================================

// setupUpdateTestObjects returns a matched old/new pair representing a valid
// existing cluster; tests then mutate the new object to provoke update errors.
func setupUpdateTestObjects() (
	obj *infrastructurev1beta2.CloudscaleCluster,
	oldObj *infrastructurev1beta2.CloudscaleCluster,
	validator CloudscaleClusterCustomValidator,
) {
	obj, oldObj, validator, _ = newClusterWebhookTestObjects()
	networks := []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}

	oldObj.Spec.Region = RegionRma
	oldObj.Spec.Zone = ZoneRma1
	oldObj.Spec.Networks = networks
	oldObj.Spec.ControlPlaneLoadBalancer.Enabled = new(true)

	obj.Spec.Region = RegionRma
	obj.Spec.Zone = ZoneRma1
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}
	obj.Spec.ControlPlaneLoadBalancer.Enabled = new(true)
	return
}

func TestClusterValidateUpdate(t *testing.T) {
	cases := []struct {
		name           string
		mutate         func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster)
		wantErr        bool
		wantSubstrings []string
	}{
		{
			name:   "no changes",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {},
		},
		{
			name: "region change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.Region = "lpg"
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.region"},
		},
		{
			name: "zone change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.Zone = "rma2"
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.zone"},
		},
		{
			name: "network CIDR change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.Networks[0].CIDR = "10.1.0.0/24"
			},
			wantErr:        true,
			wantSubstrings: []string{"cidr"},
		},
		{
			name: "network removed rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.Networks = nil
			},
			wantErr:        true,
			wantSubstrings: []string{"removing network"},
		},
		{
			name: "network added — pinned via LB.Network",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.Networks = append(obj.Spec.Networks, infrastructurev1beta2.NetworkSpec{
					Name: "extra", CIDR: "10.1.0.0/24",
				})
				oldObj.Spec.ControlPlaneLoadBalancer.Network = "main"
				obj.Spec.ControlPlaneLoadBalancer.Network = "main"
			},
		},
		{
			name: "LB.Enabled change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.controlPlaneLoadBalancer.enabled"},
		},
		{
			name: "LB.Network change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				oldObj.Spec.ControlPlaneLoadBalancer.Network = "main"
				obj.Spec.ControlPlaneLoadBalancer.Network = "other"
			},
			wantErr:        true,
			wantSubstrings: []string{"controlPlaneLoadBalancer.network"},
		},
		{
			name: "ControlPlaneEndpoint host change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
				obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "5.6.7.8", Port: 6443}
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.controlPlaneEndpoint.host"},
		},
		{
			name: "ControlPlaneEndpoint port change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
				obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 8443}
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.controlPlaneEndpoint.port"},
		},
		{
			name: "ControlPlaneEndpoint set when previously empty",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				oldObj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{}
				obj.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: "1.2.3.4", Port: 6443}
			},
		},
		{
			name: "FloatingIP added rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
					IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
				}
			},
			wantErr:        true,
			wantSubstrings: []string{"floatingIP"},
		},
		{
			name: "FloatingIP removed rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				oldObj.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
					IPFamily: new(infrastructurev1beta2.IPFamilyIPv4),
				}
				obj.Spec.FloatingIP = nil
			},
			wantErr:        true,
			wantSubstrings: []string{"floatingIP"},
		},
		{
			name: "multiple immutable changes surface all errors",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				obj.Spec.Region = "lpg"
				obj.Spec.Zone = "lpg1"
				obj.Spec.Networks[0].CIDR = "10.1.0.0/24"
			},
			wantErr:        true,
			wantSubstrings: []string{"spec.region", "spec.zone", "cidr"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			obj, oldObj, validator := setupUpdateTestObjects()
			tc.mutate(oldObj, obj)

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				for _, s := range tc.wantSubstrings {
					g.Expect(err.Error()).To(ContainSubstring(s))
				}
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

// TestClusterValidateUpdate_NetworkReorderedCIDRChangedReportsNewIndex pins the
// error-path reporting behaviour: when networks are reordered AND a CIDR is
// changed, the field path must reference the NEW index, not the old one.
func TestClusterValidateUpdate_NetworkReorderedCIDRChangedReportsNewIndex(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "net-a", CIDR: "10.1.0.0/24"},
		{Name: "net-b", CIDR: "10.2.0.0/24"},
	}
	obj.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "net-b", CIDR: "10.2.0.0/24"},
		{Name: "net-a", CIDR: "10.9.0.0/24"},
	}

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.networks[1].cidr"),
		"error message must point at net-a's new index (1), not its old index (0)")
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

// ============================================================================
// CloudscaleCluster Validating Webhook - Delete
// ============================================================================

func TestClusterValidateDelete_AlwaysSucceeds(t *testing.T) {
	g := NewWithT(t)
	obj, _, validator, _ := newClusterWebhookTestObjects()

	_, err := validator.ValidateDelete(ctx, obj)
	g.Expect(err).NotTo(HaveOccurred())
}

// ============================================================================
// Unit tests for validateGatewayInCIDR
// ============================================================================

func TestValidateGatewayInCIDR(t *testing.T) {
	path := field.NewPath("spec", "networks", "gatewayAddress")
	cases := []struct {
		name       string
		cidr       string
		gateway    string
		wantErrs   int
		wantDetail string
	}{
		{name: "valid gateway", cidr: defaultSubnetCIDR, gateway: "172.18.0.1", wantErrs: 0},
		{name: "outside CIDR", cidr: defaultSubnetCIDR, gateway: "192.168.1.1", wantErrs: 1},
		{name: "invalid IP", cidr: defaultSubnetCIDR, gateway: "invalid", wantErrs: 1, wantDetail: "invalid IP"},
		// Invalid CIDR is silently accepted — the CIDR field is validated elsewhere.
		{name: "invalid CIDR", cidr: "notacidr", gateway: "10.0.0.1", wantErrs: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			errs := validateGatewayInCIDR(tc.cidr, tc.gateway, path)
			g.Expect(errs).To(HaveLen(tc.wantErrs))
			if tc.wantDetail != "" {
				g.Expect(errs[0].Detail).To(ContainSubstring(tc.wantDetail))
			}
		})
	}
}
