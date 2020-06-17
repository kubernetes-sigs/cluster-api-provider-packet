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

package packet

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	infrastructurev1alpha3 "github.com/packethost/cluster-api-provider-packet/api/v1alpha3"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/scope"
	"github.com/packethost/packngo"
	"github.com/pkg/errors"
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

func (p *PacketClient) NewDevice(hostname, project string, machineScope *scope.MachineScope, extraTags []string) (*packngo.Device, error) {
	userDataRaw, err := machineScope.GetRawBootstrapData()
	if err != nil {
		return nil, errors.Wrap(err, "impossible to retrieve bootstrap data from secret")
	}
	userData := string(userDataRaw)
	tags := append(machineScope.PacketMachine.Spec.Tags, extraTags...)
	if machineScope.IsControlPlane() {
		// control plane machines should get the API key injected
		tmpl, err := template.New("control-plane-user-data").Parse(userData)
		if err != nil {
			return nil, fmt.Errorf("error parsing control-plane userdata template: %v", err)
		}
		stringWriter := &strings.Builder{}
		apiKeyStruct := map[string]interface{}{
			"apiKey": p.Client.APIKey,
		}
		if err := tmpl.Execute(stringWriter, apiKeyStruct); err != nil {
			return nil, fmt.Errorf("error executing control-plane userdata template: %v", err)
		}
		userData = stringWriter.String()
		tags = append(tags, infrastructurev1alpha3.MasterTag)
	} else {
		tags = append(tags, infrastructurev1alpha3.WorkerTag)
	}
	serverCreateOpts := &packngo.DeviceCreateRequest{
		Hostname:     hostname,
		ProjectID:    project,
		Facility:     machineScope.PacketMachine.Spec.Facility,
		BillingCycle: machineScope.PacketMachine.Spec.BillingCycle,
		Plan:         machineScope.PacketMachine.Spec.MachineType,
		OS:           machineScope.PacketMachine.Spec.OS,
		Tags:         tags,
		UserData:     userData,
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

func (p *PacketClient) GetDeviceByTags(project string, tags []string) (*packngo.Device, error) {
	devices, _, err := p.Devices.List(project, nil)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving devices: %v", err)
	}
	// returns the first one that matches all of the tags
	for _, device := range devices {
		if ItemsInList(device.Tags, tags) {
			return &device, nil
		}
	}
	return nil, nil
}
