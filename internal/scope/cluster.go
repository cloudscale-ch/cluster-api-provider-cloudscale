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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta2 "github.com/cloudscale-ch/cluster-api-provider-cloudscale/api/v1beta2"
	"github.com/cloudscale-ch/cluster-api-provider-cloudscale/internal/cloudscale"
)

// ClusterScopeParams defines the input parameters used to create a new ClusterScope.
type ClusterScopeParams struct {
	Client            client.Client
	Logger            logr.Logger
	Cluster           *clusterv1.Cluster
	CloudscaleCluster *infrastructurev1beta2.CloudscaleCluster
	CloudscaleClient  *cloudscale.Client
}

// ClusterScope defines the basic context for a reconciler acting on a CloudscaleCluster.
type ClusterScope struct {
	logr.Logger
	client            client.Client
	patchHelper       *patch.Helper
	Cluster           *clusterv1.Cluster
	CloudscaleCluster *infrastructurev1beta2.CloudscaleCluster
	CloudscaleClient  *cloudscale.Client
}

// NewClusterScope creates a new ClusterScope from the given parameters.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.Client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if params.Cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if params.CloudscaleCluster == nil {
		return nil, fmt.Errorf("cloudscaleCluster is required")
	}
	if params.CloudscaleClient == nil {
		return nil, fmt.Errorf("cloudscaleClient is required")
	}

	helper, err := patch.NewHelper(params.CloudscaleCluster, params.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to init patch helper: %w", err)
	}

	return &ClusterScope{
		Logger:            params.Logger,
		client:            params.Client,
		patchHelper:       helper,
		Cluster:           params.Cluster,
		CloudscaleCluster: params.CloudscaleCluster,
		CloudscaleClient:  params.CloudscaleClient,
	}, nil
}

// Close persists the CloudscaleCluster status and spec changes.
func (s *ClusterScope) Close(ctx context.Context, opts ...patch.Option) error {
	return s.patchHelper.Patch(ctx, s.CloudscaleCluster, opts...)
}

// Name returns the cluster name.
func (s *ClusterScope) Name() string {
	return s.Cluster.GetName()
}

// Namespace returns the cluster namespace.
func (s *ClusterScope) Namespace() string {
	return s.Cluster.GetNamespace()
}
