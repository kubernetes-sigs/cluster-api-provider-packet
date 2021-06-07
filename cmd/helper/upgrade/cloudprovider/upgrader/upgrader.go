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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	oldSecretName      = "packet-cloud-config"
	newSecretName      = "metal-cloud-config" //nolint: gosec
	oldDeploymentName  = "packet-cloud-controller-manager"
	csiStatefulSetName = "csi-packet-controller"
)

type Upgrader struct {
	*base.Tool
	upgradeMutex  sync.Mutex
	clusterStatus map[string]bool
}

func New(ctx context.Context, config *base.ToolConfig) (*Upgrader, error) {
	u := new(Upgrader)
	u.Tool = new(base.Tool)
	u.Configure(config)
	u.upgradeMutex.Lock()

	clusters, err := u.GetClusters(ctx)
	if err != nil {
		return u, err
	}

	u.clusterStatus = make(map[string]bool, len(clusters))
	u.upgradeMutex.Unlock()

	for _, c := range clusters {
		u.updateClusterStatus(c, false)
	}

	return u, nil
}

// TODO: update to better represent percentage by steps rather than by clusters.
func (u *Upgrader) CalculatePercentage() float64 {
	u.upgradeMutex.Lock()
	defer u.upgradeMutex.Unlock()

	if u.clusterStatus == nil {
		u.clusterStatus = make(map[string]bool)
	}

	clusters, err := u.GetClusters(context.TODO())
	if err != nil {
		return float64(0)
	}

	totalClusters := len(clusters)
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

	clusters, err := u.GetClusters(ctx)
	if err != nil {
		return
	}

	for i := range clusters {
		c := clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			u.upgradeCloudProviderForCluster(ctx, c)
		}()
	}

	wg.Wait()
}

func (u *Upgrader) upgradeCloudProviderForCluster(
	ctx context.Context,
	c *clusterv1.Cluster,
) {
	// Return early if cluster has already hit an error
	if u.HasError(c) {
		return
	}

	if err := u.migrateSecret(ctx, c); err != nil {
		u.AddErrorFor(c, err)

		return
	}

	if err := u.removeCCMDeployment(ctx, c); err != nil {
		u.AddErrorFor(c, err)

		return
	}

	if err := u.removeOldCCMSecret(ctx, c); err != nil {
		u.AddErrorFor(c, err)

		return
	}

	if err := u.installCPEM(ctx, c); err != nil {
		u.AddErrorFor(c, err)

		return
	}
}

func getLatestCPEMVersion(ctx context.Context) (string, error) {
	url := "https://github.com/equinix/cloud-provider-equinix-metal/releases/latest"
	httpClient := new(http.Client)

	versionReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	versionReq.Header.Set("Accept", "application/json")

	versionResp, err := httpClient.Do(versionReq)
	if err != nil {
		return "", err
	}

	defer versionResp.Body.Close()

	versionBody, err := io.ReadAll(versionResp.Body)
	if err != nil {
		return "", err
	}

	releaseInfo := make(map[string]interface{})
	if err := json.Unmarshal(versionBody, &releaseInfo); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s", releaseInfo["tag_name"]), nil
}

func getCPEMArtifacts(ctx context.Context, version string) ([]*unstructured.Unstructured, error) {
	httpClient := new(http.Client)
	url := fmt.Sprintf(
		"https://github.com/equinix/cloud-provider-equinix-metal/releases/download/%s/deployment.yaml",
		version,
	)

	artifactsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	artifactsResp, err := httpClient.Do(artifactsReq)
	if err != nil {
		return nil, err
	}

	defer artifactsResp.Body.Close()

	decoder := yaml.NewYAMLDecoder(artifactsResp.Body)
	defer decoder.Close()

	var resources []*unstructured.Unstructured

	for {
		obj, _, err := decoder.Decode(nil, nil)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		u := new(unstructured.Unstructured)
		u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())

		un, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}

		u.SetUnstructuredContent(un)

		resources = append(resources, u)
	}

	return resources, nil
}

func (u *Upgrader) installCPEM(ctx context.Context, c *clusterv1.Cluster) error {
	cpemVersion, err := getLatestCPEMVersion(ctx)
	if err != nil {
		return err
	}

	resources, err := getCPEMArtifacts(ctx, cpemVersion)
	if err != nil {
		return err
	}

	for _, r := range resources {
		if err := u.patchOrCreateUnstructured(ctx, c, r); err != nil {
			return err
		}
	}

	return nil
}

