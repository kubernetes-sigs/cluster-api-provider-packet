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

	metal "github.com/equinix/equinix-sdk-go/services/metalv1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func logf(format string, a ...interface{}) {
	fmt.Fprintf(GinkgoWriter, "INFO: "+format+"\n", a...)
}

// wrappedClusterProxy wraps framework.clusterProxy to add support for retrying if discovery times out
// when creating a client. This was needed because control plane upgrade tests
// where sometimes attempting to create the client before the elastic IP was migrated.
type wrappedClusterProxy struct {
	clusterProxy framework.ClusterProxy

	// This is a list of cluster names that have EIPs that need to be disposed of.
	eipClusterNames sets.Set[string]
}

func NewWrappedClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme, options ...framework.Option) *wrappedClusterProxy {
	return &wrappedClusterProxy{
		clusterProxy:    framework.NewClusterProxy(name, kubeconfigPath, scheme, options...),
		eipClusterNames: sets.New[string](),
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
		resClient, err = client.New(config, client.Options{Scheme: w.GetScheme()})
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

// GetCache returns a controller-runtime cache to create informer from.
func (w *wrappedClusterProxy) GetCache(ctx context.Context) cache.Cache {
	return w.clusterProxy.GetCache(ctx)
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
func (w *wrappedClusterProxy) GetWorkloadCluster(ctx context.Context, namespace, name string, _ ...framework.Option) framework.ClusterProxy {
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

	if w.isPacketCluster(ctx, namespace, name) {
		if !w.isEMLBCluster(ctx, namespace, name) {
			logf("Recording cluster %s for EIP Cleanup later", name)
			w.eipClusterNames.Insert(name)
		}
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

func (w *wrappedClusterProxy) isPacketCluster(ctx context.Context, namespace string, name string) bool {
	cl := w.GetClient()

	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, cluster)).To(Succeed(), "Failed to get %s", key)

	return cluster.Spec.InfrastructureRef.Kind == "PacketCluster"
}

func (w *wrappedClusterProxy) isEMLBCluster(ctx context.Context, namespace string, name string) bool {
	cl := w.GetClient()

	packetCluster := &infrav1.PacketCluster{}
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, packetCluster)).To(Succeed(), "Failed to get %s", key)

	return packetCluster.Spec.VIPManager == "EMLB"
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
			clusterNames := w.eipClusterNames.UnsortedList()
			logf("Will clean up EIPs for the following clusters: %v", clusterNames)

			for _, clusterName := range clusterNames {
				var ip *metal.IPReservation

				g.Eventually(func(g Gomega) {
					var err error
					ip, err = metalClient.GetIPByClusterIdentifier(ctx, "", clusterName, metalProjectID)
					g.Expect(err).To(SatisfyAny(Not(HaveOccurred()), MatchError(packet.ErrControlPlanEndpointNotFound)))
				}, "5m", "10s").Should(Succeed())

				ipID := ip.GetId()

				if ipID != "" {
					if len(ip.GetAssignments()) == 0 {
						logf("Deleting EIP with ID: %s, for cluster: %s", ipID, clusterName)

						g.Eventually(func(g Gomega) {
							_, err := metalClient.IPAddressesApi.DeleteIPAddress(ctx, ipID).Execute()
							Expect(err).NotTo(HaveOccurred())
						}, "5m", "10s").Should(Succeed())

						w.eipClusterNames.Delete(clusterName)
					} else {
						logf("EIP for cluster: %s with ID: %s appears to still be assigned", clusterName, ipID)
					}
				} else {
					logf("Failed to find EIP for cluster: %s", clusterName)
				}
			}

			g.Expect(w.eipClusterNames.UnsortedList()).To(BeEmpty())
		}, "30m", "1m").Should(Succeed())
	}

	w.clusterProxy.Dispose(ctx)
}
