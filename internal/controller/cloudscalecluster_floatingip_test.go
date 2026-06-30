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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/testutils"
)

// --- Test helpers ---

func newFIPTestClusterScope(fipService cloudscale.FloatingIPService) *scope.ClusterScope {
	return testutils.NewClusterScopeOpts(
		testutils.WithLBEnabled(true),
		testutils.WithFloatingIPService(fipService),
	)
}

// --- reconcileFloatingIP orchestrator tests ---

func TestReconcileFloatingIP_Disabled(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&testutils.MockFloatingIPService{})
	// No FloatingIP spec = disabled
	r := newTestReconciler()

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPDisabledReason))
}

func TestReconcileFloatingIP_PreExisting(t *testing.T) {
	g := NewWithT(t)

	fipService := &testutils.MockFloatingIPService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	r := newTestReconciler()

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("1.2.3.4"))
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPProvisionedReason))
}

func TestReconcileFloatingIP_ErrorSetsConditionFalse(t *testing.T) {
	g := NewWithT(t)

	fipService := &testutils.MockFloatingIPService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	r := newTestReconciler()

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

// --- reconcilePreExistingFloatingIP tests ---

// TestReconcilePreExistingFloatingIP covers the direct-call paths through
// reconcilePreExistingFloatingIP. The orchestrator-level tests
// (RegionMismatch, NoPublicInterface) go through reconcileFloatingIP and are
// kept standalone below.
func TestReconcilePreExistingFloatingIP(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(cs *scope.ClusterScope)
		fip        *cloudscalesdk.FloatingIP // returned by GetFn (nil ⇒ error)
		getErr     error
		lbDisabled bool
		cpMachine  *infrastructurev1beta2.CloudscaleMachine
		wantErrSub string
		wantGet    bool
		assert     func(g *WithT, cs *scope.ClusterScope, captured *cloudscalesdk.FloatingIPUpdateRequest)
	}{
		{
			name: "refetches and keeps assignment when already on the right LB",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
				cs.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
				cs.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = "1.2.3.4"
			},
			fip: &cloudscalesdk.FloatingIP{
				Network:      "1.2.3.4/32",
				LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "lb-uuid"},
			},
			wantGet: true,
			assert: func(g *WithT, cs *scope.ClusterScope, captured *cloudscalesdk.FloatingIPUpdateRequest) {
				g.Expect(captured).To(BeNil(), "Update must not fire when FIP is already assigned to the LB")
			},
		},
		{
			name: "fetches FIP and assigns to LB; sets status",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.LoadBalancerID = "lb-x"
			},
			fip: &cloudscalesdk.FloatingIP{Network: "5.6.7.8/32"},
			assert: func(g *WithT, cs *scope.ClusterScope, captured *cloudscalesdk.FloatingIPUpdateRequest) {
				g.Expect(cs.CloudscaleCluster.Status.FloatingIP).To(Equal("5.6.7.8"))
				g.Expect(captured).ToNot(BeNil())
				g.Expect(captured.LoadBalancer).To(Equal("lb-x"))
			},
		},
		{
			name:       "Get error propagates",
			setup:      func(cs *scope.ClusterScope) {},
			getErr:     fmt.Errorf("not found"),
			wantErrSub: "getting pre-existing floating IP",
		},
		{
			name: "matching region is accepted and FIP assigned to LB",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.LoadBalancerID = "lb-rma"
			},
			fip: &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Region:  &cloudscalesdk.RegionStub{Slug: "rma"},
			},
			assert: func(g *WithT, cs *scope.ClusterScope, captured *cloudscalesdk.FloatingIPUpdateRequest) {
				g.Expect(cs.CloudscaleCluster.Status.FloatingIP).To(Equal("1.2.3.4"))
				g.Expect(captured).ToNot(BeNil())
				g.Expect(captured.LoadBalancer).To(Equal("lb-rma"))
			},
		},
		{
			name:       "LB disabled — assigns to first ready CP server",
			setup:      func(cs *scope.ClusterScope) {},
			fip:        &cloudscalesdk.FloatingIP{Network: "1.2.3.4/32"},
			lbDisabled: true,
			cpMachine:  testutils.NewControlPlaneMachine("cp-machine-0", "srv-x"),
			assert: func(g *WithT, cs *scope.ClusterScope, captured *cloudscalesdk.FloatingIPUpdateRequest) {
				g.Expect(captured).ToNot(BeNil(), "Pre-existing FIP must be assigned to the CP server when LB is disabled")
				g.Expect(captured.Server).To(Equal("srv-x"))
			},
		},
		{
			name:  "global FIP (no Region) is accepted",
			setup: func(cs *scope.ClusterScope) {},
			fip:   &cloudscalesdk.FloatingIP{Network: "9.9.9.9/32", Region: nil},
			assert: func(g *WithT, cs *scope.ClusterScope, _ *cloudscalesdk.FloatingIPUpdateRequest) {
				g.Expect(cs.CloudscaleCluster.Status.FloatingIP).To(Equal("9.9.9.9"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var captured *cloudscalesdk.FloatingIPUpdateRequest
			var getCalled bool

			fipService := &testutils.MockFloatingIPService{
				GetFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
					getCalled = true
					if tc.getErr != nil {
						return nil, tc.getErr
					}
					return tc.fip, nil
				},
				UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
					captured = req
					return nil
				},
			}

			clusterScope := newFIPTestClusterScope(fipService)
			if tc.lbDisabled {
				clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
			}
			tc.setup(clusterScope)

			objs := []client.Object{}
			if tc.cpMachine != nil {
				objs = append(objs, tc.cpMachine)
			}
			r := newTestReconciler(objs...)

			err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")
			if tc.wantErrSub != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.wantErrSub))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tc.wantGet {
				g.Expect(getCalled).To(BeTrue(), "Pre-existing FIP must be refetched so the assignment can be verified")
			}
			if tc.assert != nil {
				tc.assert(g, clusterScope, captured)
			}
		})
	}
}

