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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	"github.com/docker/distribution/reference"
	"github.com/go-logr/logr"
	"github.com/kube-vip/kube-vip/pkg/kubevip"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	cloudproviderapi "k8s.io/cloud-provider/api"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capiversionutil "sigs.k8s.io/cluster-api/util/version"
	cyaml "sigs.k8s.io/cluster-api/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
)

type Migrator struct {
	*base.Tool
	mu            sync.Mutex
	steps         []step
	clusters      []*clusterv1.Cluster
	clusterStatus map[string]map[string]bool
	logger        logr.Logger
}

type step struct {
	name   string
	method func(context.Context, logr.Logger, *base.Tool, *clusterv1.Cluster) error
}

const (
	CAPPDeploymentName           = "cluster-api-provider-packet-controller-manager"
	CPEMDeploymentName           = "cloud-provider-equinix-metal"
	CPEMSecretName               = "metal-cloud-config" //nolint: gosec
	CPEMConfigKey                = "cloud-sa.json"
	CPEMMinVersion               = "3.1.0"
	CAPPMinVersion               = "0.4.0"
	KubeVIPDaemonSetName         = "kube-vip-ds"
	KubeVIPOverrideDaemonSetName = "kube-vip-controlplane-ds"
	KubeVIPMinVersion            = "0.3.8"
	DeploymentRolloutAnnotation  = "capp-helper.metal.equinix.com/eipRestartedAt"
	KubeVIPProviderConfigEnvVar  = "provider_config"
)

var (
	ErrMissingCPEMDeployment = errors.New(CPEMDeploymentName + " Deployment not found")
	ErrMissingCPEMSecret     = errors.New(CPEMSecretName + " Secret not found")
	ErrMissingCAPPDeployment = errors.New(CAPPDeploymentName + " Deployment not found")
	ErrCPEMTooOld            = errors.New(CPEMDeploymentName + " version " + CPEMMinVersion + " or greater is needed")
	ErrKubeVIPTooOld         = errors.New(KubeVIPDaemonSetName + " version " + KubeVIPMinVersion + " or greater is needed")
	ErrKubeVIPIncompatible   = errors.New(KubeVIPDaemonSetName + " daemonset not compatible with control plane load balancing")
	ErrCAPPTooOld            = errors.New("packet provider has not been upgraded yet run: " +
		"clusterctl upgrade apply --management-group capi-system/cluster-api --contract v1alpha4")
)

func New(ctx context.Context, config *base.ToolConfig) (*Migrator, error) {
	m := new(Migrator)
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger = config.Logger.WithName("MigrateKubeVIP")
	m.Tool = new(base.Tool)
	m.Configure(config)

	m.steps = []step{
		{
			name:   "ModifyCloudProviderConfig",
			method: modifyCPEMConfig,
		},
		{
			name:   "RolloutCloudProviderDeployment",
			method: rolloutCPEMDeployment,
		},
		{
			name:   "AddKubeVIPRBAC",
			method: addKubeVIPRBAC,
		},
		{
			name:   "AddKubeVIPDaemonSet",
			method: addKubeVIPDaemonSet,
		},
	}

	clusters, err := m.GetClusters(ctx)
	if err != nil {
		m.logger.Error(err, "Failed to get clusters")

		return m, err
	}

	m.clusters = clusters
	m.clusterStatus = make(map[string]map[string]bool, len(clusters))

	for _, c := range clusters {
		clusterKey := base.ObjectToName(c)

		if m.clusterStatus[clusterKey] == nil {
			m.clusterStatus[clusterKey] = make(map[string]bool, len(m.steps))
		}
	}

	return m, nil
}

