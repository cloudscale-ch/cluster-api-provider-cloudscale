package cloudscale

import (
	"fmt"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// ResourceNvidiaGPU is the resource name for NVIDIA GPUs.
	ResourceNvidiaGPU corev1.ResourceName = "nvidia.com/gpu"
)

// FlavorInfo holds cloudscale.ch flavor information for capacity calculation.
type FlavorInfo struct {
	flavors map[string]cloudscalesdk.Flavor // slug -> Flavor
}

// NewFlavorInfo creates a FlavorInfo from a list of flavors.
func NewFlavorInfo(flavors []cloudscalesdk.Flavor) *FlavorInfo {
	fi := &FlavorInfo{
		flavors: make(map[string]cloudscalesdk.Flavor),
	}

	for _, f := range flavors {
		fi.flavors[f.Slug] = f
	}

	return fi
}

// IsValidFlavor returns true if the flavor slug is valid.
func (fi *FlavorInfo) IsValidFlavor(slug string) bool {
	_, ok := fi.flavors[slug]
	return ok
}

// GetFlavor returns the flavor for a given slug, or nil if not found.
func (fi *FlavorInfo) GetFlavor(slug string) *cloudscalesdk.Flavor {
	f, ok := fi.flavors[slug]
	if !ok {
		return nil
	}
	return &f
}

// GetCapacity returns the resource capacity for a flavor slug.
// rootVolumeSizeGB is used to calculate ephemeral-storage.
func (fi *FlavorInfo) GetCapacity(slug string) (corev1.ResourceList, error) {
	flavor, ok := fi.flavors[slug]
	if !ok {
		return nil, fmt.Errorf("unknown flavor: %s", slug)
	}

	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", flavor.VCPUCount)),
		corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dGi", flavor.MemoryGB)),
	}

	if flavor.GPU != nil && flavor.GPU.Count > 0 {
		capacity[ResourceNvidiaGPU] = resource.MustParse(fmt.Sprintf("%d", flavor.GPU.Count))
	}

	return capacity, nil
}

// GetAllFlavors returns all known flavor slugs.
func (fi *FlavorInfo) GetAllFlavors() []string {
	slugs := make([]string, 0, len(fi.flavors))
	for slug := range fi.flavors {
		slugs = append(slugs, slug)
	}
	return slugs
}
