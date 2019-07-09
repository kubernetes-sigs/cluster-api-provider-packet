package util

import (
	"fmt"

	packetconfigv1 "github.com/packethost/cluster-api-provider-packet/pkg/apis/packetprovider/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/yaml"
)

const (
	machineUIDTag = "cluster-api-provider-packet:machine-uid"
	clusterIDTag  = "cluster-api-provider-packet:cluster-id"
	MasterTag     = "kubernetes.io/role:master"
	WorkerTag     = "kubernetes.io/role:node"
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
