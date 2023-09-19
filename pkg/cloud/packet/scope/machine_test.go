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

package scope

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
)

const generatedNameLength = 6

func TestNewMachineScopeNoClient(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(context.TODO(), MachineScopeParams{
		Cluster:       new(clusterv1.Cluster),
		Machine:       new(clusterv1.Machine),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: new(infrav1.PacketMachine),
	})
	g.Expect(err).To(MatchError(ErrMissingClient))
}

func TestNewMachineScopeNoCluster(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(context.TODO(), MachineScopeParams{
		Client:        fake.NewClientBuilder().Build(),
		Machine:       new(clusterv1.Machine),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: new(infrav1.PacketMachine),
	})
	g.Expect(err).To(MatchError(ErrMissingCluster))
}

func TestNewMachineScopeNoMachine(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(context.TODO(), MachineScopeParams{
		Client:        fake.NewClientBuilder().Build(),
		Cluster:       new(clusterv1.Cluster),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: new(infrav1.PacketMachine),
	})
	g.Expect(err).To(MatchError(ErrMissingMachine))
}

func TestNewMachineScopeNoPacketCluster(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(context.TODO(), MachineScopeParams{
		Client:        fake.NewClientBuilder().Build(),
		Cluster:       new(clusterv1.Cluster),
		Machine:       new(clusterv1.Machine),
		PacketMachine: new(infrav1.PacketMachine),
	})
	g.Expect(err).To(MatchError(ErrMissingPacketCluster))
}

func TestNewMachineScopeNoPacketMachine(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(context.TODO(), MachineScopeParams{
		Client:        fake.NewClientBuilder().Build(),
		Cluster:       new(clusterv1.Cluster),
		Machine:       new(clusterv1.Machine),
		PacketCluster: new(infrav1.PacketCluster),
	})
	g.Expect(err).To(MatchError(ErrMissingPacketMachine))
}

func TestNewMachineScopeExistingProviderID(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infrav1.AddToScheme(scheme)).To(Succeed())

	namespace := util.RandomString(generatedNameLength)
	customPrefix := "mycustomprefix://"
	initialInstanceID := util.RandomString(generatedNameLength)
	initialProviderID := fmt.Sprintf("%s%s", customPrefix, initialInstanceID)

	initialPacketMachine := &infrav1.PacketMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: infrav1.PacketMachineSpec{
			ProviderID: ptr.To(initialProviderID),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialPacketMachine.DeepCopy()).Build()

	machineScope, err := NewMachineScope(ctx, MachineScopeParams{
		Client:        fakeClient,
		Cluster:       new(clusterv1.Cluster),
		Machine:       new(clusterv1.Machine),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: initialPacketMachine.DeepCopy(),
	})
	g.Expect(err).NotTo(HaveOccurred())

	// Verify initial values are what we expect
	g.Expect(machineScope.GetProviderID()).To(BeEquivalentTo(initialProviderID))
	g.Expect(machineScope.GetInstanceID()).To(BeEquivalentTo(initialInstanceID))

	// Let's update the instanceID and re-verify that the prefix does not change
	updatedInstanceID := util.RandomString(generatedNameLength)
	machineScope.SetProviderID(updatedInstanceID)

	expectedInstanceID := updatedInstanceID
	expectedProviderID := fmt.Sprintf("%s%s", customPrefix, updatedInstanceID)

	g.Expect(machineScope.GetProviderID()).To(BeEquivalentTo(expectedProviderID))
	g.Expect(machineScope.GetInstanceID()).To(BeEquivalentTo(expectedInstanceID))

	// Ensure the provider ID is persisted when closing the scope
	g.Expect(machineScope.Close()).To(Succeed())

	actualPacketMachine := new(infrav1.PacketMachine)
	key := client.ObjectKeyFromObject(initialPacketMachine)
	g.Expect(fakeClient.Get(ctx, key, actualPacketMachine)).To(Succeed())
	g.Expect(actualPacketMachine.Spec.ProviderID).NotTo(BeNil())
	g.Expect(*actualPacketMachine.Spec.ProviderID).To(BeEquivalentTo(expectedProviderID))
}

func TestNewMachineScopeProviderIDExplicitInit(t *testing.T) {
	namespace := util.RandomString(generatedNameLength)

	initialKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			InitConfiguration: &bootstrapv1.InitConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					KubeletExtraArgs: map[string]string{
						"provider-id": "testmetal://{{ `{{ v1.instance_id }}` }}",
					},
				},
			},
		},
	}

	testNewMachineScopeProviderIDFromKubeConfig(t, namespace, initialKubeadmConfig, "testmetal://")
}

func TestNewMachineScopeProviderIDExplicitJoin(t *testing.T) {
	namespace := util.RandomString(generatedNameLength)

	initialKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					KubeletExtraArgs: map[string]string{
						"provider-id": "testrust://{{ `{{ v1.instance_id }}` }}",
					},
				},
			},
		},
	}
	testNewMachineScopeProviderIDFromKubeConfig(t, namespace, initialKubeadmConfig, "testrust://")
}

