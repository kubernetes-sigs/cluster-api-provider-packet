package packet

import (
	"fmt"
	"os"
	"strings"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/packngo"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	apiTokenVarName = "PACKET_API_KEY"
)

type PacketClient struct {
	*packngo.Client
}

// NewClient creates a new Client for the given Packet credentials
func NewClient(packetAPIKey string) *PacketClient {
	token := strings.TrimSpace(packetAPIKey)

	if token != "" {
		return &PacketClient{packngo.NewClientWithAuth("gardener", token, nil)}
	}

	return nil
}

func GetClient() (*PacketClient, error) {
	token := os.Getenv(apiTokenVarName)
	if token == "" {
		return nil, fmt.Errorf("env var %s is required", apiTokenVarName)
	}
	return NewClient(token), nil
}
func (p *PacketClient) GetDevice(machine *clusterv1.Machine) (*packngo.Device, error) {
	c, err := util.MachineProviderFromProviderConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("Failed to process config for providerSpec: %v", err)
	}
	devices, _, err := p.Devices.List(c.ProjectID, nil)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving devices: %v", err)
	}
	tag := util.GenerateMachineTag(string(machine.UID))
	for _, device := range devices {
		if util.ItemInList(device.Tags, tag) {
			return &device, nil
		}
	}
	return nil, nil
}