func (u *Upgrader) patchOrCreateUnstructured(
	ctx context.Context,
	c *clusterv1.Cluster,
	obj *unstructured.Unstructured,
) error {
	stdout := u.GetBufferFor(c)
	existing := obj.NewEmptyInstance()

	existingKey, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		return err
	}

	if err := u.WorkloadGet(ctx, c, existingKey, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return u.WorkloadCreate(ctx, c, obj)
		}

		return err
	}

	if !equality.Semantic.DeepDerivative(obj, existing) {
		return u.WorkloadPatch(ctx, c, obj, client.Merge)
	}

	fmt.Fprintf(
		stdout,
		"✔ %s %s/%s already up to date\n",
		obj.GetObjectKind().GroupVersionKind().Kind,
		obj.GetNamespace(),
		obj.GetName(),
	)

	return nil
}

func (u *Upgrader) removeOldCCMSecret(ctx context.Context, c *clusterv1.Cluster) error {
	stdout := u.GetBufferFor(c)
	ccmSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	csiStatefulSet := new(appsv1.StatefulSet)
	csiKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: csiStatefulSetName}

	err := u.WorkloadGet(ctx, c, csiKey, csiStatefulSet)

	switch {
	case err != nil && !apierrors.IsNotFound(err):
		return err
	case err == nil:
		fmt.Fprintf(stdout,
			"Skipping removal of Secret %s/%s because Packet CSI is deployed", ccmSecretKey.Namespace, ccmSecretKey.Name)

		return nil
	}

	ccmSecret := new(corev1.Secret)
	if err := u.WorkloadGet(ctx, c, ccmSecretKey, ccmSecret); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Fprintf(stdout, "✔ Secret %s/%s already deleted\n", ccmSecretKey.Namespace, ccmSecretKey.Name)

			return nil
		}

		return err
	}

	return u.WorkloadDelete(ctx, c, ccmSecret)
}

func (u *Upgrader) removeCCMDeployment(ctx context.Context, c *clusterv1.Cluster) error {
	stdout := u.GetBufferFor(c)
	ccmDeployment := new(appsv1.Deployment)
	ccmKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldDeploymentName}

	if err := u.WorkloadGet(ctx, c, ccmKey, ccmDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Fprintf(stdout, "✔ Deployment %s/%s already deleted\n", ccmKey.Namespace, ccmKey.Name)

			return nil
		}

		return err
	}

	return u.WorkloadDelete(ctx, c, ccmDeployment)
}

func (u *Upgrader) migrateSecret(ctx context.Context, c *clusterv1.Cluster) error {
	stdout := u.GetBufferFor(c)
	// Check to see if the CPEM secret already exists
	cpemSecret := new(corev1.Secret)
	cpemSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: newSecretName}
	err := u.WorkloadGet(ctx, c, cpemSecretKey, cpemSecret)

	switch {
	case err != nil && !apierrors.IsNotFound(err):
		return err
	case err == nil:
		// If there was no error, then the secret already exists and there is no need to proceed
		fmt.Fprintf(stdout, "✔ Secret %s/%s already exists\n", cpemSecret.Namespace, cpemSecret.Name)

		return nil
	}

	// Fetch the old CCM secret
	ccmSecret := new(corev1.Secret)
	ccmSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}

	if err := u.WorkloadGet(ctx, c, ccmSecretKey, ccmSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("secret %s/%s not found", ccmSecretKey.Namespace, ccmSecretKey.Name)
		}

		return err
	}

	newSecret := new(corev1.Secret)
	newSecret.SetNamespace(cpemSecretKey.Namespace)
	newSecret.SetName(cpemSecretKey.Name)
	newSecret.Data = ccmSecret.Data

	return u.WorkloadCreate(ctx, c, newSecret)
}

func (u *Upgrader) updateClusterStatus(c *clusterv1.Cluster, done bool) {
	u.upgradeMutex.Lock()
	defer u.upgradeMutex.Unlock()

	if u.clusterStatus == nil {
		u.clusterStatus = make(map[string]bool)
	}

	u.clusterStatus[base.ObjectToName(c)] = done
}
