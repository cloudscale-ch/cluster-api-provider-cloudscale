//go:build e2e

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

package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// newCloudscaleClient creates a new cloudscale API client from the given token.
func newCloudscaleClient(token string) *cloudscale.Client {
	return cloudscale.NewClient(token, "e2e", cloudscale.NewTransport())
}

// resourceSnapshot holds a snapshot of cloudscale API resources for leak detection.
type resourceSnapshot struct {
	ServerUUIDs       map[string]bool
	NetworkUUIDs      map[string]bool
	LoadBalancerUUIDs map[string]bool
	ServerGroupUUIDs  map[string]bool
	FloatingIPHREFs   map[string]bool
}

// takeResourceSnapshot lists all relevant cloudscale resources and records their UUIDs.
func takeResourceSnapshot(ctx context.Context, client *cloudscale.Client) (*resourceSnapshot, error) {
	snap := &resourceSnapshot{
		ServerUUIDs:       make(map[string]bool),
		NetworkUUIDs:      make(map[string]bool),
		LoadBalancerUUIDs: make(map[string]bool),
		ServerGroupUUIDs:  make(map[string]bool),
		FloatingIPHREFs:   make(map[string]bool),
	}

	servers, err := client.Servers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing servers: %w", err)
	}
	for _, s := range servers {
		snap.ServerUUIDs[s.UUID] = true
	}

	// TODO: volumes list

	floatingIPs, err := client.FloatingIPs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing floating IPs: %w", err)
	}
	for _, fip := range floatingIPs {
		snap.FloatingIPHREFs[fip.HREF] = true
	}

	networks, err := client.Networks.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	for _, n := range networks {
		snap.NetworkUUIDs[n.UUID] = true
	}

	loadBalancers, err := client.LoadBalancers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing load balancers: %w", err)
	}
	for _, lb := range loadBalancers {
		snap.LoadBalancerUUIDs[lb.UUID] = true
	}

	serverGroups, err := client.ServerGroups.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing server groups: %w", err)
	}
	for _, sg := range serverGroups {
		snap.ServerGroupUUIDs[sg.UUID] = true
	}

	return snap, nil
}

// checkForLeakedResources compares the current state of cloudscale resources against
// a previous snapshot and returns an error if any new resources were found (leaked).
func checkForLeakedResources(ctx context.Context, client *cloudscale.Client, before *resourceSnapshot) error {
	after, err := takeResourceSnapshot(ctx, client)
	if err != nil {
		return fmt.Errorf("taking post-test snapshot: %w", err)
	}

	var leaks []string

	for uuid := range after.ServerUUIDs {
		if !before.ServerUUIDs[uuid] {
			leaks = append(leaks, fmt.Sprintf("leaked server: %s", uuid))
		}
	}
	for uuid := range after.NetworkUUIDs {
		if !before.NetworkUUIDs[uuid] {
			leaks = append(leaks, fmt.Sprintf("leaked network: %s", uuid))
		}
	}
	for uuid := range after.LoadBalancerUUIDs {
		if !before.LoadBalancerUUIDs[uuid] {
			leaks = append(leaks, fmt.Sprintf("leaked load balancer: %s", uuid))
		}
	}
	for uuid := range after.ServerGroupUUIDs {
		if !before.ServerGroupUUIDs[uuid] {
			leaks = append(leaks, fmt.Sprintf("leaked server group: %s", uuid))
		}
	}
	for href := range after.FloatingIPHREFs {
		if !before.FloatingIPHREFs[href] {
			leaks = append(leaks, fmt.Sprintf("leaked floating IP: %s", href))
		}
	}

	if len(leaks) > 0 {
		return fmt.Errorf("infrastructure resource leaks detected:\n%s", strings.Join(leaks, "\n"))
	}
	return nil
}
