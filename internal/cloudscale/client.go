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
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

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

// DefaultCloudscaleRequestTimeout is the default HTTP request timeout for all Create calls.
const DefaultCloudscaleRequestTimeout = 60 * time.Second

func NewClient(token string, requestTimeout time.Duration) *Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,

		// needs to be set because we also set DialContext
		ForceAttemptHTTP2: true,
		HTTP2: &http.HTTP2Config{
			SendPingTimeout: 5 * time.Second,
			PingTimeout:     3 * time.Second,
		},

		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     0,
	}

	httpClient := &http.Client{
		Timeout: requestTimeout,
		Transport: &oauth2.Transport{
			Source: tokenSource,
			Base:   baseTransport,
		},
	}
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

// IsFloatingIPNoPublicInterface returns true if the error indicates the target
// server does not have a public interface with an IPv4 address.
func IsFloatingIPNoPublicInterface(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not have a public interface with an IPv4 address")
}

// IsTimeoutError reports whether err indicates the HTTP request timed out
// before receiving a response.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Timeout()
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	return false
}

// IsDeadlineExceeded reports whether err indicates a deadline was exceeded.
func IsDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrDeadlineExceeded)
}
