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
	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
)

// RegionInfo holds cloudscale.ch region and zone information for validation.
type RegionInfo struct {
	regions map[string][]string // region -> zones
	zones   map[string]string   // zone -> region
}

// NewRegionInfo creates a RegionInfo from the SDK region list.
func NewRegionInfo(regions []cloudscalesdk.Region) *RegionInfo {
	ri := &RegionInfo{
		regions: make(map[string][]string),
		zones:   make(map[string]string),
	}

	for _, r := range regions {
		zones := make([]string, len(r.Zones))
		for i, z := range r.Zones {
			zones[i] = z.Slug
			ri.zones[z.Slug] = r.Slug
		}
		ri.regions[r.Slug] = zones
	}

	return ri
}

// ZoneBelongsToRegion returns true if the zone belongs to the specified region.
func (ri *RegionInfo) ZoneBelongsToRegion(zone, region string) bool {
	zoneRegion, ok := ri.zones[zone]
	if !ok {
		return false
	}
	return zoneRegion == region
}

// GetDefaultZoneForRegion returns the first (default) zone for a region.
// Returns empty string if region is not found or has no zones.
func (ri *RegionInfo) GetDefaultZoneForRegion(region string) string {
	zones := ri.regions[region]
	if len(zones) == 0 {
		return ""
	}
	return zones[0]
}

// IsValidZone returns true if the zone slug is valid.
func (ri *RegionInfo) IsValidZone(zone string) bool {
	_, ok := ri.zones[zone]
	return ok
}

// GetAllRegions returns all known region slugs.
func (ri *RegionInfo) GetAllRegions() []string {
	regions := make([]string, 0, len(ri.regions))
	for slug := range ri.regions {
		regions = append(regions, slug)
	}
	return regions
}
