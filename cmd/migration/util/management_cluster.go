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
	"bytes"
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetManagementClient(kubeconfig string) (client.Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{} //nolint:exhaustivestruct
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client configuration for management cluster: %w", err)
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	return client.New(config, client.Options{Scheme: scheme}) //nolint:exhaustivestruct,wrapcheck
}

func RunMigration(
	ctx context.Context,
	mgmtClient client.Client,
	clusters []clusterv1.Cluster,
	wg *sync.WaitGroup,
) (*OutputBuffers, *ErrorCollection) {
	migrationOutputBuffers := NewOutputBuffers(len(clusters))
	migrationErrors := NewErrorCollection(len(clusters))

	for i, c := range clusters {
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)

		clusterKey, err := client.ObjectKeyFromObject(&clusters[i])
		if err != nil {
			migrationErrors.Store(outputKey, fmt.Errorf("failed to create object key: %w", err))

			continue
		}

		var buf bytes.Buffer

		migrationOutputBuffers.Store(outputKey, &buf)
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := migrateWorkloadCluster(context.TODO(), clusterKey, mgmtClient, &buf); err != nil {
				migrationErrors.Store(outputKey, err)

				return
			}
		}()
	}

	return migrationOutputBuffers, migrationErrors
}
