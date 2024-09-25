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

package v1beta1

import (
	metal "github.com/equinix/equinix-sdk-go/services/metalv1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineFinalizer allows ReconcilePacketMachine to clean up Packet resources before
	// removing it from the apiserver.
	MachineFinalizer = "packetmachine.infrastructure.cluster.x-k8s.io"
)

const (
	// DeviceReadyCondition reports on current status of the Equinix Metal device. Ready indicates the instance is in a Running state.
	DeviceReadyCondition clusterv1.ConditionType = "InstanceReady"

	// InstanceNotFoundReason used when the instance couldn't be retrieved.
	InstanceNotFoundReason = "InstanceNotFound"
	// InstanceTerminatedReason instance is in a terminated state.
	InstanceTerminatedReason = "InstanceTerminated"
	// InstanceStoppedReason instance is in a stopped state.
	InstanceStoppedReason = "InstanceStopped"
	// InstanceNotReadyReason used when the instance is in a pending state.
	InstanceNotReadyReason = "InstanceNotReady"
	// InstanceProvisionStartedReason set when the provisioning of an instance started.
	InstanceProvisionStartedReason = "InstanceProvisionStarted"
	// InstanceProvisionFailedReason used for failures during instance provisioning.
	InstanceProvisionFailedReason = "InstanceProvisionFailed"
	// WaitingForClusterInfrastructureReason used when machine is waiting for cluster infrastructure to be ready before proceeding.
	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"
	// WaitingForBootstrapDataReason used when machine is waiting for bootstrap data to be ready before proceeding.
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"
)

// PacketMachineSpec defines the desired state of PacketMachine.
type PacketMachineSpec struct {
	OS           string                              `json:"os"`
	BillingCycle metal.DeviceCreateInputBillingCycle `json:"billingCycle,omitempty"`
	MachineType  string                              `json:"machineType"`
	SSHKeys      []string                            `json:"sshKeys,omitempty"`

	// Facility represents the Packet facility for this machine.
	// Override from the PacketCluster spec.
	// +optional
	Facility string `json:"facility,omitempty"`

	// Metro represents the Packet metro for this machine
	// Override from the PacketCluster spec.
	// +optional
	Metro string `json:"metro,omitempty"`

	// IPXEUrl can be used to set the pxe boot url when using custom OSes with this provider.
	// Note that OS should also be set to "custom_ipxe" if using this value.
	// +optional
	IPXEUrl string `json:"ipxeURL,omitempty"`

	// HardwareReservationID is the unique device hardware reservation ID, a comma separated list of
	// hardware reservation IDs, or `next-available` to automatically let the Packet api determine one.
	// +optional
	HardwareReservationID string `json:"hardwareReservationID,omitempty"`

	// ProviderID is the unique identifier as specified by the cloud provider.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Tags is an optional set of tags to add to Packet resources managed by the Packet provider.
	// +optional
	Tags Tags `json:"tags,omitempty"`

	// NetworkPorts is an optional set of configurations for configuring layer2 seetings in a machine. 
	// +optional
	NetworkPorts []*Port `json:"ports,omitempty"`
	// List of Routes to be configured on the Packet Machine
    // +optional
	Routes      []*RouteSpec     `json:"routes,omitempty"`
}
// Port defines the Layer2(VLAN) Configuration that needs to be done on a port (eg: bond0).
type Port struct {
	// name of the port e.g bond0,eth0 and eth1 for 2 NIC servers.
	Name string `json:"name"`
	// port bonded or not.
	Bonded bool `json:"bonded,omitempty"`
	// convert port to layer 2. is false by default on new devices. changes result in /ports/id/convert/layer-[2|3] API calls
	Layer2 bool `json:"layer2,omitempty"`
	// Network configurations for the port
    Networks []Network `json:"networks"`	
}

// Network defines the network configuration for a port.
type Network struct {
    // network ip address range to reserve for these ports.
    // for eg: can be carved out of a VRF IP Range.
    Address string `json:"address"`
    // VLANs for EM API to find by vxlan, project, and metro match then attach to device. OS userdata template will also configure this VLAN on the bond device
    VXLAN int `json:"vxlan,omitempty"`
    // AssignmentRange is the range of IP addresses to assign to the machine from the specified IP address range.
    // for eg: if the IP address range is 10.60.10.0/24 , the assignment range can be '10.60.10.2-10.60.10.8'
    // If not specified, the first available IP address from the IP address range will be assigned.
    // This is useful when you want to reserve some IP addresses for other purposes for eg Gateways, DNS etc.
    // +optional
    AssignmentRange string `json:"assignmentRange,omitempty"`
	// Netmask is the netmask for the network.
	// eg: 255.255.255.248
	Netmask string `json:"netmask,omitempty"`
    // AddressType is the type of address to assign to the machine. It can be either Internal or External.
    // kubebuilder:validation:Enum=Internal;External
    AddressType string `json:"addressType,omitempty"`
}

// RouteSpec defines the static route configuration for a PacketMachine.
type RouteSpec struct {
    Destination string `json:"destination"`
    Gateway     string `json:"gateway"`
}

// PacketMachineStatus defines the observed state of PacketMachine.
type PacketMachineStatus struct {
	// Ready is true when the provider resource is ready.
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the Packet device associated addresses.
	Addresses []corev1.NodeAddress `json:"addresses,omitempty"`

	// InstanceStatus is the status of the Packet device instance for this machine.
	// +optional
	InstanceStatus *PacketResourceStatus `json:"instanceStatus,omitempty"`

	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureReason *capierrors.MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions defines current service state of the PacketMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=packetmachines,shortName=pma,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this PacketMachine belongs"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState",description="Packet instance state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="InstanceID",type="string",JSONPath=".spec.providerID",description="Packet instance ID"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this PacketMachine"

// PacketMachine is the Schema for the packetmachines API.
type PacketMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PacketMachineSpec   `json:"spec,omitempty"`
	Status PacketMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PacketMachineList contains a list of PacketMachine.
type PacketMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PacketMachine `json:"items"`
}

// GetConditions returns the list of conditions for an PacketMachine API object.
func (m *PacketMachine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions will set the given conditions on an PacketMachine object.
func (m *PacketMachine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

func init() {
	objectTypes = append(objectTypes, &PacketMachine{}, &PacketMachineList{})
}
