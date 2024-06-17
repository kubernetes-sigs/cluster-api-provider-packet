/*
Copyright 2024 The Kubernetes Authors.

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

// Package emlb manages authentication to the Equinix Metal Load Balancer service.
package emlb

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	lbaas "sigs.k8s.io/cluster-api-provider-packet/internal/lbaas/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet/scope"
)

const (
	// providerID is the provider id key used to talk to the Load Balancer as a Service API.
	providerID = "loadpvd-gOB_-byp5ebFo7A3LHv2B"
	// loadBalancerIDAnnotation is the anotation key representing the ID of the allocated LoadBalancer for a PacketCluster.
	loadBalancerIDAnnotation = "equinix.com/loadbalancerID"
	// loadBalancerPortNumberAnnotation is the anotation key representing the allocated listner port number for a PacketCluster.
	loadBalancerPortNumberAnnotation = "equinix.com/loadbalancerPortNumber"
	// loadBalancerMetroAnnotation is the anotation key representing the metro of the loadbalancer for a PacketCluster.
	loadBalancerMetroAnnotation = "equinix.com/loadbalancerMetro"
	// loadBalancerVIPPort is the port number of the API Server.
	loadBalancerVIPPort = 6443 // TODO Change this to a env variable
	// loadBalancerPoolIDAnnotation is the anotation key representing the ID of the origin pool for a PacketCluster.
	loadBalancerPoolIDAnnotation = "equinix.com/loadbalancerpoolID"
	// loadBalancerPoolOriginIDAnnotation is the anotation key representing the origin ID of a PacketMachine.
	loadBalancerOriginIDAnnotation = "equinix.com/loadbalanceroriginID"
	// EMLBVIPID is the stringused to refer to the EMLB load balancer and VIP Manager type.
	EMLBVIPID = "EMLB"
	// loadbalancerTokenExchangeURL is the default URL to use for Token Exchange to talk to the Equinix Metal Load Balancer API.
	loadbalancerTokenExchnageURL = "https://iam.metalctrl.io/api-keys/exchange" //nolint:gosec
)

var lbMetros = map[string]string{
	"am": "lctnloc-1ttCRz-P8aY0rda9BxOiL",
	"da": "lctnloc--uxs0GLeAELHKV8GxO_AI",
	"dc": "lctnloc-1lJjVT6Zp_Fs4UpW5LWQu",
	"ny": "lctnloc-Vy-1Qpw31mPi6RJQwVf9A",
	"sg": "lctnloc-AxCxpIrNUaaoGBkM02uuw",
	"sv": "lctnloc-H5rl2M2VL5dcFmdxhbEKx",
}

// Pools is a map of a port to Targets.
type Pools map[int32][]Target

// Target is a struct containing an IP address and a port.
type Target struct {
	IP   string
	Port int32
}

// EMLB is a client object for talking to the Equinix Metal Load Balancer API.
type EMLB struct {
	client         *lbaas.APIClient
	metro          string
	projectID      string
	tokenExchanger *TokenExchanger
}

// NewEMLB creates a new Equinix Metal Load Balancer API client object.
func NewEMLB(metalAPIKey, projectID, metro string) *EMLB {
	manager := &EMLB{}
	emlbConfig := lbaas.NewConfiguration()
	emlbConfig.Debug = checkDebugEnabled()

	manager.client = lbaas.NewAPIClient(emlbConfig)
	manager.tokenExchanger = &TokenExchanger{
		metalAPIKey:      metalAPIKey,
		tokenExchangeURL: loadbalancerTokenExchnageURL,
		client:           manager.client.GetConfig().HTTPClient,
	}
	manager.projectID = projectID
	manager.metro = metro

	return manager
}

// ReconcileLoadBalancer creates a new Equinix Metal Load Balancer and associates it with the given ClusterScope.
func (e *EMLB) ReconcileLoadBalancer(ctx context.Context, clusterScope *scope.ClusterScope) error {
	log := ctrl.LoggerFrom(ctx)

	packetCluster := clusterScope.PacketCluster
	clusterName := packetCluster.Name

	// See if the cluster already has an EMLB ID in its packetCluster annotations
	lbID, exists := packetCluster.Annotations[loadBalancerIDAnnotation]
	if !exists {
		lbID = ""
	}

	log.Info("Reconciling EMLB", "Cluster Metro", e.metro, "Cluster Name", clusterName, "Project ID", e.projectID, "Load Balancer ID", lbID)

	// Attempt to create the load balancer
	lb, lbPort, err := e.ensureLoadBalancer(ctx, lbID, getResourceName(clusterName, "capp-vip"), loadBalancerVIPPort)
	if err != nil {
		log.Error(err, "Ensure Load Balancer failed.")
		return err
	}

	log.Info("EMLB ensured", "EMLB IP", lb.GetIps()[0], "EMLB ID", lb.GetId(), "EMLB Port", lbPort.GetNumber())

	// Set the ControlPlaneEndpoint field on the PacketCluster object.
	packetCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: lb.GetIps()[0],
		Port: loadBalancerVIPPort,
	}

	// Get a string version of the EMLB Listener port number
	portNumber := strconv.Itoa(int(lbPort.GetNumber()))

	// Set the packetcluster object's annotations with load balancer info for future reference
	packetCluster.Annotations[loadBalancerIDAnnotation] = lb.GetId()
	packetCluster.Annotations[loadBalancerPortNumberAnnotation] = portNumber
	packetCluster.Annotations[loadBalancerMetroAnnotation] = e.metro

	return nil
}

// ReconcileVIPOrigin adds the external IP of a new device to the EMLB Load balancer origin pool.
func (e *EMLB) ReconcileVIPOrigin(ctx context.Context, machineScope *scope.MachineScope, deviceAddr []corev1.NodeAddress) error {
	log := ctrl.LoggerFrom(ctx)

	packetCluster := machineScope.PacketCluster

	// See if the cluster already has an EMLB ID in its packetCluster annotations.
	lbID, exists := packetCluster.Annotations[loadBalancerIDAnnotation]
	if !exists {
		lbID = ""
	}
	if lbID == "" {
		return fmt.Errorf("no Equinix Metal Load Balancer found in cluster's annotations")
	}

	// Fetch the Load Balancer object.
	lb, _, err := e.getLoadBalancer(ctx, lbID)
	if err != nil {
		return err
	}

	// See if the EMLB already has a Port ID in its packetCluster annotations.
	lbPortNumber, exists := packetCluster.Annotations[loadBalancerPortNumberAnnotation]
	if !exists {
		lbPortNumber = ""
	}
	if lbPortNumber == "" {
		return fmt.Errorf("no Equinix Metal Load Balancer Port Numberfound in cluster's annotations")
	}

	// Get an int version of the listener port number.
	portNumber, err := strconv.ParseInt(lbPortNumber, 10, 32)
	if err != nil {
		return err
	}

	// Get the entire listener port object.
	lbPort, err := e.getLoadBalancerPort(ctx, lbID, int32(portNumber))
	if err != nil {
		return err
	}

	// Fetch the listener port id.
	lbPortID := lbPort.GetId()

	// See if the cluster already has an EMLB Origin Pool ID in its packetCluster annotations.
	lbPoolID, exists := machineScope.PacketMachine.Annotations[loadBalancerPoolIDAnnotation]
	if !exists {
		lbPoolID = ""
	}

	// Get the Load Balancer pool or create it.
	lbPool, err := e.ensureLoadBalancerPool(ctx, lbPoolID, lb.GetName())
	if err != nil {
		log.Error(err, "LB Pool Creation/Validation Failed", "EMLB ID", lbID, "Pool ID", lbPoolID)
		return err
	}

	// Fetch the Pool ID.
	lbPoolID = lbPool.GetId()

	// Note the new Origin Pool ID for future reference
	machineScope.PacketMachine.Annotations[loadBalancerPoolIDAnnotation] = lbPoolID

	// See if the PacketMachine already has an EMLB Origin ID in its packetCluster annotations.
	lbOriginID, exists := machineScope.PacketMachine.Annotations[loadBalancerOriginIDAnnotation]
	if !exists {
		lbOriginID = ""
	}

	// Get the Load Balancer origin or create it.
	lbOrigin, err := e.ensureLoadBalancerOrigin(ctx, lbOriginID, lbPoolID, lb.GetName(), deviceAddr)
	if err != nil {
		log.Error(err, "LB Pool Creation/Validation Failed", "EMLB ID", lbID, "Pool ID", lbPoolID, "Origin ID", lbOriginID)
		return err
	}

	// Fetch the Origin ID.
	lbOriginID = lbOrigin.GetId()

	// Note the PacketMachine's new EMLB Origin ID for future reference
	machineScope.PacketMachine.Annotations[loadBalancerOriginIDAnnotation] = lbOriginID

	// Update the Load Balancer's Listener Port to point at the pool
	lbPort, err = e.updateListenerPort(ctx, lbPoolID, lbPortID)
	if err != nil {
		log.Error(err, "LB Port Update Failed", "EMLB ID", lbID, "Pool ID", lbPoolID, "Port ID", lbPort.GetId())
		return err
	}

	return nil
}

// DeleteLoadBalancer deletes the Equinix Metal Load Balancer associated with a given ClusterScope.
func (e *EMLB) DeleteLoadBalancer(ctx context.Context, clusterScope *scope.ClusterScope) error {
	log := ctrl.LoggerFrom(ctx)

	packetCluster := clusterScope.PacketCluster
	clusterName := packetCluster.Name

	// Make sure the cluster already has an EMLB ID in its packetCluster annotations, otherwise abort.
	lbID, exists := packetCluster.Annotations[loadBalancerIDAnnotation]
	if !exists || (lbID == "") {
		log.Info("no Equinix Metal Load Balancer found in cluster's annotations, skipping EMLB delete")
		return nil
	}

	log.Info("Deleting EMLB", "Cluster Metro", e.metro, "Cluster Name", clusterName, "Project ID", e.projectID, "Load Balancer ID", lbID)

	resp, err := e.deleteLoadBalancer(ctx, lbID)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return nil
		} else {
			log.Error(err, "LB Delete Failed", "EMLB ID", lbID, "Response Body", resp.Body)
			return err
		}
	}

	return nil
}

// DeleteLoadBalancerOrigin deletes the Equinix Metal Load Balancer associated with a given ClusterScope.
func (e *EMLB) DeleteLoadBalancerOrigin(ctx context.Context, machineScope *scope.MachineScope) error {
	// Initially, we're creating a single pool per origin, logic below needs to be updated if we move to a shared load balancer pool model.
	log := ctrl.LoggerFrom(ctx)

	clusterName := machineScope.Cluster.Name

	// Make sure the machine has an EMLB Pool ID in its packetMachine annotations, otherwise abort.
	lbPoolID, exists := machineScope.PacketMachine.Annotations[loadBalancerPoolIDAnnotation]
	if !exists || (lbPoolID == "") {
		return fmt.Errorf("no Equinix Metal Load Balancer Pool found in machine's annotations")
	}

	log.Info("Deleting EMLB Pool", "Cluster Metro", e.metro, "Cluster Name", clusterName, "Project ID", e.projectID, "Pool ID", lbPoolID)

	resp, err := e.deletePool(ctx, lbPoolID)
	if err != nil {
		if resp.StatusCode != http.StatusNotFound {
			return nil
		} else {
			log.Error(err, "LB Pool Delete Failed", "Pool ID", lbPoolID, "Response Body", resp.Body)
			return err
		}
	}

	return nil
}

// getLoadBalancer Returns a Load Balancer object given an id.
func (e *EMLB) getLoadBalancer(ctx context.Context, id string) (*lbaas.LoadBalancer, *http.Response, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)

	LoadBalancer, resp, err := e.client.LoadBalancersApi.GetLoadBalancer(ctx, id).Execute()
	return LoadBalancer, resp, err
}

// getLoadBalancerPort Returns a Load Balancer Port object given an id.
func (e *EMLB) getLoadBalancerPort(ctx context.Context, id string, portNumber int32) (*lbaas.LoadBalancerPort, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)

	LoadBalancerPort, _, err := e.client.PortsApi.GetLoadBalancerPort(ctx, id, portNumber).Execute()
	return LoadBalancerPort, err
}

// getLoadBalancerPool Returns a Load Balancer Pool object given an id.
func (e *EMLB) getLoadBalancerPool(ctx context.Context, id string) (*lbaas.LoadBalancerPool, *http.Response, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)

	LoadBalancerPool, resp, err := e.client.PoolsApi.GetLoadBalancerPool(ctx, id).Execute()
	return LoadBalancerPool, resp, err
}

// EnsureLoadBalancerOrigin takes the devices list of IP addresses in a Load Balancer Origin Pool and ensures an origin
// for the first IPv4 address in the list exists.
func (e *EMLB) ensureLoadBalancerOrigin(ctx context.Context, originID, poolID, lbName string, deviceAddr []corev1.NodeAddress) (*lbaas.LoadBalancerPoolOrigin, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)
	log := ctrl.LoggerFrom(ctx)

	if originID == "" {
		target, err := getExternalIPv4Target(deviceAddr)
		if err != nil {
			return nil, err
		}
		originCreated, _, err := e.createOrigin(ctx, poolID, getResourceName(lbName, "origin"), target)
		if err != nil {
			return nil, err
		}

		originID = originCreated.GetId()
	}

	// Regardless of whether we just created it, fetch the loadbalancer pool object.
	lbOrigins, _, err := e.client.PoolsApi.ListLoadBalancerPoolOrigins(ctx, poolID).Execute()
	if err != nil {
		return nil, err
	}

	// Create a Origin pointer to return later
	var found *lbaas.LoadBalancerPoolOrigin

	// Go through the full list of origins
	for i, lbOrigin := range lbOrigins.Origins {
		// Doesn't match the id, move to the next one
		if lbOrigin.Id != originID {
			continue
		}
		target, err := getExternalIPv4Target(deviceAddr)
		if err != nil {
			return nil, err
		}
		if lbOrigin.Target == target.IP {
			if *lbOrigin.GetPortNumber().Int32 == target.Port {
				found = &lbOrigins.Origins[i]
				break
			}
		}
		log.Info("Pool Origin with ID does not have correct IP address")
		_, err = e.client.OriginsApi.DeleteLoadBalancerOrigin(ctx, lbOrigin.Id).Execute()
		if err != nil {
			return nil, err
		}
		break
	}
	return found, err
}

// ensureLoadBalancerPool checks if the poolID exists and if not, creates it.
func (e *EMLB) ensureLoadBalancerPool(ctx context.Context, poolID, lbName string) (*lbaas.LoadBalancerPool, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)

	// Pool doesn't exist, so let's create it.
	if poolID == "" {
		poolCreated, _, err := e.createPool(ctx, getResourceName(lbName, "pool"))
		if err != nil {
			return nil, err
		}

		poolID = poolCreated.GetId()
	}

	// Regardless of whether we just created it, fetch the loadbalancer pool object.
	lbPool, _, err := e.client.PoolsApi.GetLoadBalancerPool(ctx, poolID).Execute()
	return lbPool, err
}

// ensureLoadBalancer Takes a  Load Balancer id and ensures those pools and ensures it exists.
func (e *EMLB) ensureLoadBalancer(ctx context.Context, lbID, lbname string, portNumber int32) (*lbaas.LoadBalancer, *lbaas.LoadBalancerPort, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)

	// EMLB doesn't exist, so let's create it.
	if lbID == "" {
		locationID, ok := lbMetros[e.metro]
		if !ok {
			return nil, nil, fmt.Errorf("could not determine load balancer location for metro %v; valid values are %v", e.metro, reflect.ValueOf(lbMetros).MapKeys())
		}

		lbCreated, _, err := e.createLoadBalancer(ctx, lbname, locationID, providerID)
		if err != nil {
			return nil, nil, err
		}

		lbID = lbCreated.GetId()
		if lbID == "" {
			return nil, nil, fmt.Errorf("error creating Load Balancer")
		}

		_, _, err = e.createListenerPort(ctx, lbID, getResourceName(lbname, "port"), portNumber)
		if err != nil {
			return nil, nil, err
		}
	}

	// Regardless of whether we just created it, fetch the loadbalancer object.
	lb, _, err := e.getLoadBalancer(ctx, lbID)
	if err != nil {
		return nil, nil, err
	}
	lbPort, err := e.getLoadBalancerPort(ctx, lbID, portNumber)
	if err != nil {
		return nil, nil, err
	}
	return lb, lbPort, err
}

func (e *EMLB) createLoadBalancer(ctx context.Context, lbName, locationID, providerID string) (*lbaas.ResourceCreatedResponse, *http.Response, error) {
	lbCreateRequest := lbaas.LoadBalancerCreate{
		Name:       lbName,
		LocationId: locationID,
		ProviderId: providerID,
	}

	return e.client.ProjectsApi.CreateLoadBalancer(ctx, e.projectID).LoadBalancerCreate(lbCreateRequest).Execute()
}

func (e *EMLB) createListenerPort(ctx context.Context, lbID, portName string, portNumber int32) (*lbaas.ResourceCreatedResponse, *http.Response, error) {
	portRequest := lbaas.LoadBalancerPortCreate{
		Name:   portName,
		Number: portNumber,
	}

	return e.client.PortsApi.CreateLoadBalancerPort(ctx, lbID).LoadBalancerPortCreate(portRequest).Execute()
}

func (e *EMLB) createPool(ctx context.Context, name string) (*lbaas.ResourceCreatedResponse, *http.Response, error) {
	createPoolRequest := lbaas.LoadBalancerPoolCreate{
		Name: name,
		Protocol: lbaas.LoadBalancerPoolCreateProtocol{
			LoadBalancerPoolProtocol: lbaas.LOADBALANCERPOOLPROTOCOL_TCP.Ptr(),
		},
	}
	return e.client.ProjectsApi.CreatePool(ctx, e.projectID).LoadBalancerPoolCreate(createPoolRequest).Execute()
}

func (e *EMLB) createOrigin(ctx context.Context, poolID, originName string, target *Target) (*lbaas.ResourceCreatedResponse, *http.Response, error) {
	createOriginRequest := lbaas.LoadBalancerPoolOriginCreate{
		Name:       originName,
		Target:     target.IP,
		PortNumber: lbaas.Int32AsLoadBalancerPoolOriginPortNumber(&target.Port),
		Active:     true,
		PoolId:     poolID,
	}
	return e.client.PoolsApi.CreateLoadBalancerPoolOrigin(ctx, poolID).LoadBalancerPoolOriginCreate(createOriginRequest).Execute()
}

func (e *EMLB) deleteLoadBalancer(ctx context.Context, lbID string) (*http.Response, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)
	return e.client.LoadBalancersApi.DeleteLoadBalancer(ctx, lbID).Execute()
}

func (e *EMLB) deletePool(ctx context.Context, poolID string) (*http.Response, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)
	return e.client.PoolsApi.DeleteLoadBalancerPool(ctx, poolID).Execute()
}

func (e *EMLB) updateListenerPort(ctx context.Context, poolID, lbPortID string) (*lbaas.LoadBalancerPort, error) {
	ctx = context.WithValue(ctx, lbaas.ContextOAuth2, e.tokenExchanger)

	// Create a listener port update request that adds the provided load balancer origin pool to the listener port.
	portUpdateRequest := lbaas.LoadBalancerPortUpdate{
		AddPoolIds: []string{poolID},
	}

	// Do the actual listener port update.
	lbPort, _, err := e.client.PortsApi.UpdateLoadBalancerPort(ctx, lbPortID).LoadBalancerPortUpdate(portUpdateRequest).Execute()
	if err != nil {
		return nil, err
	}

	return lbPort, nil
}

func getResourceName[T any](loadBalancerName string, resourceType T) string {
	return fmt.Sprintf("%v-%v", loadBalancerName, resourceType)
}

func checkDebugEnabled() bool {
	_, legacyVarIsSet := os.LookupEnv("PACKNGO_DEBUG")
	return legacyVarIsSet
}

func convertToTarget(devaddr corev1.NodeAddress) *Target {
	target := &Target{
		IP:   devaddr.Address,
		Port: loadBalancerVIPPort,
	}

	return target
}

func getExternalIPv4Target(deviceAddr []corev1.NodeAddress) (*Target, error) {
	// Find main external IPv4 address
	// We make the assumption that the first External IPv4 address is the one we want.
	for _, addr := range deviceAddr {
		if addr.Type == corev1.NodeExternalIP {
			ip := net.ParseIP(addr.Address)
			if ip == nil {
				// Invalid IP address in list, move on to the next one.
				continue
			}
			if ip.To4() != nil {
				return convertToTarget(addr), nil
			}
		}
	}

	err := fmt.Errorf("no external IPv4 addresses found")
	return nil, err
}
