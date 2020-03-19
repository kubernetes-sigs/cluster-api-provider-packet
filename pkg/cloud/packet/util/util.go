package util

import (
	"errors"
	"fmt"

	packetconfigv1 "github.com/packethost/cluster-api-provider-packet/pkg/apis/packetprovider/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/yaml"
)

const (
	machineUIDTag = "cluster-api-provider-packet:machine-uid"
	clusterIDTag  = "cluster-api-provider-packet:cluster-id"
	MasterTag     = "kubernetes.io/role:master"
	WorkerTag     = "kubernetes.io/role:node"
	ControlPort   = 6443
	AnnotationUID = "cluster.k8s.io/machine-uid"
	caKeyName     = "key"
	caCertName    = "certificate"
)

func MachineProviderFromProviderConfig(providerConfig clusterv1.ProviderSpec) (*packetconfigv1.PacketMachineProviderConfig, error) {
	var config packetconfigv1.PacketMachineProviderConfig
	if err := yaml.Unmarshal(providerConfig.Value.Raw, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
func ClusterProviderFromProviderConfig(providerConfig clusterv1.ProviderSpec) (*packetconfigv1.PacketClusterProviderSpec, error) {
	var config packetconfigv1.PacketClusterProviderSpec
	if err := yaml.Unmarshal(providerConfig.Value.Raw, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
func ClusterProviderConfigFromProvider(config *packetconfigv1.PacketClusterProviderSpec) (clusterv1.ProviderSpec, error) {
	provider := clusterv1.ProviderSpec{}
	raw, err := json.Marshal(config)
	if err != nil {
		return provider, err
	}
	provider.Value = &runtime.RawExtension{
		Raw: raw,
	}
	return provider, nil
}
func GenerateMachineTag(ID string) string {
	return fmt.Sprintf("%s:%s", machineUIDTag, ID)
}
func GenerateClusterTag(ID string) string {
	return fmt.Sprintf("%s:%s", clusterIDTag, ID)
}

// ItemInList checks if one item is in the list
func ItemInList(list []string, item string) bool {
	for _, elm := range list {
		if elm == item {
			return true
		}
	}
	return false
}

// ItemsInList checks if all items are in the list
func ItemsInList(list []string, items []string) bool {
	// convert the items against which we are mapping into a map
	itemMap := map[string]bool{}
	for _, elm := range items {
		itemMap[elm] = false
	}
	// every one that is matched goes from false to true in the map
	for _, elm := range list {
		if _, ok := itemMap[elm]; ok {
			itemMap[elm] = true
		}
	}
	// go through the map; if any is false, return false, else all matched so return true
	for _, v := range itemMap {
		if !v {
			return false
		}
	}
	return true
}

func SecretName(cluster *clusterv1.Cluster) (string, error) {
	return fmt.Sprintf("%s%s-%s", CAPPPrefix, "ca", cluster.Name), nil
}

func GetCAFromSecret(secretsClient clientv1.SecretInterface, cluster *clusterv1.Cluster) ([]byte, []byte, error) {
	secretName, err := SecretName(cluster)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get the name of the secret: %v", err)
	}
	if secretsClient == nil {
		return nil, nil, errors.New("kubernetes client not set, cannot retrieve secret")
	}
	secret, err := secretsClient.Get(secretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get secret %s/%s: %v", cluster.Namespace, secretName, err)
	}
	return secret.Data[caKeyName], secret.Data[caCertName], nil
}

func SaveCAToSecret(caKey, caCert []byte, secretsClient clientv1.SecretInterface, cluster *clusterv1.Cluster) error {
	secretName, err := SecretName(cluster)
	if err != nil {
		return fmt.Errorf("unable to get the name of the secret: %v", err)
	}
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "core/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: CAPPNamespace,
		},
		Data: map[string][]byte{
			caKeyName:  caKey,
			caCertName: caCert,
		},
	}
	if _, err := secretsClient.Update(secret); err != nil {
		return fmt.Errorf("failed to save updated secret %s/%s for %s: %v", CAPPNamespace, secretName, cluster.Name, err)
	}
	return nil
}
