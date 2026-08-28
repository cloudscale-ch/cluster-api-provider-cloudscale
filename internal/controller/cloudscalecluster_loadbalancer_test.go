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
	"testing"
	"time"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v10"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

const (
	testLBUUID       = "lb-uuid"
	testPoolUUID     = "pool-uuid"
	testListenerUUID = "listener-uuid"
	testHMUUID       = "hm-uuid"
)

// newLBClusterScope builds a ClusterScope with LB enabled and every
// cloudscale LB service stubbed to "exists/running" defaults. Tests pass
// additional testutils.ClusterScopeOption values to override the slice of
// behaviour they care about.
func newLBClusterScope(opts ...testutils.ClusterScopeOption) *scope.ClusterScope {
	defaults := []testutils.ClusterScopeOption{ //nolint:prealloc
		testutils.WithLBEnabled(true),
		testutils.WithLBService(&testutils.MockLoadBalancerService{
			GetFn: func(ctx context.Context, _ string) (*cloudscalesdk.LoadBalancer, error) {
				return &cloudscalesdk.LoadBalancer{UUID: "lb-1", Status: LoadBalancerRunningStatus}, nil
			},
		}),
		testutils.WithPoolService(&testutils.MockLoadBalancerPoolService{
			GetFn: func(ctx context.Context, _ string) (*cloudscalesdk.LoadBalancerPool, error) {
				return &cloudscalesdk.LoadBalancerPool{UUID: "pool-1"}, nil
			},
		}),
		testutils.WithListenerService(&testutils.MockLoadBalancerListenerService{
			GetFn: func(ctx context.Context, _ string) (*cloudscalesdk.LoadBalancerListener, error) {
				return &cloudscalesdk.LoadBalancerListener{UUID: "listener-1"}, nil
			},
		}),
		testutils.WithHMService(&testutils.MockLoadBalancerHealthMonitorService{
			GetFn: func(ctx context.Context, _ string) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
				return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: "hm-1"}, nil
			},
		}),
		testutils.WithMemberService(&testutils.MockLoadBalancerPoolMemberService{
			ListFn: func(ctx context.Context, _ string, _ ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
				return nil, nil
			},
		}),
		testutils.WithPreExistingNetwork("test", "net-uuid", "subnet-uuid", "10.0.0.0/24"),
	}
	return testutils.NewClusterScopeOpts(append(defaults, opts...)...)
}

// lbServiceWithStatus returns a mock LoadBalancerService whose Get returns an
// LB in the requested status. Used by orchestrator tests that exercise specific
// LB lifecycle states (changing/error/degraded).
func lbServiceWithStatus(status string) *testutils.MockLoadBalancerService {
	return &testutils.MockLoadBalancerService{
		GetFn: func(ctx context.Context, _ string) (*cloudscalesdk.LoadBalancer, error) {
			return &cloudscalesdk.LoadBalancer{UUID: "lb-uuid", Status: status}, nil
		},
	}
}

// ============================================================================
// Tests for reconcileLB
// ============================================================================

func TestReconcileLB(t *testing.T) {
	cases := []struct {
		name       string
		flavor     string
		wantFlavor string
	}{
		{name: "default flavor wired through", flavor: "lb-standard", wantFlavor: "lb-standard"},
		{name: "custom flavor wired through", flavor: "lb-flex-2", wantFlavor: "lb-flex-2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var capturedReq *cloudscalesdk.LoadBalancerRequest
			lbService := &testutils.MockLoadBalancerService{
				ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancer, error) {
					return nil, nil
				},
				CreateFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
					capturedReq = req
					return &cloudscalesdk.LoadBalancer{
						UUID:   "lb-uuid-123",
						Name:   req.Name,
						Status: LoadBalancerChangingStatus,
					}, nil
				},
			}

			clusterScope := newLBClusterScope(
				testutils.WithLBService(lbService),
				testutils.WithFlavor(tc.flavor),
			)

			_, err := newTestReconciler().reconcileLB(context.Background(), clusterScope)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerID).To(Equal("lb-uuid-123"))
			g.Expect(capturedReq.Name).To(Equal("test-cluster-cp-lb"))
			g.Expect(capturedReq.Zone).To(Equal("rma1"))
			g.Expect(capturedReq.Flavor).To(Equal(tc.wantFlavor))
		})
	}
}

