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

// Package packet implements a client to the Equinix Metal API.
package packet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"

	metal "github.com/equinix/equinix-sdk-go/services/metalv1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-packet/internal/layer2"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet/scope"
	"sigs.k8s.io/cluster-api-provider-packet/version"
)

const (
	apiTokenVarName = "PACKET_API_KEY" //nolint:gosec
	ipxeOS          = "custom_ipxe"
	envVarLocalASN  = "METAL_LOCAL_ASN"
	envVarBGPPass   = "METAL_BGP_PASS" //nolint:gosec
	// DefaultLocalASN sets the Local ASN for BGP to our default of 65000.
	DefaultLocalASN = 65000
	legacyDebugVar  = "PACKNGO_DEBUG" // For backwards compatibility with packngo
)

var (
	clientName     = "CAPP-v1beta1"
	clientUAFormat = "cluster-api-provider-packet/%s %s"
	// ErrControlPlanEndpointNotFound is returned when the control plane endpoint is not found.
	ErrControlPlanEndpointNotFound = errors.New("control plane not found")
	// ErrElasticIPQuotaExceeded is returned when the quota for elastic IPs is exceeded.
	ErrElasticIPQuotaExceeded = errors.New("could not create an Elastic IP due to quota limits on the account, please contact Equinix Metal support")
	// ErrInvalidIP is returned when the IP is invalid.
	ErrInvalidIP = errors.New("invalid IP")
	// ErrMissingEnvVar is returned when a required environment variable is missing.
	ErrMissingEnvVar = errors.New("missing required env var")
	// ErrInvalidRequest is returned when the request is invalid.
	ErrInvalidRequest = errors.New("invalid request")
)

// Client is a wrapper around the Equinix Metal API client.
type Client struct {
	*metal.APIClient
}

// NewClient creates a new Client for the given Packet credentials.
func NewClient(packetAPIKey string) *Client {
	token := strings.TrimSpace(packetAPIKey)

	if token != "" {
		configuration := metal.NewConfiguration()
		configuration.Debug = checkEnvForDebug()
		configuration.AddDefaultHeader("X-Auth-Token", token)
		configuration.AddDefaultHeader("X-Consumer-Token", clientName)
		configuration.UserAgent = fmt.Sprintf(clientUAFormat, version.Get(), configuration.UserAgent)
		metalClient := &Client{metal.NewAPIClient(configuration)}
		return metalClient
	}

	return nil
}

// GetClient returns a new Equinix Metal client.
func GetClient() (*Client, error) {
	token := os.Getenv(apiTokenVarName)
	if token == "" {
		return nil, fmt.Errorf("%w: %s", ErrMissingEnvVar, apiTokenVarName)
	}
	return NewClient(token), nil
}

// GetDevice returns the device with the given ID.
func (p *Client) GetDevice(ctx context.Context, deviceID string) (*metal.Device, *http.Response, error) {
	dev, resp, err := p.DevicesApi.FindDeviceById(ctx, deviceID).Execute()
	return dev, resp, err
}

// CreateDeviceRequest is an object representing the API request to create a Device.
type CreateDeviceRequest struct {
	ExtraTags            []string
	MachineScope         *scope.MachineScope
	ControlPlaneEndpoint string
	CPEMLBConfig         string
	EMLBID               string
	IPAddresses          []IPAddressCfg
}

type IPAddressCfg struct {
	Netmask  string
	VXLAN    int
	Address  string
	PortName string
	Layer2   bool
	Bonded   bool
	Routes   []layer2.RouteSpec
}

