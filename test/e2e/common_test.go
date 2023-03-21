//go:build e2e
// +build e2e

/*
Copyright 2020 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"fmt"
	"net/url"
	"os"
	goruntime "runtime"
	"strings"

	"github.com/blang/semver"
	"github.com/docker/distribution/reference"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/packethost/packngo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
	clusterv1old "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	capiversionutil "sigs.k8s.io/cluster-api/util/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func logf(format string, a ...interface{}) {
	fmt.Fprintf(GinkgoWriter, "INFO: "+format+"\n", a...)
}

// wrappedClusterProxy wraps framework.clusterProxy to add support for retrying if discovery times out
// when creating a client. This was needed because control plane upgrade tests
// where sometimes attempting to create the client before the elastic IP was migrated.
type wrappedClusterProxy struct {
	clusterProxy framework.ClusterProxy

	clusterNames sets.String
}

func NewWrappedClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme, options ...framework.Option) *wrappedClusterProxy {
	return &wrappedClusterProxy{
		clusterProxy: framework.NewClusterProxy(name, kubeconfigPath, scheme, options...),
		clusterNames: sets.NewString(),
	}
}

// newFromAPIConfig returns a clusterProxy given a api.Config and the scheme defining the types hosted in the cluster.
func newFromAPIConfig(name string, config *api.Config, scheme *runtime.Scheme) *wrappedClusterProxy {
	// NB. the ClusterProvider is responsible for the cleanup of this file
	f, err := os.CreateTemp("", "e2e-kubeconfig")
	Expect(err).ToNot(HaveOccurred(), "Failed to create kubeconfig file for the kind cluster %q")
	kubeconfigPath := f.Name()

	err = clientcmd.WriteToFile(*config, kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to write kubeconfig for the kind cluster to a file %q")

	return NewWrappedClusterProxy(name, kubeconfigPath, scheme)
}

// GetName returns the name of the cluster.
func (w *wrappedClusterProxy) GetName() string {
	return w.clusterProxy.GetName()
}

// GetKubeconfigPath returns the path to the kubeconfig file to be used to access the Kubernetes cluster.
func (w *wrappedClusterProxy) GetKubeconfigPath() string {
	return w.clusterProxy.GetKubeconfigPath()
}

// GetScheme returns the scheme defining the types hosted in the Kubernetes cluster.
// It is used when creating a controller-runtime client.
func (w *wrappedClusterProxy) GetScheme() *runtime.Scheme {
	return w.clusterProxy.GetScheme()
}

// GetClient returns a controller-runtime client to the Kubernetes cluster.
func (w *wrappedClusterProxy) GetClient() client.Client {
	config := w.GetRESTConfig()

	var resClient client.Client

	Eventually(func(g Gomega) {
		var err error
		resClient, err = NewWrappedClient(config, client.Options{Scheme: w.GetScheme()}, w)
		g.Expect(err).NotTo(HaveOccurred())
	}, "5m", "10s").Should(Succeed())

	return resClient
}

// GetClientSet returns a client-go client to the Kubernetes cluster.
func (w *wrappedClusterProxy) GetClientSet() *kubernetes.Clientset {
	return w.clusterProxy.GetClientSet()
}

// GetRESTConfig returns the REST config for direct use with client-go if needed.
func (w *wrappedClusterProxy) GetRESTConfig() *rest.Config {
	return w.clusterProxy.GetRESTConfig()
}

// GetLogCollector returns the machine log collector for the Kubernetes cluster.
func (w *wrappedClusterProxy) GetLogCollector() framework.ClusterLogCollector {
	return w.clusterProxy.GetLogCollector()
}

// Apply to apply YAML to the Kubernetes cluster, `kubectl apply`.
func (w *wrappedClusterProxy) Apply(ctx context.Context, resources []byte, args ...string) error {
	return w.clusterProxy.Apply(ctx, resources, args...)
}

// GetWorkloadCluster returns ClusterProxy for the workload cluster.
func (w *wrappedClusterProxy) GetWorkloadCluster(ctx context.Context, namespace, name string) framework.ClusterProxy {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetWorkloadCluster")
	Expect(namespace).NotTo(BeEmpty(), "namespace is required for GetWorkloadCluster")
	Expect(name).NotTo(BeEmpty(), "name is required for GetWorkloadCluster")

	// gets the kubeconfig from the cluster
	config := w.getKubeconfig(ctx, namespace, name)

	// if we are on mac and the cluster is a DockerCluster, it is required to fix the control plane address
	// by using localhost:load-balancer-host-port instead of the address used in the docker network.
	if goruntime.GOOS == "darwin" && w.isDockerCluster(ctx, namespace, name) {
		w.fixConfig(ctx, name, config)
	}

	return newFromAPIConfig(name, config, w.GetScheme())
}

func (w *wrappedClusterProxy) fixConfig(ctx context.Context, name string, config *api.Config) {
	containerRuntime, err := container.NewDockerClient()
	Expect(err).ToNot(HaveOccurred(), "Failed to get Docker runtime client")

	lbContainerName := name + "-lb"
	port, err := containerRuntime.GetHostPort(ctx, lbContainerName, "6443/tcp")
	Expect(err).ToNot(HaveOccurred(), "Failed to get load balancer port")

	controlPlaneURL := &url.URL{
		Scheme: "https",
		Host:   "127.0.0.1:" + port,
	}
	currentCluster := config.Contexts[config.CurrentContext].Cluster
	config.Clusters[currentCluster].Server = controlPlaneURL.String()
}

func (w *wrappedClusterProxy) isDockerCluster(ctx context.Context, namespace string, name string) bool {
	cl := w.GetClient()

	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, cluster)).To(Succeed(), "Failed to get %s", key)

	return cluster.Spec.InfrastructureRef.Kind == "DockerCluster"
}

func (w *wrappedClusterProxy) getKubeconfig(ctx context.Context, namespace string, name string) *api.Config {
	cl := w.GetClient()

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s-kubeconfig", name),
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, secret)).To(Succeed(), "Failed to get %s", key)
	Expect(secret.Data).To(HaveKey("value"), "Invalid secret %s", key)

	config, err := clientcmd.Load(secret.Data["value"])
	Expect(err).ToNot(HaveOccurred(), "Failed to convert %s into a kubeconfig file", key)

	return config
}

// CollectWorkloadClusterLogs collects machines logs from the workload cluster.
func (w *wrappedClusterProxy) CollectWorkloadClusterLogs(ctx context.Context, namespace, name, outputPath string) {
	w.clusterProxy.CollectWorkloadClusterLogs(ctx, namespace, name, outputPath)
}

// Dispose proxy's internal resources (the operation does not affects the Kubernetes cluster).
// This should be implemented as a synchronous function.
func (w *wrappedClusterProxy) Dispose(ctx context.Context) {
	metalAuthToken := os.Getenv(AuthTokenEnvVar)
	metalProjectID := os.Getenv(ProjectIDEnvVar)
	if metalAuthToken != "" && metalProjectID != "" {
		metalClient := packet.NewClient(metalAuthToken)

		Eventually(func(g Gomega) {
			clusterNames := w.clusterNames.UnsortedList()
			logf("Will clean up EIPs for the following clusters: %v", clusterNames)

			for _, clusterName := range clusterNames {
				var ip packngo.IPAddressReservation

				g.Eventually(func(g Gomega) {
					var err error
					ip, err = metalClient.GetIPByClusterIdentifier("", clusterName, metalProjectID)
					g.Expect(err).To(SatisfyAny(Not(HaveOccurred()), MatchError(packet.ErrControlPlanEndpointNotFound)))
				}, "5m", "10s").Should(Succeed())

				if ip.ID != "" {
					if len(ip.Assignments) == 0 {
						logf("Deleting EIP with ID: %s, for cluster: %s", ip.ID, clusterName)

						g.Eventually(func(g Gomega) {
							_, err := metalClient.ProjectIPs.Remove(ip.ID)
							Expect(err).NotTo(HaveOccurred())
						}, "5m", "10s").Should(Succeed())

						w.clusterNames.Delete(clusterName)
					} else {
						logf("EIP for cluster: %s with ID: %s appears to still be assigned", clusterName, ip.ID)
					}
				} else {
					logf("Failed to find EIP for cluster: %s", clusterName)
				}
			}

			g.Expect(w.clusterNames.UnsortedList()).To(BeEmpty())
		}, "30m", "1m").Should(Succeed())
	}

	w.clusterProxy.Dispose(ctx)
}

func NewWrappedClient(config *rest.Config, options client.Options, clusterProxy *wrappedClusterProxy) (*wrappedClient, error) {
	client, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return &wrappedClient{client: client, clusterProxy: clusterProxy}, nil
}

type wrappedClient struct {
	client       client.Client
	clusterProxy *wrappedClusterProxy
}

func (wc *wrappedClient) recordClusterNameForResource(obj client.Object) error {
	var clusterName string

	gvk, err := apiutil.GVKForObject(obj, wc.client.Scheme())
	if err != nil {
		return err
	}

	if gvk.Group == clusterv1.GroupVersion.Group && gvk.Kind == "Cluster" {
		clusterName = obj.GetName()
	}

	labeledCluster, ok := obj.GetLabels()[clusterv1.ClusterLabelName]
	if ok {
		clusterName = labeledCluster
	}

	if clusterName != "" {
		logf("Recording cluster %s for EIP Cleanup later", clusterName)
		wc.clusterProxy.clusterNames.Insert(clusterName)
	}

	return nil
}

func (wc *wrappedClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := wc.recordClusterNameForResource(obj)
	if err != nil {
		return err
	}

	return wc.client.Create(ctx, obj, opts...)
}

func (wc *wrappedClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := wc.recordClusterNameForResource(obj)
	if err != nil {
		return err
	}

	return wc.client.Delete(ctx, obj, opts...)
}

func (wc *wrappedClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	err := wc.recordClusterNameForResource(obj)
	if err != nil {
		return err
	}

	return wc.client.Update(ctx, obj, opts...)
}

func (wc *wrappedClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := wc.recordClusterNameForResource(obj)
	if err != nil {
		return err
	}

	return wc.client.Patch(ctx, obj, patch, opts...)
}

func (wc *wrappedClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	// Nothing in the e2e framework appears to be using DeleteAllOf, so we can likely ignore it.
	return wc.client.DeleteAllOf(ctx, obj, opts...)
}

func (wc *wrappedClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	err := wc.recordClusterNameForResource(obj)
	if err != nil {
		return err
	}

	return wc.client.Get(ctx, key, obj)
}

func (wc *wrappedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	err := wc.client.List(ctx, list, opts...)
	if err != nil {
		return err
	}

	switch cl := list.(type) {
	case *clusterv1.ClusterList:
		for _, c := range cl.Items {
			logf("Recording cluster %s for EIP Cleanup later", c.GetName())
			wc.clusterProxy.clusterNames.Insert(c.GetName())
		}
	case *clusterv1old.ClusterList:
		for _, c := range cl.Items {
			logf("Recording cluster %s for EIP Cleanup later", c.GetName())
			wc.clusterProxy.clusterNames.Insert(c.GetName())
		}
	}

	return nil
}

func (wc *wrappedClient) RESTMapper() meta.RESTMapper {
	return wc.client.RESTMapper()
}

func (wc *wrappedClient) Scheme() *runtime.Scheme {
	return wc.client.Scheme()
}

func (wc *wrappedClient) Status() client.StatusWriter {
	return wc.client.Status()
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
		return false, nil
	}

	// If the image tag starts with sha-, assume we are running in CI and can assume the version is new enough
	if strings.HasPrefix(tag, "sha-") {
		return false, nil
	}

	imageVersion, err := capiversionutil.ParseMajorMinorPatchTolerant(tag)
	if err != nil {
		return false, fmt.Errorf("failed to get version from image: %w", err)
	}

	return imageVersion.GTE(version), nil
}
