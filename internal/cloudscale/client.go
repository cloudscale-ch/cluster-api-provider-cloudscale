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
	"context"
	"errors"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	"golang.org/x/oauth2"
)

type Client struct {
	Networks                   NetworkService
	Subnets                    SubnetService
	Regions                    RegionService
	Servers                    ServerService
	ServerGroups               ServerGroupService
	LoadBalancers              LoadBalancerService
	LoadBalancerPools          LoadBalancerPoolService
	LoadBalancerPoolMembers    LoadBalancerPoolMemberService
	LoadBalancerListeners      LoadBalancerListenerService
	LoadBalancerHealthMonitors LoadBalancerHealthMonitorService
	FloatingIPs                FloatingIPService
	Flavors                    FlavorService
}

func NewClient(token string) *Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), tokenSource)
	sdkClient := cloudscalesdk.NewClient(httpClient)

	return &Client{
		Networks:                   sdkClient.Networks,
		Subnets:                    sdkClient.Subnets,
		Regions:                    sdkClient.Regions,
		Servers:                    sdkClient.Servers,
		ServerGroups:               sdkClient.ServerGroups,
		LoadBalancers:              sdkClient.LoadBalancers,
		LoadBalancerPools:          sdkClient.LoadBalancerPools,
		LoadBalancerPoolMembers:    sdkClient.LoadBalancerPoolMembers,
		LoadBalancerListeners:      sdkClient.LoadBalancerListeners,
		LoadBalancerHealthMonitors: sdkClient.LoadBalancerHealthMonitors,
		FloatingIPs:                sdkClient.FloatingIPs,
		Flavors:                    sdkClient.Flavors,
	}
}

// IsNotFound checks if an error from the cloudscale.ch API indicates a 404 Not Found response.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	var errResp *cloudscalesdk.ErrorResponse
	if ok := errors.As(err, &errResp); !ok {
		return false
	}
	return errResp.StatusCode == 404
}
