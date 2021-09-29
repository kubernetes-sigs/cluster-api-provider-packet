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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/packethost/packngo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	infrastructurev1 "sigs.k8s.io/cluster-api-provider-packet/api/v1alpha4"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet/scope"
)

const (
	apiTokenVarName = "PACKET_API_KEY" //nolint:gosec
	clientName      = "CAPP-v1alpha4"
	ipxeOS          = "custom_ipxe"
)

var (
	ErrControlPlanEndpointNotFound = errors.New("control plane not found")
	ErrElasticIPQuotaExceeded      = errors.New("could not create an Elastic IP due to quota limits on the account, please contact Equinix Metal support")
	ErrInvalidIP                   = errors.New("invalid IP")
	ErrMissingEnvVar               = errors.New("missing required env var")
	ErrInvalidRequest              = errors.New("invalid request")
)

type Client struct {
	*packngo.Client
}

// NewClient creates a new Client for the given Packet credentials
func NewClient(packetAPIKey string) *Client {
	token := strings.TrimSpace(packetAPIKey)

	if token != "" {
		return &Client{packngo.NewClientWithAuth(clientName, token, nil)}
	}

	return nil
}

func GetClient() (*Client, error) {
	token := os.Getenv(apiTokenVarName)
	if token == "" {
		return nil, fmt.Errorf("%w: %s", ErrMissingEnvVar, apiTokenVarName)
	}
	return NewClient(token), nil
}

func (p *Client) GetDevice(deviceID string) (*packngo.Device, error) {
	dev, _, err := p.Client.Devices.Get(deviceID, nil)
	return dev, err
}

type CreateDeviceRequest struct {
	ExtraTags            []string
	MachineScope         *scope.MachineScope
	ControlPlaneEndpoint string
}

func (p *Client) NewDevice(ctx context.Context, req CreateDeviceRequest) (*packngo.Device, error) {
	if req.MachineScope.PacketMachine.Spec.IPXEUrl != "" {
		// Error if pxe url and OS conflict
		if req.MachineScope.PacketMachine.Spec.OS != ipxeOS {
			return nil, fmt.Errorf("os should be set to custom_pxe when using pxe urls: %w", ErrInvalidRequest)
		}
	}

	userDataRaw, err := req.MachineScope.GetRawBootstrapData(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve bootstrap data from secret: %w", err)
	}

	stringWriter := &strings.Builder{}
	userData := string(userDataRaw)
	userDataValues := map[string]interface{}{
		"kubernetesVersion": pointer.StringPtrDerefOr(req.MachineScope.Machine.Spec.Version, ""),
	}

	tags := make([]string, 0, len(req.MachineScope.PacketMachine.Spec.Tags)+len(req.ExtraTags))
	copy(tags, req.MachineScope.PacketMachine.Spec.Tags)
	tags = append(tags, req.ExtraTags...)

	tmpl, err := template.New("user-data").Parse(userData)
	if err != nil {
		return nil, fmt.Errorf("error parsing userdata template: %w", err)
	}

	if req.MachineScope.IsControlPlane() {
		// control plane machines should get the API key injected
		userDataValues["apiKey"] = p.Client.APIKey

		if req.ControlPlaneEndpoint != "" {
			userDataValues["controlPlaneEndpoint"] = req.ControlPlaneEndpoint
		}

		tags = append(tags, infrastructurev1.ControlPlaneTag)
	} else {
		tags = append(tags, infrastructurev1.WorkerTag)
	}

	if err := tmpl.Execute(stringWriter, userDataValues); err != nil {
		return nil, fmt.Errorf("error executing userdata template: %w", err)
	}

	userData = stringWriter.String()

	// Allow to override the facility for each PacketMachineTemplate
	var facility = req.MachineScope.PacketCluster.Spec.Facility
	if req.MachineScope.PacketMachine.Spec.Facility != "" {
		facility = req.MachineScope.PacketMachine.Spec.Facility
	}

	serverCreateOpts := &packngo.DeviceCreateRequest{
		Hostname:      req.MachineScope.Name(),
		ProjectID:     req.MachineScope.PacketCluster.Spec.ProjectID,
		Facility:      []string{facility},
		BillingCycle:  req.MachineScope.PacketMachine.Spec.BillingCycle,
		Plan:          req.MachineScope.PacketMachine.Spec.MachineType,
		OS:            req.MachineScope.PacketMachine.Spec.OS,
		IPXEScriptURL: req.MachineScope.PacketMachine.Spec.IPXEUrl,
		Tags:          tags,
		UserData:      userData,
	}

	reservationIDs := strings.Split(req.MachineScope.PacketMachine.Spec.HardwareReservationID, ",")

	// If there are no reservationIDs to process, go ahead and return early
	if len(reservationIDs) == 0 {
		dev, _, err := p.Client.Devices.Create(serverCreateOpts)
		return dev, err
	}

	// Do a naive loop through the list of reservationIDs, continuing if we hit any error
	// TODO: if we can determine how to differentiate a failure based on the reservation
	// being in use vs other errors, then we can make this a bit smarter in the future.
	var lastErr error

	for _, resID := range reservationIDs {
		serverCreateOpts.HardwareReservationID = resID
		dev, _, err := p.Client.Devices.Create(serverCreateOpts)
		if err != nil {
			lastErr = err
			continue
		}

		return dev, nil
	}

	return nil, lastErr
}

