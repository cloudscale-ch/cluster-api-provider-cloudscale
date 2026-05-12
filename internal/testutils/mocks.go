package testutils

import (
	"context"
	"errors"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
)

// --- Network Service Mock ---

type MockNetworkService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.Network, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error)
	DeleteFn func(ctx context.Context, id string) error
}

func (m *MockNetworkService) Create(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockNetworkService) Get(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockNetworkService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockNetworkService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

// --- Subnet Service Mock ---

type MockSubnetService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error)
	DeleteFn func(ctx context.Context, id string) error
}

func (m *MockSubnetService) Create(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockSubnetService) Get(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockSubnetService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockSubnetService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

// --- FloatingIP Service Mock ---

type MockFloatingIPService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error)
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error
	DeleteFn func(ctx context.Context, id string) error
}

func (m *MockFloatingIPService) Create(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockFloatingIPService) Get(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockFloatingIPService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockFloatingIPService) Update(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

func (m *MockFloatingIPService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

// --- Server Service Mock ---

type MockServerService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.Server, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error)
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.ServerUpdateRequest) error
	DeleteFn func(ctx context.Context, id string) error
}

func (m *MockServerService) Create(ctx context.Context, req *cloudscalesdk.ServerRequest) (*cloudscalesdk.Server, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockServerService) Get(ctx context.Context, id string) (*cloudscalesdk.Server, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockServerService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Server, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockServerService) Update(ctx context.Context, id string, req *cloudscalesdk.ServerUpdateRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

func (m *MockServerService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

// --- ServerGroup Service Mock ---

type MockServerGroupService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.ServerGroup, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error)
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.ServerGroupRequest) error
	DeleteFn func(ctx context.Context, id string) error
}

func (m *MockServerGroupService) Create(ctx context.Context, req *cloudscalesdk.ServerGroupRequest) (*cloudscalesdk.ServerGroup, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockServerGroupService) Get(ctx context.Context, id string) (*cloudscalesdk.ServerGroup, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockServerGroupService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.ServerGroup, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockServerGroupService) Update(ctx context.Context, id string, req *cloudscalesdk.ServerGroupRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

func (m *MockServerGroupService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

// --- Load Balancer Service Mock ---

type MockLoadBalancerService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error)
	DeleteFn func(ctx context.Context, id string) error
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerRequest) error
}

func (m *MockLoadBalancerService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockLoadBalancerService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockLoadBalancerService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockLoadBalancerService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

func (m *MockLoadBalancerService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

// --- Load Balancer Pool Service Mock ---

type MockLoadBalancerPoolService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error)
	DeleteFn func(ctx context.Context, id string) error
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerPoolRequest) error
}

func (m *MockLoadBalancerPoolService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockLoadBalancerPoolService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerPool, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockLoadBalancerPoolService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockLoadBalancerPoolService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

func (m *MockLoadBalancerPoolService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerPoolRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

// --- Load Balancer Listener Service Mock ---

type MockLoadBalancerListenerService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error)
	DeleteFn func(ctx context.Context, id string) error
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerListenerRequest) error
}

func (m *MockLoadBalancerListenerService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockLoadBalancerListenerService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerListener, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockLoadBalancerListenerService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockLoadBalancerListenerService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

func (m *MockLoadBalancerListenerService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerListenerRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

// --- Load Balancer Health Monitor Service Mock ---

type MockLoadBalancerHealthMonitorService struct {
	CreateFn func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error)
	GetFn    func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error)
	ListFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error)
	DeleteFn func(ctx context.Context, id string) error
	UpdateFn func(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) error
}

func (m *MockLoadBalancerHealthMonitorService) Create(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, req)
	}
	return nil, errors.New("mock(Create) not configured")
}

func (m *MockLoadBalancerHealthMonitorService) Get(ctx context.Context, id string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, errors.New("mock(Get) not configured")
}

func (m *MockLoadBalancerHealthMonitorService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, modifiers...)
	}
	return nil, errors.New("mock(List) not configured")
}

func (m *MockLoadBalancerHealthMonitorService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return errors.New("mock(Delete) not configured")
}

func (m *MockLoadBalancerHealthMonitorService) Update(ctx context.Context, id string, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, req)
	}
	return errors.New("mock(Update) not configured")
}

// --- Load Balancer Pool Member Service Mock ---

type MockLoadBalancerPoolMemberService struct {
	CreateFn func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error)
	GetFn    func(ctx context.Context, poolID, memberID string) (*cloudscalesdk.LoadBalancerPoolMember, error)
	ListFn   func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error)
	DeleteFn func(ctx context.Context, poolID, memberID string) error
	UpdateFn func(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error
}

func (m *MockLoadBalancerPoolMemberService) Create(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, poolID, req)
	}
	return nil, errors.New("mock not configured")
}

func (m *MockLoadBalancerPoolMemberService) Get(ctx context.Context, poolID, memberID string) (*cloudscalesdk.LoadBalancerPoolMember, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, poolID, memberID)
	}
	return nil, errors.New("mock not configured")
}

func (m *MockLoadBalancerPoolMemberService) List(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, poolID, modifiers...)
	}
	return nil, errors.New("mock not configured")
}

func (m *MockLoadBalancerPoolMemberService) Delete(ctx context.Context, poolID, memberID string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, poolID, memberID)
	}
	return nil
}

func (m *MockLoadBalancerPoolMemberService) Update(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, poolID, memberID, req)
	}
	return errors.New("mock(Update) not configured")
}
