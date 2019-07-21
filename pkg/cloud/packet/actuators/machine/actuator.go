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
	"strings"
	"time"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/machine/machineconfig"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/deployer"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/packngo"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	capiutil "sigs.k8s.io/cluster-api/pkg/util"
)

const (
	ProviderName              = "packet"
	retryIntervalMachineReady = 10 * time.Second
	timeoutMachineReady       = 10 * time.Minute
)

// Add RBAC rules to access cluster-api resources
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=machines;machines/status;machinedeployments;machinedeployments/status;machinesets;machinesets/status;machineclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes;events,verbs=get;list;watch;create;update;patch;delete

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	machinesGetter      client.MachinesGetter
	packetClient        *packet.PacketClient
	machineConfigGetter machineconfig.Getter
	deployer            *deployer.Deployer
	controlPort         int
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	MachinesGetter      client.MachinesGetter
	MachineConfigGetter machineconfig.Getter
	Client              *packet.PacketClient
	Deployer            *deployer.Deployer
	ControlPort         int
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		machinesGetter:      params.MachinesGetter,
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
	clusterConfig, err := util.ClusterProviderFromProviderConfig(cluster.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("unable to unpack cluster provider: %v", err)
	}

	tags := []string{
		util.GenerateMachineTag(string(machine.UID)),
		util.GenerateClusterTag(string(cluster.Name)),
	}
	// generate userdata from the template
	// first we need to find the correct userdata
	userdataTmpl, containerRuntime, err := a.machineConfigGetter.GetUserdata(machineConfig.OS, machine.Spec.Versions)
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
		caCert = clusterConfig.CAKeyPair.Cert
		caKey = clusterConfig.CAKeyPair.Key
		if len(caCert) == 0 {
			return fmt.Errorf("CA Certificate not yet created")
		}
		if len(caKey) == 0 {
			return fmt.Errorf("CA Key not yet created")
		}
		tags = append(tags, util.MasterTag)
	} else {
		token, err = a.deployer.NewBootstrapToken(cluster)
		if err != nil {
			return fmt.Errorf("failed to create and save token for cluster %q: %v", cluster.Name, err)
		}
		tags = append(tags, util.WorkerTag)
	}

	userdata, err := parseUserdata(userdataTmpl, role, cluster, machine, machineConfig.OS, token, caCert, caKey, a.controlPort, containerRuntime)
	if err != nil {
		return fmt.Errorf("Unable to generate userdata: %v", err)
	}

	log.Printf("Creating machine %v for cluster %v.", machine.Name, cluster.Name)
	serverCreateOpts := &packngo.DeviceCreateRequest{
		Hostname:     machine.Name,
		UserData:     userdata,
		ProjectID:    machineConfig.ProjectID,
		Facility:     machineConfig.Facilities,
		BillingCycle: machineConfig.BillingCycle,
		Plan:         machineConfig.InstanceType,
		OS:           machineConfig.OS,
		Tags:         tags,
	}

	device, _, err := a.packetClient.Devices.Create(serverCreateOpts)
	if err != nil {
		return fmt.Errorf("failed to create server: %v", err)
	}

	// we need to loop here until the device exists and has an IP address
	log.Printf("Created device, waiting for it to be ready")
	a.waitForMachineReady(device)

	// add the annotations so that cluster-api knows it is there (also, because it is useful to have)
	if machine.Annotations == nil {
		machine.Annotations = map[string]string{}
	}

	machine.Annotations["cluster-api-provider-packet"] = "true"
	machine.Annotations["cluster.k8s.io/machine"] = cluster.Name

	if _, err = a.updateMachine(cluster, machine); err != nil {
		return fmt.Errorf("error updating Machine object with annotations: %v", err)
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
	c, err := util.ClusterProviderFromProviderConfig(cluster.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("unable to unpack cluster provider: %v", err)
	}

	log.Printf("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	// how to update the machine:
	// - get the current parameters of the machine from the API
	// - check if anything immutable has changed, in which case we return an error
	// - check if anything mutable has changed, in which case we update the instance
	device, err := a.packetClient.GetDevice(machine)
	if err != nil {
		return fmt.Errorf("error retrieving machine status %s: %v", machine.UID, err)
	}
	if device == nil {
		return fmt.Errorf("received nil machine")
	}
	// check immutable
	if err := changedImmutable(c.ProjectID, device, machine); err != nil {
		return err
	}
	// check mutable
	update, err := changedMutable(c.ProjectID, device, machine)
	if err != nil {
		return fmt.Errorf("error checking changed mutable information: %v", err)
	}
	if update != nil {
		_, _, err := a.packetClient.Devices.Update(device.ID, update)
		if err != nil {
			return fmt.Errorf("failed to update device %s: %v", device.Hostname, err)
		}
	}

	return nil
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

func (a *Actuator) updateMachine(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	var (
		machineClient  client.MachineInterface
		updatedMachine *clusterv1.Machine
		err            error
	)
	if a.machinesGetter != nil {
		machineClient = a.machinesGetter.Machines(cluster.Namespace)
	}
	if updatedMachine, err = machineClient.Update(machine); err != nil {
		return nil, err
	}
	return updatedMachine, nil
}

func (a *Actuator) waitForMachineReady(device *packngo.Device) error {
	err := capiutil.PollImmediate(retryIntervalMachineReady, timeoutMachineReady, func() (bool, error) {
		fmt.Printf("Waiting for device %v to become ready...", device.ID)
		dev, _, err := a.packetClient.Devices.Get(device.ID, nil)
		if err != nil {
			return false, nil
		}

		ready := dev.Network == nil || len(dev.Network) == 0 || dev.Network[0].Address == ""
		return ready, nil
	})

	return err
}

func changedImmutable(projectID string, device *packngo.Device, machine *clusterv1.Machine) error {
	errors := []string{}
	machineConfig, err := util.MachineProviderFromProviderConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("Unable to read providerSpec from machine config: %v", err)
	}
	// immutable: Facility, MachineType, ProjectID, OS, BillingCycle
	facility := device.Facility.Code
	if !util.ItemInList(machineConfig.Facilities, facility) {
		errors = append(errors, "Facility")
	}
	if device.Plan.Name != machineConfig.InstanceType {
		errors = append(errors, "MachineType")
	}
	if projectID != machineConfig.ProjectID {
		errors = append(errors, "ProjectID")
	}
	// it could be indicated in one of several ways
	if device.OS.Slug != machineConfig.OS && device.OS.Name != machineConfig.OS {
		errors = append(errors, "OS")
	}
	if device.BillingCycle != machineConfig.BillingCycle {
		errors = append(errors, "BillingCycle")
	}
	if len(errors) > 0 {
		return fmt.Errorf("attempted to change immutable characteristics: %s", strings.Join(errors, ","))
	}
	return nil
}
func changedMutable(projectID string, device *packngo.Device, machine *clusterv1.Machine) (*packngo.DeviceUpdateRequest, error) {
	update := packngo.DeviceUpdateRequest{}
	updated := false
	// mutable: Hostname (machine.Name), Roles, SshKeys
	if device.Hostname != machine.Name {
		// copy it to get a clean pointer
		newName := machine.Name
		update.Hostname = &newName
		updated = true
	}
	if updated {
		return &update, nil
	}
	// Roles and SshKeys affect the userdata, which we would update
	// TODO: Update userdata based on Roles and SshKeys
	return nil, nil
}
