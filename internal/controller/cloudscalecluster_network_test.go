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

package controller

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v9"
	. "github.com/onsi/gomega"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const netUUID = "net-uuid-123"

// --- Orchestrator tests ---

func TestReconcileNetwork_CreatesResources(t *testing.T) {
	tests := []struct {
		name string
		spec func() (networkService *testutils.MockNetworkService, subnetService *testutils.MockSubnetService, clusterScope *scope.ClusterScope)
		want func(g *WithT, capturedNetReq *cloudscalesdk.NetworkCreateRequest, capturedSubReq *cloudscalesdk.SubnetCreateRequest, clusterScope *scope.ClusterScope)
	}{
		{
			name: "creates both resources",
			spec: func() (*testutils.MockNetworkService, *testutils.MockSubnetService, *scope.ClusterScope) {
				networkService := &testutils.MockNetworkService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
						return &cloudscalesdk.Network{UUID: netUUID, Name: req.Name}, nil
					},
				}
				subnetService := &testutils.MockSubnetService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
						return &cloudscalesdk.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
					},
				}

				clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
				return networkService, subnetService, clusterScope
			},
			want: func(g *WithT, capturedNetReq *cloudscalesdk.NetworkCreateRequest, capturedSubReq *cloudscalesdk.SubnetCreateRequest, clusterScope *scope.ClusterScope) {
				ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
				g.Expect(ns).ToNot(BeNil())
				g.Expect(ns.NetworkID).To(Equal(netUUID))
				g.Expect(ns.SubnetID).To(Equal("subnet-uuid-123"))
				g.Expect(capturedNetReq.Name).To(Equal("test"))
				g.Expect(capturedNetReq.Zone).To(Equal("rma1"))
				g.Expect(capturedNetReq.AutoCreateIPV4Subnet).ToNot(BeNil())
				g.Expect(*capturedNetReq.AutoCreateIPV4Subnet).To(BeFalse())
				g.Expect(capturedSubReq.Network).To(Equal(netUUID))
				g.Expect(capturedSubReq.CIDR).To(Equal("10.0.0.0/24"))
				g.Expect(capturedSubReq.GatewayAddress).To(Equal(""))
			},
		},
		{
			name: "custom CIDR",
			spec: func() (*testutils.MockNetworkService, *testutils.MockSubnetService, *scope.ClusterScope) {
				networkService := &testutils.MockNetworkService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
						return &cloudscalesdk.Network{UUID: netUUID, Name: req.Name}, nil
					},
				}
				subnetService := &testutils.MockSubnetService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
						return &cloudscalesdk.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
					},
				}

				clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
				clusterScope.CloudscaleCluster.Spec.Networks[0].CIDR = "192.168.0.0/16"
				return networkService, subnetService, clusterScope
			},
			want: func(g *WithT, capturedNetReq *cloudscalesdk.NetworkCreateRequest, capturedSubReq *cloudscalesdk.SubnetCreateRequest, clusterScope *scope.ClusterScope) {
				g.Expect(capturedSubReq.CIDR).To(Equal("192.168.0.0/16"))
			},
		},
		{
			name: "explicit gateway",
			spec: func() (*testutils.MockNetworkService, *testutils.MockSubnetService, *scope.ClusterScope) {
				networkService := &testutils.MockNetworkService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
						return &cloudscalesdk.Network{UUID: netUUID, Name: req.Name}, nil
					},
				}
				subnetService := &testutils.MockSubnetService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
						return &cloudscalesdk.Subnet{UUID: "subnet-uuid-123", CIDR: req.CIDR}, nil
					},
				}

				clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
				clusterScope.CloudscaleCluster.Spec.Networks[0].GatewayAddress = "10.0.0.254"
				return networkService, subnetService, clusterScope
			},
			want: func(g *WithT, capturedNetReq *cloudscalesdk.NetworkCreateRequest, capturedSubReq *cloudscalesdk.SubnetCreateRequest, clusterScope *scope.ClusterScope) {
				g.Expect(capturedSubReq.GatewayAddress).To(Equal("10.0.0.254"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var capturedNetReq *cloudscalesdk.NetworkCreateRequest
			var capturedSubReq *cloudscalesdk.SubnetCreateRequest

			networkService, subnetService, clusterScope := tc.spec()

			// Override to capture requests if they haven't been captured yet (needed for table-driven closure capture)
			if networkService.CreateFn != nil {
				origCreateFn := networkService.CreateFn
				networkService.CreateFn = func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
					capturedNetReq = req
					return origCreateFn(ctx, req)
				}
			}
			if subnetService.CreateFn != nil {
				origCreateFn := subnetService.CreateFn
				subnetService.CreateFn = func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
					capturedSubReq = req
					return origCreateFn(ctx, req)
				}
			}

			r := newTestReconciler()

			_, err := r.reconcileNetwork(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			tc.want(g, capturedNetReq, capturedSubReq, clusterScope)
		})
	}
}

