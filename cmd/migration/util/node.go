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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func migrateNode(ctx context.Context, node corev1.Node, workloadClient client.Client, stdout io.Writer) error {
	if strings.HasPrefix(node.Spec.ProviderID, "equinixmetal") {
		fmt.Fprintf(stdout, "✔ Node %s already has the updated providerID\n", node.Name)

		return nil
	}

	if err := workloadClient.Delete(ctx, &node); err != nil {
		return fmt.Errorf("failed to delete existing node resource: %w", err)
	}

	node.SetResourceVersion("")
	node.Spec.ProviderID = strings.Replace(node.Spec.ProviderID, "packet", "equinixmetal", 1)

	if err := retry.OnError(
		retry.DefaultRetry,
		func(err error) bool {
			return true
		},
		func() error {
			return workloadClient.Create(ctx, &node) //nolint:wrapcheck
		},
	); err != nil {
		return fmt.Errorf("failed to create replacement node resource: %w", err)
	}

	fmt.Fprintf(stdout, "✅ Node %s has been successfully migrated\n", node.Name)

	return nil
}
