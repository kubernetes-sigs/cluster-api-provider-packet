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

package v1alpha4

import (
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
)

// ConvertTo converts this PacketMachine to the Hub version (v1beta1).
func (src *PacketMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.PacketMachine)

	if err := Convert_v1alpha4_PacketMachine_To_v1beta1_PacketMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations.
	restored := &v1beta1.PacketMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status.Conditions = restored.Status.Conditions.DeepCopy()

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *PacketMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.PacketMachine)

	if err := Convert_v1beta1_PacketMachine_To_v1alpha4_PacketMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this PacketMachineList to the Hub version (v1beta1).
func (src *PacketMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.PacketMachineList)
	return Convert_v1alpha4_PacketMachineList_To_v1beta1_PacketMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version.
func (dst *PacketMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.PacketMachineList)
	return Convert_v1beta1_PacketMachineList_To_v1alpha4_PacketMachineList(src, dst, nil)
}