// ============================================================================
// Tests for reconcileLBPool
// ============================================================================

func TestReconcileLBPool(t *testing.T) {
	cases := []struct {
		name          string
		algorithm     string
		wantAlgorithm string
	}{
		{name: "round_robin", algorithm: "round_robin", wantAlgorithm: "round_robin"},
		{name: "least_connections", algorithm: "least_connections", wantAlgorithm: "least_connections"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var capturedReq *cloudscalesdk.LoadBalancerPoolRequest
			poolService := &testutils.MockLoadBalancerPoolService{
				ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPool, error) {
					return nil, nil
				},
				CreateFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerPoolRequest) (*cloudscalesdk.LoadBalancerPool, error) {
					capturedReq = req
					return &cloudscalesdk.LoadBalancerPool{UUID: "pool-uuid-123", Name: req.Name}, nil
				},
			}

			clusterScope := newLBClusterScope(
				testutils.WithPoolService(poolService),
				testutils.WithAlgorithm(tc.algorithm),
			)
			clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID

			_, err := newTestReconciler().reconcileLBPool(context.Background(), clusterScope)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID).To(Equal("pool-uuid-123"))
			g.Expect(capturedReq.Name).To(Equal("test-cluster-cp-pool"))
			g.Expect(capturedReq.LoadBalancer).To(Equal(testLBUUID))
			g.Expect(capturedReq.Algorithm).To(Equal(tc.wantAlgorithm))
			g.Expect(capturedReq.Protocol).To(Equal("tcp"))
		})
	}
}

// ============================================================================
// Tests for reconcileLBListener
// ============================================================================

func TestReconcileLBListener(t *testing.T) {
	cases := []struct {
		name     string
		port     int32
		wantPort int
	}{
		{name: "default port 6443", port: 6443, wantPort: 6443},
		{name: "custom port 8443", port: 8443, wantPort: 8443},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var capturedReq *cloudscalesdk.LoadBalancerListenerRequest
			listenerService := &testutils.MockLoadBalancerListenerService{
				ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerListener, error) {
					return nil, nil
				},
				CreateFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerListenerRequest) (*cloudscalesdk.LoadBalancerListener, error) {
					capturedReq = req
					return &cloudscalesdk.LoadBalancerListener{UUID: "listener-uuid-123", Name: req.Name}, nil
				},
			}

			clusterScope := newLBClusterScope(
				testutils.WithListenerService(listenerService),
				testutils.WithAPIServerPort(tc.port),
			)
			clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID

			_, err := newTestReconciler().reconcileLBListener(context.Background(), clusterScope)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID).To(Equal("listener-uuid-123"))
			g.Expect(capturedReq.Name).To(Equal("test-cluster-cp-listener"))
			g.Expect(capturedReq.Pool).To(Equal(testPoolUUID))
			g.Expect(capturedReq.Protocol).To(Equal("tcp"))
			g.Expect(capturedReq.ProtocolPort).To(Equal(tc.wantPort))
		})
	}
}

// ============================================================================
// Tests for reconcileLBHealthMonitor
// ============================================================================