func (p *Client) GetDeviceAddresses(device *packngo.Device) []corev1.NodeAddress {
	addrs := make([]corev1.NodeAddress, 0)
	for _, addr := range device.Network {
		addrType := corev1.NodeInternalIP
		if addr.IpAddressCommon.Public {
			addrType = corev1.NodeExternalIP
		}
		a := corev1.NodeAddress{
			Type:    addrType,
			Address: addr.Address,
		}
		addrs = append(addrs, a)
	}
	return addrs
}

func (p *Client) GetDeviceByTags(project string, tags []string) (*packngo.Device, error) {
	devices, _, err := p.Devices.List(project, nil)
	if err != nil {
		return nil, fmt.Errorf("error retrieving devices: %w", err)
	}
	// returns the first one that matches all of the tags
	for _, device := range devices {
		if ItemsInList(device.Tags, tags) {
			return &device, nil
		}
	}
	return nil, nil
}

// CreateIP reserves an IP via Packet API. The request fails straight if no IP are available for the specified project.
// This prevent the cluster to become ready.
func (p *Client) CreateIP(namespace, clusterName, projectID, facility string) (net.IP, error) {
	req := packngo.IPReservationRequest{
		Type:                   packngo.PublicIPv4,
		Quantity:               1,
		Facility:               &facility,
		FailOnApprovalRequired: true,
		Tags:                   []string{generateElasticIPIdentifier(clusterName)},
	}

	r, resp, err := p.ProjectIPs.Request(projectID, &req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, ErrElasticIPQuotaExceeded
	}

	ip := net.ParseIP(r.Address)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse IP: %s, %w", r.Address, ErrInvalidIP)
	}
	return ip, nil
}

func (p *Client) GetIPByClusterIdentifier(namespace, name, projectID string) (packngo.IPAddressReservation, error) {
	var err error
	var reservedIP packngo.IPAddressReservation

	listOpts := &packngo.ListOptions{}
	reservedIPs, _, err := p.ProjectIPs.List(projectID, listOpts)
	if err != nil {
		return reservedIP, err
	}
	for _, reservedIP := range reservedIPs {
		for _, v := range reservedIP.Tags {
			if v == generateElasticIPIdentifier(name) {
				return reservedIP, nil
			}
		}
	}
	return reservedIP, ErrControlPlanEndpointNotFound
}

func generateElasticIPIdentifier(name string) string {
	return fmt.Sprintf("cluster-api-provider-packet:cluster-id:%s", name)
}
