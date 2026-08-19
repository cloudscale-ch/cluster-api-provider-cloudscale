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
	"fmt"
	"net/netip"
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

// routerNetworks is the network set the router defaulting cases start from.
func routerNetworks(cidr string) []infrastructurev1beta2.NetworkSpec {
	return []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: cidr}}
}

// routerWith builds a single router named "r" attached to the given interfaces.
func routerWith(ifaces ...infrastructurev1beta2.RouterInterfaceSpec) []infrastructurev1beta2.RouterSpec {
	return []infrastructurev1beta2.RouterSpec{{Name: "r", InternetGateway: true, Interfaces: ifaces}}
}

// routerIface builds an interface on network "main"; an empty address leaves it for the
// defaulter to fill in.
func routerIface(address string) infrastructurev1beta2.RouterInterfaceSpec {
	return infrastructurev1beta2.RouterInterfaceSpec{Network: "main", Address: address}
}

// adoptedRouterWith builds a single router named "r" that adopts a pre-existing router by
// uuid, which is the only kind that can carry interfaces to adopt.
func adoptedRouterWith(ifaces ...infrastructurev1beta2.RouterInterfaceSpec) []infrastructurev1beta2.RouterSpec {
	return []infrastructurev1beta2.RouterSpec{{Name: "r", UUID: "router-uuid", Interfaces: ifaces}}
}

// routerIfaceAdopted builds an interface on network "main" that adopts a pre-existing
// interface, whose address is read off that interface rather than requested.
func routerIfaceAdopted() infrastructurev1beta2.RouterInterfaceSpec {
	iface := routerIface("")
	iface.UUID = "iface-uuid"
	return iface
}

// routerIfaceNoGateway is a routerIface that leaves the subnet gateway to someone else.
func routerIfaceNoGateway(address string) infrastructurev1beta2.RouterInterfaceSpec {
	iface := routerIface(address)
	iface.ConfigureSubnetGateway = new(false)
	return iface
}

// routersEach builds one router per interface, named r0, r1, ... Interfaces that share a
// network have to sit on separate routers: spec.routers[].interfaces is a list-map keyed
// by network, so a single router can hold at most one interface per network.
func routersEach(ifaces ...infrastructurev1beta2.RouterInterfaceSpec) []infrastructurev1beta2.RouterSpec {
	routers := make([]infrastructurev1beta2.RouterSpec, len(ifaces))
	for i, iface := range ifaces {
		routers[i] = infrastructurev1beta2.RouterSpec{
			Name:       fmt.Sprintf("r%d", i),
			Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{iface},
		}
	}
	return routers
}

