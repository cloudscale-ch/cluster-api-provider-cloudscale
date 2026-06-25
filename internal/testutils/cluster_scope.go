package testutils

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// ClusterScopeOption configures a ClusterScope for testing.
type ClusterScopeOption func(*scope.ClusterScope)

// NewClusterScopeOpts builds a ClusterScope with opinionated defaults and optional overrides.
func NewClusterScopeOpts(opts ...ClusterScopeOption) *scope.ClusterScope {
	clusterScope := &scope.ClusterScope{
		Logger: logr.Discard(),
		Cluster: &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		},
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				Region: "rma",
				Zone:   "rma1",
				Networks: []infrastructurev1beta2.NetworkSpec{
					{Name: "test", CIDR: "10.0.0.0/24"},
				},
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled:       new(false),
					APIServerPort: 6443,
					HealthMonitor: infrastructurev1beta2.HealthMonitorSpec{
						DelayS:        5,
						TimeoutS:      3,
						UpThreshold:   2,
						DownThreshold: 3,
					},
				},
			},
		},
		CloudscaleClient: &cloudscale.Client{},
	}
	for _, opt := range opts {
		opt(clusterScope)
	}
	return clusterScope
}

// WithLBService wires a custom LoadBalancer service into the client.
func WithLBService(svc cloudscale.LoadBalancerService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.LoadBalancers = svc }
}

// WithPoolService wires a custom LoadBalancerPool service into the client.
func WithPoolService(svc cloudscale.LoadBalancerPoolService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.LoadBalancerPools = svc }
}

// WithListenerService wires a custom LoadBalancerListener service into the client.
func WithListenerService(svc cloudscale.LoadBalancerListenerService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.LoadBalancerListeners = svc }
}

// WithHMService wires a custom LoadBalancerHealthMonitor service into the client.
func WithHMService(svc cloudscale.LoadBalancerHealthMonitorService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.LoadBalancerHealthMonitors = svc }
}

// WithMemberService wires a custom LoadBalancerPoolMember service into the client.
func WithMemberService(svc cloudscale.LoadBalancerPoolMemberService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.LoadBalancerPoolMembers = svc }
}

// WithNetworkService wires a custom Network service into the client.
func WithNetworkService(svc cloudscale.NetworkService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.Networks = svc }
}

// WithSubnetService wires a custom Subnet service into the client.
func WithSubnetService(svc cloudscale.SubnetService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.Subnets = svc }
}

// WithServerGroupService wires a custom ServerGroup service into the client.
func WithServerGroupService(svc cloudscale.ServerGroupService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.ServerGroups = svc }
}

// WithFloatingIPService wires a custom FloatingIP service into the client.
func WithFloatingIPService(svc cloudscale.FloatingIPService) ClusterScopeOption {
	return func(cs *scope.ClusterScope) { cs.CloudscaleClient.FloatingIPs = svc }
}

// WithLBEnabled sets whether the load balancer is enabled in the spec.
func WithLBEnabled(enabled bool) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(enabled)
	}
}

// WithFlavor sets the LB flavor.
func WithFlavor(flavor string) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Flavor = flavor
	}
}

// WithAlgorithm sets the LB algorithm.
func WithAlgorithm(alg string) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Algorithm = alg
	}
}

// WithAPIServerPort sets the API server port.
func WithAPIServerPort(port int32) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.APIServerPort = port
	}
}

// WithHealthMonitorParams sets all health-monitor tunables at once.
func WithHealthMonitorParams(delayS, timeoutS, upThreshold, downThreshold int) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.HealthMonitor = infrastructurev1beta2.HealthMonitorSpec{
			DelayS:        delayS,
			TimeoutS:      timeoutS,
			UpThreshold:   upThreshold,
			DownThreshold: downThreshold,
		}
	}
}

// WithPreExistingNetwork adds a NetworkStatus entry so the scope thinks the network already exists.
func WithPreExistingNetwork(name, netID, subnetID, cidr string) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Status.Networks = append(cs.CloudscaleCluster.Status.Networks,
			infrastructurev1beta2.NetworkStatus{
				Name:      name,
				NetworkID: netID,
				SubnetID:  subnetID,
				CIDR:      cidr,
				Managed:   true,
			},
		)
	}
}

// WithGeneration sets the generation (used for status conditions).
func WithGeneration(gen int64) ClusterScopeOption {
	return func(cs *scope.ClusterScope) {
		cs.CloudscaleCluster.Generation = gen
	}
}

// MachineScopeOption configures a MachineScope for testing.
type MachineScopeOption func(*scope.MachineScope)

// NewMachineScope builds a MachineScope with opinionated defaults and optional overrides.
func NewMachineScope(serverService cloudscale.ServerService, opts ...MachineScopeOption) *scope.MachineScope {
	cloudscaleMachine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
		},
		Spec: infrastructurev1beta2.CloudscaleMachineSpec{
			Flavor: "flex-8-4",
			Image:  "ubuntu-24.04",
		},
	}

	bootstrapSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bootstrap-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"value": []byte("#!/bin/bash\necho 'bootstrap script'"),
		},
	}

	cloudscaleCluster := &infrastructurev1beta2.CloudscaleCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: infrastructurev1beta2.CloudscaleClusterSpec{
			Region: "rma",
			Zone:   "rma1",
		},
		Status: infrastructurev1beta2.CloudscaleClusterStatus{
			Networks: []infrastructurev1beta2.NetworkStatus{
				{Name: "test", NetworkID: "net-uuid-123", SubnetID: "subnet-uuid-123", Managed: true},
			},
		},
	}

	fakeClient := NewFakeClient(cloudscaleMachine, bootstrapSecret)

	machineScope, _ := scope.NewMachineScope(scope.MachineScopeParams{
		Client: fakeClient,
		Logger: logr.Discard(),
		Cluster: &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		},
		Machine: &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: "default",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: new("bootstrap-secret"),
				},
			},
		},
		CloudscaleCluster: cloudscaleCluster,
		CloudscaleMachine: cloudscaleMachine,
		CloudscaleClient: &cloudscale.Client{
			Servers: serverService,
		},
	})

	for _, opt := range opts {
		opt(machineScope)
	}
	return machineScope
}

// WithMachineServerGroupService wires a ServerGroup service into the machine's cloudscale client.
func WithMachineServerGroupService(svc cloudscale.ServerGroupService) MachineScopeOption {
	return func(ms *scope.MachineScope) {
		ms.CloudscaleClient.ServerGroups = svc
	}
}

// WithServerGroup sets the ServerGroup field on the machine spec.
func WithServerGroup(sg *infrastructurev1beta2.ServerGroupSpec) MachineScopeOption {
	return func(ms *scope.MachineScope) {
		ms.CloudscaleMachine.Spec.ServerGroup = sg
	}
}
