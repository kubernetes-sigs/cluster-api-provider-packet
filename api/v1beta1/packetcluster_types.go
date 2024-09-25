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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// ClusterFinalizer allows DockerClusterReconciler to clean up resources associated with DockerCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "packetcluster.infrastructure.cluster.x-k8s.io"
	// NetworkInfrastructureReadyCondition reports of current status of cluster infrastructure.
	NetworkInfrastructureReadyCondition clusterv1.ConditionType = "NetworkInfrastructureReady"
	// EMLBVIPID is the string used to refer to the EMLB load balancer and VIP Manager type.
	EMLBVIPID = "EMLB"
	// CPEMID is the string used to refer to the CPEM load balancer and VIP Manager type.
	CPEMID = "CPEM"
	// KUBEVIPID is the string used to refer to the Kube VIP load balancer and VIP Manager type.
	KUBEVIPID = "KUBE_VIP"
)

// AssignmentType describes the component responsible for allocating IP addresses to the machines.
type AssignmentType string

const (
    AssignmentClusterAPI AssignmentType = "cluster-api"
    AssignmentDHCP       AssignmentType = "dhcp"
)

// VIPManagerType describes if the VIP will be managed by CPEM or kube-vip or Equinix Metal Load Balancer.
type VIPManagerType string

// PacketClusterSpec defines the desired state of PacketCluster.
type PacketClusterSpec struct {
	// ProjectID represents the Packet Project where this cluster will be placed into
	ProjectID string `json:"projectID"`

	// Facility represents the Packet facility for this cluster
	// +optional
	Facility string `json:"facility,omitempty"`

	// Metro represents the Packet metro for this cluster
	// +optional
	Metro string `json:"metro,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

	// VIPManager represents whether this cluster uses CPEM or kube-vip or Equinix Metal Load Balancer to
	// manage its vip for the api server IP
	// +kubebuilder:validation:Enum=CPEM;KUBE_VIP;EMLB
	// +kubebuilder:default:=CPEM
	VIPManager VIPManagerType `json:"vipManager"`

	// Networks is a list of network configurations for the PacketCluster
    Networks []NetworkSpec `json:"networks,omitempty"`
}

// NetworkSpec defines the network configuration for a PacketCluster.
type NetworkSpec struct {
    // Name of the network, e.g. "storage VLAN", is optional
    // +optional
    Name        string         	`json:"name,omitempty"`
    // Description of the network, e.g. "Storage network", is optional
    // +optional
    Description string         	`json:"description,omitempty"`
    // AddressRange for the cluster network for eg: VRF IP Ranges
    Addresses []string      	`json:"addresses,omitempty"`
    // Assignment is component responsible for allocating IP addresses to the machines, either cluster-api or dhcp
	// +kubebuilder:validation:Enum=cluster-api;dhcp
    Assignment  AssignmentType 	`json:"assignment,omitempty"`
}

// PacketClusterStatus defines the observed state of PacketCluster.
type PacketClusterStatus struct {
	// Ready denotes that the cluster (infrastructure) is ready.
	// +optional
	Ready bool `json:"ready"`

	// Conditions defines current service state of the PacketCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:resource:path=packetclusters,shortName=pcl,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this PacketCluster belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="PacketCluster ready status"

// PacketCluster is the Schema for the packetclusters API.
type PacketCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PacketClusterSpec   `json:"spec,omitempty"`
	Status PacketClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PacketClusterList contains a list of PacketCluster.
type PacketClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PacketCluster `json:"items"`
}

// GetConditions returns the list of conditions for an PacketCluster API object.
func (c *PacketCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions will set the given conditions on an PacketCluster object.
func (c *PacketCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

func init() {
	objectTypes = append(objectTypes, &PacketCluster{}, &PacketClusterList{})
}
