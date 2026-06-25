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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
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

const (
	// ReadTimeout is the context timeout for Get/List API calls.
	ReadTimeout = 10 * time.Second

	// WriteTimeout is the context timeout for Create/Update API calls.
	// Creates can take 60s+ under API load.
	WriteTimeout = 2 * time.Minute

	// DeleteTimeout is the context timeout for Delete API calls.
	DeleteTimeout = 1 * time.Minute
)

// NewTransport creates an http.Transport configured for the cloudscale.ch API.
// The returned transport should be created once and shared across all clients
// to benefit from connection pooling and HTTP/2 multiplexing.
func NewTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout: 5 * time.Second,

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
}

// NewClient creates a cloudscale.ch API client using the given token and
// shared transport. The transport should be created once via NewTransport()
// and reused across clients. Each client gets its own oauth2 token injection
// but shares the underlying connection pool.
//
// version is appended to the SDK's User-Agent header (e.g.
// "cloudscale/v9.0.0 capcs/<version>") so the API server can identify
// the controller making the call.
//
// No global HTTP timeout is set on the client. Instead, callers must use
// context.WithTimeout with ReadTimeout, WriteTimeout, or DeleteTimeout
// for each API call.
func NewClient(token, version string, transport http.RoundTripper) *Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})

	httpClient := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
			Base:   transport,
		},
	}
	sdkClient := cloudscalesdk.NewClient(httpClient)
	sdkClient.UserAgent = sdkClient.UserAgent + " capcs/" + version

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
	return errResp.StatusCode == http.StatusNotFound
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

	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return urlErr.Timeout()
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	return false
}
