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

package cloudscale

import (
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	. "github.com/onsi/gomega"
)

func newTestRegionInfo() *RegionInfo {
	return NewRegionInfo([]cloudscalesdk.Region{
		{Slug: "rma", Zones: []cloudscalesdk.ZoneStub{{Slug: "rma1"}}},
		{Slug: "lpg", Zones: []cloudscalesdk.ZoneStub{{Slug: "lpg1"}}},
	})
}

func TestRegionInfo_ZoneBelongsToRegion(t *testing.T) {
	ri := newTestRegionInfo()

	tests := []struct {
		name     string
		zone     string
		region   string
		expected bool
	}{
		{"rma1 belongs to rma", "rma1", "rma", true},
		{"lpg1 belongs to lpg", "lpg1", "lpg", true},
		{"rma1 does not belong to lpg", "rma1", "lpg", false},
		{"lpg1 does not belong to rma", "lpg1", "rma", false},
		{"unknown zone returns false", "xyz1", "rma", false},
		{"unknown region returns false", "rma1", "xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := ri.ZoneBelongsToRegion(tt.zone, tt.region)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestRegionInfo_GetDefaultZoneForRegion(t *testing.T) {
	ri := newTestRegionInfo()

	tests := []struct {
		name     string
		region   string
		expected string
	}{
		{"default zone for rma is rma1", "rma", "rma1"},
		{"default zone for lpg is lpg1", "lpg", "lpg1"},
		{"unknown region returns empty string", "xyz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := ri.GetDefaultZoneForRegion(tt.region)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestRegionInfo_IsValidZone(t *testing.T) {
	ri := newTestRegionInfo()

	tests := []struct {
		name     string
		zone     string
		expected bool
	}{
		{"rma1 is valid", "rma1", true},
		{"lpg1 is valid", "lpg1", true},
		{"xyz1 is not valid", "xyz1", false},
		{"empty zone is not valid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := ri.IsValidZone(tt.zone)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestRegionInfo_GetAllRegions(t *testing.T) {
	g := NewWithT(t)
	ri := newTestRegionInfo()

	regions := ri.GetAllRegions()

	g.Expect(regions).To(HaveLen(2))
	g.Expect(regions).To(ContainElement("rma"))
	g.Expect(regions).To(ContainElement("lpg"))
}