func TestNewMachineScopeProviderIDPacketCCMPost(t *testing.T) {
	namespace := util.RandomString(generatedNameLength)

	initialKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			PostKubeadmCommands: []string{
				"systemctl restart networking",
				`kubectl --kubeconfig /etc/kubernetes/admin.conf create secret generic -n kube-system packet-cloud-config --from-literal=cloud-sa.json=''{"apiKey": "{{ .apiKey }}","projectID": "${PROJECT_ID}", "eipTag": "cluster-api-provider-packet:cluster-id:${CLUSTER_NAME}"}''`,
				"kubectl apply --kubeconfig /etc/kubernetes/admin.conf -f https://github.com/packethost/packet-ccm/releases/download/v1.1.0/deployment.yaml",
			},
		},
	}
	testNewMachineScopeProviderIDFromKubeConfig(t, namespace, initialKubeadmConfig, "packet://")
}

func TestNewMachineScopeProviderIDCPEMPost(t *testing.T) {
	namespace := util.RandomString(generatedNameLength)

	initialKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			PostKubeadmCommands: []string{
				`'if [ -f "/run/kubeadm/kubeadm.yaml" ]; then kubectl --kubeconfig /etc/kubernetes/admin.conf create secret generic -n kube-system metal-cloud-config --from-literal=cloud-sa.json=''{"apiKey": "{{ .apiKey }}","projectID": "${PROJECT_ID}", "eipTag": ""}''; kubectl apply --kubeconfig /etc/kubernetes/admin.conf -f https://github.com/equinix/cloud-provider-equinix-metal/releases/download/v3.2.2/deployment.yaml; fi'`,
			},
		},
	}
	testNewMachineScopeProviderIDFromKubeConfig(t, namespace, initialKubeadmConfig, "equinixmetal://")
}

func TestNewMachineScopeProviderIDFallbackDefault(t *testing.T) {
	namespace := util.RandomString(generatedNameLength)

	initialKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
	}
	testNewMachineScopeProviderIDFromKubeConfig(t, namespace, initialKubeadmConfig, "equinixmetal://")
}

