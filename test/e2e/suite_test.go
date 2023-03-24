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
	"crypto/rand"
	"crypto/rsa"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/packethost/packngo"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/runtime"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"

	"sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
)

const (
	AuthTokenEnvVar = "PACKET_API_KEY"
	ProjectIDEnvVar = "PROJECT_ID"
)

// Test suite flags
var (
	// configPath is the path to the e2e config file.
	configPath string

	// useExistingCluster instructs the test to use the current cluster instead of creating a new one (default discovery rules apply).
	useExistingCluster bool

	// artifactFolder is the folder to store e2e test artifacts.
	artifactFolder string

	// skipCleanup prevents cleanup of test resources e.g. for debug purposes.
	skipCleanup bool
)

// Test suite global vars
var (
	// e2eConfig to be used for this test, read from configPath.
	e2eConfig *clusterctl.E2EConfig

	// clusterctlConfigPath to be used for this test, created by generating a clusterctl local repository
	// with the providers specified in the configPath.
	clusterctlConfigPath string

	// bootstrapClusterProvider manages provisioning of the the bootstrap cluster to be used for the e2e tests.
	// Please note that provisioning will be skipped if e2e.use-existing-cluster is provided.
	bootstrapClusterProvider bootstrap.ClusterProvider

	// bootstrapClusterProxy allows to interact with the bootstrap cluster to be used for the e2e tests.
	bootstrapClusterProxy framework.ClusterProxy

	// sshKeyID is the id for the generated ssh key, this is used
	// to cleanup the ssh key in SynchronizedAfterSuite
	sshKeyID string
)

func init() {
	flag.StringVar(&configPath, "e2e.config", "", "path to the e2e config file")
	flag.StringVar(&artifactFolder, "e2e.artifacts-folder", "", "folder where e2e test artifact should be stored")
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
	flag.BoolVar(&useExistingCluster, "e2e.use-existing-cluster", false, "if true, the test uses the current cluster instead of creating a new one (default discovery rules apply)")
}

func TestE2E(t *testing.T) {
	g := NewWithT(t)

	// ensure the artifacts folder exists
	g.Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", artifactFolder) //nolint:gosec

	RegisterFailHandler(Fail)

	RunSpecs(t, "capp-e2e")
}

// Using a SynchronizedBeforeSuite for controlling how to create resources shared across ParallelNodes (~ginkgo threads).
// The local clusterctl repository & the bootstrap cluster are created once and shared across all the tests.
var _ = SynchronizedBeforeSuite(func() []byte {
	// Before all ParallelNodes.
	Expect(os.Getenv(AuthTokenEnvVar)).NotTo(BeEmpty())
	Expect(os.Getenv(ProjectIDEnvVar)).NotTo(BeEmpty())
	Expect(os.Getenv("FACILITY")).NotTo(BeEmpty())
	Expect(os.Getenv("CONTROLPLANE_NODE_TYPE")).NotTo(BeEmpty())
	Expect(os.Getenv("WORKER_NODE_TYPE")).NotTo(BeEmpty())

	Expect(configPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", artifactFolder)

	By("Initializing a runtime.Scheme with all the GVK relevant for this test")
	scheme := initScheme()

	capi_e2e.Byf("Loading the e2e test configuration from %q", configPath)
	e2eConfig := loadE2EConfig(configPath)

	var sshKeyName string
	sshKeyID, sshKeyName = generateSSHKeyIfNeeded()

	capi_e2e.Byf("Creating a clusterctl local repository into %q", artifactFolder)
	clusterctlConfigPath := createClusterctlLocalRepository(e2eConfig, filepath.Join(artifactFolder, "repository"))

	By("Setting up the bootstrap cluster")
	var bootstrapClusterProxy framework.ClusterProxy
	bootstrapClusterProvider, bootstrapClusterProxy = setupBootstrapCluster(e2eConfig, scheme, useExistingCluster)

	By("Initializing the bootstrap cluster")
	initBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

	return []byte(
		strings.Join([]string{
			clusterctlConfigPath,
			bootstrapClusterProxy.GetKubeconfigPath(),
			sshKeyName,
		}, ","),
	)
}, func(data []byte) {
	// Before each ParallelNode.

	parts := strings.Split(string(data), ",")
	Expect(parts).To(HaveLen(3))

	clusterctlConfigPath = parts[0]
	kubeconfigPath := parts[1]
	sshKeyName := parts[2]

	if e2eConfig == nil {
		e2eConfig = loadE2EConfig(configPath)
	}

	ensureSSHKeyName(sshKeyName)

	bootstrapClusterProxy = NewWrappedClusterProxy("bootstrap", kubeconfigPath, initScheme())
})

// Using a SynchronizedAfterSuite for controlling how to delete resources shared across ParallelNodes (~ginkgo threads).
// The bootstrap cluster is shared across all the tests, so it should be deleted only after all ParallelNodes completes.
// The local clusterctl repository is preserved like everything else created into the artifact folder.
var _ = SynchronizedAfterSuite(func() {
	// After each ParallelNode.
	if !skipCleanup {
		By("Cleaning up the cluster proxy")
		tearDown(nil, bootstrapClusterProxy)
	}
}, func() {
	// After all ParallelNodes.

	if !skipCleanup {
		By("Tearing down the management cluster")
		tearDown(bootstrapClusterProvider, nil)

		metalAuthToken := os.Getenv(AuthTokenEnvVar)
		if metalAuthToken != "" && sshKeyID != "" {
			By("Cleaning up the generated SSH Key")
			metalClient := packet.NewClient(metalAuthToken)
			_, err := metalClient.SSHKeys.Delete(sshKeyID)
			Expect(err).NotTo(HaveOccurred())
		}
	}
})

func initScheme() *runtime.Scheme {
	sc := runtime.NewScheme()
	framework.TryAddDefaultSchemes(sc)
	Expect(v1beta1.AddToScheme(sc)).To(Succeed())

	return sc
}

func loadE2EConfig(configPath string) *clusterctl.E2EConfig {
	config := clusterctl.LoadE2EConfig(context.TODO(), clusterctl.LoadE2EConfigInput{ConfigPath: configPath})
	Expect(config).ToNot(BeNil(), "Failed to load E2E config from %s", configPath)

	return config
}

func createClusterctlLocalRepository(config *clusterctl.E2EConfig, repositoryFolder string) string {
	createRepositoryInput := clusterctl.CreateRepositoryInput{
		E2EConfig:        config,
		RepositoryFolder: repositoryFolder,
	}

	// Ensuring a CNI file is defined in the config and register a FileTransformation to inject the referenced file as in place of the CNI_RESOURCES envSubst variable.
	Expect(config.Variables).To(HaveKey(capi_e2e.CNIPath), "Missing %s variable in the config", capi_e2e.CNIPath)
	cniPath := config.GetVariable(capi_e2e.CNIPath)
	Expect(cniPath).To(BeAnExistingFile(), "The %s variable should resolve to an existing file", capi_e2e.CNIPath)
	createRepositoryInput.RegisterClusterResourceSetConfigMapTransformation(cniPath, capi_e2e.CNIResources)

	clusterctlConfig := clusterctl.CreateRepository(context.TODO(), createRepositoryInput)
	Expect(clusterctlConfig).To(BeAnExistingFile(), "The clusterctl config file does not exists in the local repository %s", repositoryFolder)

	return clusterctlConfig
}

func setupBootstrapCluster(config *clusterctl.E2EConfig, scheme *runtime.Scheme, useExistingCluster bool) (bootstrap.ClusterProvider, framework.ClusterProxy) {
	var clusterProvider bootstrap.ClusterProvider
	kubeconfigPath := ""
	if !useExistingCluster {
		clusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(context.TODO(), bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:               config.ManagementClusterName,
			RequiresDockerSock: config.HasDockerProvider(),
			Images:             config.Images,
		})
		Expect(clusterProvider).ToNot(BeNil(), "Failed to create a bootstrap cluster")

		kubeconfigPath = clusterProvider.GetKubeconfigPath()
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the bootstrap cluster")
	}

	clusterProxy := NewWrappedClusterProxy("bootstrap", kubeconfigPath, scheme)
	Expect(clusterProxy).ToNot(BeNil(), "Failed to get a bootstrap cluster proxy")

	return clusterProvider, clusterProxy
}

