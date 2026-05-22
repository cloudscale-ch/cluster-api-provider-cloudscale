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

package scope

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// MachineScopeParams defines the input parameters used to create a new MachineScope.
type MachineScopeParams struct {
	Client            client.Client
	Logger            logr.Logger
	Cluster           *clusterv1.Cluster
	Machine           *clusterv1.Machine
	CloudscaleCluster *infrastructurev1beta2.CloudscaleCluster
	CloudscaleMachine *infrastructurev1beta2.CloudscaleMachine
	CloudscaleClient  *cloudscale.Client
}

// MachineScope defines the basic context for a reconciler acting on a CloudscaleMachine.
type MachineScope struct {
	logr.Logger
	client            client.Client
	patchHelper       *patch.Helper
	Cluster           *clusterv1.Cluster
	Machine           *clusterv1.Machine
	CloudscaleCluster *infrastructurev1beta2.CloudscaleCluster
	CloudscaleMachine *infrastructurev1beta2.CloudscaleMachine
	CloudscaleClient  *cloudscale.Client
}

// NewMachineScope creates a new MachineScope from the given parameters.
func NewMachineScope(params MachineScopeParams) (*MachineScope, error) {
	if params.Client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if params.Cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if params.Machine == nil {
		return nil, fmt.Errorf("machine is required")
	}
	if params.CloudscaleCluster == nil {
		return nil, fmt.Errorf("cloudscaleCluster is required")
	}
	if params.CloudscaleMachine == nil {
		return nil, fmt.Errorf("cloudscaleMachine is required")
	}
	if params.CloudscaleClient == nil {
		return nil, fmt.Errorf("cloudscaleClient is required")
	}

	helper, err := patch.NewHelper(params.CloudscaleMachine, params.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to init patch helper: %w", err)
	}

	return &MachineScope{
		Logger:            params.Logger,
		client:            params.Client,
		patchHelper:       helper,
		Cluster:           params.Cluster,
		Machine:           params.Machine,
		CloudscaleCluster: params.CloudscaleCluster,
		CloudscaleMachine: params.CloudscaleMachine,
		CloudscaleClient:  params.CloudscaleClient,
	}, nil
}

// Close persists the CloudscaleMachine status and spec changes.
func (s *MachineScope) Close(ctx context.Context) error {
	return s.patchHelper.Patch(ctx, s.CloudscaleMachine)
}

// Name returns the machine name.
func (s *MachineScope) Name() string {
	return s.CloudscaleMachine.GetName()
}

// Namespace returns the machine namespace.
func (s *MachineScope) Namespace() string {
	return s.CloudscaleMachine.GetNamespace()
}

// IsControlPlane returns true if the machine is a control plane node.
func (s *MachineScope) IsControlPlane() bool {
	_, ok := s.Machine.Labels[clusterv1.MachineControlPlaneLabel]
	return ok
}

// GetBootstrapData returns the bootstrap data from the Machine's bootstrap secret.
func (s *MachineScope) GetBootstrapData(ctx context.Context) (string, error) {
	if s.Machine.Spec.Bootstrap.DataSecretName == nil {
		return "", fmt.Errorf("bootstrap data secret name is nil")
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: s.Machine.Namespace,
		Name:      *s.Machine.Spec.Bootstrap.DataSecretName,
	}
	if err := s.client.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("getting bootstrap data secret: %w", err)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return "", fmt.Errorf("bootstrap data secret missing 'value' key")
	}

	return string(value), nil
}

// GetProviderID returns the provider ID for the server.
func (s *MachineScope) GetProviderID() string {
	if s.CloudscaleMachine.Spec.ProviderID != nil {
		return *s.CloudscaleMachine.Spec.ProviderID
	}
	return ""
}

// SetProviderID sets the provider ID for the server.
func (s *MachineScope) SetProviderID(serverID string) {
	s.CloudscaleMachine.Spec.ProviderID = new(fmt.Sprintf("cloudscale://%s", serverID))
}
