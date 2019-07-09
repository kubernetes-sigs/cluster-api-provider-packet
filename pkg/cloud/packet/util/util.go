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
)

func MachineProviderFromProviderConfig(providerConfig clusterv1.ProviderSpec) (*packetconfigv1.PacketMachineProviderConfig, error) {
	var config packetconfigv1.PacketMachineProviderConfig
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

func ItemInList(list []string, item string) bool {
	for _, elm := range list {
		if elm == item {
			return true
		}
	}
	return false
}
