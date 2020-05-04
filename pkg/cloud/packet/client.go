package packet

import (
	"fmt"
	"os"
	"strings"

	infrav1 "github.com/packethost/cluster-api-provider-packet/api/v1alpha3"
	"github.com/packethost/packngo"
	corev1 "k8s.io/api/core/v1"
)

const (
	apiTokenVarName = "PACKET_API_KEY"
	clientName      = "CAPP-v1alpha3"
)

type PacketClient struct {
	*packngo.Client
}

// NewClient creates a new Client for the given Packet credentials
func NewClient(packetAPIKey string) *PacketClient {
	token := strings.TrimSpace(packetAPIKey)

	if token != "" {
		return &PacketClient{packngo.NewClientWithAuth(clientName, token, nil)}
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

func (p *PacketClient) GetDevice(deviceID string) (*packngo.Device, error) {
	dev, _, err := p.Client.Devices.Get(deviceID, nil)
	return dev, err
}

func (p *PacketClient) NewDevice(hostname, project string, spec infrav1.PacketMachineSpec) (*packngo.Device, error) {
	serverCreateOpts := &packngo.DeviceCreateRequest{
		Hostname:     hostname,
		ProjectID:    project,
		Facility:     spec.Facility,
		BillingCycle: spec.BillingCycle,
		Plan:         spec.MachineType,
		OS:           spec.OS,
		Tags:         spec.Tags,
	}

	dev, _, err := p.Client.Devices.Create(serverCreateOpts)
	return dev, err
}

func (p *PacketClient) GetDeviceAddresses(device *packngo.Device) ([]corev1.NodeAddress, error) {
	addrs := make([]corev1.NodeAddress, 0)
	for _, addr := range device.Network {
		addrType := corev1.NodeInternalIP
		if addr.IpAddressCommon.Public {
			addrType = corev1.NodeExternalIP
		}
		a := corev1.NodeAddress{
			Type:    addrType,
			Address: addr.String(),
		}
		addrs = append(addrs, a)
	}
	return addrs, nil
}