func testNewMachineScopeProviderIDFromKubeConfig(t *testing.T, namespace string, initialKubeadmConfig *bootstrapv1.KubeadmConfig, expectedPrefix string) {
	t.Helper()
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infrav1.AddToScheme(scheme)).To(Succeed())

	initialPacketMachine := &infrav1.PacketMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: infrav1.PacketMachineSpec{},
	}

	initialMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "KubeadmConfig",
					Namespace:  initialKubeadmConfig.Namespace,
					Name:       initialKubeadmConfig.Name,
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       "PacketMachine",
				Namespace:  namespace,
				Name:       initialPacketMachine.Name,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialPacketMachine.DeepCopy(),
		initialKubeadmConfig.DeepCopy()).Build()

	fakeWorkloadClient := fake.NewClientBuilder().Build()
	machineScope, err := NewMachineScope(ctx, MachineScopeParams{
		Client:        fakeClient,
		Cluster:       new(clusterv1.Cluster),
		Machine:       initialMachine.DeepCopy(),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: initialPacketMachine.DeepCopy(),
		workloadClientGetter: func(_ context.Context, _ string, _ client.Client, _ client.ObjectKey) (client.Client, error) {
			return fakeWorkloadClient, nil
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	// Let's update the instanceID and verify that the prefix matches the cpem prefix
	updatedInstanceID := util.RandomString(generatedNameLength)
	machineScope.SetProviderID(updatedInstanceID)

	expectedInstanceID := updatedInstanceID
	expectedProviderID := expectedPrefix + updatedInstanceID

	g.Expect(machineScope.GetProviderID()).To(BeEquivalentTo(expectedProviderID))
	g.Expect(machineScope.GetInstanceID()).To(BeEquivalentTo(expectedInstanceID))

	// Ensure the provider ID is persisted when closing the scope
	g.Expect(machineScope.Close()).To(Succeed())

	actualPacketMachine := new(infrav1.PacketMachine)
	key := client.ObjectKeyFromObject(initialPacketMachine)
	g.Expect(fakeClient.Get(ctx, key, actualPacketMachine)).To(Succeed())
	g.Expect(actualPacketMachine.Spec.ProviderID).NotTo(BeNil())
	g.Expect(*actualPacketMachine.Spec.ProviderID).To(BeEquivalentTo(expectedProviderID))
}

func TestNewMachineScopeWorkloadClientError(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infrav1.AddToScheme(scheme)).To(Succeed())

	namespace := util.RandomString(generatedNameLength)

	initialKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
	}

	initialPacketMachine := &infrav1.PacketMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: infrav1.PacketMachineSpec{},
	}

	initialMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      util.RandomString(generatedNameLength),
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "KubeadmConfig",
					Namespace:  initialKubeadmConfig.Namespace,
					Name:       initialKubeadmConfig.Name,
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       "PacketMachine",
				Namespace:  namespace,
				Name:       initialPacketMachine.Name,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialPacketMachine.DeepCopy(),
		initialKubeadmConfig.DeepCopy()).Build()

	machineScope, err := NewMachineScope(ctx, MachineScopeParams{
		Client:        fakeClient,
		Cluster:       new(clusterv1.Cluster),
		Machine:       initialMachine.DeepCopy(),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: initialPacketMachine.DeepCopy(),
		workloadClientGetter: func(_ context.Context, _ string, _ client.Client, _ client.ObjectKey) (client.Client, error) {
			return nil, &url.Error{
				Op:  "Get",
				URL: "https://localhost:6443/api",
				Err: &timeoutError{true},
			}
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	// Let's update the instanceID and verify that the prefix matches the cpem prefix
	updatedInstanceID := util.RandomString(generatedNameLength)
	machineScope.SetProviderID(updatedInstanceID)

	expectedInstanceID := updatedInstanceID
	expectedProviderID := fmt.Sprintf("equinixmetal://%s", updatedInstanceID)
	g.Expect(machineScope.GetProviderID()).To(BeEquivalentTo(expectedProviderID))
	g.Expect(machineScope.GetInstanceID()).To(BeEquivalentTo(expectedInstanceID))

	// Ensure the provider ID is persisted when closing the scope
	g.Expect(machineScope.Close()).To(Succeed())

	actualPacketMachine := new(infrav1.PacketMachine)
	key := client.ObjectKeyFromObject(initialPacketMachine)
	g.Expect(fakeClient.Get(ctx, key, actualPacketMachine)).To(Succeed())
	g.Expect(actualPacketMachine.Spec.ProviderID).NotTo(BeNil())
	g.Expect(*actualPacketMachine.Spec.ProviderID).To(BeEquivalentTo(expectedProviderID))
}

type timeoutError struct {
	timeout bool
}

func (e *timeoutError) Error() string { return "timeout error" }
func (e *timeoutError) Timeout() bool { return e.timeout }

func TestNewMachineScopeCCMDeployed(t *testing.T) {
	testNewMachineScopeCloudProviderDeployed(t, "packet-cloud-controller-manager", "packet://")
}

func TestNewMachineScopeCPEMDeployed(t *testing.T) {
	testNewMachineScopeCloudProviderDeployed(t, "cloud-provider-equinix-metal", "equinixmetal://")
}

func testNewMachineScopeCloudProviderDeployed(t *testing.T, deploymentName, expectedPrefix string) {
	t.Helper()
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infrav1.AddToScheme(scheme)).To(Succeed())

	initialPacketMachine := &infrav1.PacketMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.RandomString(generatedNameLength),
			Name:      util.RandomString(generatedNameLength),
		},
	}

	cloudProviderDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      deploymentName,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialPacketMachine.DeepCopy()).Build()

	fakeWorkloadClient := fake.NewClientBuilder().WithRuntimeObjects(cloudProviderDeployment.DeepCopy()).Build()

	machineScope, err := NewMachineScope(ctx, MachineScopeParams{
		Client:        fakeClient,
		Cluster:       new(clusterv1.Cluster),
		Machine:       new(clusterv1.Machine),
		PacketCluster: new(infrav1.PacketCluster),
		PacketMachine: initialPacketMachine.DeepCopy(),
		workloadClientGetter: func(_ context.Context, _ string, _ client.Client, _ client.ObjectKey) (client.Client, error) {
			return fakeWorkloadClient, nil
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	// Let's update the instanceID and verify that the prefix matches the cpem prefix
	updatedInstanceID := util.RandomString(generatedNameLength)
	machineScope.SetProviderID(updatedInstanceID)

	expectedInstanceID := updatedInstanceID
	expectedProviderID := expectedPrefix + updatedInstanceID

	g.Expect(machineScope.GetProviderID()).To(BeEquivalentTo(expectedProviderID))
	g.Expect(machineScope.GetInstanceID()).To(BeEquivalentTo(expectedInstanceID))

	// Ensure the provider ID is persisted when closing the scope
	g.Expect(machineScope.Close()).To(Succeed())

	actualPacketMachine := new(infrav1.PacketMachine)
	key := client.ObjectKeyFromObject(initialPacketMachine)
	g.Expect(fakeClient.Get(ctx, key, actualPacketMachine)).To(Succeed())
	g.Expect(actualPacketMachine.Spec.ProviderID).NotTo(BeNil())
	g.Expect(*actualPacketMachine.Spec.ProviderID).To(BeEquivalentTo(expectedProviderID))
}