// NewDevice creates a new device.
func (p *Client) NewDevice(ctx context.Context, req CreateDeviceRequest) (*metal.Device, error) {
	packetMachineSpec := req.MachineScope.PacketMachine.Spec
	packetClusterSpec := req.MachineScope.PacketCluster.Spec
	if packetMachineSpec.IPXEUrl != "" {
		// Error if pxe url and OS conflict
		if packetMachineSpec.OS != ipxeOS {
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
		"kubernetesVersion": ptr.Deref(req.MachineScope.Machine.Spec.Version, ""),
	}

	tags := make([]string, 0, len(packetMachineSpec.Tags)+len(req.ExtraTags))
	copy(tags, packetMachineSpec.Tags)
	tags = append(tags, req.ExtraTags...)

	tmpl, err := template.New("user-data").Parse(userData)
	if err != nil {
		return nil, fmt.Errorf("error parsing userdata template: %w", err)
	}

	if req.MachineScope.IsControlPlane() {
		// control plane machines should get the API key injected
		userDataValues["apiKey"] = p.APIClient.GetConfig().DefaultHeader["X-Auth-Token"]

		if req.ControlPlaneEndpoint != "" {
			userDataValues["controlPlaneEndpoint"] = req.ControlPlaneEndpoint
		}

		if req.CPEMLBConfig != "" {
			userDataValues["cpemConfig"] = req.CPEMLBConfig
		}

		if req.EMLBID != "" {
			userDataValues["emlbID"] = req.EMLBID
		}

		tags = append(tags, infrav1.ControlPlaneTag)
	} else {
		tags = append(tags, infrav1.WorkerTag)
	}

	if err := tmpl.Execute(stringWriter, userDataValues); err != nil {
		return nil, fmt.Errorf("error executing userdata template: %w", err)
	}

	// Todo: move this to a separate function
	var layer2UserData string
	// check if layer2 is enabled and add the layer2 user data
	if packetMachineSpec.NetworkPorts != nil {
		layer2Config := layer2.NewConfig()

		for _, ipAddr := range req.IPAddresses {
			layer2Config.AddPortNetwork(ipAddr.PortName, ipAddr.VXLAN, ipAddr.Address, ipAddr.Netmask, ipAddr.Routes)
		}

		layer2UserData, err = layer2Config.GetTemplate()
		if err != nil {
			return nil, fmt.Errorf("error generating layer2 user data: %w", err)
		}
		userData, err = layer2.NewCloudConfigMerger().MergeConfigs(stringWriter.String(), layer2UserData)
		if err != nil {
			return nil, fmt.Errorf("error combining user data: %w", err)
		}
	} else {
		userData = stringWriter.String()
	}
	// If Metro or Facility are specified at the Machine level, we ignore the
	// values set at the Cluster level
	facility := packetClusterSpec.Facility
	metro := packetClusterSpec.Metro

	if packetMachineSpec.Facility != "" || packetMachineSpec.Metro != "" {
		metro = packetMachineSpec.Metro
		facility = packetMachineSpec.Facility
	}

	hostname := req.MachineScope.Name()

	serverCreateOpts := metal.CreateDeviceRequest{}

	if facility != "" {
		serverCreateOpts.DeviceCreateInFacilityInput = &metal.DeviceCreateInFacilityInput{
			Hostname:        &hostname,
			Facility:        []string{facility},
			BillingCycle:    &req.MachineScope.PacketMachine.Spec.BillingCycle,
			Plan:            req.MachineScope.PacketMachine.Spec.MachineType,
			OperatingSystem: req.MachineScope.PacketMachine.Spec.OS,
			IpxeScriptUrl:   &req.MachineScope.PacketMachine.Spec.IPXEUrl,
			Tags:            tags,
			Userdata:        &userData,
		}
	} else {
		serverCreateOpts.DeviceCreateInMetroInput = &metal.DeviceCreateInMetroInput{
			Hostname:        &hostname,
			Metro:           metro,
			BillingCycle:    &req.MachineScope.PacketMachine.Spec.BillingCycle,
			Plan:            req.MachineScope.PacketMachine.Spec.MachineType,
			OperatingSystem: req.MachineScope.PacketMachine.Spec.OS,
			IpxeScriptUrl:   &req.MachineScope.PacketMachine.Spec.IPXEUrl,
			Tags:            tags,
			Userdata:        &userData,
		}
	}

	reservationIDs := strings.Split(packetMachineSpec.HardwareReservationID, ",")

	// If there are no reservationIDs to process, go ahead and return early
	if len(reservationIDs) <= 1 {
		apiRequest := p.DevicesApi.CreateDevice(ctx, req.MachineScope.PacketCluster.Spec.ProjectID)
		dev, _, err := apiRequest.CreateDeviceRequest(serverCreateOpts).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
		return dev, err
	}

	// Do a naive loop through the list of reservationIDs, continuing if we hit any error
	// TODO: if we can determine how to differentiate a failure based on the reservation
	// being in use vs other errors, then we can make this a bit smarter in the future.
	var lastErr error

	for _, resID := range reservationIDs {
		reservationID := resID
		serverCreateOpts.DeviceCreateInFacilityInput.HardwareReservationId = &reservationID
		apiRequest := p.DevicesApi.CreateDevice(ctx, req.MachineScope.PacketCluster.Spec.ProjectID)
		dev, _, err := apiRequest.CreateDeviceRequest(serverCreateOpts).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
		if err != nil {
			lastErr = err
			continue
		}

		return dev, nil
	}

	return nil, lastErr
}

// GetDeviceAddresses returns the addresses of the device.
func (p *Client) GetDeviceAddresses(device *metal.Device) []corev1.NodeAddress {
	addrs := make([]corev1.NodeAddress, 0)
	for _, addr := range device.IpAddresses {
		addrType := corev1.NodeInternalIP
		if addr.GetPublic() {
			addrType = corev1.NodeExternalIP
		}
		a := corev1.NodeAddress{
			Type:    addrType,
			Address: addr.GetAddress(),
		}
		addrs = append(addrs, a)
	}
	return addrs
}

// GetDeviceByTags returns the first device that matches all of the tags.
func (p *Client) GetDeviceByTags(ctx context.Context, project string, tags []string) (*metal.Device, error) {
	devices, _, err := p.DevicesApi.FindProjectDevices(ctx, project).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
	if err != nil {
		return nil, fmt.Errorf("error retrieving devices: %w", err)
	}
	// returns the first one that matches all of the tags
	for _, device := range devices.Devices {
		if ItemsInList(device.Tags, tags) {
			return &device, nil
		}
	}
	return nil, nil
}

// CreateIP reserves an IP via Packet API. The request fails straight if no IP are available for the specified project.
// This prevent the cluster to become ready.
func (p *Client) CreateIP(ctx context.Context, _, clusterName, projectID, facility, metro string) (net.IP, error) {
	failOnApprovalRequired := true
	req := metal.IPReservationRequestInput{
		Type:                   "public_ipv4",
		Quantity:               1,
		Facility:               &facility,
		Metro:                  &metro,
		FailOnApprovalRequired: &failOnApprovalRequired,
		Tags:                   []string{generateElasticIPIdentifier(clusterName)},
	}

	apiRequest := p.IPAddressesApi.RequestIPReservation(ctx, projectID)
	r, resp, err := apiRequest.RequestIPReservationRequest(metal.RequestIPReservationRequest{
		IPReservationRequestInput: &req,
	}).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, ErrElasticIPQuotaExceeded
	}

	rawIP := r.IPReservation.GetAddress()
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse IP: %s, %w", rawIP, ErrInvalidIP)
	}
	return ip, nil
}