func rolloutCPEMDeployment(ctx context.Context, logger logr.Logger, m *base.Tool, c *clusterv1.Cluster) error {
	cpemDeployment := new(appsv1.Deployment)
	cpemDeploymentKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: CPEMDeploymentName}
	if err := m.WorkloadGet(ctx, c, cpemDeploymentKey, cpemDeployment); err != nil {
		return err
	}

	annotations := cpemDeployment.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	// If the annotaiton has already been added, then we can skip rolling out the deployment
	if _, ok := annotations[DeploymentRolloutAnnotation]; ok {
		stdout := m.GetBufferFor(c)
		logger.Info("cloud provider deployment has already been rolled out to pick up new EIP configuration")
		fmt.Fprintf(stdout, "%scloud provider deployment has already been rolled out to pick up new EIP configuration\n", base.NoOpPrefix)
	}

	annotations[DeploymentRolloutAnnotation] = time.Now().Format(time.RFC3339)
	updatedDeployment := cpemDeployment.DeepCopy()
	updatedDeployment.SetAnnotations(annotations)

	return m.WorkloadPatch(ctx, logger, c, cpemDeployment, updatedDeployment)
}

func modifyCPEMConfig(ctx context.Context, logger logr.Logger, m *base.Tool, c *clusterv1.Cluster) error {
	cpemSecret := new(corev1.Secret)
	cpemSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: CPEMSecretName}
	if err := m.WorkloadGet(ctx, c, cpemSecretKey, cpemSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrMissingCPEMSecret
		}

		return err
	}

	encoded := cpemSecret.Data[CPEMConfigKey]

	decoded, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return err
	}

	var config map[string]string
	if err := json.Unmarshal(decoded, &config); err != nil {
		return fmt.Errorf("failed to unmarshal cloud-provider config: %w", err)
	}

	if config["eipTag"] == "" {
		stdout := m.GetBufferFor(c)
		logger.Info("Cloud Provider config already updated")
		fmt.Fprintf(stdout, "%sCloud Provider config already updated\n", base.NoOpPrefix)
		return nil
	}

	config["eipTag"] = ""

	updated, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal cloud-provider config: %w", err)
	}

	updatedSecret := cpemSecret.DeepCopy()
	updatedSecret.Data[CPEMConfigKey] = []byte(base64.StdEncoding.EncodeToString(updated))

	return m.WorkloadPatch(ctx, logger, c, cpemSecret, updatedSecret)
}

func addKubeVIPRBAC(ctx context.Context, logger logr.Logger, m *base.Tool, c *clusterv1.Cluster) error {
	// Get and apply RBAC artifacts for kube-vip
	resources, err := getKubeVIPRBACArtifacts(ctx)
	if err != nil {
		return err
	}

	for _, r := range resources {
		if err := m.WorkloadPatchOrCreateUnstructured(ctx, logger, c, r); err != nil {
			return err
		}
	}

	return nil
}

func verifyTolerations(ds *appsv1.DaemonSet) error {
	tolerations := ds.Spec.Template.Spec.Tolerations
	taintsToTolerate := []corev1.Taint{
		{
			Key:    corev1.TaintNodeNotReady,
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    corev1.TaintNodeNetworkUnavailable,
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    "node-role.kubernetes.io/control-plane",
			Effect: corev1.TaintEffectNoSchedule,
		},
		{
			Key:    "node-role.kubernetes.io/master",
			Effect: corev1.TaintEffectNoSchedule,
		},
		{
			Key:    cloudproviderapi.TaintExternalCloudProvider,
			Effect: corev1.TaintEffectNoSchedule,
		},
	}

	unmatchedTaint, hasUnmatched := corev1helpers.FindMatchingUntoleratedTaint(taintsToTolerate, tolerations, nil)
	if hasUnmatched {
		return fmt.Errorf("toleration for taint: %s needed for running on control plane nodes missing: %w", unmatchedTaint.ToString(), ErrKubeVIPIncompatible)
	}

	return nil
}

func verifyNodeAffinity(ds *appsv1.DaemonSet) error {
	nodeSelectorSize := len(ds.Spec.Template.Spec.NodeSelector)
	var nodeAffinitySize int

	if ds.Spec.Template.Spec.Affinity != nil {
		nodeAffinitySize = ds.Spec.Template.Spec.Affinity.NodeAffinity.Size()
	}

	if nodeSelectorSize == 0 && nodeAffinitySize == 0 {
		return fmt.Errorf("node selector and/or node affinity needed for running on control plane nodes missing: %w", ErrKubeVIPIncompatible)
	}

	// Verify NodeAffinity/NodeSelector
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Labels:    ds.Spec.Template.Labels,
		},
		Spec: *ds.Spec.Template.Spec.DeepCopy(),
	}

	rna := nodeaffinity.GetRequiredNodeAffinity(testPod)
	ok, err := rna.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "",
			},
		},
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("node selector and/or node affinity needed for running on control plane nodes missing: %w", ErrKubeVIPIncompatible)
	}

	ok, err = rna.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
		},
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("node selector and/or node affinity needed for running on control plane nodes missing: %w", ErrKubeVIPIncompatible)
	}

	return nil
}