func TestReconcileLBHealthMonitor(t *testing.T) {
	cases := []struct {
		name                                      string
		delayS, timeoutS, upThreshold, downThresh int
	}{
		{name: "default thresholds", delayS: 5, timeoutS: 3, upThreshold: 2, downThresh: 3},
		{name: "custom thresholds", delayS: 10, timeoutS: 5, upThreshold: 3, downThresh: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var capturedReq *cloudscalesdk.LoadBalancerHealthMonitorRequest
			healthMonitorService := &testutils.MockLoadBalancerHealthMonitorService{
				ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerHealthMonitor, error) {
					return nil, nil
				},
				CreateFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerHealthMonitorRequest) (*cloudscalesdk.LoadBalancerHealthMonitor, error) {
					capturedReq = req
					return &cloudscalesdk.LoadBalancerHealthMonitor{UUID: "hm-uuid-123"}, nil
				},
			}

			clusterScope := newLBClusterScope(
				testutils.WithHMService(healthMonitorService),
				testutils.WithHealthMonitorParams(tc.delayS, tc.timeoutS, tc.upThreshold, tc.downThresh),
			)
			clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID

			_, err := newTestReconciler().reconcileLBHealthMonitor(context.Background(), clusterScope)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID).To(Equal("hm-uuid-123"))
			g.Expect(capturedReq.Pool).To(Equal(testPoolUUID))
			g.Expect(capturedReq.Type).To(Equal("tcp"))
			g.Expect(capturedReq.DelayS).To(Equal(tc.delayS))
			g.Expect(capturedReq.TimeoutS).To(Equal(tc.timeoutS))
			g.Expect(capturedReq.UpThreshold).To(Equal(tc.upThreshold))
			g.Expect(capturedReq.DownThreshold).To(Equal(tc.downThresh))
		})
	}
}

// ============================================================================
// Tests for reconcileLoadBalancer (main orchestrator)
// ============================================================================

func TestReconcileLoadBalancer_SkipsWhenDisabled(t *testing.T) {
	g := NewWithT(t)

	lbService := &testutils.MockLoadBalancerService{
		CreateFn: func(ctx context.Context, req *cloudscalesdk.LoadBalancerRequest) (*cloudscalesdk.LoadBalancer, error) {
			g.Fail("Create should not be called when LB is disabled")
			return nil, nil
		},
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
			g.Fail("Get should not be called when LB is disabled")
			return nil, nil
		},
	}

	clusterScope := newLBClusterScope(
		testutils.WithLBEnabled(false), // Disabled for external control plane
		testutils.WithLBService(lbService),
	)

	r := newTestReconciler()

	status, result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeZero())
	g.Expect(status.ready).To(BeTrue(), "disabled LB reports running so reconcileNormal does not poll")
}

func TestReconcileLoadBalancer_ChangingStatusBlocksAndRequeues(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope(
		testutils.WithLBService(lbServiceWithStatus(LoadBalancerChangingStatus)),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID

	r := newTestReconciler()

	status, result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(10*time.Second), "changing status should requeue after 10s")
	g.Expect(status.ready).To(BeFalse())
}

func TestReconcileLoadBalancer_ErrorStatusProceedsWithMemberReconciliation(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope(
		testutils.WithLBService(lbServiceWithStatus(LoadBalancerErrorStatus)),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID

	r := newTestReconciler()

	status, result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeZero(), "error status does not requeue, reconcileNormal polls due to the ready: false flag")
	g.Expect(status.ready).To(BeFalse())

	// LoadBalancerReadyCondition should be False because LB is not running
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.LoadBalancerReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.LoadBalancerNotReadyReason))
}

func TestReconcileLoadBalancer_DegradedStatusProceedsWithMemberReconciliation(t *testing.T) {
	g := NewWithT(t)

	var deletedMemberUUID string

	clusterScope := newLBClusterScope(
		testutils.WithLBService(lbServiceWithStatus(LoadBalancerDegradedStatus)),
		testutils.WithMemberService(&testutils.MockLoadBalancerPoolMemberService{
			ListFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
				return []cloudscalesdk.LoadBalancerPoolMember{
					{Name: "stale-cp-0", UUID: "stale-uuid", Address: "10.0.0.10"},
				}, nil
			},
			DeleteFn: func(ctx context.Context, poolID, memberID string) error {
				deletedMemberUUID = memberID
				return nil
			},
		}),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID

	r := newTestReconciler()

	status, result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeZero(), "degraded status does not requeue, reconcileNormal polls due to the ready: false flag")
	g.Expect(status.ready).To(BeFalse())

	// Stale member should have been removed
	g.Expect(deletedMemberUUID).To(Equal("stale-uuid"))

	// LoadBalancerReadyCondition should be False because LB is degraded
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.LoadBalancerReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.LoadBalancerNotReadyReason))
}

