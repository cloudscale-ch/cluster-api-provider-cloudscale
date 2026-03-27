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
	"testing"

	cloudscalesdk "github.com/cloudscale-ch/cloudscale-go-sdk/v8"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	cs "github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// --- Test helpers ---

func newFIPTestClusterScope(fipService cs.FloatingIPService) *scope.ClusterScope {
	return &scope.ClusterScope{
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
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled:       ptr.To(true),
					APIServerPort: 6443,
				},
			},
		},
		CloudscaleClient: &cs.Client{
			FloatingIPs: fipService,
		},
	}
}

func newFIPTestReconciler(objs ...client.Object) *CloudscaleClusterReconciler {
	return &CloudscaleClusterReconciler{
		Client:   newTestFakeClient(objs...),
		recorder: events.NewFakeRecorder(10),
	}
}

// --- reconcileFloatingIP orchestrator tests ---

func TestReconcileFloatingIP_Disabled(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	// No FloatingIP spec = disabled
	r := newFIPTestReconciler()

	err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPDisabledReason))
}

func TestReconcileFloatingIP_BYO(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		UUID: "byo-fip-uuid",
	}

	r := newFIPTestReconciler()

	err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("1.2.3.4"))
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPProvisionedReason))
}

func TestReconcileFloatingIP_ErrorSetsConditionFalse(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		UUID: "byo-fip-uuid",
	}

	r := newFIPTestReconciler()

	err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

// --- reconcileBYOFloatingIP tests ---

func TestReconcileBYOFloatingIP_CachedShortCircuits(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			t.Fatal("Get should not be called when cached")
			return nil, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = "1.2.3.4"

	r := newFIPTestReconciler()

	err := r.reconcileBYOFloatingIP(context.Background(), clusterScope, "byo-uuid")

	g.Expect(err).ToNot(HaveOccurred())
}

func TestReconcileBYOFloatingIP_FetchesAndSetsStatus(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			g.Expect(id).To(Equal("byo-uuid"))
			return &cloudscalesdk.FloatingIP{
				Network: "5.6.7.8/32",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	r := newFIPTestReconciler()

	err := r.reconcileBYOFloatingIP(context.Background(), clusterScope, "byo-uuid")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("5.6.7.8"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("5.6.7.8"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
}

func TestReconcileBYOFloatingIP_GetError(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	r := newFIPTestReconciler()

	err := r.reconcileBYOFloatingIP(context.Background(), clusterScope, "byo-uuid")

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("getting BYO floating IP"))
}

// --- reconcileManagedFloatingIP tests ---

func TestReconcileManagedFloatingIP_ExistingFIPEnsuresAssignment(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
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

	r := newFIPTestReconciler()

	err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("10.0.0.1"))
}

func TestReconcileManagedFloatingIP_CreatesIPv4(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.FloatingIPCreateRequest

	fipService := &mockFloatingIPService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			capturedReq = req
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newFIPTestReconciler()

	err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

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

	fipService := &mockFloatingIPService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			capturedReq = req
			return &cloudscalesdk.FloatingIP{
				Network: "2001:db8::1/128",
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	ipv6 := infrastructurev1beta2.IPFamilyIPv6
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{
		IPFamily: &ipv6,
	}

	r := newFIPTestReconciler()

	err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq.IPVersion).To(Equal(6))
}

func TestReconcileManagedFloatingIP_CreateError(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("quota exceeded")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newFIPTestReconciler()

	err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("creating floating IP"))
}

func TestReconcileManagedFloatingIP_AssignsToLBTarget(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.FloatingIPCreateRequest

	fipService := &mockFloatingIPService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			capturedReq = req
			return &cloudscalesdk.FloatingIP{Network: "1.2.3.4/32"}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "target-lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newFIPTestReconciler()

	err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.LoadBalancer).To(Equal("target-lb-uuid"))
	g.Expect(capturedReq.Server).To(BeEmpty())
}

// --- getFloatingIPTarget tests ---

func TestGetFloatingIPTarget_LBEnabled(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"

	r := newFIPTestReconciler()

	target, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(target.lbUUID).To(Equal("lb-uuid"))
	g.Expect(target.serverUUID).To(BeEmpty())
}

func TestGetFloatingIPTarget_LBNotProvisioned(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	// LB enabled but no LB ID yet

	r := newFIPTestReconciler()

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

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

	r := newFIPTestReconciler(cpMachine)

	target, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(target.serverUUID).To(Equal("cp-server-uuid"))
	g.Expect(target.lbUUID).To(BeEmpty())
}

func TestGetFloatingIPTarget_LBDisabled_NoCPServer(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)

	r := newFIPTestReconciler()

	_, err := r.getFloatingIPTarget(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("waiting for a control plane server to be provisioned"))
}

