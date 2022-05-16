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
	// NetworkInfrastructureReadyCondition reports of current status of cluster infrastructure.
	NetworkInfrastructureReadyCondition clusterv1.ConditionType = "NetworkInfrastructureReady"
)

// VIPManagerType describes if the VIP will be managed by CPEM or kube-vip
type VIPManagerType string

// PacketClusterSpec defines the desired state of PacketCluster
type PacketClusterSpec struct {
	// ProjectID represents the Packet Project where this cluster will be placed into
	ProjectID string `json:"projectID"`

	// Facility represents the Packet facility for this cluster
	Facility string `json:"facility,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

	// VIPManager represents whether this cluster uses CPEM or kube-vip to
	// manage its vip for the api server IP
	// +kubebuilder:validation:Enum=CPEM;KUBE_VIP
	// +kubebuilder:default:=CPEM
	VIPManager VIPManagerType `json:"vipManager"`
}

// PacketClusterStatus defines the observed state of PacketCluster
type PacketClusterStatus struct {
	// Ready denotes that the cluster (infrastructure) is ready.
	// +optional
	Ready bool `json:"ready"`

	// Conditions defines current service state of the PacketCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:resource:path=packetclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this PacketCluster belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="PacketCluster ready status"

// PacketCluster is the Schema for the packetclusters API
type PacketCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PacketClusterSpec   `json:"spec,omitempty"`
	Status PacketClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PacketClusterList contains a list of PacketCluster
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
	SchemeBuilder.Register(&PacketCluster{}, &PacketClusterList{})
}
