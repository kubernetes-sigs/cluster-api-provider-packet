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

package upgrader

import (
	"context"
	"errors"
	"sync"

	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

type Upgrader struct {
	base.Tool
	mu            sync.Mutex
	clusterStatus map[string]bool
}

var ErrMissingKubeConfig = errors.New("kubeconfig was nil")

func (u *Upgrader) Initialize(ctx context.Context, kubeconfig *string) error {
	if err := u.Tool.Initialize(ctx, kubeconfig); err != nil {
		return err
	}

	u.clusterStatus = make(map[string]bool, len(u.Clusters))
	for _, c := range u.Clusters {
		u.updateClusterStatus(c, false)
	}

	return nil
}

func (u *Upgrader) CalculatePercentage() float64 {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.clusterStatus == nil {
		u.clusterStatus = make(map[string]bool)
	}

	totalClusters := len(u.Clusters)
	doneClusters := 0

	for _, cluster := range u.clusterStatus {
		if cluster {
			doneClusters++
		}
	}

	if totalClusters == 0 {
		return float64(0)
	}

	return float64(doneClusters) / float64(totalClusters)
}

func (u *Upgrader) CheckPrerequisites(ctx context.Context) error {
	return nil
}

func (u *Upgrader) Run(ctx context.Context) {
	wg := new(sync.WaitGroup)

	// for i := range u.Clusters {
	// 	// c := u.Clusters[i]

	// 	wg.Add(1)

	// 	go func() {
	// 		defer wg.Done()

	// 		//			u.upgradeCloudProviderForCluster(ctx, c)
	// 	}()
	// }

	wg.Wait()
}

func (u *Upgrader) updateClusterStatus(c *clusterv1.Cluster, done bool) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.clusterStatus == nil {
		u.clusterStatus = make(map[string]bool)
	}

	u.clusterStatus[base.ClusterToKey(c)] = done
}