// EnableProjectBGP enables bgp on the project.
func (p *Client) EnableProjectBGP(ctx context.Context, projectID string) error {
	// first check if it is enabled before trying to create it
	bgpConfig, _, err := p.BGPApi.FindBgpConfigByProject(ctx, projectID).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
	// if we already have a config, just return
	// we need some extra handling logic because the API always returns 200, even if
	// not BGP config is in place.
	// We treat it as valid config already exists only if ALL of the above is true:
	// - no error
	// - bgpConfig struct exists
	// - bgpConfig struct has non-blank ID
	// - bgpConfig struct does not have Status=="disabled"
	if err != nil {
		return err
	} else if bgpConfig != nil && bgpConfig.GetId() != "" && bgpConfig.GetStatus() != metal.BGPCONFIGSTATUS_DISABLED {
		return nil
	}

	// get the local ASN
	localASN := os.Getenv(envVarLocalASN)
	var outLocalASN int
	switch {
	case localASN != "":
		localASNNo, err := strconv.Atoi(localASN)
		if err != nil {
			return fmt.Errorf("env var %s must be a number, was %s: %w", envVarLocalASN, localASN, err)
		}
		outLocalASN = localASNNo
	default:
		outLocalASN = DefaultLocalASN
	}

	var outBGPPass string
	bgpPass := os.Getenv(envVarBGPPass)
	if bgpPass != "" {
		outBGPPass = bgpPass
	}

	// we did not have a valid one, so create it
	useCase := "kubernetes-load-balancer"
	apiRequest := p.BGPApi.RequestBgpConfig(ctx, projectID)
	_, err = apiRequest.BgpConfigRequestInput(metal.BgpConfigRequestInput{
		Asn:            int64(outLocalASN),
		Md5:            &outBGPPass,
		DeploymentType: "local",
		UseCase:        &useCase,
	}).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
	return err
}

// EnsureNodeBGPEnabled check if the node has bgp enabled, and set it if it does not.
func (p *Client) EnsureNodeBGPEnabled(ctx context.Context, id string) error {
	// fortunately, this is idempotent, so just create
	addressFamily := metal.BGPSESSIONINPUTADDRESSFAMILY_IPV4
	req := metal.BGPSessionInput{
		AddressFamily: &addressFamily,
	}
	_, response, err := p.DevicesApi.CreateBgpSession(ctx, id).BGPSessionInput(req).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
	// if we already had one, then we can ignore the error
	// this really should be a 409, but 422 is what is returned
	if response != nil && response.StatusCode == http.StatusUnprocessableEntity && strings.Contains(err.Error(), "already has session") {
		err = nil
	}
	return err
}

// GetIPByClusterIdentifier returns the IP reservation for the given cluster identifier.
func (p *Client) GetIPByClusterIdentifier(ctx context.Context, _, name, projectID string) (*metal.IPReservation, error) {
	var err error
	var ipReservation *metal.IPReservation

	eipIdentifier := generateElasticIPIdentifier(name)
	reservedIPs, _, err := p.IPAddressesApi.FindIPReservations(ctx, projectID).Execute() //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
	if err != nil {
		return ipReservation, err
	}
	for _, reservedIPWrapper := range reservedIPs.IpAddresses {
		ipReservation = reservedIPWrapper.IPReservation
		if ipReservation != nil {
			for _, tag := range ipReservation.Tags {
				if tag == eipIdentifier {
					return ipReservation, nil
				}
			}
		}
	}
	return ipReservation, ErrControlPlanEndpointNotFound
}

func generateElasticIPIdentifier(name string) string {
	return fmt.Sprintf("cluster-api-provider-packet:cluster-id:%s", name)
}

// This function provides backwards compatibility for the packngo
// debug environment variable while allowing us to introduce a new
// debug variable in the future that is not tied to packngo.
func checkEnvForDebug() bool {
	return os.Getenv(legacyDebugVar) != ""
}
