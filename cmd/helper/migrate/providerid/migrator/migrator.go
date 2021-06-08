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
	"github.com/docker/distribution/reference"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capiutil "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	oldProviderIDPrefix = "packet://"
	newProviderIDPrefix = "equinixmetal://"
)

type Migrator struct {
	*base.Tool
	mu           sync.Mutex
	clusters     []*clusterv1.Cluster
	clusterNodes map[string][]*corev1.Node
	nodeStatus   map[string]map[string]bool
}

const (
	CAPPDeploymentName = "cluster-api-provider-packet-controller-manager"
	CPEMDeploymentName = "cloud-provider-equinix-metal"
	CCMDeploymentName  = "packet-cloud-controller-manager"
	CPEMMinVersion     = "3.1.0"
	CAPPMinVersion     = "0.4.0"
)

var (
	ErrMissingCPEMDeployment    = errors.New(CPEMDeploymentName + " Deployment not found")
	ErrMissingCAPPDeployment    = errors.New(CAPPDeploymentName + " Deployment not found")
	ErrPacketCloudProviderFound = errors.New(CCMDeploymentName + " found, run: " +
		"capp-helper upgrade cloudprovider")
	ErrCPEMTooOld = errors.New(CPEMDeploymentName + " version " + CPEMMinVersion + " or greater is needed")
	ErrCAPPTooOld = errors.New("packet provider has not been upgraded yet run: " +
		"clusterctl upgrade apply --management-group capi-system/cluster-api --contract v1alpha3")
)

func New(ctx context.Context, config *base.ToolConfig) (*Migrator, error) {
	m := new(Migrator)
	m.Tool = new(base.Tool)
	m.Configure(config)

	// Initialize the node status
	m.mu.Lock()
	defer m.mu.Unlock()

	clusters, err := m.GetClusters(ctx)
	if err != nil {
		return m, err
	}

	m.clusters = clusters
	m.nodeStatus = make(map[string]map[string]bool, len(clusters))
	m.clusterNodes = make(map[string][]*corev1.Node)

	for _, c := range clusters {
		clusterKey := base.ObjectToName(c)

		nodeList := new(corev1.NodeList)
		if err := m.WorkloadList(ctx, c, nodeList); err != nil {
			m.AddErrorFor(c, err)

			continue
		}

		for i := range nodeList.Items {
			m.clusterNodes[clusterKey] = append(m.clusterNodes[clusterKey], &nodeList.Items[i])
		}
	}

	return m, nil
}

func (m *Migrator) CheckPrerequisites(ctx context.Context) error {
	cappDeployment := new(appsv1.Deployment)
	if err := m.ManagementGet(
		ctx,
		client.ObjectKey{
			Namespace: m.TargetNamespace(),
			Name:      CAPPDeploymentName,
		},
		cappDeployment,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrMissingCAPPDeployment
		}

		return fmt.Errorf("failed to get CAPP deployment: %w", err)
	}

	ok, err := containerImageGTE(cappDeployment.Spec.Template.Spec.Containers[0], semver.MustParse(CAPPMinVersion))
	if err != nil {
		return err
	}

	if !ok {
		return ErrCAPPTooOld
	}

	wg := new(sync.WaitGroup)

	for i := range m.clusters {
		c := m.clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			// These errors are non-blocking but will prevent further processing of the cluster
			if err := m.validateCloudProviderForCluster(ctx, c); err != nil {
				m.AddErrorFor(c, err)
			}
		}()
	}

	wg.Wait()

	return nil
}