func TestReconcileNetwork_SkipsIfBothExist(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
			return &cloudscalesdk.Network{UUID: id}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
			g.Fail("Network create should not be called")
			return nil, nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
			return &cloudscalesdk.Subnet{UUID: id}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
			g.Fail("Create should not be called when subnet is found by tag")
			return nil, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "existing-net", SubnetID: "existing-subnet", Managed: true},
	}

	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("existing-net"))
	g.Expect(ns.SubnetID).To(Equal("existing-subnet"))
}

func TestReconcileNetwork_NetworkErrorStopsSubnet(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
			return nil, fmt.Errorf("api error")
		},
	}
	subnetService := &testutils.MockSubnetService{
		CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
			g.Fail("Subnet create should not be called when network fails")
			return nil, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("api error"))
	g.Expect(clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")).To(BeNil())
}

func TestReconcileNetwork_SubnetErrorSurfaced(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
			return &cloudscalesdk.Network{UUID: "net-uuid"}, nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
			return nil, fmt.Errorf("subnet api error")
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("subnet api error"))
}

// --- Managed network sub-resource tests (via reconcileNetwork orchestrator) ---

func TestReconcileNetwork_FindsByTag(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
			return []cloudscalesdk.Network{
				{UUID: "found-net-uuid", Name: "test-cluster"},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
			g.Fail("Create should not be called when network is found by tag")
			return nil, nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
			return []cloudscalesdk.Subnet{
				{UUID: "found-subnet-uuid", CIDR: "10.0.0.0/24"},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
			t.Fatal("Create should not be called when subnet is found by tag")
			return nil, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("found-net-uuid"))
	g.Expect(ns.SubnetID).To(Equal("found-subnet-uuid"))
}

func TestReconcileNetwork_MultipleResourcesError(t *testing.T) {
	tests := []struct {
		name         string
		networkSetup func() *testutils.MockNetworkService
		subnetSetup  func() *testutils.MockSubnetService
		statusSetup  func(clusterScope *scope.ClusterScope)
		wantErr      string
	}{
		{
			name: "errors on multiple networks",
			networkSetup: func() *testutils.MockNetworkService {
				return &testutils.MockNetworkService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return []cloudscalesdk.Network{
							{UUID: "net-uuid-1"},
							{UUID: "net-uuid-2"},
						}, nil
					},
				}
			},
			subnetSetup: func() *testutils.MockSubnetService { return &testutils.MockSubnetService{} },
			statusSetup: func(clusterScope *scope.ClusterScope) {},
			wantErr:     "found 2 network/tests matching tag filter",
		},
		{
			name: "subnet errors on multiple",
			networkSetup: func() *testutils.MockNetworkService {
				return &testutils.MockNetworkService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
						return &cloudscalesdk.Network{UUID: id}, nil
					},
				}
			},
			subnetSetup: func() *testutils.MockSubnetService {
				return &testutils.MockSubnetService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return []cloudscalesdk.Subnet{
							{UUID: "subnet-uuid-1"},
							{UUID: "subnet-uuid-2"},
						}, nil
					},
				}
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "test", NetworkID: netUUID, Managed: true},
				}
			},
			wantErr: "found 2 subnet/tests matching tag filter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			networkService := tc.networkSetup()
			subnetService := tc.subnetSetup()
			clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
			tc.statusSetup(clusterScope)
			r := newTestReconciler()

			_, err := r.reconcileNetwork(context.Background(), clusterScope)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

