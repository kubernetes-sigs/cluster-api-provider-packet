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

package migrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/blang/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capiutil "sigs.k8s.io/cluster-api/util"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Migrator struct {
	base.Tool
	mu         sync.Mutex
	nodeStatus map[string]map[string]bool
}

var (
	ErrMissingCPEMDeployment    = errors.New("cloud-provider-equinix-metal Deployment not found")
	ErrMissingCAPPDeployment    = errors.New("cluster-api-proviider-packet-controller-manager Deployment not found")
	ErrPacketCloudProviderFound = errors.New("packet-cloud-controller-manager found, run: " +
		"capp-helper upgrade cloudprovider")
	ErrCPEMTooOld = errors.New("cloud-provider-equinix-metal v3.1.0 or greater is needed")
	ErrCAPPTooOld = errors.New("packet provider has not been upgraded yet run: " +
		"clusterctl upgrade apply --management-group capi-system/cluster-api --contract v1alpha3")
)

func (m *Migrator) Initialize(ctx context.Context, kubeconfig *string) error {
	if err := m.Tool.Initialize(ctx, kubeconfig); err != nil {
		return err
	}

	m.nodeStatus = make(map[string]map[string]bool, len(m.Clusters))

	for _, c := range m.Clusters {
		nodes, err := m.getNodes(ctx, c)
		if err != nil {
			m.AddErrorFor(c, err)

			continue
		}

		for i := range nodes.Items {
			m.updateNodeStatus(c, &nodes.Items[i], false)
		}
	}

	return nil
}

func (m *Migrator) CheckPrerequisites(ctx context.Context) error {
	wg := new(sync.WaitGroup)

	for i := range m.Clusters {
		c := m.Clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := m.validateCloudProviderForCluster(ctx, c); err != nil {
				m.AddErrorFor(c, err)
			}
		}()
	}

	wg.Wait()

	cappDeployment, err := getDeployment(
		ctx,
		m.MgmtClient,
		"cluster-api-provider-packet-system",
		"cluster-api-provider-packet-controller-manager",
	)
	if err != nil {
		return fmt.Errorf("failed to get CAPP deployment: %w", err)
	}

	if cappDeployment == nil {
		return ErrMissingCAPPDeployment
	}

	ok, err := containerImageGTE(cappDeployment.Spec.Template.Spec.Containers[0], semver.MustParse("0.4.0"))
	if err != nil {
		return err
	}

	if !ok {
		return ErrCAPPTooOld
	}

	return nil
}

func containerImageGTE(container corev1.Container, version semver.Version) (bool, error) {
	image, err := containerutil.ImageFromString(container.Image)
	if err != nil {
		return false, fmt.Errorf("failed to get image from container: %w", err)
	}

	imageVersion, err := capiutil.ParseMajorMinorPatch(image.Tag)
	if err != nil {
		return false, fmt.Errorf("failed to get version from image: %w", err)
	}

	return imageVersion.GTE(version), nil
}

func (m *Migrator) validateCloudProviderForCluster(ctx context.Context, c *clusterv1.Cluster) error {
	// Return early if the processing for the cluster has already hit an error
	if m.HasError(c) {
		return nil
	}

	workloadClient, err := m.GetWorkloadClient(ctx, c)
	if err != nil {
		return err
	}

	packetCCMDeployment, err := getDeployment(ctx, workloadClient, "kube-system", "packet-cloud-controller-manager")
	if err != nil {
		return err
	}

	if packetCCMDeployment != nil {
		return ErrPacketCloudProviderFound
	}

	cpemDeployment, err := getDeployment(ctx, workloadClient, "kube-system", "cloud-provider-equinix-metal")
	if err != nil {
		return err
	}

	if cpemDeployment == nil {
		return ErrMissingCPEMDeployment
	}

	ok, err := containerImageGTE(cpemDeployment.Spec.Template.Spec.Containers[0], semver.MustParse("3.1.0"))
	if err != nil {
		return err
	}

	if !ok {
		return ErrCPEMTooOld
	}

	return nil
}

func getDeployment(
	ctx context.Context,
	workloadClient client.Client,
	namespace, name string,
) (*appsv1.Deployment, error) {
	deployment := new(appsv1.Deployment)
	key := client.ObjectKey{Namespace: namespace, Name: name}

	if err := workloadClient.Get(ctx, key, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return deployment, nil
}

func (m *Migrator) CalculatePercentage() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.nodeStatus == nil {
		m.nodeStatus = make(map[string]map[string]bool)
	}

	var totalNodes, doneNodes int

	for _, c := range m.Clusters {
		clusterKey := base.ClusterToKey(c)

		if m.nodeStatus[clusterKey] == nil {
			m.nodeStatus[clusterKey] = make(map[string]bool)
		}

		totalNodes += len(m.nodeStatus[clusterKey])

		for _, node := range m.nodeStatus[clusterKey] {
			if node {
				doneNodes++
			}
		}
	}

	if totalNodes == 0 {
		return float64(0)
	}

	return float64(doneNodes) / float64(totalNodes)
}

func (m *Migrator) Run(ctx context.Context) {
	wg := new(sync.WaitGroup)

	for i := range m.Clusters {
		c := m.Clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			m.migrateWorkloadCluster(ctx, c)
		}()
	}

	wg.Wait()
}

func (m *Migrator) updateNodeStatus(c *clusterv1.Cluster, n *corev1.Node, done bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.nodeStatus == nil {
		m.nodeStatus = make(map[string]map[string]bool)
	}

	clusterKey := base.ClusterToKey(c)

	if m.nodeStatus[clusterKey] == nil {
		m.nodeStatus[clusterKey] = make(map[string]bool)
	}

	m.nodeStatus[clusterKey][n.Name] = done
}

func (m *Migrator) getNodes(
	ctx context.Context,
	cluster *clusterv1.Cluster,
) (*corev1.NodeList, error) {
	workloadClient, err := m.GetWorkloadClient(ctx, cluster)
	if err != nil {
		return nil, err
	}

	nodeList := new(corev1.NodeList)
	if err := workloadClient.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return nodeList, nil
}

func (m *Migrator) migrateWorkloadCluster(
	ctx context.Context,
	cluster *clusterv1.Cluster,
) {
	// Return early if cluster has already hit an error
	if m.HasError(cluster) {
		return
	}

	nodeList, err := m.getNodes(ctx, cluster)
	if err != nil {
		m.AddErrorFor(cluster, fmt.Errorf("failed to list nodes: %w", err))

		return
	}

	for _, n := range nodeList.Items {
		// TODO: should this stop at first error, or attempt to continue?
		// TODO: should probably give some additional safety to users since this will be
		// deleting and re-creating Node resources
		if err := m.migrateNode(ctx, n, cluster); err != nil {
			m.AddErrorFor(cluster, err)

			return
		}
	}
}

func (m *Migrator) migrateNode(
	ctx context.Context,
	node corev1.Node,
	cluster *clusterv1.Cluster,
) error {
	stdout := m.GetBufferFor(cluster)
	if strings.HasPrefix(node.Spec.ProviderID, "equinixmetal") {
		fmt.Fprintf(stdout, "✔ Node %s already has the updated providerID\n", node.Name)
		m.updateNodeStatus(cluster, &node, true)

		return nil
	}

	workloadClient, err := m.GetWorkloadClient(ctx, cluster)
	if err != nil {
		return err
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

	m.updateNodeStatus(cluster, &node, true)

	return nil
}
