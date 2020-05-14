package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PacketMachineTemplateSpec defines the desired state of PacketMachineTemplate
type PacketMachineTemplateSpec struct {
	Template PacketMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=packetmachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// PacketMachineTemplate is the Schema for the packetmachinetemplates API
type PacketMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PacketMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// PacketMachineTemplateList contains a list of PacketMachineTemplate
type PacketMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PacketMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PacketMachineTemplate{}, &PacketMachineTemplateList{})
}