func verifyExistingDaemonSet(c *clusterv1.Cluster, ds *appsv1.DaemonSet) (bool, error) { //nolint:gocyclo
	// Determine if existing kube-vip appears to be
	// configured correctly
	var cpEnabled bool
	var metalEnabled bool
	var providerConfig string
	var project string
	var projectID string
	var authToken string
	var address string
	var port string

	var kubeVIPContainer *corev1.Container
	for i, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == "kube-vip" {
			kubeVIPContainer = &ds.Spec.Template.Spec.Containers[i]
		}
	}

	if kubeVIPContainer != nil {
		for _, e := range kubeVIPContainer.Env {
			switch e.Name {
			case "cp_enable":
				var err error
				cpEnabled, err = strconv.ParseBool(e.Value)
				if err != nil {
					return false, err
				}
			case "vip_packet", "vip_metal":
				if e.Value != "" {
					var err error
					metalEnabled, err = strconv.ParseBool(e.Value)
					if err != nil {
						return false, err
					}
				}
			case KubeVIPProviderConfigEnvVar:
				providerConfig = e.Value
			case "vip_packetproject", "vip_metalproject":
				if e.Value != "" {
					project = e.Value
				}
			case "vip_packetprojectid", "vip_metalprojectid":
				if e.Value != "" {
					projectID = e.Value
				}
			case "PACKET_AUTH_TOKEN", "METAL_AUTH_TOKEN":
				if e.Value != "" {
					authToken = e.Value
				}
			case "address", "vip_address":
				if e.Value != "" {
					address = e.Value
				}
			case "port":
				port = e.Value
			}
		}
	}

	// If the existing daemonset isn't configured for controlplane
	// load balancing, then we should be able to create a new daemonset
	// for control plane load balancing, as long as we override the Name,
	// Selector, and labels appropriately
	if !cpEnabled {
		return true, nil
	}

	// Verify container image version
	if kubeVIPContainer != nil {
		ok, err := containerImageGTE(*kubeVIPContainer, semver.MustParse(KubeVIPMinVersion))
		if err != nil {
			return false, err
		}

		if !ok {
			return false, ErrKubeVIPTooOld
		}
	}

	// Verify Equnix Metal configuration
	if !metalEnabled {
		return false, fmt.Errorf("not configured for Equinix Metal: %w", ErrKubeVIPIncompatible)
	}

	if providerConfig == "" && authToken == "" {
		return false, fmt.Errorf("auth configuration missing for Equinix Metal: %w", ErrKubeVIPIncompatible)
	}

	if providerConfig == "" && project == "" && projectID == "" {
		return false, fmt.Errorf("project configuration missing for Equinix Metal: %w", ErrKubeVIPIncompatible)
	}

	// Verify Address
	if address != c.Spec.ControlPlaneEndpoint.Host {
		return false, fmt.Errorf("address configured: %s does not match control plane endpoint address: %s : %w", address, c.Spec.ControlPlaneEndpoint.Host, ErrKubeVIPIncompatible)
	}

	// Verify Port
	if port != strconv.Itoa(int(c.Spec.ControlPlaneEndpoint.Port)) {
		return false, fmt.Errorf("port configured: %s does not match control plane endpoint port: %d : %w", address, c.Spec.ControlPlaneEndpoint.Port, ErrKubeVIPIncompatible)
	}

	// Verify NodeAffinity/NodeSelector
	if err := verifyNodeAffinity(ds); err != nil {
		return false, err
	}

	// Verify tolerations
	if err := verifyTolerations(ds); err != nil {
		return false, err
	}

	return false, nil
}

