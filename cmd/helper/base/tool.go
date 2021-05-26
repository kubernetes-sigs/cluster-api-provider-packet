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

package base

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"

	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/util"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ClusterToKey(c *clusterv1.Cluster) string {
	return fmt.Sprintf("%s/%s", c.Name, c.Namespace)
}

type Tool struct {
	kubeconfig      string
	MgmtClient      client.Client
	baseMutex       sync.Mutex
	clusters        []*clusterv1.Cluster
	workloadClients map[string]client.Client
	errors          map[string]error
	outputBuffers   map[string]*bytes.Buffer
	outputContents  map[string]string
}

var ErrMissingKubeConfig = errors.New("kubeconfig was nil")

func (t *Tool) GetClusters() []*clusterv1.Cluster {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	return t.clusters
}

func (t *Tool) Initialize(ctx context.Context, kubeconfig *string) error {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if kubeconfig == nil {
		return ErrMissingKubeConfig
	}

	t.kubeconfig = *kubeconfig

	mgmtClient, err := util.GetManagementClient(*kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create management cluster client: %w", err)
	}

	t.MgmtClient = mgmtClient

	clusterList := new(clusterv1.ClusterList)
	if err := mgmtClient.List(ctx, clusterList); err != nil {
		return fmt.Errorf("failed to list workload clusters in management cluster: %w", err)
	}

	size := len(clusterList.Items)
	clusters := make([]*clusterv1.Cluster, 0, size)

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		clusters = append(clusters, cluster)
	}

	t.clusters = clusters
	t.workloadClients = make(map[string]client.Client, size)
	t.errors = make(map[string]error, size)
	t.outputBuffers = make(map[string]*bytes.Buffer, size)
	t.outputContents = make(map[string]string, size)

	return nil
}

func (t *Tool) HasError(c *clusterv1.Cluster) bool {
	return t.GetErrorFor(c) != nil
}

func (t *Tool) GetErrorFor(c *clusterv1.Cluster) error {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.errors == nil {
		return nil
	}

	return t.errors[ClusterToKey(c)]
}

func (t *Tool) GetOutputFor(c *clusterv1.Cluster) string {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	t.flushBuffers()

	return t.outputContents[ClusterToKey(c)]
}

func (t *Tool) AddErrorFor(c *clusterv1.Cluster, err error) {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.errors == nil {
		t.errors = make(map[string]error)
	}

	t.errors[ClusterToKey(c)] = err
}

func (t *Tool) GetBufferFor(c *clusterv1.Cluster) *bytes.Buffer {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.outputBuffers == nil {
		t.outputBuffers = make(map[string]*bytes.Buffer)
	}

	key := ClusterToKey(c)

	if t.outputBuffers[key] == nil {
		t.outputBuffers[key] = new(bytes.Buffer)
	}

	return t.outputBuffers[key]
}

func (t *Tool) flushBuffers() {
	if t.outputBuffers == nil {
		t.outputBuffers = make(map[string]*bytes.Buffer)
	}

	if t.outputContents == nil {
		t.outputContents = make(map[string]string)
	}

	for key, buf := range t.outputBuffers {
		out, err := ioutil.ReadAll(buf)
		if err != nil {
			continue
		}

		t.outputContents[key] += string(out)
	}
}

func (t *Tool) GetWorkloadClient(
	ctx context.Context,
	cluster *clusterv1.Cluster,
) (client.Client, error) {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.workloadClients == nil {
		t.workloadClients = make(map[string]client.Client)
	}

	key := ClusterToKey(cluster)

	if _, ok := t.workloadClients[key]; !ok {
		clusterKey, err := client.ObjectKeyFromObject(cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to create object key: %w", err)
		}

		workloadClient, err := remote.NewClusterClient(ctx, t.MgmtClient, clusterKey, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client: %w", err)
		}

		t.workloadClients[key] = workloadClient
	}

	return t.workloadClients[key], nil
}