func TestReconcileNetwork_RecreatesIfDeleted(t *testing.T) {
	tests := []struct {
		name              string
		networkSetup      func() (*testutils.MockNetworkService, *bool)
		subnetSetup       func() (*testutils.MockSubnetService, *bool)
		statusSetup       func(clusterScope *scope.ClusterScope)
		wantNetID         string
		wantSubnetID      string
		wantNetCreated    bool
		wantSubnetCreated bool
	}{
		{
			name: "network recreates if deleted externally",
			networkSetup: func() (*testutils.MockNetworkService, *bool) {
				var created bool
				return &testutils.MockNetworkService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
						return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
					},
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
						created = true
						return &cloudscalesdk.Network{UUID: "new-net-uuid", Name: req.Name}, nil
					},
				}, &created
			},
			subnetSetup: func() (*testutils.MockSubnetService, *bool) {
				return &testutils.MockSubnetService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
						return &cloudscalesdk.Subnet{UUID: "new-subnet-uuid", CIDR: req.CIDR}, nil
					},
				}, nil
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "test", NetworkID: "deleted-net-uuid", Managed: true},
				}
			},
			wantNetID:      "new-net-uuid",
			wantSubnetID:   "new-subnet-uuid",
			wantNetCreated: true,
		},
		{
			name: "subnet recreates if deleted externally",
			networkSetup: func() (*testutils.MockNetworkService, *bool) {
				return &testutils.MockNetworkService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
						return &cloudscalesdk.Network{UUID: id}, nil
					},
				}, nil
			},
			subnetSetup: func() (*testutils.MockSubnetService, *bool) {
				var created bool
				return &testutils.MockSubnetService{
					GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Subnet, error) {
						return nil, &cloudscalesdk.ErrorResponse{StatusCode: 404}
					},
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
						created = true
						return &cloudscalesdk.Subnet{UUID: "new-subnet-uuid", CIDR: req.CIDR}, nil
					},
				}, &created
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "test", NetworkID: netUUID, SubnetID: "deleted-subnet-uuid", Managed: true},
				}
			},
			wantNetID:         netUUID,
			wantSubnetID:      "new-subnet-uuid",
			wantSubnetCreated: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			networkService, netCreated := tc.networkSetup()
			subnetService, subnetCreated := tc.subnetSetup()
			clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
			tc.statusSetup(clusterScope)
			r := newTestReconciler()

			_, err := r.reconcileNetwork(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.wantNetCreated {
				g.Expect(*netCreated).To(BeTrue(), "Should create a new network when old one was deleted")
			}
			if tc.wantSubnetCreated {
				g.Expect(*subnetCreated).To(BeTrue(), "Should create a new subnet when old one was deleted")
			}
			ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
			g.Expect(ns).ToNot(BeNil())
			g.Expect(ns.NetworkID).To(Equal(tc.wantNetID))
			if tc.wantSubnetID != "" {
				g.Expect(ns.SubnetID).To(Equal(tc.wantSubnetID))
			}
		})
	}
}

// --- Subnet sub-resource tests (via reconcileNetwork orchestrator) ---

func TestReconcileNetwork_SubnetFindsByTag(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
			return &cloudscalesdk.Network{UUID: netUUID, Name: req.Name}, nil
		},
	}
	subnetService := &testutils.MockSubnetService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
			return []cloudscalesdk.Subnet{
				{UUID: "found-subnet-uuid", CIDR: "10.0.0.0/24"},
			}, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
			t.Fatal("Create should not be called when subnet is found by tag")
			return nil, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
	// Pre-populate network status so the network part is resolved via Get
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: netUUID, Managed: true},
	}
	// The networkService Get should return the existing network
	networkService.GetFn = func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
		return &cloudscalesdk.Network{UUID: id}, nil
	}

	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("test")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.SubnetID).To(Equal("found-subnet-uuid"))
}

// --- Delete tests ---

