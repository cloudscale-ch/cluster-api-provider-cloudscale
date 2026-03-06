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
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

func TestCloudscaleClusterReconciler_ResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	controllerReconciler := &CloudscaleClusterReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	_, err := controllerReconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	g.Expect(err).NotTo(HaveOccurred())
}

func TestCloudscaleClusterReconciler_NoOwnerCluster(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	resource := &infrastructurev1beta2.CloudscaleCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-owner-cluster",
			Namespace: "default",
		},
		Spec: infrastructurev1beta2.CloudscaleClusterSpec{
			Region: "rma",
			CredentialsRef: infrastructurev1beta2.CloudscaleCredentialsReference{
				Name: "cloudscale-credentials",
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	defer func() {
		g.Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
	}()

	controllerReconciler := &CloudscaleClusterReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: resource.Name, Namespace: resource.Namespace},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
}

func TestCloudscaleClusterReconciler_IsInfrastructureProvisioned_LBEnabledAllResources(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "1.2.3.4",
					Port: 6443,
				},
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled: ptr.To(true),
				},
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				NetworkID:              "network-123",
				SubnetID:               "subnet-123",
				LoadBalancerID:         "lb-123",
				LoadBalancerPoolID:     "pool-123",
				LoadBalancerListenerID: "listener-123",
			},
		},
	}

	g.Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeTrue())
}

func TestCloudscaleClusterReconciler_IsInfrastructureProvisioned_LBEnabledMissingResources(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "1.2.3.4",
					Port: 6443,
				},
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled: ptr.To(true),
				},
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				NetworkID: "network-123",
				SubnetID:  "subnet-123",
				// LB resources missing
			},
		},
	}

	g.Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeFalse())
}

func TestCloudscaleClusterReconciler_IsInfrastructureProvisioned_LBDisabledExternalEndpoint(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "external-controlplane.example.com",
					Port: 6443,
				},
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled: ptr.To(false), // LB disabled for external control plane
				},
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				NetworkID: "network-123",
				SubnetID:  "subnet-123",
				// No LB resources needed
			},
		},
	}

	g.Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeTrue())
}

func TestCloudscaleClusterReconciler_IsInfrastructureProvisioned_LBDisabledNoEndpoint(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			Spec: infrastructurev1beta2.CloudscaleClusterSpec{
				ControlPlaneLoadBalancer: infrastructurev1beta2.LoadBalancerSpec{
					Enabled: ptr.To(false), // LB disabled for external control plane
				},
				// ControlPlaneEndpoint not set
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				NetworkID: "network-123",
				SubnetID:  "subnet-123",
			},
		},
	}

	g.Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeFalse())
}

func TestCloudscaleClusterReconciler_SetReadyCondition_AllTrue(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  "default",
				Generation: 1,
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:               infrastructurev1beta2.NetworkReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             "Provisioned",
						ObservedGeneration: 1,
					},
					{
						Type:               infrastructurev1beta2.LoadBalancerReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             "Provisioned",
						ObservedGeneration: 1,
					},
				},
			},
		},
	}

	reconciler.setReadyCondition(clusterScope)

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).NotTo(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.ReadyReason))
}

func TestCloudscaleClusterReconciler_SetReadyCondition_NetworkFalse(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  "default",
				Generation: 1,
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:               infrastructurev1beta2.NetworkReadyCondition,
						Status:             metav1.ConditionFalse,
						Reason:             "Provisioning",
						Message:            "Network is being provisioned",
						ObservedGeneration: 1,
					},
					{
						Type:               infrastructurev1beta2.LoadBalancerReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             "Provisioned",
						ObservedGeneration: 1,
					},
				},
			},
		},
	}

	reconciler.setReadyCondition(clusterScope)

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).NotTo(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal("Provisioning"))
	g.Expect(readyCond.Message).To(Equal("Network is being provisioned"))
}

func TestCloudscaleClusterReconciler_SetReadyCondition_LoadBalancerFalse(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  "default",
				Generation: 1,
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:               infrastructurev1beta2.NetworkReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             "Provisioned",
						ObservedGeneration: 1,
					},
					{
						Type:               infrastructurev1beta2.LoadBalancerReadyCondition,
						Status:             metav1.ConditionFalse,
						Reason:             infrastructurev1beta2.LoadBalancerNotReadyReason,
						Message:            "Load balancer is not running",
						ObservedGeneration: 1,
					},
				},
			},
		},
	}

	reconciler.setReadyCondition(clusterScope)

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).NotTo(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.LoadBalancerNotReadyReason))
	g.Expect(readyCond.Message).To(Equal("Load balancer is not running"))
}

func TestCloudscaleClusterReconciler_SetReadyCondition_MissingConditions(t *testing.T) {
	g := NewWithT(t)
	reconciler := &CloudscaleClusterReconciler{}

	clusterScope := &scope.ClusterScope{
		CloudscaleCluster: &infrastructurev1beta2.CloudscaleCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  "default",
				Generation: 1,
			},
			Status: infrastructurev1beta2.CloudscaleClusterStatus{
				// No conditions set at all
			},
		},
	}

	reconciler.setReadyCondition(clusterScope)

	readyCond := conditions.Get(clusterScope.CloudscaleCluster, infrastructurev1beta2.ReadyCondition)
	g.Expect(readyCond).NotTo(BeNil())
	g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.NotReadyReason))
	g.Expect(readyCond.Message).To(ContainSubstring("Waiting for"))
}