func TestReconcileLoadBalancer_ErrorStatusMessageNamesDownMembers(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope(
		testutils.WithLBService(lbServiceWithStatus(LoadBalancerErrorStatus)),
		testutils.WithMemberService(&testutils.MockLoadBalancerPoolMemberService{
			ListFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
				return []cloudscalesdk.LoadBalancerPoolMember{
					{Name: "cp-0", UUID: "cp-uuid", Address: "10.0.0.110", ProtocolPort: 6443, MonitorStatus: "down"},
				}, nil
			},
		}),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID

	r := newTestReconciler()

	_, _, err := r.reconcileLoadBalancer(context.Background(), clusterScope)
	g.Expect(err).ToNot(HaveOccurred())

	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.LoadBalancerReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.LoadBalancerNotReadyReason))
	g.Expect(cond.Message).To(ContainSubstring("error"))
	g.Expect(cond.Message).To(ContainSubstring("cp-0@10.0.0.110:6443"))
}

// TestReconcileLoadBalancer_ObservesVIPAndRunning verifies reconcileLoadBalancer
// reports the VIP and ready status via lbStatus and does not set the
// control plane endpoint itself (that is reconcileControlPlaneEndpoint's job).
func TestReconcileLoadBalancer_ObservesVIPAndRunning(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope(
		testutils.WithLBService(&testutils.MockLoadBalancerService{
			GetFn: func(ctx context.Context, id string) (*cloudscalesdk.LoadBalancer, error) {
				return &cloudscalesdk.LoadBalancer{
					UUID:   id,
					Status: LoadBalancerRunningStatus,
					VIPAddresses: []cloudscalesdk.VIPAddress{
						{Address: "203.0.113.10"},
					},
				}, nil
			},
		}),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID

	r := newTestReconciler()

	status, result, err := r.reconcileLoadBalancer(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(status.ipAddress).To(Equal("203.0.113.10"))
	g.Expect(status.ready).To(BeTrue())
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(BeEmpty(),
		"reconcileLoadBalancer does not set the endpoint")
}

// ============================================================================
// Tests for deleteLoadBalancer
// ============================================================================

func TestDeleteLoadBalancer(t *testing.T) {
	cases := []struct {
		name         string
		lbEnabled    bool
		deleteErr    error
		expectDelete bool
		expectClear  bool
	}{
		{
			// LB disabled: delete must not be called and status must not be cleared.
			name:      "skips when LB disabled",
			lbEnabled: false,
		},
		{
			// LB enabled + delete succeeds: deletes only the LB (children cascade)
			// and clears all four status IDs + member IDs.
			name:         "deletes only the LB and clears all status IDs",
			lbEnabled:    true,
			expectDelete: true,
			expectClear:  true,
		},
		{
			// 404 on delete is idempotent; status still cleared.
			name:         "ignores 404 and clears status",
			lbEnabled:    true,
			deleteErr:    &cloudscalesdk.ErrorResponse{StatusCode: 404},
			expectDelete: true,
			expectClear:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			var lbDeleteCalled bool
			var deletedLBID string
			lbService := &testutils.MockLoadBalancerService{
				DeleteFn: func(ctx context.Context, id string) error {
					if !tc.lbEnabled {
						g.Fail("Delete should not be called when LB is disabled")
					}
					lbDeleteCalled = true
					deletedLBID = id
					return tc.deleteErr
				},
			}
			// Child services must never be hit — the API cascade-deletes.
			poolService := &testutils.MockLoadBalancerPoolService{
				DeleteFn: func(ctx context.Context, id string) error {
					g.Fail("pool delete should not be called")
					return nil
				},
			}
			listenerService := &testutils.MockLoadBalancerListenerService{
				DeleteFn: func(ctx context.Context, id string) error {
					g.Fail("listener delete should not be called")
					return nil
				},
			}
			healthMonitorService := &testutils.MockLoadBalancerHealthMonitorService{
				DeleteFn: func(ctx context.Context, id string) error {
					g.Fail("health monitor delete should not be called")
					return nil
				},
			}

			clusterScope := newLBClusterScope(
				testutils.WithLBEnabled(tc.lbEnabled),
				testutils.WithLBService(lbService),
				testutils.WithPoolService(poolService),
				testutils.WithListenerService(listenerService),
				testutils.WithHMService(healthMonitorService),
			)
			clusterScope.CloudscaleCluster.Status.LoadBalancerID = testLBUUID
			clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
			clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID = testListenerUUID
			clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID = testHMUUID
			clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"member-1", "member-2"}

			err := newTestReconciler().deleteLoadBalancer(context.Background(), clusterScope)
			g.Expect(err).ToNot(HaveOccurred())

			if tc.expectDelete {
				g.Expect(lbDeleteCalled).To(BeTrue())
				g.Expect(deletedLBID).To(Equal(testLBUUID))
			}
			if tc.expectClear {
				g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerID).To(BeEmpty())
				g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID).To(BeEmpty())
				g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerListenerID).To(BeEmpty())
				g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerHealthMonitorID).To(BeEmpty())
				g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs).To(BeNil())
			} else {
				// LB-disabled branch keeps the IDs around.
				g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerID).To(Equal(testLBUUID))
			}
		})
	}
}

