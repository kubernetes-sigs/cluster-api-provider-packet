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
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func ObjectToName(obj controllerutil.Object) string {
	if obj.GetNamespace() != "" {
		return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
	}

	return obj.GetName()
}

const (
	DefaultTargetNamespace   = "cluster-api-provider-packet-system"
	DefaultWatchingNamespace = metav1.NamespaceAll
)

type ToolConfig struct {
	Kubeconfig        string
	Context           string
	TargetNamespace   string
	WatchingNamespace string
	DryRun            bool
	NoTUI             bool
	Logger            logr.Logger

	RestConfig           *rest.Config
	MgmtClient           client.Client
	WorkloadClientGetter remote.ClusterClientGetter
}

type Tool struct {
	mgmtClient      client.Client
	scheme          *runtime.Scheme
	config          *ToolConfig
	baseMutex       sync.Mutex
	clusters        []*clusterv1.Cluster
	workloadClients map[string]client.Client
	errors          map[string]error
	outputBuffers   map[string]*bytes.Buffer
	outputContents  map[string]string
}

func (t *Tool) WatchingNamespace() string {
	return t.config.WatchingNamespace
}

func (t *Tool) TargetNamespace() string {
	return t.config.TargetNamespace
}

func (t *Tool) DryRun() bool {
	return t.config.DryRun
}

func (t *Tool) WorkloadPatchOrCreateUnstructured(
	ctx context.Context,
	logger logr.Logger,
	c *clusterv1.Cluster,
	obj *unstructured.Unstructured,
) error {
	stdout := t.GetBufferFor(c)
	existing := obj.NewEmptyInstance()

	existingKey, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		return err
	}

	if err := t.WorkloadGet(ctx, c, existingKey, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return t.WorkloadCreate(ctx, logger, c, obj)
		}

		return err
	}

	if !equality.Semantic.DeepDerivative(obj, existing) {
		return t.WorkloadPatch(ctx, logger, c, obj, client.Merge)
	}

	kind := obj.GetObjectKind().GroupVersionKind().Kind
	name := ObjectToName(obj)
	logger.Info("Resource already up to date", "kind", kind, "name", name)
	fmt.Fprintf(stdout, "✔ %s %s already up to date\n", kind, name)

	return nil
}

func (t *Tool) WorkloadPatch(
	ctx context.Context,
	logger logr.Logger,
	c *clusterv1.Cluster,
	obj controllerutil.Object,
	patch client.Patch,
) error {
	var opts []client.PatchOption
	if t.DryRun() {
		opts = append(opts, client.DryRunAll)
	}

	workloadClient, err := t.getWorkloadClient(ctx, c)
	if err != nil {
		return err
	}

	if err := workloadClient.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(obj, t.scheme)
	if err != nil {
		return err
	}

	name := ObjectToName(obj)
	if t.DryRun() {
		// TODO: show diff
		logger.Info("(Dry Run) Would patch resource", "kind", gvk.Kind, "name", name)
		fmt.Fprintf(t.GetBufferFor(c), "(Dry Run) Would patch %s %s\n", gvk.Kind, name)

		return nil
	}

	logger.Info("Successfully patched resource", "kind", gvk.Kind, "name", name)
	fmt.Fprintf(t.GetBufferFor(c), "✅ %s %s has been successfully patched\n", gvk.Kind, name)

	return nil
}

func (t *Tool) WorkloadCreate(ctx context.Context, logger logr.Logger, c *clusterv1.Cluster, obj controllerutil.Object) error {
	var opts []client.CreateOption
	if t.DryRun() {
		opts = append(opts, client.DryRunAll)
	}

	workloadClient, err := t.getWorkloadClient(ctx, c)
	if err != nil {
		return err
	}

	if err := workloadClient.Create(ctx, obj, opts...); err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(obj, t.scheme)
	if err != nil {
		return err
	}

	name := ObjectToName(obj)

	if t.DryRun() {
		logger.Info("(Dry Run) Would create resource", "kind", gvk.Kind, "name", name)
		fmt.Fprintf(t.GetBufferFor(c), "(Dry Run) Would create %s %s\n", gvk.Kind, name)

		return nil
	}

	logger.Info("Successfully created resource", "kind", gvk.Kind, "name", name)
	fmt.Fprintf(t.GetBufferFor(c), "✅ %s %s has been successfully created\n", gvk.Kind, name)

	return nil
}

func (t *Tool) WorkloadDelete(ctx context.Context, logger logr.Logger, c *clusterv1.Cluster, obj controllerutil.Object) error {
	var opts []client.DeleteOption
	if t.DryRun() {
		opts = append(opts, client.DryRunAll)
	}

	workloadClient, err := t.getWorkloadClient(ctx, c)
	if err != nil {
		return err
	}

	if err := workloadClient.Delete(ctx, obj, opts...); err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(obj, t.scheme)
	if err != nil {
		return err
	}

	name := ObjectToName(obj)

	if t.DryRun() {
		logger.Info("(Dry Run) Would delete resource", "kind", gvk.Kind, "name", name)
		fmt.Fprintf(t.GetBufferFor(c), "(Dry Run) Would delete %s %s\n", gvk.Kind, name)

		return nil
	}

	logger.Info("Successfully deleted resource", "kind", gvk.Kind, "name", name)
	fmt.Fprintf(t.GetBufferFor(c), "✅ %s %s has been successfully deleted\n", gvk.Kind, name)

	return nil
}