func addKubeVIPDaemonSet(ctx context.Context, logger logr.Logger, m *base.Tool, c *clusterv1.Cluster) error {
	// TODO: should we look for kube-vip static pods as well???

	// test for existence of new parallel daemonset first
	existingCPDS := new(appsv1.DaemonSet)
	existingCPDSKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: KubeVIPOverrideDaemonSetName}
	err := m.WorkloadGet(ctx, c, existingCPDSKey, existingCPDS)

	switch {
	case err != nil && !apierrors.IsNotFound(err):
		// any error other than is not found
		return err
	case err == nil:
		// parallel deployment found, validate it
		if _, err := verifyExistingDaemonSet(c, existingCPDS); err != nil {
			return err
		}

		stdout := m.GetBufferFor(c)
		logger.Info("kube-vip already deployed and configured for control plane management")
		fmt.Fprintf(stdout, "%skube-vip already deployed and configured for control plane management\n", base.NoOpPrefix)

		return nil
	}

	// test for existence of default kube-vip daemonset
	existingDS := new(appsv1.DaemonSet)
	existingDSKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: KubeVIPDaemonSetName}
	err = m.WorkloadGet(ctx, c, existingDSKey, existingDS)

	switch {
	case err != nil && !apierrors.IsNotFound(err):
		// any error other than is not found
		return err
	case err != nil && apierrors.IsNotFound(err):
		// default kube-vip daemonset not found, so let's create it
		return createKubeVIPDaemonSet(ctx, logger, m, c, kubeVIPOverrides{})
	}

	okToCreateParallel, err := verifyExistingDaemonSet(c, existingDS)
	if err != nil {
		// TODO: what to do if incompatible, since we've already updated the CPEM config???
		return err
	}
	if okToCreateParallel {
		return createKubeVIPDaemonSet(ctx, logger, m, c, kubeVIPOverrides{
			Name: pointer.String(KubeVIPOverrideDaemonSetName),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": KubeVIPOverrideDaemonSetName,
				},
			},
			PodTemplateLabels: map[string]string{
				"name": KubeVIPOverrideDaemonSetName,
			},
		})
	}

	stdout := m.GetBufferFor(c)
	logger.Info("kube-vip already deployed and configured for control plane management")
	fmt.Fprintf(stdout, "%skube-vip already deployed and configured for control plane management\n", base.NoOpPrefix)

	return nil
}

type kubeVIPOverrides struct {
	Name              *string
	Selector          *metav1.LabelSelector
	PodTemplateLabels map[string]string
}

func createKubeVIPDaemonSet(ctx context.Context, logger logr.Logger, m *base.Tool, c *clusterv1.Cluster, overrides kubeVIPOverrides) error {
	config := &kubevip.Config{
		EnableARP:         true,
		EnableControlPane: true,
		LeaderElection: kubevip.LeaderElection{
			EnableLeaderElection: true,
			LeaseDuration:        5,
			RenewDeadline:        3,
			RetryPeriod:          1,
		},
		Address:        c.Spec.ControlPlaneEndpoint.Host,
		Port:           int(c.Spec.ControlPlaneEndpoint.Port),
		Interface:      "bond0",
		EnableMetal:    true,
		Namespace:      metav1.NamespaceSystem,
		ProviderConfig: "/etc/cloud-sa/cloud-sa.json",
	}

	imageVersion := "v0.3.8"
	manifest := kubevip.GenerateDeamonsetManifestFromConfig(config, imageVersion, true, true)
	daemonset := new(appsv1.DaemonSet)
	if err := yaml.Unmarshal([]byte(manifest), daemonset); err != nil {
		return fmt.Errorf("failed to unmarshal kube-vip daemonset: %w", err)
	}

	if overrides.Name != nil {
		daemonset.Name = *overrides.Name
	}

	if overrides.Selector != nil {
		daemonset.Spec.Selector = overrides.Selector
	}

	if overrides.PodTemplateLabels != nil {
		daemonset.Spec.Template.ObjectMeta.Labels = overrides.PodTemplateLabels
	}

	return m.WorkloadCreate(ctx, logger, c, daemonset)
}

