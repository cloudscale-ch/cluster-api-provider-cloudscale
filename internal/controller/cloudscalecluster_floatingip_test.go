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

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPDisabledReason))
}

func TestReconcileFloatingIP_PreExisting(t *testing.T) {
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
		Address: "1.2.3.4",
	}

	r := newFIPTestReconciler()

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

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("api error")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	r := newFIPTestReconciler()

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

// --- reconcilePreExistingFloatingIP tests ---

func TestReconcilePreExistingFloatingIP_RefetchesAndKeepsAssignmentWhenCached(t *testing.T) {
	g := NewWithT(t)

	getCalled := false
	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			getCalled = true
			return &cloudscalesdk.FloatingIP{
				Network:      "1.2.3.4/32",
				LoadBalancer: &cloudscalesdk.LoadBalancerStub{UUID: "lb-uuid"},
			}, nil
		},
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			t.Fatal("Update must not fire when FIP is already assigned to the LB")
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = "1.2.3.4"

	r := newFIPTestReconciler()

	err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(getCalled).To(BeTrue(), "Pre-existing FIP must be refetched so the assignment can be verified")
}

func TestReconcilePreExistingFloatingIP_FetchesAndSetsStatus(t *testing.T) {
	g := NewWithT(t)

	var capturedUpdateID string
	var capturedUpdateReq *cloudscalesdk.FloatingIPUpdateRequest
	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			g.Expect(id).To(Equal("7.7.7.7"))
			return &cloudscalesdk.FloatingIP{
				Network: "5.6.7.8/32",
			}, nil
		},
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			capturedUpdateID = id
			capturedUpdateReq = req
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-x"
	r := newFIPTestReconciler()

	err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("5.6.7.8"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("5.6.7.8"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
	g.Expect(capturedUpdateID).To(Equal("5.6.7.8"), "Pre-existing FIP must be assigned to the LB via Update")
	g.Expect(capturedUpdateReq).ToNot(BeNil())
	g.Expect(capturedUpdateReq.LoadBalancer).To(Equal("lb-x"))
}

func TestReconcilePreExistingFloatingIP_GetError(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	r := newFIPTestReconciler()

	err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("getting pre-existing floating IP"))
}

func TestReconcilePreExistingFloatingIP_RegionMismatchErrors(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Region:  &cloudscalesdk.RegionStub{Slug: "lpg"},
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}

	r := newFIPTestReconciler()

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("lpg"))
	g.Expect(err.Error()).To(ContainSubstring("rma"))
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(BeEmpty())
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

func TestReconcilePreExistingFloatingIP_NoPublicInterfaceSetsConditionAndEvent(t *testing.T) {
	g := NewWithT(t)
	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Region:  &cloudscalesdk.RegionStub{Slug: "rma"},
			}, nil
		},
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			return &cloudscalesdk.ErrorResponse{
				StatusCode: 400,
				Message: map[string]string{
					"detail": "This server does not have a public interface with an IPv4 address.",
				},
			}
		},
	}
	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "1.2.3.4",
	}
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"
	cpMachine := &infrastructurev1beta2.CloudscaleMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp-0",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Status: infrastructurev1beta2.CloudscaleMachineStatus{
			ServerID: "server-uuid",
		},
	}
	r := newFIPTestReconciler(cpMachine)

	_, err := r.reconcileFloatingIP(context.Background(), clusterScope)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("public interface"))
	g.Expect(err.Error()).To(ContainSubstring("control-plane machine template"))
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(infrastructurev1beta2.FloatingIPErrorReason))
}

func TestReconcilePreExistingFloatingIP_RegionMatches(t *testing.T) {
	g := NewWithT(t)

	var capturedUpdateReq *cloudscalesdk.FloatingIPUpdateRequest
	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
				Region:  &cloudscalesdk.RegionStub{Slug: "rma"},
			}, nil
		},
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			capturedUpdateReq = req
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-rma"
	r := newFIPTestReconciler()

	err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("1.2.3.4"))
	g.Expect(capturedUpdateReq).ToNot(BeNil())
	g.Expect(capturedUpdateReq.LoadBalancer).To(Equal("lb-rma"))
}