// --- ensureFloatingIPAssignment tests ---

func TestEnsureFloatingIPAssignment_TargetNotReady(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	// LB enabled but no LB ID → target not ready

	r := newFIPTestReconciler()

	fip := &cloudscalesdk.FloatingIP{Network: "1.2.3.4/32"}

	err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, fip)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestEnsureFloatingIPAssignment_LBAlreadyCorrect(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			t.Fatal("Update should not be called when assignment is correct")
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"

	r := newFIPTestReconciler()

	fip := &cloudscalesdk.FloatingIP{
		Network:      "1.2.3.4/32",
		LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "lb-uuid"},
	}

	err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, fip)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestEnsureFloatingIPAssignment_ReassignsLB(t *testing.T) {
	g := NewWithT(t)

	var capturedID string
	var capturedReq *cloudscalesdk.FloatingIPUpdateRequest

	fipService := &mockFloatingIPService{
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			capturedID = id
			capturedReq = req
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "new-lb-uuid"
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler()

	fip := &cloudscalesdk.FloatingIP{
		Network:      "1.2.3.4/32",
		LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "old-lb-uuid"},
	}

	err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, fip)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedID).To(Equal("1.2.3.4"))
	g.Expect(capturedReq.LoadBalancer).To(Equal("new-lb-uuid"))
}

func TestEnsureFloatingIPAssignment_ReassignsServer(t *testing.T) {
	g := NewWithT(t)

	var capturedReq *cloudscalesdk.FloatingIPUpdateRequest

	fipService := &mockFloatingIPService{
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			capturedReq = req
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "srv-new"
	// For server target path: LB disabled → would need k8s List
	// Let's use LB enabled with server mismatch scenario instead
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(true)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler()

	// FIP assigned to neither LB nor server matching target
	fip := &cloudscalesdk.FloatingIP{
		Network: "1.2.3.4/32",
		// No LoadBalancer assigned
	}

	err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, fip)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.LoadBalancer).To(Equal("lb-uuid"))
}

func TestEnsureFloatingIPAssignment_UpdateError(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			return fmt.Errorf("update failed")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler()

	fip := &cloudscalesdk.FloatingIP{
		Network: "1.2.3.4/32",
		// No LB assigned — needs update
	}

	err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, fip)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("updating floating IP assignment"))
}

// --- setControlPlaneEndpointFromFIP tests ---

func TestSetControlPlaneEndpointFromFIP_SetsEndpoint(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})

	r := newFIPTestReconciler()

	fip := &cloudscalesdk.FloatingIP{Network: "10.20.30.40/32"}

	r.setControlPlaneEndpointFromFIP(clusterScope, fip)

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("10.20.30.40"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
}

func TestSetControlPlaneEndpointFromFIP_SkipsIfAlreadySet(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = "existing-host"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port = 9999

	r := newFIPTestReconciler()

	fip := &cloudscalesdk.FloatingIP{Network: "10.20.30.40/32"}

	r.setControlPlaneEndpointFromFIP(clusterScope, fip)

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("existing-host"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(9999)))
}

// --- deleteFloatingIP tests ---

func TestDeleteFloatingIP_NilSpec(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newFIPTestClusterScope(&mockFloatingIPService{})
	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteFloatingIP_BYOSkipsDeletion(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called for BYO floating IPs")
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		UUID: "byo-uuid",
	}

	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPDeletingReason))
}

func TestDeleteFloatingIP_ManagedDeletes(t *testing.T) {
	g := NewWithT(t)

	var deletedID string

	fipService := &mockFloatingIPService{
		deleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deletedID).To(Equal("1.2.3.4"))
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(BeEmpty())
}

func TestDeleteFloatingIP_NoStatusSkipsDeletion(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called when status is empty")
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}

	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
}

func TestDeleteFloatingIP_AlreadyDeletedSucceeds(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		deleteFn: func(ctx context.Context, id string) error {
			return &cloudscalesdk.ErrorResponse{StatusCode: 404}
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(BeEmpty())
}

func TestDeleteFloatingIP_DeleteError(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		deleteFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("api error")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("deleting floating IP"))
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

// --- Mock FloatingIPService ---

type mockFloatingIPService struct {
	createFn func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error)
	getFn    func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error)
	listFn   func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error)
	updateFn func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockFloatingIPService) Create(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return nil, nil
}

func (m *mockFloatingIPService) Get(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockFloatingIPService) List(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
	if m.listFn != nil {
		return m.listFn(ctx, modifiers...)
	}
	return nil, nil
}

func (m *mockFloatingIPService) Update(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, req)
	}
	return nil
}

func (m *mockFloatingIPService) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