// setupRouterCluster fills in the region/zone a valid cluster needs and attaches the
// given networks and routers.
func setupRouterCluster(c *infrastructurev1beta2.CloudscaleCluster, networks []infrastructurev1beta2.NetworkSpec, routers []infrastructurev1beta2.RouterSpec) {
	c.Spec.Region = RegionRma
	c.Spec.Zone = ZoneRma1
	c.Spec.Networks = networks
	c.Spec.Routers = routers
}

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
		{
			name: "interface address and configureSubnetGateway are both defaulted",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.0/24")
				c.Spec.Routers = routerWith(routerIface(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				iface := c.Spec.Routers[0].Interfaces[0]
				g.Expect(iface.Address).To(Equal("10.0.0.1"))
				g.Expect(iface.ConfigureSubnetGateway).To(Equal(new(true)))
			},
		},
		{
			name: "two routers on one network get distinct addresses",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.0/24")
				c.Spec.Routers = routersEach(routerIface(""), routerIfaceNoGateway(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.1"))
				g.Expect(c.Spec.Routers[1].Interfaces[0].Address).To(Equal("10.0.0.2"))
			},
		},
		{
			name: "explicit address is never handed out twice",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.0/24")
				c.Spec.Routers = routersEach(routerIface("10.0.0.1"), routerIfaceNoGateway(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.1"))
				g.Expect(c.Spec.Routers[1].Interfaces[0].Address).To(Equal("10.0.0.2"))
			},
		},
		{
			name: "gateway owner takes the network's gatewayAddress",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: "10.0.0.0/24", GatewayAddress: "10.0.0.9"},
				}
				c.Spec.Routers = routerWith(routerIface(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.9"))
			},
		},
		{
			name: "non-gateway interface does not take the network's gatewayAddress",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: "10.0.0.0/24", GatewayAddress: "10.0.0.9"},
				}
				iface := routerIface("")
				iface.ConfigureSubnetGateway = new(false)
				c.Spec.Routers = routerWith(iface)
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.1"))
			},
		},
		{
			name: "CIDR with host bits allocates from the masked network address",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.5/24")
				c.Spec.Routers = routerWith(routerIface(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.1"))
			},
		},
		{
			// A network referenced by uuid has no CIDR here, so there is nothing to derive
			// an address from. It must be left empty rather than filled with garbage.
			name: "interface on a uuid network is left empty",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", UUID: "net-uuid"}}
				c.Spec.Routers = routerWith(routerIface(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(BeEmpty())
			},
		},
		{
			// An adopted interface holds whatever address it already has, which no
			// amount of CIDR arithmetic here can know.
			name: "adopted interface is left empty even on a CIDR network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.0/24")
				c.Spec.Routers = adoptedRouterWith(routerIfaceAdopted())
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(BeEmpty())
				// Everything else is still defaulted as usual.
				g.Expect(c.Spec.Routers[0].Interfaces[0].ConfigureSubnetGateway).To(HaveValue(BeTrue()))
			},
		},
		{
			// The address an adopted interface does not take must stay available.
			name: "adopted interface does not consume an address",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.0/24")
				c.Spec.Routers = routersEach(routerIfaceAdopted(), routerIface(""))
				c.Spec.Routers[0].UUID = "router-uuid"
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(BeEmpty())
				g.Expect(c.Spec.Routers[1].Interfaces[0].Address).To(Equal("10.0.0.1"))
			},
		},
		{
			// A /30 holds .0 through .3, of which only .1 and .2 are assignable: .0 is the
			// network address and .3 the broadcast address. The third router has nowhere
			// to go.
			name: "exhausted CIDR leaves the surplus interface empty",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Networks = routerNetworks("10.0.0.0/30")
				c.Spec.Routers = routersEach(routerIface(""),
					routerIfaceNoGateway(""), routerIfaceNoGateway(""))
			},
			assert: func(g *WithT, c *infrastructurev1beta2.CloudscaleCluster) {
				g.Expect(c.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.1"))
				g.Expect(c.Spec.Routers[1].Interfaces[0].Address).To(Equal("10.0.0.2"))
				g.Expect(c.Spec.Routers[2].Interfaces[0].Address).To(BeEmpty())
				// The broadcast address must go to nobody rather than to the last router.
				for _, router := range c.Spec.Routers {
					g.Expect(router.Interfaces[0].Address).ToNot(Equal("10.0.0.3"))
				}
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
	g.Expect(obj.Spec.Routers).To(BeEmpty())
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
			wantSubstrings: []string{"controlPlaneLoadBalancer.network", "controlPlaneLoadBalancer.poolMemberNetwork"},
		},
		{
			name: "public LB with multiple networks accepts poolMemberNetwork instead",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
					{Name: "main", CIDR: defaultSubnetCIDR},
					{Name: "aux", CIDR: "10.1.0.0/24"},
				}
				c.Spec.ControlPlaneLoadBalancer.Network = ""
				c.Spec.ControlPlaneLoadBalancer.PoolMemberNetwork = "aux"
			},
		},
		{
			name: "LB.PoolMemberNetwork references unknown network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.Region = RegionRma
				c.Spec.Zone = ZoneRma1
				c.Spec.Networks = []infrastructurev1beta2.NetworkSpec{{Name: "main", CIDR: defaultSubnetCIDR}}
				c.Spec.ControlPlaneLoadBalancer.PoolMemberNetwork = "nope"
			},
			wantErr:        true,
			wantSubstrings: []string{"controlPlaneLoadBalancer.poolMemberNetwork"},
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
		{
			name: "valid router",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routerWith(routerIface("10.0.0.1")))
			},
		},
		{
			name: "nil configureSubnetGateway is treated as true",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				iface := routerIface("10.0.0.1")
				iface.ConfigureSubnetGateway = nil
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routerWith(iface))
			},
		},
		{
			name: "router interface address inside the DHCP pool",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				networks := routerNetworks("10.0.0.0/24")
				networks[0].GatewayAddress = "10.0.0.150"
				setupRouterCluster(c, networks, routerWith(routerIface("10.0.0.150")))
			},
			wantErr:        true,
			wantSubstrings: []string{"DHCP pool .101-.254", "cannot be assigned to a router interface"},
		},
		{
			name: "router interface on the last address below the DHCP pool",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				networks := routerNetworks("10.0.0.0/24")
				networks[0].GatewayAddress = "10.0.0.100"
				setupRouterCluster(c, networks, routerWith(routerIface("10.0.0.100")))
			},
		},
		{
			name: "router interface on the broadcast address",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				networks := routerNetworks("10.0.0.0/24")
				networks[0].GatewayAddress = "10.0.0.255"
				setupRouterCluster(c, networks, routerWith(routerIface("10.0.0.255")))
			},
			wantErr:        true,
			wantSubstrings: []string{"it is the broadcast address"},
		},
		{
			name: "router interface address outside the network CIDR",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routerWith(routerIface("192.168.1.1")))
			},
			wantErr:        true,
			wantSubstrings: []string{"must be within CIDR"},
		},
		{
			name: "router interface referencing an unknown network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				iface := routerIface("10.0.0.1")
				iface.Network = "nope"
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routerWith(iface))
			},
			wantErr:        true,
			wantSubstrings: []string{"routers[0].interfaces[0].network"},
		},
		{
			name: "duplicate addresses on the same network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"),
					routersEach(routerIface("10.0.0.1"), routerIfaceNoGateway("10.0.0.1")))
			},
			wantErr:        true,
			wantSubstrings: []string{"Duplicate value", "10.0.0.1"},
		},
		{
			name: "two subnet gateway owners on the same network",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"),
					routersEach(routerIface("10.0.0.1"), routerIface("10.0.0.2")))
			},
			wantErr:        true,
			wantSubstrings: []string{"only one interface may set configureSubnetGateway=true"},
		},
		{
			name: "same network on two different routers is allowed",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"),
					routersEach(routerIface("10.0.0.1"), routerIfaceNoGateway("10.0.0.2")))
			},
		},
		{
			name: "gateway owner address contradicts the network's gatewayAddress",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				networks := routerNetworks("10.0.0.0/24")
				networks[0].GatewayAddress = "10.0.0.5"
				setupRouterCluster(c, networks, routerWith(routerIface("10.0.0.1")))
			},
			wantErr:        true,
			wantSubstrings: []string{"must equal the network's gatewayAddress", "10.0.0.5"},
		},
		{
			name: "gateway owner address matching the network's gatewayAddress is valid",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				networks := routerNetworks("10.0.0.0/24")
				networks[0].GatewayAddress = "10.0.0.5"
				setupRouterCluster(c, networks, routerWith(routerIface("10.0.0.5")))
			},
		},
		{
			name: "uuid and internetGateway are mutually exclusive",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				routers := routerWith(routerIface("10.0.0.1"))
				routers[0].UUID = "router-uuid"
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routers)
			},
			wantErr:        true,
			wantSubstrings: []string{"uuid and internetGateway are mutually exclusive"},
		},
		{
			name: "adopted interface is accepted without an address",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c,
					[]infrastructurev1beta2.NetworkSpec{{Name: "main", UUID: "net-uuid"}},
					adoptedRouterWith(routerIfaceAdopted()))
			},
		},
		{
			name: "adopted interface with a requested address rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				iface := routerIfaceAdopted()
				iface.Address = "10.0.0.1"
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), adoptedRouterWith(iface))
			},
			wantErr:        true,
			wantSubstrings: []string{"routers[0].interfaces[0].address", "cannot be requested"},
		},
		{
			name: "adopted interface on a managed router rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routerWith(routerIfaceAdopted()))
			},
			wantErr:        true,
			wantSubstrings: []string{"routers[0].interfaces[0].uuid", "referenced by uuid"},
		},
		{
			name: "adopted gateway owner does not have to match the network's gatewayAddress",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				networks := routerNetworks("10.0.0.0/24")
				networks[0].GatewayAddress = "10.0.0.5"
				setupRouterCluster(c, networks, adoptedRouterWith(routerIfaceAdopted()))
			},
		},
		{
			name: "interface on a uuid network with an explicit address is accepted",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c,
					[]infrastructurev1beta2.NetworkSpec{{Name: "main", UUID: "net-uuid"}},
					routerWith(routerIface("192.168.77.1")))
			},
		},
		{
			name: "address-less interface on a uuid network rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c,
					[]infrastructurev1beta2.NetworkSpec{{Name: "main", UUID: "net-uuid"}},
					routerWith(routerIface("")))
			},
			wantErr:        true,
			wantSubstrings: []string{"must be set explicitly", "referenced by uuid"},
		},
		{
			name: "address-less interface on an exhausted CIDR rejected",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				setupRouterCluster(c, routerNetworks("10.0.0.0/24"), routerWith(routerIface("")))
			},
			wantErr:        true,
			wantSubstrings: []string{"no free address left"},
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