// ============================================================================
// Tests for reconcileLBMembers
// ============================================================================

func TestReconcileLBMembers_AddsMissingMember(t *testing.T) {
	g := NewWithT(t)

	var createdReq *cloudscalesdk.LoadBalancerPoolMemberRequest

	poolMemberService := &testutils.MockLoadBalancerPoolMemberService{
		ListFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return nil, nil // no existing members
		},
		CreateFn: func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
			createdReq = req
			return &cloudscalesdk.LoadBalancerPoolMember{UUID: "new-member-uuid", Name: req.Name, Address: req.Address}, nil
		},
	}

	machine := &infrastructurev1beta2.CloudscaleMachine{
		Name:      "cp-machine-1",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel:         "test-cluster",
			clusterv1.MachineControlPlaneLabel: "",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
			},
		},
	}

	clusterScope := newLBClusterScope(
		testutils.WithMemberService(poolMemberService),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "net-uuid", SubnetID: "subnet-uuid", CIDR: "10.0.0.0/24", Managed: true}}

	r := newTestReconciler(machine)

	_, err := r.reconcileLBMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(createdReq).ToNot(BeNil())
	g.Expect(createdReq.Name).To(Equal("cp-machine-1"))
	g.Expect(createdReq.Address).To(Equal("10.0.0.1"))
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs).To(ContainElement("new-member-uuid"))
}

func TestReconcileLBMembers_RemovesStaleMember(t *testing.T) {
	g := NewWithT(t)

	var deletedMemberID string

	poolMemberService := &testutils.MockLoadBalancerPoolMemberService{
		ListFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return []cloudscalesdk.LoadBalancerPoolMember{
				{UUID: "stale-member-uuid", Name: "removed-machine", Address: "10.0.0.99"},
			}, nil
		},
		DeleteFn: func(ctx context.Context, poolID, memberID string) error {
			deletedMemberID = memberID
			return nil
		},
	}

	// No machines in cluster
	clusterScope := newLBClusterScope(
		testutils.WithMemberService(poolMemberService),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"stale-member-uuid"}

	r := newTestReconciler()

	_, err := r.reconcileLBMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedMemberID).To(Equal("stale-member-uuid"))
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs).ToNot(ContainElement("stale-member-uuid"))
}

func TestReconcileLBMembers_UpdatesChangedAddress(t *testing.T) {
	g := NewWithT(t)

	var updatedMemberID string
	var updatedReq *cloudscalesdk.LoadBalancerPoolMemberRequest

	poolMemberService := &testutils.MockLoadBalancerPoolMemberService{
		ListFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return []cloudscalesdk.LoadBalancerPoolMember{
				{UUID: "member-uuid-1", Name: "cp-machine-1", Address: "10.0.0.1"},
			}, nil
		},
		UpdateFn: func(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error {
			updatedMemberID = memberID
			updatedReq = req
			return nil
		},
	}

	machine := &infrastructurev1beta2.CloudscaleMachine{
		Name:      "cp-machine-1",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel:         "test-cluster",
			clusterv1.MachineControlPlaneLabel: "",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.2"}, // changed IP
			},
		},
	}

	clusterScope := newLBClusterScope(
		testutils.WithMemberService(poolMemberService),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"member-uuid-1"}

	r := newTestReconciler(machine)

	_, err := r.reconcileLBMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updatedMemberID).To(Equal("member-uuid-1"))
	g.Expect(updatedReq).ToNot(BeNil())
	g.Expect(updatedReq.Address).To(Equal("10.0.0.2"))
}