// TestReconcileFloatingIP_RegionMismatchErrors goes through reconcileFloatingIP
// (orchestrator path) to exercise the region-mismatch error wiring including
// the FloatingIPReadyCondition rollup.
func TestReconcileFloatingIP_RegionMismatchErrors(t *testing.T) {
	g := NewWithT(t)

	fipService := &testutils.MockFloatingIPService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Region:  &cloudscalesdk.RegionStub{Slug: "lpg"},
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{Address: "1.2.3.4"}

	_, err := newTestReconciler().reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("lpg"))
	g.Expect(err.Error()).To(ContainSubstring("rma"))
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(BeEmpty())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

// TestReconcileFloatingIP_NoPublicInterfaceSetsConditionAndEvent exercises the
// specific 400/no-public-interface error path through the orchestrator.
func TestReconcileFloatingIP_NoPublicInterfaceSetsConditionAndEvent(t *testing.T) {
	g := NewWithT(t)
	fipService := &testutils.MockFloatingIPService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Region:  &cloudscalesdk.RegionStub{Slug: "rma"},
			}, nil
		},
		UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			return &cloudscalesdk.ErrorResponse{
				StatusCode: 400,
				Message: map[string]string{
					"detail": "This server does not have a public interface with an IPv4 address.",
				},
			}
		},
	}
	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{Address: "1.2.3.4"}
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newTestReconciler(testutils.NewControlPlaneMachine("cp-0", "server-uuid"))

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("public interface"))
	g.Expect(err.Error()).To(ContainSubstring("control-plane machine template"))
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

// --- reconcileManagedFloatingIP tests ---

func TestReconcileManagedFloatingIP_ExistingFIPEnsuresAssignment(t *testing.T) {
	g := NewWithT(t)

	fipService := &testutils.MockFloatingIPService{
		GetFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network:      "10.0.0.1/32",
				LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "lb-uuid"},
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.FloatingIP = "10.0.0.1"
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newTestReconciler()

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("10.0.0.1"))
}

func TestReconcileManagedFloatingIP_CreatesIPv4(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.FloatingIPCreateRequest

	fipService := &testutils.MockFloatingIPService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			capturedReq = req
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newTestReconciler()

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.IPVersion).To(Equal(4))
	g.Expect(capturedReq.LoadBalancer).To(Equal("lb-uuid"))
	g.Expect(capturedReq.Region).To(Equal("rma"))
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("1.2.3.4"))
}

func TestReconcileManagedFloatingIP_CreatesIPv6(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.FloatingIPCreateRequest

	fipService := &testutils.MockFloatingIPService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			capturedReq = req
			return &cloudscalesdk.FloatingIP{
				Network: "2001:db8::1/128",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: new(infrastructurev1beta2.IPFamilyIPv6),
	}

	r := newTestReconciler()

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq.IPVersion).To(Equal(6))
}

func TestReconcileManagedFloatingIP_CreateError(t *testing.T) {
	g := NewWithT(t)

	fipService := &testutils.MockFloatingIPService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("quota exceeded")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newTestReconciler()

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("creating floating IP"))
}

