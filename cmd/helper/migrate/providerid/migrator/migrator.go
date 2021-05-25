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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/blang/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/util"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capiutil "sigs.k8s.io/cluster-api/util"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func clusterToKey(c *clusterv1.Cluster) string {
	return fmt.Sprintf("%s/%s", c.Name, c.Namespace)
}

type Migrator struct {
	Clusters []*clusterv1.Cluster

	kubeconfig string
	mgmtClient client.Client

	mu              sync.Mutex
	workloadClients map[string]client.Client
	errors          map[string]error
	outputBuffers   map[string]*bytes.Buffer
	outputContents  map[string]string
	nodeStatus      map[string]map[string]bool
}

var (
	ErrMissingKubeConfig        = errors.New("kubeconfig was nil")
	ErrMissingCPEMDeployment    = errors.New("cloud-provider-equinix-metal Deployment not found")
	ErrMissingCAPPDeployment    = errors.New("cluster-api-proviider-packet-controller-manager Deployment not found")
	ErrPacketCloudProviderFound = errors.New("packet-cloud-controller-manager found, run: " +
		"capp-helper upgrade cloudprovider")
	ErrCPEMTooOld = errors.New("cloud-provider-equinix-metal v3.1.0 or greater is needed")
	ErrCAPPTooOld = errors.New("packet provider has not been upgraded yet run: " +
		"clusterctl upgrade apply --management-group capi-system/cluster-api --contract v1alpha3")
)

func NewMigrator(ctx context.Context, kubeconfig *string) (*Migrator, error) {
	if kubeconfig == nil {
		return nil, ErrMissingKubeConfig
	}

	mgmtClient, err := util.GetManagementClient(*kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create management cluster client: %w", err)
	}

	clusterList := new(clusterv1.ClusterList)
	if err := mgmtClient.List(ctx, clusterList); err != nil {
		return nil, fmt.Errorf("failed to list workload clusters in management cluster: %w", err)
	}

	size := len(clusterList.Items)

	migrator := &Migrator{
		kubeconfig:      *kubeconfig,
		mgmtClient:      mgmtClient,
		Clusters:        make([]*clusterv1.Cluster, 0, size),
		mu:              sync.Mutex{},
		workloadClients: make(map[string]client.Client, size),
		errors:          make(map[string]error, size),
		outputBuffers:   make(map[string]*bytes.Buffer, size),
		outputContents:  make(map[string]string, size),
		nodeStatus:      make(map[string]map[string]bool, size),
	}

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		migrator.Clusters = append(migrator.Clusters, cluster)

		nodes, err := migrator.getNodes(ctx, cluster)
		if err != nil {
			migrator.addErrorFor(cluster, err)

			continue
		}

		for i := range nodes.Items {
			migrator.updateNodeStatus(cluster, &nodes.Items[i], false)
		}
	}

	return migrator, nil
}

func (m *Migrator) CheckPrerequisites(ctx context.Context) error {
	wg := new(sync.WaitGroup)

	for i := range m.Clusters {
		c := m.Clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := m.validateCloudProviderForCluster(ctx, c); err != nil {
				m.addErrorFor(c, err)
			}
		}()
	}

	wg.Wait()

	cappDeployment, err := getDeployment(
		ctx,
		m.mgmtClient,
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
	if m.hasError(c) {
		// Return early if the processing for the cluster has already hit an error
		return nil
	}

	workloadClient, err := m.getWorkloadClient(ctx, c)
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
		clusterKey := clusterToKey(c)

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

func (m *Migrator) hasError(c *clusterv1.Cluster) bool {
	return m.GetErrorFor(c) != nil
}

func (m *Migrator) GetErrorFor(c *clusterv1.Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errors == nil {
		return nil
	}

	return m.errors[clusterToKey(c)]
}

func (m *Migrator) GetOutputFor(c *clusterv1.Cluster) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.flushBuffers()

	return m.outputContents[clusterToKey(c)]
}

func (m *Migrator) updateNodeStatus(c *clusterv1.Cluster, n *corev1.Node, done bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.nodeStatus == nil {
		m.nodeStatus = make(map[string]map[string]bool)
	}

	clusterKey := clusterToKey(c)

	if m.nodeStatus[clusterKey] == nil {
		m.nodeStatus[clusterKey] = make(map[string]bool)
	}

	m.nodeStatus[clusterKey][n.Name] = done
}

func (m *Migrator) addErrorFor(c *clusterv1.Cluster, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errors == nil {
		m.errors = make(map[string]error)
	}

	m.errors[clusterToKey(c)] = err
}

func (m *Migrator) getBufferFor(c *clusterv1.Cluster) *bytes.Buffer {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.outputBuffers == nil {
		m.outputBuffers = make(map[string]*bytes.Buffer)
	}

	key := clusterToKey(c)

	if m.outputBuffers[key] == nil {
		m.outputBuffers[key] = new(bytes.Buffer)
	}

	return m.outputBuffers[key]
}

func (m *Migrator) flushBuffers() {
	if m.outputBuffers == nil {
		m.outputBuffers = make(map[string]*bytes.Buffer)
	}

	if m.outputContents == nil {
		m.outputContents = make(map[string]string)
	}

	for key, buf := range m.outputBuffers {
		out, err := ioutil.ReadAll(buf)
		if err != nil {
			continue
		}

		m.outputContents[key] += string(out)
	}
}

func (m *Migrator) getWorkloadClient(
	ctx context.Context,
	cluster *clusterv1.Cluster,
) (client.Client, error) {
	if m.workloadClients == nil {
		m.workloadClients = make(map[string]client.Client)
	}

	key := clusterToKey(cluster)

	if _, ok := m.workloadClients[key]; !ok {
		clusterKey, err := client.ObjectKeyFromObject(cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to create object key: %w", err)
		}

		workloadClient, err := remote.NewClusterClient(ctx, m.mgmtClient, clusterKey, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client: %w", err)
		}

		m.workloadClients[key] = workloadClient
	}

	return m.workloadClients[key], nil
}

func (m *Migrator) getNodes(
	ctx context.Context,
	cluster *clusterv1.Cluster,
) (*corev1.NodeList, error) {
	workloadClient, err := m.getWorkloadClient(ctx, cluster)
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
	if m.hasError(cluster) {
		return
	}

	nodeList, err := m.getNodes(ctx, cluster)
	if err != nil {
		m.addErrorFor(cluster, fmt.Errorf("failed to list nodes: %w", err))

		return
	}

	for _, n := range nodeList.Items {
		// TODO: should this stop at first error, or attempt to continue?
		// TODO: should probably give some additional safety to users since this will be
		// deleting and re-creating Node resources
		if err := m.migrateNode(ctx, n, cluster); err != nil {
			m.addErrorFor(cluster, err)

			return
		}
	}
}

func (m *Migrator) migrateNode(
	ctx context.Context,
	node corev1.Node,
	cluster *clusterv1.Cluster,
) error {
	stdout := m.getBufferFor(cluster)
	if strings.HasPrefix(node.Spec.ProviderID, "equinixmetal") {
		fmt.Fprintf(stdout, "✔ Node %s already has the updated providerID\n", node.Name)
		m.updateNodeStatus(cluster, &node, true)

		return nil
	}

	workloadClient, err := m.getWorkloadClient(ctx, cluster)
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