func TestReconcileLBMembers_NoopWhenInSync(t *testing.T) {
	g := NewWithT(t)

	poolMemberService := &testutils.MockLoadBalancerPoolMemberService{
		ListFn: func(ctx context.Context, poolID string, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.LoadBalancerPoolMember, error) {
			return []cloudscalesdk.LoadBalancerPoolMember{
				{UUID: "member-uuid-1", Name: "cp-machine-1", Address: "10.0.0.1"},
			}, nil
		},
		CreateFn: func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
			g.Fail("Create should not be called when in sync")
			return nil, nil
		},
		DeleteFn: func(ctx context.Context, poolID, memberID string) error {
			g.Fail("Delete should not be called when in sync")
			return nil
		},
		UpdateFn: func(ctx context.Context, poolID, memberID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) error {
			g.Fail("Update should not be called when in sync")
			return nil
		},
	}

	machine := &infrastructurev1beta2.CloudscaleMachine{
		Name:      "cp-machine-1",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel:         "test-cluster",
			clusterv1.MachineControlPlaneLabel: "",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
			},
		},
	}

	clusterScope := newLBClusterScope(
		testutils.WithMemberService(poolMemberService),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"member-uuid-1"}

	r := newTestReconciler(machine)

	_, err := r.reconcileLBMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

// ============================================================================
// Tests for getDesiredLoadBalancerMembers
// ============================================================================

func TestGetDesiredLoadBalancerMembers_SkipsMachinesWithoutIP(t *testing.T) {
	g := NewWithT(t)

	machineWithIP := &infrastructurev1beta2.CloudscaleMachine{
		Name:      "cp-machine-1",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel:         "test-cluster",
			clusterv1.MachineControlPlaneLabel: "",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.1"},
			},
		},
	}
	machineWithoutIP := &infrastructurev1beta2.CloudscaleMachine{
		Name:      "cp-machine-2",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel:         "test-cluster",
			clusterv1.MachineControlPlaneLabel: "",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: nil, // no addresses yet
		},
	}

	clusterScope := newLBClusterScope()
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "net-uuid", SubnetID: "subnet-uuid", CIDR: "10.0.0.0/24", Managed: true}}

	r := newTestReconciler(machineWithIP, machineWithoutIP)

	members, err := r.getDesiredLoadBalancerMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(members).To(HaveLen(1))
	g.Expect(members[0].Name).To(Equal("cp-machine-1"))
	g.Expect(members[0].Address).To(Equal("10.0.0.1"))
}

func TestGetDesiredLoadBalancerMembers_NoMachines(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope()

	r := newTestReconciler()

	members, err := r.getDesiredLoadBalancerMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(members).To(BeEmpty())
}

func TestGetDesiredLoadBalancerMembers_PicksAddressInMemberSubnet(t *testing.T) {
	g := NewWithT(t)

	machine := &infrastructurev1beta2.CloudscaleMachine{
		Name:      "cp-machine-1",
		Namespace: "default",
		Labels: map[string]string{
			clusterv1.ClusterNameLabel:         "test-cluster",
			clusterv1.MachineControlPlaneLabel: "",
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{Type: clusterv1.MachineInternalIP, Address: "192.168.1.5"}, // wrong network
				{Type: clusterv1.MachineInternalIP, Address: "10.0.0.15"},   // correct network
				{Type: clusterv1.MachineExternalIP, Address: "185.1.2.3"},
			},
		},
	}

	clusterScope := newLBClusterScope()
	clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
		{Name: "main", CIDR: "10.0.0.0/24"},
	}
	clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
		{Name: "main", NetworkID: "net-uuid", SubnetID: "subnet-uuid", CIDR: "10.0.0.0/24", Managed: true},
	}

	r := newTestReconciler(machine)

	members, err := r.getDesiredLoadBalancerMembers(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(members).To(HaveLen(1))
	g.Expect(members[0].Address).To(Equal("10.0.0.15"))
}

