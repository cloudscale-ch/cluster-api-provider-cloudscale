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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

// TestCloudscaleClusterReconciler_Reconcile_EntryPoint covers the cheap
// early-exit paths of Reconcile against the envtest API server.
func TestCloudscaleClusterReconciler_Reconcile_EntryPoint(t *testing.T) {
	t.Run("ResourceNotFound", func(t *testing.T) {
		g := NewWithT(t)
		r := &CloudscaleClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		_, err := r.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
		})
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("NoOwnerCluster", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.Background()

		resource := &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "no-owner-cluster", Namespace: "default"},
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				Region:         "rma",
				CredentialsRef: infrastructurev1beta2.CloudscaleCredentialsReference{Name: "cloudscale-credentials"},
			},
		}
		g.Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		defer func() {
			g.Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}()

		r := &CloudscaleClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		result, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace},
		})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.IsZero()).To(BeTrue())
	})
}

func TestReconcileControlPlaneEndpoint_FromLBVIP(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope() // LB enabled, no floating IP
	r := newTestReconciler()

	r.reconcileControlPlaneEndpoint(clusterScope, "203.0.113.10")

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("203.0.113.10"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
}

func TestReconcileControlPlaneEndpoint_FloatingIPTakesPrecedenceOverVIP(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope()
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
	clusterScope.CloudscaleCluster.Status.FloatingIP = "10.20.30.40"
	r := newTestReconciler()

	r.reconcileControlPlaneEndpoint(clusterScope, "203.0.113.10")

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("10.20.30.40"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
}

func TestReconcileControlPlaneEndpoint_SkipsIfAlreadySet(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope()
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host = "existing-host"
	clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port = 9999
	r := newTestReconciler()

	r.reconcileControlPlaneEndpoint(clusterScope, "203.0.113.10")

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("existing-host"))
	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(9999)))
}

func TestReconcileControlPlaneEndpoint_WaitsWhenVIPMissing(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope() // LB enabled, no floating IP
	r := newTestReconciler()

	r.reconcileControlPlaneEndpoint(clusterScope, "")

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(BeEmpty())
}

func TestReconcileControlPlaneEndpoint_WaitsWhenFloatingIPNotProvisioned(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope()
	clusterScope.CloudscaleCluster.Spec.FloatingIP = &infrastructurev1beta2.FloatingIPSpec{}
	// Status.FloatingIP is empty: the FIP is configured but not provisioned yet.
	r := newTestReconciler()

	r.reconcileControlPlaneEndpoint(clusterScope, "203.0.113.10")

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(BeEmpty(),
		"must not fall back to the VIP when a floating IP is configured but not yet provisioned")
}

func TestReconcileControlPlaneEndpoint_ExternalControlPlaneNoSource(t *testing.T) {
	g := NewWithT(t)

	clusterScope := newLBClusterScope()
	clusterScope.CloudscaleCluster.Spec.ControlPlaneLoadBalancer.Enabled = new(false) // external CP, no LB, no FIP
	r := newTestReconciler()

	r.reconcileControlPlaneEndpoint(clusterScope, "")

	g.Expect(clusterScope.CloudscaleCluster.Spec.ControlPlaneEndpoint.Host).To(BeEmpty())
}

// TestIsInfrastructureProvisioned exercises the readiness predicate used by
// reconcileNormal to decide when to flip Status.Initialization.Provisioned.
func TestIsInfrastructureProvisioned(t *testing.T) {
	cases := []struct {
		name             string
		lbEnabled        bool
		networks         []infrastructurev1beta2.NetworkStatus
		endpointHost     string
		endpointPort     int32
		lbID             string
		lbPoolID         string
		lbListenerID     string
		floatingIP       *infrastructurev1beta2.FloatingIPSpec
		statusFloatingIP string
		wantProvisioned  bool
	}{
		{
			name:            "LB enabled and all resources present",
			lbEnabled:       true,
			networks:        []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "network-123", SubnetID: "subnet-123", Managed: true}},
			endpointHost:    "1.2.3.4",
			endpointPort:    6443,
			lbID:            "lb-123",
			lbPoolID:        "pool-123",
			lbListenerID:    "listener-123",
			wantProvisioned: true,
		},
		{
			name:         "LB enabled but LB resources missing",
			lbEnabled:    true,
			networks:     []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "network-123", SubnetID: "subnet-123", Managed: true}},
			endpointHost: "1.2.3.4",
			endpointPort: 6443,
		},
		{
			name:            "LB disabled with external endpoint",
			lbEnabled:       false,
			networks:        []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "network-123", SubnetID: "subnet-123", Managed: true}},
			endpointHost:    "external-controlplane.example.com",
			endpointPort:    6443,
			wantProvisioned: true,
		},
		{
			name:      "LB disabled without endpoint not provisioned",
			lbEnabled: false,
			networks:  []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "network-123", SubnetID: "subnet-123", Managed: true}},
		},
		{
			name:             "Floating IP requested but status floating IP empty",
			lbEnabled:        false,
			networks:         []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "network-123", SubnetID: "subnet-123", Managed: true}},
			endpointHost:     "1.2.3.4",
			endpointPort:     6443,
			floatingIP:       &infrastructurev1beta2.FloatingIPSpec{Address: "203.0.113.10"},
			statusFloatingIP: "",
			wantProvisioned:  false,
		},
		{
			name:             "Floating IP requested and status floating IP set",
			lbEnabled:        false,
			networks:         []infrastructurev1beta2.NetworkStatus{{Name: "test", NetworkID: "network-123", SubnetID: "subnet-123", Managed: true}},
			endpointHost:     "1.2.3.4",
			endpointPort:     6443,
			floatingIP:       &infrastructurev1beta2.FloatingIPSpec{Address: "185.0.0.0"},
			statusFloatingIP: "185.0.0.0",
			wantProvisioned:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			clusterScope := &scope.ClusterScope{
				CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
					Spec: infrastructurev1beta2.CloudscaleClusterSpec{
						ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: tc.endpointHost, Port: tc.endpointPort},
						ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
							Enabled: new(tc.lbEnabled),
						},
						FloatingIP: tc.floatingIP,
					},
					Status: infrastructurev1beta2.CloudscaleClusterStatus{
						Networks:               tc.networks,
						LoadBalancerID:         tc.lbID,
						LoadBalancerPoolID:     tc.lbPoolID,
						LoadBalancerListenerID: tc.lbListenerID,
						FloatingIP:             tc.statusFloatingIP,
					},
				},
			}

			g.Expect((&CloudscaleClusterReconciler{}).isInfrastructureProvisioned(clusterScope)).To(Equal(tc.wantProvisioned))
		})
	}
}

