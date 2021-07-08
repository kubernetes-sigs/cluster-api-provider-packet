/*
Copyright 2021 The Kubernetes Authors.

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
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha4 "sigs.k8s.io/cluster-api-provider-packet/api/v1alpha4"
)

// ConvertTo converts this PacketMachineTemplate to the Hub version (v1alpha4).
func (src *PacketMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.PacketMachineTemplate)

	if err := Convert_v1alpha3_PacketMachineTemplate_To_v1alpha4_PacketMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha4) to this version.
func (dst *PacketMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.PacketMachineTemplate)

	if err := Convert_v1alpha4_PacketMachineTemplate_To_v1alpha3_PacketMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this PacketMachineTemplateList to the Hub version (v1alpha4).
func (src *PacketMachineTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.PacketMachineTemplateList)
	return Convert_v1alpha3_PacketMachineTemplateList_To_v1alpha4_PacketMachineTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1alpha4) to this version.
func (dst *PacketMachineTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.PacketMachineTemplateList)
	return Convert_v1alpha4_PacketMachineTemplateList_To_v1alpha3_PacketMachineTemplateList(src, dst, nil)
}