func TestReconcilePreExistingFloatingIP_AssignsToFirstReadyCPServer(t *testing.T) {
	g := NewWithT(t)

	var capturedUpdateReq *cloudscalesdk.FloatingIPUpdateRequest
	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "1.2.3.4/32",
			}, nil
		},
		updateFn: func(ctx context.Context, id string, req *cloudscalesdk.FloatingIPUpdateRequest) error {
			capturedUpdateReq = req
			return nil
		},
	}

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
			ServerID: "srv-x",
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)
	r := newFIPTestReconciler(cpMachine)

	err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedUpdateReq).ToNot(BeNil(), "Pre-existing FIP must be assigned to the CP server when LB is disabled")
	g.Expect(capturedUpdateReq.Server).To(Equal("srv-x"))
}

func TestReconcilePreExistingFloatingIP_GlobalFIPAccepted(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		getFn: func(ctx context.Context, id string) (*cloudscalesdk.FloatingIP, error) {
			return &cloudscalesdk.FloatingIP{
				Network: "9.9.9.9/32",
				Region:  nil,
			}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	r := newFIPTestReconciler()

	err := r.reconcilePreExistingFloatingIP(context.Background(), clusterScope, "7.7.7.7")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterScope.CloudscaleCluster.Status.FloatingIP).To(Equal("9.9.9.9"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("9.9.9.9"))
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

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

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

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

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

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

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

	_, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

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
			ServerID: "srv-new",
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = ptr.To(false)
	clusterScope.CloudscaleCluster.Status.FloatingIP = "1.2.3.4"

	r := newFIPTestReconciler(cpMachine)

	// FIP assigned to an old server
	fip := &cloudscalesdk.FloatingIP{
		Network: "1.2.3.4/32",
		Server:  &cloudscalesdk.ServerStub{UUID: "srv-old"},
	}

	err := r.ensureFloatingIPAssignment(context.Background(), clusterScope, fip)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(capturedReq).ToNot(BeNil())
	g.Expect(capturedReq.Server).To(Equal("srv-new"))
	g.Expect(capturedReq.LoadBalancer).To(BeEmpty())
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

func TestDeleteFloatingIP_PreExistingSkipsDeletionAndLeavesConditionUntouched(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		deleteFn: func(ctx context.Context, id string) error {
			t.Fatal("Delete should not be called for pre-existing floating IPs")
			return nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{
		Address: "9.9.9.9",
	}

	r := newFIPTestReconciler()

	err := r.deleteFloatingIP(context.Background(), clusterScope)

	g.Expect(err).ToNot(HaveOccurred())
	// Pre-Existing FIPs are not deleted, and no condition should be set (the defer is
	// not registered for pre-existing so that the condition does not falsely report
	// "Floating IP has been deleted").
	cond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.FloatingIPReadyCondition)
	g.Expect(cond).To(BeNil())
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

// --- Timeout handling tests for Create() calls ---

func TestReconcileManagedFloatingIP_CreateTimeoutRequeues(t *testing.T) {
	g := NewWithT(t)

	fipService := &mockFloatingIPService{
		listFn: func(ctx context.Context, modifiers ...cloudscalesdk.ListRequestModifier) ([]cloudscalesdk.FloatingIP, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, req *cloudscalesdk.FloatingIPCreateRequest) (*cloudscalesdk.FloatingIP, error) {
			if req.IPVersion == 4 {
				// Simulate timeout via context deadline exceeded wrapped in url.Error
				return nil, &url.Error{Op: "Post", URL: "https://api.example.com/v1/floating_ips", Err: os.ErrDeadlineExceeded}
			}
			// IPv6 path: return success so we can verify timeout path above is hit first
			return &cloudscalesdk.FloatingIP{Network: "2001:db8::1/128"}, nil
		},
	}

	clusterScope := newFIPTestClusterScope(fipService)
	clusterScope.CloudscaleCluster.Status.LoadBalancerID = "lb-uuid"
	fipSpec := &infrastructurev1beta2.FloatingIPSpec{}

	r := newFIPTestReconciler()

	result, err := r.reconcileManagedFloatingIP(context.Background(), clusterScope, fipSpec)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.RequeueAfter).To(Equal(CreateTimeoutRequeueInterval),
		"Should requeue after CreateTimeoutRequeueInterval on timeout error")
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
