/*
Copyright 2020 The Kubernetes Authors.

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

// Package scope contains the scope interfaces for the Packet provider.
package scope

import (
	"context"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	Client        client.Client
	Cluster       *clusterv1.Cluster
	PacketCluster *infrav1.PacketCluster
}

// NewClusterScope creates a new ClusterScope from the supplied parameters.
// This is meant to be called for each reconcile iteration only on PacketClusterReconciler.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.Cluster == nil {
		return nil, errors.New("Cluster is required when creating a ClusterScope")
	}
	if params.PacketCluster == nil {
		return nil, errors.New("PacketCluster is required when creating a ClusterScope")
	}

	helper, err := patch.NewHelper(params.PacketCluster, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}

	return &ClusterScope{
		client:        params.Client,
		Cluster:       params.Cluster,
		PacketCluster: params.PacketCluster,
		patchHelper:   helper,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	client      client.Client
	patchHelper *patch.Helper

	Cluster       *clusterv1.Cluster
	PacketCluster *infrav1.PacketCluster
}

// Close closes the current scope persisting the cluster configuration and status.
func (s *ClusterScope) Close(ctx context.Context) error {
	return s.patchHelper.Patch(ctx, s.PacketCluster)
}

// Name returns the cluster name.
func (s *ClusterScope) Name() string {
	return s.Cluster.GetName()
}

// Namespace returns the cluster namespace.
func (s *ClusterScope) Namespace() string {
	return s.Cluster.GetNamespace()
}

// SetReady sets the PacketCluster Ready Status.
func (s *ClusterScope) SetReady() {
	s.PacketCluster.Status.Ready = true
}