// TestSetReadyCondition exercises the rollup of sub-conditions into the
// top-level ReadyCondition.
func TestSetReadyCondition(t *testing.T) {
	cases := []struct {
		name              string
		subConditions     []metav1.Condition
		expectStatus      metav1.ConditionStatus
		expectReason      string
		expectMsgContains string
	}{
		{
			name: "all sub-conditions True yields Ready=True",
			subConditions: []metav1.Condition{
				{Type: infrastructurev1beta2.NetworkReadyCondition, Status: metav1.ConditionTrue, Reason: "Provisioned", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.RouterReadyCondition, Status: metav1.ConditionTrue, Reason: "RouterDisabled", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.LoadBalancerReadyCondition, Status: metav1.ConditionTrue, Reason: "Provisioned", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.FloatingIPReadyCondition, Status: metav1.ConditionTrue, Reason: "Provisioned", ObservedGeneration: 1},
			},
			expectStatus: metav1.ConditionTrue,
			expectReason: infrastructurev1beta2.ReadyReason,
		},
		{
			name: "Network=False propagates reason and message",
			subConditions: []metav1.Condition{
				{Type: infrastructurev1beta2.NetworkReadyCondition, Status: metav1.ConditionFalse, Reason: "Provisioning", Message: "Network is being provisioned", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.RouterReadyCondition, Status: metav1.ConditionTrue, Reason: "RouterDisabled", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.LoadBalancerReadyCondition, Status: metav1.ConditionTrue, Reason: "Provisioned", ObservedGeneration: 1},
			},
			expectStatus:      metav1.ConditionFalse,
			expectReason:      "Provisioning",
			expectMsgContains: "Network is being provisioned",
		},
		{
			name: "LoadBalancer=False propagates reason and message",
			subConditions: []metav1.Condition{
				{Type: infrastructurev1beta2.NetworkReadyCondition, Status: metav1.ConditionTrue, Reason: "Provisioned", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.RouterReadyCondition, Status: metav1.ConditionTrue, Reason: "RouterDisabled", ObservedGeneration: 1},
				{Type: infrastructurev1beta2.LoadBalancerReadyCondition, Status: metav1.ConditionFalse, Reason: infrastructurev1beta2.LoadBalancerNotReadyReason, Message: "Load balancer is not running", ObservedGeneration: 1},
			},
			expectStatus:      metav1.ConditionFalse,
			expectReason:      infrastructurev1beta2.LoadBalancerNotReadyReason,
			expectMsgContains: "Load balancer is not running",
		},
		{
			name:              "no sub-conditions yields Ready=False/NotReady",
			subConditions:     nil,
			expectStatus:      metav1.ConditionFalse,
			expectReason:      infrastructurev1beta2.NotReadyReason,
			expectMsgContains: "Waiting for",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			clusterScope := &scope.ClusterScope{
				CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default", Generation: 1},
					Status:     infrastructurev1beta2.CloudscaleClusterStatus{Conditions: tc.subConditions},
				},
			}

			(&CloudscaleClusterReconciler{}).setReadyCondition(clusterScope)

			readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
			g.Expect(readyCond).NotTo(BeNil())
			g.Expect(readyCond.Status).To(Equal(tc.expectStatus))
			g.Expect(readyCond.Reason).To(Equal(tc.expectReason))
			if tc.expectMsgContains != "" {
				g.Expect(readyCond.Message).To(ContainSubstring(tc.expectMsgContains))
			}
		})
	}
}