func TestDeleteNetwork_BasicScenarios(t *testing.T) {
	tests := []struct {
		name          string
		setup         func() (*testutils.MockNetworkService, *string)
		statusSetup   func(clusterScope *scope.ClusterScope)
		wantDeleted   string
		wantStatusLen int
	}{
		{
			name: "deletes network and clears status",
			setup: func() (*testutils.MockNetworkService, *string) {
				var deletedID string
				return &testutils.MockNetworkService{
					DeleteFn: func(ctx context.Context, id string) error {
						deletedID = id
						return nil
					},
				}, &deletedID
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "test", NetworkID: "net-to-delete", SubnetID: "subnet-to-cascade", Managed: true},
				}
			},
			wantDeleted:   "net-to-delete",
			wantStatusLen: 0,
		},
		{
			name: "skips if no network",
			setup: func() (*testutils.MockNetworkService, *string) {
				return &testutils.MockNetworkService{
					DeleteFn: func(ctx context.Context, id string) error {
						t.Fatal("Delete should not be called when no network exists")
						return nil
					},
				}, nil
			},
			statusSetup:   func(clusterScope *scope.ClusterScope) {},
			wantDeleted:   "",
			wantStatusLen: 0,
		},
		{
			name: "ignores already deleted",
			setup: func() (*testutils.MockNetworkService, *string) {
				return &testutils.MockNetworkService{
					DeleteFn: func(ctx context.Context, id string) error {
						return &cloudscalesdk.ErrorResponse{StatusCode: 404}
					},
				}, nil
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "test", NetworkID: "already-deleted-net", SubnetID: "already-deleted-subnet", Managed: true},
				}
			},
			wantDeleted:   "",
			wantStatusLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			networkService, deletedID := tc.setup()
			clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
			tc.statusSetup(clusterScope)
			r := newTestReconciler()

			err := r.deleteNetwork(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			if tc.wantDeleted != "" {
				g.Expect(*deletedID).To(Equal(tc.wantDeleted))
			}
			if tc.wantStatusLen == 0 {
				g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(BeNil())
			} else {
				g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(HaveLen(tc.wantStatusLen))
			}
		})
	}
}

func TestDeleteNetwork_SkipsPreExistingNetwork(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		DeleteFn: func(ctx context.Context, id string) error {
			g.Fail("Delete should not be called for Pre-existing networks")
			return nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "pre-existing", NetworkID: "pre-existing-net", SubnetID: "pre-existing-subnet", Managed: false},
	}

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	// Pre-existing networks are preserved in status even during deletion
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(HaveLen(1))
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks[0].Name).To(Equal("pre-existing"))
	g.Expect(clusterScope.CloudscaleCluster.Status.Networks[0].Managed).To(BeFalse())
}

func TestDeleteNetwork_PartialFailure(t *testing.T) {
	tests := []struct {
		name           string
		setup          func() (*testutils.MockNetworkService, *[]string)
		statusSetup    func(clusterScope *scope.ClusterScope)
		wantErr        string
		wantDeletedIDs []string
		wantStatusLen  int
		wantStatus0    infrastructurev1beta2.NetworkStatus
		wantStatus1    *infrastructurev1beta2.NetworkStatus
	}{
		{
			name: "keeps failed in status",
			setup: func() (*testutils.MockNetworkService, *[]string) {
				var deletedIDs []string
				return &testutils.MockNetworkService{
					DeleteFn: func(ctx context.Context, id string) error {
						if id == "net-2" {
							return fmt.Errorf("api timeout")
						}
						deletedIDs = append(deletedIDs, id)
						return nil
					},
				}, &deletedIDs
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "net-a", NetworkID: "net-1", Managed: true},
					{Name: "net-b", NetworkID: "net-2", Managed: true},
					{Name: "net-c", NetworkID: "net-3", Managed: true},
				}
			},
			wantErr:        "deleting network net-b",
			wantDeletedIDs: []string{"net-1", "net-3"},
			wantStatusLen:  1,
			wantStatus0:    infrastructurev1beta2.NetworkStatus{Name: "net-b", NetworkID: "net-2"},
		},
		{
			name: "preserves pre-existing",
			setup: func() (*testutils.MockNetworkService, *[]string) {
				var deletedIDs []string
				return &testutils.MockNetworkService{
					DeleteFn: func(ctx context.Context, id string) error {
						if id == "managed-net-2" {
							return fmt.Errorf("api timeout")
						}
						deletedIDs = append(deletedIDs, id)
						return nil
					},
				}, &deletedIDs
			},
			statusSetup: func(clusterScope *scope.ClusterScope) {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "managed-a", NetworkID: "managed-net-1", Managed: true},
					{Name: "managed-b", NetworkID: "managed-net-2", Managed: true},
					{Name: "pre-existing-net", NetworkID: "pre-existing-net-uuid", SubnetID: "pre-existing-subnet-uuid", Managed: false},
				}
			},
			wantErr:        "deleting network managed-b",
			wantDeletedIDs: []string{"managed-net-1"},
			wantStatusLen:  2,
			wantStatus0:    infrastructurev1beta2.NetworkStatus{Name: "managed-b", NetworkID: "managed-net-2"},
			wantStatus1:    &infrastructurev1beta2.NetworkStatus{Name: "pre-existing-net", NetworkID: "pre-existing-net-uuid", SubnetID: "pre-existing-subnet-uuid", Managed: false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			networkService, deletedIDs := tc.setup()
			clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
			tc.statusSetup(clusterScope)
			r := newTestReconciler()

			err := r.deleteNetwork(context.Background(), clusterScope)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(tc.wantErr))
			g.Expect(*deletedIDs).To(ConsistOf(tc.wantDeletedIDs))
			g.Expect(clusterScope.CloudscaleCluster.Status.Networks).To(HaveLen(tc.wantStatusLen))
			g.Expect(clusterScope.CloudscaleCluster.Status.Networks[0].Name).To(Equal(tc.wantStatus0.Name))
			if tc.wantStatus1 != nil {
				g.Expect(clusterScope.CloudscaleCluster.Status.Networks[1].Name).To(Equal(tc.wantStatus1.Name))
				g.Expect(clusterScope.CloudscaleCluster.Status.Networks[1].Managed).To(BeFalse())
			}
		})
	}
}