// TestClusterDefaultThenValidate_NonOwnerSquattingGatewayAddress runs both webhooks in
// the order the API server does. A non-owner interface claiming the network's declared
// gatewayAddress forces the defaulter to allocate the gateway owner a different address,
// which the validator must then reject as contradicting the network.
func TestClusterDefaultThenValidate_NonOwnerSquattingGatewayAddress(t *testing.T) {
	g := NewWithT(t)

	obj, _, validator, defaulter := newClusterWebhookTestObjects()

	networks := routerNetworks("10.0.0.0/24")
	networks[0].GatewayAddress = "10.0.0.1"
	squatter := routerIface("10.0.0.1")
	squatter.ConfigureSubnetGateway = new(false)
	owner := routerIface("")
	owner.ConfigureSubnetGateway = new(true)
	setupRouterCluster(obj, networks, []infrastructurev1beta2.RouterSpec{
		{Name: "r", Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{squatter}},
		{Name: "r2", Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{owner}},
	})

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())
	// The declared gateway address was taken, so the owner got the next free one.
	g.Expect(obj.Spec.Routers[1].Interfaces[0].Address).To(Equal("10.0.0.2"))

	_, err := validator.ValidateCreate(ctx, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("must equal the network's gatewayAddress"))
}

func TestClusterDefaulting_InterfaceAddedLater(t *testing.T) {
	g := NewWithT(t)

	obj, _, _, defaulter := newClusterWebhookTestObjects()
	setupRouterCluster(obj, routerNetworks("10.0.0.0/24"), routerWith(routerIface("10.0.0.1")))
	// A second router shows up later and asks for an address on the same network.
	obj.Spec.Routers = append(obj.Spec.Routers, infrastructurev1beta2.RouterSpec{
		Name:       "later",
		Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{routerIfaceNoGateway("")},
	})

	g.Expect(defaulter.Default(ctx, obj)).To(Succeed())

	g.Expect(obj.Spec.Routers[0].Interfaces[0].Address).To(Equal("10.0.0.1"))
	g.Expect(obj.Spec.Routers[1].Interfaces[0].Address).To(Equal("10.0.0.2"))
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

// seedRouter puts the same valid router on both sides of an update, so cases only need
// to express the change they are testing. The address sits inside defaultSubnetCIDR.
func seedRouter(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
	router := infrastructurev1beta2.RouterSpec{
		Name:            "r",
		InternetGateway: true,
		Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
			{Network: "main", Address: "172.18.0.1", ConfigureSubnetGateway: new(true)},
		},
	}
	oldObj.Spec.Routers = []infrastructurev1beta2.RouterSpec{*router.DeepCopy()}
	obj.Spec.Routers = []infrastructurev1beta2.RouterSpec{*router.DeepCopy()}
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
		{
			name: "router unchanged",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
			},
		},
		{
			name: "router uuid change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers[0].UUID = "other-uuid"
				obj.Spec.Routers[0].InternetGateway = false
			},
			wantErr:        true,
			wantSubstrings: []string{"routers[0].uuid", "immutable"},
		},
		{
			name: "router internetGateway change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers[0].InternetGateway = false
			},
			wantErr:        true,
			wantSubstrings: []string{"routers[0].internetGateway", "immutable"},
		},
		{
			name: "router interface address change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers[0].Interfaces[0].Address = "172.18.0.9"
			},
			wantErr:        true,
			wantSubstrings: []string{"address for network", "immutable"},
		},
		{
			name: "router interface uuid change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers[0].Interfaces[0].UUID = "iface-uuid"
			},
			wantErr:        true,
			wantSubstrings: []string{"uuid for network", "immutable"},
		},
		{
			name: "router interface configureSubnetGateway change rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers[0].Interfaces[0].ConfigureSubnetGateway = new(false)
			},
			wantErr:        true,
			wantSubstrings: []string{"configureSubnetGateway for network", "immutable"},
		},
		{
			name: "router interface removal rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers[0].Interfaces = nil
			},
			wantErr:        true,
			wantSubstrings: []string{"removing interface for network"},
		},
		{
			name: "router removal rejected",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers = nil
			},
			wantErr:        true,
			wantSubstrings: []string{"removing router"},
		},
		{
			name: "router added on update is validated",
			mutate: func(oldObj, obj *infrastructurev1beta2.CloudscaleCluster) {
				seedRouter(oldObj, obj)
				obj.Spec.Routers = append(obj.Spec.Routers, infrastructurev1beta2.RouterSpec{
					Name: "second",
					Interfaces: []infrastructurev1beta2.RouterInterfaceSpec{
						{Network: "main", Address: "192.168.1.1", ConfigureSubnetGateway: new(false)},
					},
				})
			},
			wantErr:        true,
			wantSubstrings: []string{"routers[1].interfaces[0].address", "must be within CIDR"},
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
			name: "PoolMemberNetwork",
			mutate: func(c *infrastructurev1beta2.CloudscaleCluster) {
				c.Spec.ControlPlaneLoadBalancer.PoolMemberNetwork = "aux"
			},
			errPath: "controlPlaneLoadBalancer.poolMemberNetwork",
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

// TestClusterValidateUpdate_PoolMemberNetworkStrictlyImmutable pins the stricter rule
// for poolMemberNetwork: unlike the sibling network field (immutable *once set*), even
// setting it on a cluster that left it empty is rejected. The controller does not move
// live pool members between subnets.
func TestClusterValidateUpdate_PoolMemberNetworkStrictlyImmutable(t *testing.T) {
	g := NewWithT(t)
	obj, oldObj, validator := setupUpdateTestObjects()
	oldObj.Spec.ControlPlaneLoadBalancer.PoolMemberNetwork = ""
	obj.Spec.ControlPlaneLoadBalancer = *oldObj.Spec.ControlPlaneLoadBalancer.DeepCopy()
	obj.Spec.ControlPlaneLoadBalancer.PoolMemberNetwork = "main"

	_, err := validator.ValidateUpdate(ctx, oldObj, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("controlPlaneLoadBalancer.poolMemberNetwork"))
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
// Unit tests for unassignableReason
// ============================================================================

// TestUnassignableReason pins the boundaries of the rule that the defaulter and the
// validator share. They belong here rather than in the defaulting table, where reaching
// the DHCP pool would mean claiming a hundred addresses first.
func TestUnassignableReason(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		addr   string
		want   string
	}{
		{name: "first host address", prefix: "10.0.0.0/24", addr: "10.0.0.1"},
		{name: "last address below the pool", prefix: "10.0.0.0/24", addr: "10.0.0.100"},
		{name: "first address of the pool", prefix: "10.0.0.0/24", addr: "10.0.0.101", want: "DHCP pool"},
		{name: "last address of the pool", prefix: "10.0.0.0/24", addr: "10.0.0.254", want: "DHCP pool"},
		{name: "broadcast address", prefix: "10.0.0.0/24", addr: "10.0.0.255", want: "broadcast address"},
		{name: "network address", prefix: "10.0.0.0/24", addr: "10.0.0.0", want: "network address"},
		// The pool is a last-octet range, so it recurs in a prefix wider than a /24 while
		// the broadcast address belongs to the prefix as a whole.
		{name: "pool recurs inside a /16", prefix: "10.0.0.0/16", addr: "10.0.3.150", want: "DHCP pool"},
		{name: "x.x.n.255 is a host address in a /16", prefix: "10.0.0.0/16", addr: "10.0.3.255"},
		{name: "broadcast of a /16", prefix: "10.0.0.0/16", addr: "10.0.255.255", want: "broadcast address"},
		// IPv6 has none of these concepts, so its last address is assignable.
		{name: "IPv6 last address", prefix: "2001:db8::/126", addr: "2001:db8::3"},
		{name: "IPv6 subnet-router anycast", prefix: "2001:db8::/126", addr: "2001:db8::"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			prefix, err := netip.ParsePrefix(tc.prefix)
			g.Expect(err).ToNot(HaveOccurred())
			addr, err := netip.ParseAddr(tc.addr)
			g.Expect(err).ToNot(HaveOccurred())

			got := unassignableReason(prefix, addr)
			if tc.want == "" {
				g.Expect(got).To(BeEmpty(), "expected %s to be assignable in %s", tc.addr, tc.prefix)
				return
			}
			g.Expect(got).To(ContainSubstring(tc.want))
		})
	}
}

// ============================================================================
// Unit tests for validateAddressInCIDR
// ============================================================================

func TestValidateAddressInCIDR(t *testing.T) {
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
			errs := validateAddressInCIDR(tc.cidr, tc.gateway, path)
			g.Expect(errs).To(HaveLen(tc.wantErrs))
			if tc.wantDetail != "" {
				g.Expect(errs[0].Detail).To(ContainSubstring(tc.wantDetail))
			}
		})
	}
}
