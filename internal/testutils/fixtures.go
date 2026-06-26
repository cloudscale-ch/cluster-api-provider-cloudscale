package testutils

import (
	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// NewControlPlaneMachine builds a control-plane CloudscaleMachine fixture in
// the "default" namespace, labeled as control-plane of "test-cluster" and
// with the given ServerID.
func NewControlPlaneMachine(name, serverID string) *infrastructurev1beta2.CloudscaleMachine {
	m := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			ServerID: serverID,
		},
	}
	return m
}

// NewTestFlavorInfo returns a FlavorInfo populated with a stable set of
// flavors used across webhook and template-controller tests.
func NewTestFlavorInfo() *cloudscale.FlavorInfo {
	return cloudscale.NewFlavorInfo([]cloudscalesdk.Flavor{
		{Slug: "flex-4-2", Name: "Flex-4-2", VCPUCount: 2, MemoryGB: 4},
		{Slug: "flex-8-4", Name: "Flex-8-4", VCPUCount: 4, MemoryGB: 8},
		{Slug: "flex-16-8", VCPUCount: 16, MemoryGB: 8},
		{Slug: "plus-16-8", Name: "Plus-16-8", VCPUCount: 8, MemoryGB: 16},
		{Slug: "plus-32-16", VCPUCount: 32, MemoryGB: 16},
		{Slug: "gpu2-640-80-4-400", Name: "GPU2-640-80-4-400", VCPUCount: 80, MemoryGB: 640, GPU: &cloudscalesdk.FlavorGPU{
			Name:         "RTX PRO 6000 Max-Q",
			Count:        4,
			VRAMPerGPUGB: 96,
		}},
	})
}

// NewTestRegionInfo returns a RegionInfo populated with two regions used by
// webhook tests.
func NewTestRegionInfo() *cloudscale.RegionInfo {
	return cloudscale.NewRegionInfo([]cloudscalesdk.Region{
		{Slug: "rma", Zones: []cloudscalesdk.ZoneStub{{Slug: "rma1"}}},
		{Slug: "lpg", Zones: []cloudscalesdk.ZoneStub{{Slug: "lpg1"}}},
	})
}
