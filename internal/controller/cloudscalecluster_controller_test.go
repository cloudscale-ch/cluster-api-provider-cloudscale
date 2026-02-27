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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/scope"
)

var _ = Describe("CloudscaleCluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		cloudscalecluster := &infrastructurev1beta2.CloudscaleCluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind CloudscaleCluster")
			err := k8sClient.Get(ctx, typeNamespacedName, cloudscalecluster)
			if err != nil && apierrors.IsNotFound(err) {
				resource := &infrastructurev1beta2.CloudscaleCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: infrastructurev1beta2.CloudscaleClusterSpec{
						Region: "rma",
						CredentialsRef: infrastructurev1beta2.CloudscaleCredentialsReference{
							Name: "cloudscale-credentials",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &infrastructurev1beta2.CloudscaleCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance CloudscaleCluster")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CloudscaleClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("isInfrastructureProvisioned", func() {
		var reconciler *CloudscaleClusterReconciler

		BeforeEach(func() {
			reconciler = &CloudscaleClusterReconciler{}
		})

		It("should return true when LB enabled and all resources exist", func() {
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

			Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeTrue())
		})

		It("should return false when LB enabled but LB resources missing", func() {
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

			Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeFalse())
		})

		It("should return true when LB disabled and endpoint set externally", func() {
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

			Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeTrue())
		})

		It("should return false when LB disabled but endpoint not set", func() {
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

			Expect(reconciler.isInfrastructureProvisioned(clusterScope)).To(BeFalse())
		})
	})

	Context("setReadyCondition", func() {
		var reconciler *CloudscaleClusterReconciler

		BeforeEach(func() {
			reconciler = &CloudscaleClusterReconciler{}
		})

		It("should set Ready=True when all sub-conditions are True", func() {
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
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.ReadyReason))
		})

		It("should set Ready=False when NetworkReady is False", func() {
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
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("Provisioning"))
			Expect(readyCond.Message).To(Equal("Network is being provisioned"))
		})

		It("should set Ready=False when LoadBalancerReady is False", func() {
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
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.LoadBalancerNotReadyReason))
			Expect(readyCond.Message).To(Equal("Load balancer is not running"))
		})

		It("should use fallback reason/message when sub-condition is missing", func() {
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
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(infrastructurev1beta2.NotReadyReason))
			Expect(readyCond.Message).To(ContainSubstring("Waiting for"))
		})
	})
})