// ============================================================================
// Tests for createLoadBalancerMember / deleteLoadBalancerMember
// ============================================================================

func TestCreateLoadBalancerMember_AppendsToStatus(t *testing.T) {
	g := NewWithT(t)

	poolMemberService := &testutils.MockLoadBalancerPoolMemberService{
		CreateFn: func(ctx context.Context, poolID string, req *cloudscalesdk.LoadBalancerPoolMemberRequest) (*cloudscalesdk.LoadBalancerPoolMember, error) {
			return &cloudscalesdk.LoadBalancerPoolMember{UUID: "new-uuid", Name: req.Name}, nil
		},
	}

	clusterScope := newLBClusterScope(
		testutils.WithMemberService(poolMemberService),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"existing-uuid"}

	r := newTestReconciler()

	_, err := r.createLoadBalancerMember(context.Background(), clusterScope, cloudscalesdk.LoadBalancerPoolMemberRequest{
		Name:    "cp-machine-1",
		Address: "10.0.0.1",
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs).To(Equal([]string{"existing-uuid", "new-uuid"}))
}

func TestDeleteLoadBalancerMember_RemovesFromStatus(t *testing.T) {
	g := NewWithT(t)

	poolMemberService := &testutils.MockLoadBalancerPoolMemberService{
		DeleteFn: func(ctx context.Context, poolID, memberID string) error {
			return nil
		},
	}

	clusterScope := newLBClusterScope(
		testutils.WithMemberService(poolMemberService),
	)
	clusterScope.CloudscaleCluster.Status.LoadBalancerPoolID = testPoolUUID
	clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs = []string{"keep-uuid", "remove-uuid"}

	r := newTestReconciler()

	err := r.deleteLoadBalancerMember(context.Background(), clusterScope, cloudscalesdk.LoadBalancerPoolMember{
		UUID: "remove-uuid",
		Name: "cp-machine-old",
	})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.LoadBalancerMemberIDs).To(Equal([]string{"keep-uuid"}))
}

// ============================================================================
// Tests for getPoolMemberSubnetID
// ============================================================================

func TestGetPoolMemberSubnetID(t *testing.T) {
	cases := []struct {
		name              string
		lbNetwork         string
		poolMemberNetwork string
		want              string
		wantErr           string
	}{
		{name: "poolMemberNetwork wins over network", lbNetwork: "vip", poolMemberNetwork: "nodes", want: "subnet-nodes"},
		{name: "poolMemberNetwork with public VIP", poolMemberNetwork: "nodes", want: "subnet-nodes"},
		{name: "falls back to network", lbNetwork: "vip", want: "subnet-vip"},
		{name: "falls back to first network", want: "subnet-vip"},
		{name: "unprovisioned poolMemberNetwork errors", poolMemberNetwork: "missing", wantErr: "not yet provisioned"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			clusterScope := newLBClusterScope()
			clusterScope.CloudscaleCluster.Spec.Networks = []infrastructurev1beta2.NetworkSpec{
				{Name: "vip", CIDR: "10.0.0.0/24"},
				{Name: "nodes", CIDR: "10.1.0.0/24"},
			}
			clusterScope.CloudscaleCluster.Status.Networks = []infrastructurev1beta2.NetworkStatus{
				{Name: "vip", NetworkID: "net-vip", SubnetID: "subnet-vip", CIDR: "10.0.0.0/24", Managed: true},
				{Name: "nodes", NetworkID: "net-nodes", SubnetID: "subnet-nodes", CIDR: "10.1.0.0/24", Managed: true},
			}
			clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Network = tc.lbNetwork
			clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.PoolMemberNetwork = tc.poolMemberNetwork

			subnetID, err := newTestReconciler().getPoolMemberSubnetID(clusterScope)

			if tc.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.wantErr))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(subnetID).To(Equal(tc.want))
		})
	}
}
