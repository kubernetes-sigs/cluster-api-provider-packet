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

package v1alpha3

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineFinalizer allows ReconcilePacketMachine to clean up Packet resources before
	// removing it from the apiserver.
	MachineFinalizer = "packetmachine.infrastructure.cluster.x-k8s.io"
)

// PacketMachineSpec defines the desired state of PacketMachine
type PacketMachineSpec struct {
	OS           string   `json:"OS"` //nolint: tagliatelle
	BillingCycle string   `json:"billingCycle"`
	MachineType  string   `json:"machineType"`
	SshKeys      []string `json:"sshKeys,omitempty"` //nolint: stylecheck

	// Facility represents the Packet facility for this cluster.
	// Override from the PacketCluster spec.
	// +optional
	Facility string `json:"facility,omitempty"`

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
}

// PacketMachineStatus defines the observed state of PacketMachine
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
	ErrorReason *capierrors.MachineStatusError `json:"errorReason,omitempty"`

	// ErrorMessage will be set in the event that there is a terminal problem
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
	ErrorMessage *string `json:"errorMessage,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=packetmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this PacketMachine belongs"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState",description="Packet instance state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="InstanceID",type="string",JSONPath=".spec.providerID",description="Packet instance ID"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this PacketMachine"

// PacketMachine is the Schema for the packetmachines API
type PacketMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PacketMachineSpec   `json:"spec,omitempty"`
	Status PacketMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PacketMachineList contains a list of PacketMachine
type PacketMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PacketMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PacketMachine{}, &PacketMachineList{})
}
