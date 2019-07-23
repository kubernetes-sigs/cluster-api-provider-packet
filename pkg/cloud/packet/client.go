package packet

import (
	"fmt"
	"os"
	"strings"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/packngo"
	"k8s.io/klog"
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
	// if there are no annotations, or the annotation we want does not exist, nothing to do
	if machine.Annotations == nil {
		klog.Infof("No annotations with machine UID for %s, machine does not exist", machine.Name)
		return nil, nil
	}
	var (
		mUID string
		ok   bool
	)
	if mUID, ok = machine.Annotations[util.AnnotationUID]; !ok {
		klog.Infof("No UID annotation %s with machine UID for %s, machine does not exist", util.AnnotationUID, machine.Name)
		return nil, nil
	}
	tag := util.GenerateMachineTag(mUID)
	return p.GetDeviceByTags(c.ProjectID, []string{tag})
}
func (p *PacketClient) GetDeviceByTags(project string, tags []string) (*packngo.Device, error) {
	klog.Infof("getting device by tags for project %s, tags %v", project, tags)
	devices, _, err := p.Devices.List(project, nil)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving devices: %v", err)
	}
	// returns the first one that matches all of the tags
	for _, device := range devices {
		if util.ItemsInList(device.Tags, tags) {
			klog.Infof("found device %s", device.ID)
			return &device, nil
		}
	}
	klog.Info("no devices found")
	return nil, nil
}