func TestReconcileManagedFloatingIP_AssignsToLBTarget(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.FloatingIPCreateRequest

	fipService := &testutils.MockFloatingIPService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			capturedReq = req
			return &cloudscalesdk.FloatingIP{Network: "1.2.3.4/32"}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "target-lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newTestReconciler()

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.LoadBalancer).To(Equal("target-lb-uuid"))
	g.Expect(capturedReq.Server).To(BeEmpty())
}

// --- getFloatingIPTarget tests ---

func TestGetFloatingIPTarget_LBEnabled(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&testutils.MockFloatingIPService{})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"

	r := newTestReconciler()

	target, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(target.lbUUID).To(Equal("lb-uuid"))
	g.Expect(target.serverUUID).To(BeEmpty())
}

func TestGetFloatingIPTarget_LBNotProvisioned(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&testutils.MockFloatingIPService{})
	// LB enabled but no LB ID yet

	r := newTestReconciler()

	_, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("waiting for load balancer"))
}

func TestGetFloatingIPTarget_LBDisabled_FindsCPServer(t *testing.T) {
	g := NewWithT(t)

	cpMachine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-machine-0",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			ServerID: "cp-server-uuid",
		},
	}

	clusterScope := newFIPTestClusterScope(&testutils.MockFloatingIPService{})
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(false)

	r := newTestReconciler(cpMachine)

	target, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(target.serverUUID).To(Equal("cp-server-uuid"))
	g.Expect(target.lbUUID).To(BeEmpty())
}

func TestGetFloatingIPTarget_LBDisabled_NoCPServer(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&testutils.MockFloatingIPService{})
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(false)

	r := newTestReconciler()

	_, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("waiting for a control plane server to be provisioned"))
}

// --- ensureFloatingIPAssignment tests ---

func TestEnsureFloatingIPAssignment(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(cs *scope.ClusterScope)
		fip        *cloudscalesdk.FloatingIP
		updateErr  error
		lbDisabled bool
		cpMachine  *infrastructurev1beta2.CloudscaleMachine
		wantUpdate bool
		wantErrSub string
		assert     func(g *WithT, captured *cloudscalesdk.FloatingIPUpdateRequest, capturedID string)
	}{
		{
			name:  "target not ready (LB enabled but no LB ID) is a no-op",
			setup: func(cs *scope.ClusterScope) {},
			fip:   &cloudscalesdk.FloatingIP{Network: "1.2.3.4/32"},
		},
		{
			name: "LB already correctly assigned — no Update",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
			},
			fip: &cloudscalesdk.FloatingIP{
				Network:      "1.2.3.4/32",
				LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "lb-uuid"},
			},
		},
		{
			name: "reassigns LB when assignment is stale",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.LoadBalancerID = "new-lb-uuid"
				cs.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
			},
			fip: &cloudscalesdk.FloatingIP{
				Network:      "1.2.3.4/32",
				LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "old-lb-uuid"},
			},
			wantUpdate: true,
			assert: func(g *WithT, captured *cloudscalesdk.FloatingIPUpdateRequest, capturedID string) {
				g.Expect(capturedID).To(Equal("1.2.3.4"))
				g.Expect(captured.LoadBalancer).To(Equal("new-lb-uuid"))
			},
		},
		{
			name: "LB disabled — reassigns to first CP server",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
			},
			fip: &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Server:  &cloudscalesdk.ServerStub{UUID: "srv-old"},
			},
			lbDisabled: true,
			cpMachine:  testutils.NewControlPlaneMachine("cp-machine-0", "srv-new"),
			wantUpdate: true,
			assert: func(g *WithT, captured *cloudscalesdk.FloatingIPUpdateRequest, _ string) {
				g.Expect(captured.Server).To(Equal("srv-new"))
				g.Expect(captured.LoadBalancer).To(BeEmpty())
			},
		},
		{
			name: "Update error surfaces",
			setup: func(cs *scope.ClusterScope) {
				cs.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
				cs.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
			},
			fip: &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				// No LB assigned ⇒ needs update
			},
			updateErr:  fmt.Errorf("update failed"),
			wantErrSub: "updating floating IP assignment",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var captured *cloudscalesdk.FloatingIPUpdateRequest
			var capturedID string

			fipService := &testutils.MockFloatingIPService{
				UpdateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
					if tc.updateErr == nil && !tc.wantUpdate {
						g.Fail("Update should not be called when assignment is correct")
					}
					capturedID = id
					captured = req
					return tc.updateErr
				},
			}

			clusterScope := newFIPTestClusterScope(fipService)
			if tc.lbDisabled {
				clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(false)
			}
			tc.setup(clusterScope)

			objs := []client.Object{}
			if tc.cpMachine != nil {
				objs = append(objs, tc.cpMachine)
			}
			r := newTestReconciler(objs...)

			err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, tc.fip)
			if tc.wantErrSub != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.wantErrSub))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tc.assert != nil {
				g.Expect(captured).ToNot(BeNil())
				tc.assert(g, captured, capturedID)
			}
		})
	}
}