func (t *Tool) WorkloadGet(ctx context.Context, c *clusterv1.Cluster, key client.ObjectKey, obj runtime.Object) error {
	workloadClient, err := t.getWorkloadClient(ctx, c)
	if err != nil {
		return err
	}

	return workloadClient.Get(ctx, key, obj)
}

func (t *Tool) WorkloadList(ctx context.Context, c *clusterv1.Cluster, obj runtime.Object) error {
	workloadClient, err := t.getWorkloadClient(ctx, c)
	if err != nil {
		return err
	}

	return workloadClient.List(ctx, obj)
}

func (t *Tool) ManagementGet(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	mgmtClient, err := t.ManagementClient()
	if err != nil {
		return err
	}

	return mgmtClient.Get(ctx, key, obj)
}

func (t *Tool) GetClusters(ctx context.Context) ([]*clusterv1.Cluster, error) {
	mgmtClient, err := t.ManagementClient()
	if err != nil {
		return nil, err
	}

	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.clusters != nil {
		return t.clusters, nil
	}

	clusterList := new(clusterv1.ClusterList)
	if err := mgmtClient.List(ctx, clusterList, client.InNamespace(t.WatchingNamespace())); err != nil {
		return nil, fmt.Errorf("failed to list workload clusters in management cluster: %w", err)
	}

	size := len(clusterList.Items)
	clusters := make([]*clusterv1.Cluster, 0, size)

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		clusters = append(clusters, cluster)
	}

	t.clusters = clusters

	return t.clusters, nil
}

func (t *Tool) ManagementClient() (client.Client, error) {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.scheme == nil {
		t.scheme = runtime.NewScheme()

		if err := scheme.AddToScheme(t.scheme); err != nil {
			return nil, fmt.Errorf("failed to add clientgo scheme: %w", err)
		}

		if err := apiextensionsv1.AddToScheme(t.scheme); err != nil {
			return nil, fmt.Errorf("failed to add apiextensions scheme: %w", err)
		}

		if err := clusterv1.AddToScheme(t.scheme); err != nil {
			return nil, fmt.Errorf("failed to add cluster-api scheme: %w", err)
		}
	}

	if t.mgmtClient != nil {
		return t.mgmtClient, nil
	}

	if t.config.MgmtClient != nil {
		t.mgmtClient = t.config.MgmtClient
		return t.mgmtClient, nil
	}

	if t.config.RestConfig == nil {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		loadingRules.ExplicitPath = t.config.Kubeconfig

		configOverrides := &clientcmd.ConfigOverrides{ //nolint:exhaustivestruct
			CurrentContext: t.config.Context,
		}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		config, err := kubeConfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create client configuration for management cluster: %w", err)
		}

		t.config.RestConfig = config
	}

	c, err := client.New(t.config.RestConfig, client.Options{Scheme: t.scheme}) //nolint:exhaustivestruct
	if err != nil {
		return nil, fmt.Errorf("failed to create managmement cluster client: %w", err)
	}

	t.mgmtClient = c

	return c, nil
}

func (t *Tool) Configure(toolConfig *ToolConfig) {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	t.config = toolConfig
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

	return t.errors[ObjectToName(c)]
}

func (t *Tool) GetOutputFor(c *clusterv1.Cluster) string {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	t.flushBuffers()

	if t.outputContents == nil {
		return ""
	}

	return t.outputContents[ObjectToName(c)]
}

func (t *Tool) AddErrorFor(c *clusterv1.Cluster, err error) {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.errors == nil {
		t.errors = make(map[string]error)
	}

	t.errors[ObjectToName(c)] = err
}

func (t *Tool) GetBufferFor(c *clusterv1.Cluster) *bytes.Buffer {
	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.outputBuffers == nil {
		t.outputBuffers = make(map[string]*bytes.Buffer)
	}

	key := ObjectToName(c)

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

func (t *Tool) getWorkloadClient(ctx context.Context, cluster *clusterv1.Cluster) (client.Client, error) {
	mgmtClient, err := t.ManagementClient()
	if err != nil {
		return nil, err
	}

	t.baseMutex.Lock()
	defer t.baseMutex.Unlock()

	if t.workloadClients == nil {
		t.workloadClients = make(map[string]client.Client)
	}

	key := ObjectToName(cluster)

	if _, ok := t.workloadClients[key]; !ok {
		clusterKey, err := client.ObjectKeyFromObject(cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to create object key: %w", err)
		}

		if t.config.WorkloadClientGetter == nil {
			t.config.WorkloadClientGetter = remote.NewClusterClient
		}

		workloadClient, err := t.config.WorkloadClientGetter(ctx, mgmtClient, clusterKey, scheme.Scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to create client: %w", err)
		}

		t.workloadClients[key] = workloadClient
	}

	return t.workloadClients[key], nil
}