func containerImageGTE(container corev1.Container, version semver.Version) (bool, error) {
	ref, err := reference.ParseNormalizedNamed(container.Image)
	if err != nil {
		return false, fmt.Errorf("failed to parse container reference %s: %w", container.Image, err)
	}

	ref = reference.TagNameOnly(ref)
	tagged, _ := ref.(reference.Tagged)
	tag := tagged.Tag()

	if tag == "latest" {
		return true, nil
	}

	imageVersion, err := capiutil.ParseMajorMinorPatch(tag)
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

	packetCCMDeployment := new(appsv1.Deployment)
	if err := m.WorkloadGet(
		ctx,
		c,
		client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: CCMDeploymentName},
		packetCCMDeployment,
	); err != nil {
		if !apierrors.IsNotFound(err) {
			// Ignore IsNotFound errors, since this is what we want to proceed
			// We hit an unexpected error
			return fmt.Errorf("failed to get deployment: %w", err)
		}
	} else {
		// If we successfully retrieved the deployment, that means that the prerequisite step
		// for upgrading the Cloud Provider has not been run yet
		return ErrPacketCloudProviderFound
	}

	cpemDeployment := new(appsv1.Deployment)
	if err := m.WorkloadGet(
		ctx,
		c,
		client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: CPEMDeploymentName},
		cpemDeployment,
	); err != nil {
		if apierrors.IsNotFound(err) {
			// Missing expected CPEM deployment
			return ErrMissingCPEMDeployment
		}

		// We hit an unexpected error
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	ok, err := containerImageGTE(cpemDeployment.Spec.Template.Spec.Containers[0], semver.MustParse(CPEMMinVersion))
	if err != nil {
		return err
	}

	if !ok {
		return ErrCPEMTooOld
	}

	return nil
}

func (m *Migrator) CalculatePercentage() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.nodeStatus == nil {
		m.nodeStatus = make(map[string]map[string]bool)
	}

	totalNodes := 0
	doneNodes := 0

	for clusterKey, nodes := range m.clusterNodes {
		totalNodes += len(nodes)

		for _, n := range nodes {
			if m.nodeStatus[clusterKey][n.Name] {
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

	for i := range m.clusters {
		c := m.clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			m.MigrateWorkloadCluster(ctx, c)
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

	clusterKey := base.ObjectToName(c)

	if m.nodeStatus[clusterKey] == nil {
		m.nodeStatus[clusterKey] = make(map[string]bool)
	}

	m.nodeStatus[clusterKey][n.Name] = done
}

func (m *Migrator) MigrateWorkloadCluster(ctx context.Context, c *clusterv1.Cluster) {
	// Return early if cluster has already hit an error
	if m.HasError(c) {
		return
	}

	for _, n := range m.clusterNodes[base.ObjectToName(c)] {
		// TODO: should this stop at first error, or attempt to continue?
		// TODO: should probably give some additional safety to users since this will be
		// deleting and re-creating Node resources
		if err := m.MigrateNode(ctx, n, c); err != nil {
			m.AddErrorFor(c, err)

			return
		}
	}
}

func (m *Migrator) MigrateNode(ctx context.Context, node *corev1.Node, c *clusterv1.Cluster) error {
	if strings.HasPrefix(node.Spec.ProviderID, newProviderIDPrefix) {
		fmt.Fprintf(m.GetBufferFor(c), "✔ Node %s already has the updated providerID\n", node.Name)
		m.updateNodeStatus(c, node, true)

		return nil
	}

	if err := m.WorkloadDelete(ctx, c, node.DeepCopy()); err != nil {
		return fmt.Errorf("failed to delete existing node resource: %w", err)
	}

	node.SetResourceVersion("")
	node.Spec.ProviderID = strings.Replace(node.Spec.ProviderID, oldProviderIDPrefix, newProviderIDPrefix, 1)

	if err := retry.OnError(
		retry.DefaultRetry,
		func(err error) bool {
			return true
		},
		func() error {
			if err := m.WorkloadCreate(ctx, c, node); err != nil {
				if m.DryRun() && apierrors.IsAlreadyExists(err) {
					// add dry run success output here since Create will fail with an already exists error during dry run
					fmt.Fprintf(m.GetBufferFor(c), "(Dry Run) Would create Node %s\n", base.ObjectToName(node))

					return nil
				}

				return err //nolint:wrapcheck
			}

			return nil
		},
	); err != nil {
		// TODO: give user actionable output/log to remediate
		return fmt.Errorf("failed to create replacement node resource: %w", err)
	}

	m.updateNodeStatus(c, node, true)

	return nil
}