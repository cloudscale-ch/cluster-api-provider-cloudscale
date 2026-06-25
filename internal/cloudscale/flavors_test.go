package cloudscale

import (
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestFlavorInfo(t *testing.T) {
	flavors := []cloudscalesdk.Flavor{
		{
			Slug:      "small",
			VCPUCount: 2,
			MemoryGB:  4,
		},
		{
			Slug:      "gpu-large",
			VCPUCount: 8,
			MemoryGB:  32,
			GPU: &cloudscalesdk.FlavorGPU{
				Count: 1,
			},
		},
	}

	fi := NewFlavorInfo(flavors)

	t.Run("IsValidFlavor", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(fi.IsValidFlavor("small")).To(BeTrue())
		g.Expect(fi.IsValidFlavor("gpu-large")).To(BeTrue())
		g.Expect(fi.IsValidFlavor("non-existent")).To(BeFalse())
	})

	t.Run("GetFlavor", func(t *testing.T) {
		g := NewWithT(t)
		f := fi.GetFlavor("small")
		g.Expect(f).NotTo(BeNil())
		g.Expect(f.Slug).To(Equal("small"))

		f = fi.GetFlavor("non-existent")
		g.Expect(f).To(BeNil())
	})

	t.Run("GetCapacity", func(t *testing.T) {
		g := NewWithT(t)
		// Test standard flavor
		capSmall, err := fi.GetCapacity("small")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(capSmall[corev1.ResourceCPU]).To(Equal(resource.MustParse("2")))
		g.Expect(capSmall[corev1.ResourceMemory]).To(Equal(resource.MustParse("4Gi")))
		g.Expect(capSmall[ResourceNvidiaGPU]).To(BeZero())

		// Test GPU flavor
		capGPU, err := fi.GetCapacity("gpu-large")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(capGPU[corev1.ResourceCPU]).To(Equal(resource.MustParse("8")))
		g.Expect(capGPU[corev1.ResourceMemory]).To(Equal(resource.MustParse("32Gi")))
		g.Expect(capGPU[ResourceNvidiaGPU]).To(Equal(resource.MustParse("1")))

		// Test unknown flavor
		capUnknown, err := fi.GetCapacity("unknown")
		g.Expect(err).To(HaveOccurred())
		g.Expect(capUnknown).To(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("unknown flavor: unknown"))
	})

	t.Run("GetAllFlavors", func(t *testing.T) {
		g := NewWithT(t)
		slugs := fi.GetAllFlavors()
		g.Expect(slugs).To(HaveLen(2))
		g.Expect(slugs).To(ContainElement("small"))
		g.Expect(slugs).To(ContainElement("gpu-large"))
	})
}