// --- deleteFloatingIP tests ---

func TestDeleteFloatingIP(t *testing.T) {
	cases := []struct {
		name            string
		spec            *infrastructurev1beta2.FloatingIPSpec
		statusFIP       string
		deleteErr       error
		wantDelete      bool
		wantClearStatus bool
		wantErrSub      string
		// Condition assertion: assertCondNil pins "must not be set" (pre-existing path).
		// wantCondStatus pins a specific Status/Reason (error path). If both are
		// zero, the condition is not asserted on (managed-delete happy paths set
		// FloatingIPDeleting via the defer, which we don't need to re-assert here).
		assertCondNil  bool
		wantCondStatus *metav1.ConditionStatus
		wantCondReason string
	}{
		{
			name: "nil spec — no-op",
		},
		{
			// Pre-existing FIPs are not deleted, and the condition is not set
			// (the defer that sets it is skipped for pre-existing IPs to avoid
			// falsely reporting "Floating IP has been deleted").
			name:          "pre-existing FIP skips deletion and leaves condition untouched",
			spec:          &infrastructurev1beta2.FloatingIPSpec{Address: "9.9.9.9"},
			assertCondNil: true,
		},
		{
			name:            "managed FIP deletes and clears status",
			spec:            &infrastructurev1beta2.FloatingIPSpec{},
			statusFIP:       "1.2.3.4",
			wantDelete:      true,
			wantClearStatus: true,
		},
		{
			name: "managed FIP with empty status skips deletion",
			spec: &infrastructurev1beta2.FloatingIPSpec{},
		},
		{
			name:            "already-deleted (404) is idempotent",
			spec:            &infrastructurev1beta2.FloatingIPSpec{},
			statusFIP:       "1.2.3.4",
			deleteErr:       &cloudscalesdk.ErrorResponse{StatusCode: 404},
			wantDelete:      true,
			wantClearStatus: true,
		},
		{
			name:           "delete error surfaces and sets FloatingIPReadyCondition=False",
			spec:           &infrastructurev1beta2.FloatingIPSpec{},
			statusFIP:      "1.2.3.4",
			deleteErr:      fmt.Errorf("api error"),
			wantDelete:     true,
			wantErrSub:     "deleting floating IP",
			wantCondStatus: new(metav1.ConditionFalse),
			wantCondReason: infrastructurev1beta2.FloatingIPErrorReason,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			var deletedID string

			fipService := &testutils.MockFloatingIPService{
				DeleteFn: func(ctx context.Context, id string) error {
					if !tc.wantDelete {
						g.Fail("Delete should not be called for this case")
					}
					deletedID = id
					return tc.deleteErr
				},
			}

			clusterScope := newFIPTestClusterScope(fipService)
			clusterScope.CloudscaleCluster.Spec.FloatingIP = tc.spec
			clusterScope.CloudscaleCluster.Status.FloatingIP = tc.statusFIP

			err := newTestReconciler().deleteFloatingIP(context.Background(), clusterScope)

			if tc.wantErrSub != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.wantErrSub))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.wantDelete {
				g.Expect(deletedID).To(Equal(tc.statusFIP))
			}
			if tc.wantClearStatus {
				g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(BeEmpty())
			}

			cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
			switch {
			case tc.assertCondNil:
				g.Expect(cond).To(BeNil())
			case tc.wantCondStatus != nil:
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.Status).To(Equal(*tc.wantCondStatus))
				if tc.wantCondReason != "" {
					g.Expect(cond.Reason).To(Equal(tc.wantCondReason))
				}
			}
		})
	}
}

// --- Timeout handling tests for Create() calls ---

func TestReconcileManagedFloatingIP_CreateTimeoutRequeues(t *testing.T) {
	g := NewWithT(t)

	fipService := &testutils.MockFloatingIPService{
		ListFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		CreateFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			// Simulate timeout via context deadline exceeded wrapped in url.Error
			return nil, &url.Error{Op: "Post", URL: "https://api.example.com/v1/floating_ips", Err: os.ErrDeadlineExceeded}
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newTestReconciler()

	result, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(createFloatingIPTimeoutRequeueAfter),
		"Should requeue after createFloatingIPTimeoutRequeueAfter on timeout error")
}

// --- Mock FloatingIPService ---