func initBootstrapCluster(bootstrapClusterProxy framework.ClusterProxy, config *clusterctl.E2EConfig, clusterctlConfig, artifactFolder string) {
	clusterctl.InitManagementClusterAndWatchControllerLogs(context.TODO(), clusterctl.InitManagementClusterAndWatchControllerLogsInput{
		ClusterProxy:            bootstrapClusterProxy,
		ClusterctlConfigPath:    clusterctlConfig,
		InfrastructureProviders: config.InfrastructureProviders(),
		LogFolder:               filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName()),
	}, config.GetIntervals(bootstrapClusterProxy.GetName(), "wait-controllers")...)
}

func tearDown(bootstrapClusterProvider bootstrap.ClusterProvider, bootstrapClusterProxy framework.ClusterProxy) {
	if bootstrapClusterProxy != nil {
		bootstrapClusterProxy.Dispose(context.TODO())
	}
	if bootstrapClusterProvider != nil {
		bootstrapClusterProvider.Dispose(context.TODO())
	}
}

func ensureSSHKeyName(sshKeyName string) {
	sshKey, ok := os.LookupEnv("SSH_KEY")
	if !ok || sshKey == "" {
		sshKey = sshKeyName
		Expect(os.Setenv("SSH_KEY", sshKey)).To(Succeed())
	}

	logf("Using ssh key: %s", sshKey)
}

func generateSSHKeyIfNeeded() (string, string) {
	if sshKey, ok := os.LookupEnv("SSH_KEY"); ok && sshKey != "" {
		return "", sshKey
	}

	By("Generating an SSH Key for use in tests")
	return generateSSHKey()
}

func generateSSHKey() (string, string) {
	metalAuthToken := os.Getenv(AuthTokenEnvVar)
	Expect(metalAuthToken).NotTo(BeEmpty(), "%s not set in environment", AuthTokenEnvVar)

	// TODO: do we need to write these keys out to disk at all?
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	Expect(err).NotTo(HaveOccurred())

	metalClient := packet.NewClient(metalAuthToken)
	res, _, err := metalClient.SSHKeys.Create(
		&packngo.SSHKeyCreateRequest{
			Label: fmt.Sprintf("capp-e2e-%s", util.RandomString(6)),
			Key:   string(ssh.MarshalAuthorizedKey(pub)),
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(res.ID).NotTo(BeEmpty())
	Expect(res.Label).NotTo(BeEmpty())

	return res.ID, res.Label
}
