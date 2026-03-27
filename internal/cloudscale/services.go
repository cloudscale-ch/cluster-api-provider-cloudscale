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

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
)

type NetworkService interface {
	Create(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.Network, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error)
	Delete(ctx context.Context, id string) error
}

type SubnetService interface {
	Create(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.Subnet, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error)
	Delete(ctx context.Context, id string) error
}

type RegionService interface {
	List(ctx context.Context) ([]cloudscalesdk.Region, error)
}

type ServerService interface {
	Create(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.Server, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id string, req *cloudscalesdk.ServerUpdateRequest) error
}

type ServerGroupService interface {
	Create(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.ServerGroup, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id string, req *cloudscalesdk.ServerGroupRequest) error
}

type LoadBalancerService interface {
	Create(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerRequest) error
}

type LoadBalancerPoolService interface {
	Create(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerPoolRequest) error
}

type LoadBalancerPoolMemberService interface {
	Create(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error)
	Get(ctx context.Context, poolID, memberID string) (*cloudscalesdk.LoadBalancerPoolMember, error)
	List(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error)
	Delete(ctx context.Context, poolID, memberID string) error
	Update(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error
}

type LoadBalancerListenerService interface {
	Create(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerListenerRequest) error
}

type LoadBalancerHealthMonitorService interface {
	Create(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) error
}

type FloatingIPService interface {
	Create(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error)
	Get(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error)
	List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error)
	Update(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error
	Delete(ctx context.Context, id string) error
}

type FlavorService interface {
	List(ctx context.Context) ([]cloudscalesdk.Flavor, error)
}
