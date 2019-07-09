/*
Copyright 2019 Packet Inc.

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

package machine

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/machine/machineconfig"
	ca "github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/ca"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/deployer"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/cluster-api-provider-packet/pkg/tokens"
	"github.com/packethost/packngo"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	ProviderName    = "packet"
	defaultTokenTTL = 10 * time.Minute
)

// Add RBAC rules to access cluster-api resources
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=machines;machines/status;machinedeployments;machinedeployments/status;machinesets;machinesets/status;machineclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes;events,verbs=get;list;watch;create;update;patch;delete

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	packetClient        *packet.PacketClient
	machineConfigGetter machineconfig.Getter
	deployer            *deployer.Deployer
	controlPort         int
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	MachineConfigGetter machineconfig.Getter
	Client              *packet.PacketClient
	Deployer            *deployer.Deployer
	ControlPort         int
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		packetClient:        params.Client,
		machineConfigGetter: params.MachineConfigGetter,
		deployer:            params.Deployer,
		controlPort:         params.ControlPort,
	}, nil
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if cluster == nil {
		return fmt.Errorf("cannot create machine in nil cluster")
	}
	if machine == nil {
		return fmt.Errorf("cannot create nil machine")
	}
	machineConfig, err := util.MachineProviderFromProviderConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Unable to read providerSpec from machine config: %v", err)
	}
	// generate userdata from the template
	// first we need to find the correct userdata
	userdataTmpl, err := a.machineConfigGetter.GetUserdata(machineConfig.OS, machine.Spec.Versions)
	if err != nil {
		return fmt.Errorf("Unable to read userdata: %v", err)
	}
	var (
		token  = ""
		role   = "node"
		caCert []byte
		caKey  []byte
	)
	if machine.Spec.Versions.ControlPlane != "" {
		role = "master"
		// generate a cluster CA cert and key
		caCertAndKey, err := ca.GenerateCertAndKey(cluster.Name, "")
		if err != nil {
			return fmt.Errorf("unable to generate CA cert and key: %v", err)
		}
		caCert = caCertAndKey.Certificate
		caKey = caCertAndKey.PrivateKey
	} else {
		coreClient, err := a.deployer.CoreV1Client(cluster)
		if err != nil {
			return fmt.Errorf("failed to retrieve corev1 client for cluster %q: %v", cluster.Name, err)
		}
		// generate a new bootstrap token, then save it as valid
		token, err = tokens.NewBootstrap(coreClient, defaultTokenTTL)
		if err != nil {
			return fmt.Errorf("failed to create or save new bootstrap token: %v", err)
		}
	}

	userdata, err := parseUserdata(userdataTmpl, role, cluster, machine, machineConfig.OS, token, caCert, caKey, a.controlPort)
	if err != nil {
		return fmt.Errorf("Unable to generate userdata: %v", err)
	}

	log.Printf("Creating machine %v for cluster %v.", machine.Name, cluster.Name)
	serverCreateOpts := &packngo.DeviceCreateRequest{
		Hostname:     machine.Spec.Name,
		UserData:     userdata,
		ProjectID:    machineConfig.ProjectID,
		Facility:     machineConfig.Facilities,
		BillingCycle: machineConfig.BillingCycle,
		Plan:         machineConfig.InstanceType,
		OS:           machineConfig.OS,
		Tags: []string{
			util.GenerateMachineTag(string(machine.UID)),
			util.GenerateClusterTag(string(cluster.Name)),
		},
	}

	_, _, err = a.packetClient.Devices.Create(serverCreateOpts)
	if err != nil {
		return fmt.Errorf("failed to create server: %v", err)
	}

	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if cluster == nil {
		return fmt.Errorf("cannot delete machine in nil cluster")
	}
	if machine == nil {
		return fmt.Errorf("cannot delete nil machine")
	}
	log.Printf("Deleting machine %v for cluster %v.", machine.Name, cluster.Name)
	device, err := a.packetClient.GetDevice(machine)
	if err != nil {
		return fmt.Errorf("error retrieving machine status %s: %v", machine.UID, err)
	}
	if device == nil {
		return fmt.Errorf("machine does not exist: %s", machine.UID)
	}

	_, err = a.packetClient.Devices.Delete(device.ID)
	if err != nil {
		return fmt.Errorf("failed to delete the machine: %v", err)
	}

	return nil
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if cluster == nil {
		return fmt.Errorf("cannot update machine in nil cluster")
	}
	if machine == nil {
		return fmt.Errorf("cannot update nil machine")
	}
	log.Printf("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	return fmt.Errorf("TODO: Not yet implemented")
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	if cluster == nil {
		return false, fmt.Errorf("cannot check if machine exists in nil cluster")
	}
	if machine == nil {
		return false, fmt.Errorf("cannot check if nil machine exists")
	}
	log.Printf("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)
	device, err := a.packetClient.GetDevice(machine)
	if err != nil {
		return false, fmt.Errorf("error retrieving machine status %s: %v", machine.UID, err)
	}
	if device == nil {
		return false, nil
	}

	return true, nil
}

// The Machine Actuator interface must implement GetIP and GetKubeConfig functions as a workaround for issues
// cluster-api#158 (https://github.com/kubernetes-sigs/cluster-api/issues/158) and cluster-api#160
// (https://github.com/kubernetes-sigs/cluster-api/issues/160).

// GetIP returns IP address of the machine in the cluster.
func (a *Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cannot get IP of machine in nil cluster")
	}
	if machine == nil {
		return "", fmt.Errorf("cannot get IP of process nil machine")
	}
	log.Printf("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)
	device, err := a.packetClient.GetDevice(machine)
	if err != nil {
		return "", fmt.Errorf("error retrieving machine status %s: %v", machine.UID, err)
	}
	if device == nil {
		return "", fmt.Errorf("machine does not exist: %s", machine.UID)
	}
	// TODO: validate that this address exists, so we don't hit nil pointer
	// TODO: check which address to return
	// TODO: check address format (cidr, subnet, etc.)
	return device.Network[0].Address, nil
}

// GetKubeConfig gets a kubeconfig from the master.
func (a *Actuator) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cannot get kubeconfig for nil cluster")
	}
	if master == nil {
		return "", fmt.Errorf("cannot get kubeconfig for nil master")
	}
	log.Printf("Getting IP of machine %v for cluster %v.", master.Name, cluster.Name)
	return "", fmt.Errorf("TODO: Not yet implemented")
}

func (a *Actuator) get(machine *clusterv1.Machine) (*packngo.Device, error) {
	device, err := a.packetClient.GetDevice(machine)
	if err != nil {
		return nil, err
	}
	if device != nil {
		return device, nil
	}

	return nil, fmt.Errorf("Device %s not found", machine.UID)
}