// --- pre-existing network tests ---

func TestReconcileNetwork_PreExistingCachedShortCircuits(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
			g.Fail("Get should not be called when pre-existing status is cached")
			return nil, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "pre-existing-net", UUID: "pre-existing-uuid"},
	}
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "pre-existing-net", NetworkID: "pre-existing-uuid", SubnetID: "pre-existing-subnet-uuid", CIDR: "10.0.0.0/24", Managed: false},
	}

	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("pre-existing-net")
	g.Expect(ns).ToNot(BeNil())
	g.Expect(ns.NetworkID).To(Equal("pre-existing-uuid"))
	g.Expect(ns.SubnetID).To(Equal("pre-existing-subnet-uuid"))
}

func TestReconcileNetwork_PreExistingFetchesData(t *testing.T) {
	tests := []struct {
		name           string
		getFn          func(ctx context.Context, id string) (*cloudscalesdk.Network, error)
		wantCIDR       string
		wantUUID       string
		wantSubnetUUID string
		wantManaged    bool
	}{
		{
			name: "re-discovers when CIDR missing",
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
				return &cloudscalesdk.Network{
					UUID: "pre-existing-uuid",
					ZonalResource: cloudscalesdk.ZonalResource{
						Zone: cloudscalesdk.ZoneStub{Slug: "rma1"},
					},
					Subnets: []cloudscalesdk.SubnetStub{
						{UUID: "pre-existing-subnet-uuid", CIDR: "192.168.0.0/24"},
					},
				}, nil
			},
			wantCIDR:       "192.168.0.0/24",
			wantUUID:       "pre-existing-uuid",
			wantSubnetUUID: "pre-existing-subnet-uuid",
			wantManaged:    false,
		},
		{
			name: "fetches and sets status",
			getFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
				return &cloudscalesdk.Network{
					UUID: "pre-existing-uuid",
					ZonalResource: cloudscalesdk.ZonalResource{
						Zone: cloudscalesdk.ZoneStub{Slug: "rma1"},
					},
					Subnets: []cloudscalesdk.SubnetStub{
						{UUID: "discovered-subnet-uuid"},
					},
				}, nil
			},
			wantCIDR:       "",
			wantUUID:       "pre-existing-uuid",
			wantSubnetUUID: "discovered-subnet-uuid",
			wantManaged:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			networkService := &testutils.MockNetworkService{
				GetFn: tc.getFn,
			}

			clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
			clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
				{Name: "pre-existing-net", UUID: "pre-existing-uuid"},
			}
			// For re-discovers case, CIDR is missing — simulates stale/upgrade status
			if tc.wantCIDR != "" {
				clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
					{Name: "pre-existing-net", NetworkID: "pre-existing-uuid", SubnetID: "pre-existing-subnet-uuid", Managed: false},
				}
			}

			r := newTestReconciler()

			_, err := r.reconcileNetwork(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			ns := clusterScope.CloudscaleCluster.Status.GetNetworkStatus("pre-existing-net")
			g.Expect(ns).ToNot(BeNil())
			g.Expect(ns.NetworkID).To(Equal(tc.wantUUID))
			g.Expect(ns.SubnetID).To(Equal(tc.wantSubnetUUID))
			if tc.wantCIDR != "" {
				g.Expect(ns.CIDR).To(Equal(tc.wantCIDR))
			}
			g.Expect(ns.Managed).To(BeFalse())
		})
	}
}

func TestReconcileNetwork_PreExistingGetError(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
			return nil, fmt.Errorf("network not found")
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "pre-existing-net", UUID: "pre-existing-uuid"},
	}

	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("network not found"))
}