func getKubeVIPRBACArtifacts(ctx context.Context) ([]*unstructured.Unstructured, error) {
	httpClient := new(http.Client)
	url := "https://kube-vip.io/manifests/rbac.yaml"

	artifactsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	artifactsResp, err := httpClient.Do(artifactsReq)
	if err != nil {
		return nil, err
	}

	defer artifactsResp.Body.Close()

	decoder := cyaml.NewYAMLDecoder(artifactsResp.Body)
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

func (m *Migrator) CheckPrerequisites(ctx context.Context) error {
	m.logger.Info("Checking Prerequisites")

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

		m.logger.Error(err, "Failed to get CAPP Deployment")

		return fmt.Errorf("failed to get CAPP deployment: %w", err)
	}

	ok, err := containerImageGTE(cappDeployment.Spec.Template.Spec.Containers[0], semver.MustParse(CAPPMinVersion))
	if err != nil {
		m.logger.Error(err, "Failed to compare container image")

		return err
	}

	cappDeploymentOk := ok

	wg := new(sync.WaitGroup)

	for i := range m.clusters {
		c := m.clusters[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			// These errors are non-blocking but will prevent further processing of the cluster
			if err := m.validateCloudProviderForCluster(ctx, c); err != nil {
				m.logger.Error(
					err,
					"Cloud Provider needs to be upgraded on workload cluster prior to running this tool",
					"cluster",
					base.ObjectToName(c),
				)
				m.AddErrorFor(c, err)
			}
		}()
	}

	wg.Wait()

	clusterErrors := make([]error, 0, len(m.clusters))

	for _, c := range m.clusters {
		if err := m.GetErrorFor(c); err != nil {
			clusterErrors = append(clusterErrors, err)
		}
	}

	// If all workload clusters are in an error state, report that
	if len(clusterErrors) == len(m.clusters) {
		err := kerrors.NewAggregate(clusterErrors)
		m.logger.Error(err, "workload cluster prerequisites failed")

		return err
	}

	if !cappDeploymentOk {
		m.logger.Error(ErrCAPPTooOld, "CAPP needs to be upgraded prior to running this tool")

		return ErrCAPPTooOld
	}

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

	// If the image tag starts with sha-, assume we are running in CI and can assume the version is new enough
	if strings.HasPrefix(tag, "sha-") {
		return true, nil
	}

	imageVersion, err := capiversionutil.ParseMajorMinorPatchTolerant(tag)
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

	if m.clusterStatus == nil {
		m.clusterStatus = make(map[string]map[string]bool)
	}

	totalClusters := len(m.clusterStatus)
	totalSteps := totalClusters * len(m.steps)
	doneSteps := 0

	for cKey := range m.clusterStatus {
		for _, sDone := range m.clusterStatus[cKey] {
			if sDone {
				doneSteps++
			}
		}
	}

	if totalSteps == 0 {
		return float64(0)
	}

	return float64(doneSteps) / float64(totalSteps)
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

func (m *Migrator) MigrateWorkloadCluster(ctx context.Context, c *clusterv1.Cluster) {
	logger := m.logger.WithValues("cluster", base.ObjectToName(c))
	logger.Info("Started migration for cluster")

	// Return early if cluster has already hit an error
	if m.HasError(c) {
		logger.Info("Cluster previously ran into an error, skipping further processing")

		return
	}

	for _, s := range m.steps {
		stepLogger := logger.WithValues("step", s.name)
		stepLogger.Info("Started running step")

		if err := s.method(ctx, stepLogger, m.Tool, c); err != nil {
			stepLogger.Error(err, "Failure running step")
			m.AddErrorFor(c, err)

			return
		}

		m.updateStepStatus(c, s, true)
		stepLogger.Info("Finished running step")
	}

	logger.Info("Finished migration for cluster")
}

func (m *Migrator) updateStepStatus(c *clusterv1.Cluster, step step, done bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.clusterStatus == nil {
		m.clusterStatus = make(map[string]map[string]bool)
	}

	clusterKey := base.ObjectToName(c)

	if m.clusterStatus[clusterKey] == nil {
		m.clusterStatus[clusterKey] = make(map[string]bool, len(m.steps))
	}

	m.clusterStatus[clusterKey][step.name] = done
}
