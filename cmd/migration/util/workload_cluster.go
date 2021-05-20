/*
Copyright 2021 The Kubernetes Authors.
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

package util

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func migrateWorkloadCluster(
	ctx context.Context,
	clusterKey client.ObjectKey,
	mgmtClient client.Client,
	stdout io.Writer,
) error {
	c, err := remote.NewClusterClient(ctx, mgmtClient, clusterKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	nodeList := &corev1.NodeList{} //nolint:exhaustivestruct
	if err := c.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, n := range nodeList.Items {
		// TODO: should this stop at first error, or attempt to continue?
		// TODO: should probably give some additional safety to users since this will be
		// deleting and re-creating Node resources
		if err := migrateNode(ctx, n, c, stdout); err != nil {
			return err
		}
	}

	return nil
}