func TestReconcileNetwork_PreExistingZoneMismatchErrors(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
			return &cloudscalesdk.Network{
				UUID: "pre-existing-uuid",
				ZonalResource: cloudscalesdk.ZonalResource{
					Zone: cloudscalesdk.ZoneStub{Slug: "lpg1"},
				},
				Subnets: []cloudscalesdk.SubnetStub{
					{UUID: "discovered-subnet-uuid"},
				},
			}, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "pre-existing-net", UUID: "pre-existing-uuid"},
	}

	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("lpg1"))
	g.Expect(err.Error()).To(ContainSubstring("rma1"))
	g.Expect(clusterScope.CloudscaleCluster.Status.GetNetworkStatus("pre-existing-net")).To(BeNil())
}

func TestReconcileNetwork_PreExistingNoSubnetsErrors(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.Network, error) {
			return &cloudscalesdk.Network{
				UUID: "pre-existing-uuid",
				ZonalResource: cloudscalesdk.ZonalResource{
					Zone: cloudscalesdk.ZoneStub{Slug: "rma1"},
				},
				Subnets: []cloudscalesdk.SubnetStub{},
			}, nil
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "pre-existing-net", UUID: "pre-existing-uuid"},
	}

	r := newTestReconciler()

	_, err := r.reconcileNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("has no subnets"))
}

// --- Timeout handling tests for Create() calls ---

func TestReconcileNetwork_CreateTimeoutRequeues(t *testing.T) {
	tests := []struct {
		name           string
		networkSetup   func() *testutils.MockNetworkService
		subnetSetup    func() *testutils.MockSubnetService
		wantNetCreated bool
	}{
		{
			name: "network create timeout requeues",
			networkSetup: func() *testutils.MockNetworkService {
				return &testutils.MockNetworkService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
						return nil, &url.Error{Op: "Post", URL: "https://api.example.com/v1/networks", Err: os.ErrDeadlineExceeded}
					},
				}
			},
			subnetSetup: func() *testutils.MockSubnetService {
				return &testutils.MockSubnetService{}
			},
			wantNetCreated: false,
		},
		{
			name: "subnet create timeout requeues",
			networkSetup: func() *testutils.MockNetworkService {
				return &testutils.MockNetworkService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Network, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
						return &cloudscalesdk.Network{UUID: "new-net-uuid", Name: req.Name}, nil
					},
				}
			},
			subnetSetup: func() *testutils.MockSubnetService {
				return &testutils.MockSubnetService{
					ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.Subnet, error) {
						return nil, nil
					},
					CreateFn: func(ctx context.Context, req *cloudscalesdk.SubnetCreateRequest) (*cloudscalesdk.Subnet, error) {
						return nil, &url.Error{Op: "Post", URL: "https://api.example.com/v1/subnets", Err: os.ErrDeadlineExceeded}
					},
				}
			},
			wantNetCreated: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			networkService := tc.networkSetup()
			subnetService := tc.subnetSetup()
			clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService), testutils.WithSubnetService(subnetService))
			r := newTestReconciler()

			netCreated := false
			if tc.wantNetCreated {
				oldCreateFn := networkService.CreateFn
				networkService.CreateFn = func(ctx context.Context, req *cloudscalesdk.NetworkCreateRequest) (*cloudscalesdk.Network, error) {
					netCreated = true
					return oldCreateFn(ctx, req)
				}
			}

			result, err := r.reconcileNetwork(context.Background(), clusterScope)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(netCreated).To(Equal(tc.wantNetCreated))
			g.Expect(result.RequeueAfter).To(Equal(createNetworkTimeoutRequeueAfter),
				"Should requeue after createNetworkTimeoutRequeueAfter on timeout error")
		})
	}
}

// --- LB pool-member error test ---

func TestDeleteNetwork_RequeuesOnLBPoolMembersError(t *testing.T) {
	g := NewWithT(t)

	networkService := &testutils.MockNetworkService{
		DeleteFn: func(ctx context.Context, id string) error {
			//goland:noinspection GoErrorStringFormat
			return fmt.Errorf("There are still one or more load balancer pool members in this network.") //nolint:revive // this is an actual response from the API
		},
	}

	clusterScope := testutils.NewClusterScopeOpts(testutils.WithNetworkService(networkService))
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "test", NetworkID: "net-with-pool-members", SubnetID: "subnet-uuid", Managed: true},
	}

	r := newTestReconciler()

	err := r.deleteNetwork(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("network has pending dependencies"))
}
