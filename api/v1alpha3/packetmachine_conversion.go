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
	unsafe "unsafe"

	apiconversion "k8s.io/apimachinery/pkg/conversion"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha4 "sigs.k8s.io/cluster-api-provider-packet/api/v1alpha4"
)

// ConvertTo converts this PacketMachine to the Hub version (v1alpha4).
func (src *PacketMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.PacketMachine)

	if err := Convert_v1alpha3_PacketMachine_To_v1alpha4_PacketMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations.
	restored := &v1alpha4.PacketMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status.Conditions = restored.Status.Conditions.DeepCopy()

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha4) to this version.
func (dst *PacketMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.PacketMachine)

	if err := Convert_v1alpha4_PacketMachine_To_v1alpha3_PacketMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this PacketMachineList to the Hub version (v1alpha4).
func (src *PacketMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.PacketMachineList)
	return Convert_v1alpha3_PacketMachineList_To_v1alpha4_PacketMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1alpha4) to this version.
func (dst *PacketMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.PacketMachineList)
	return Convert_v1alpha4_PacketMachineList_To_v1alpha3_PacketMachineList(src, dst, nil)
}

// Convert_v1alpha3_PacketMachineSpec_To_v1alpha4_PacketMachineSpec handles the manual conversion steps necessary.
func Convert_v1alpha3_PacketMachineSpec_To_v1alpha4_PacketMachineSpec(in *PacketMachineSpec, out *v1alpha4.PacketMachineSpec, s apiconversion.Scope) error {
	out.SSHKeys = *(*[]string)(unsafe.Pointer(&in.SshKeys))
	return autoConvert_v1alpha3_PacketMachineSpec_To_v1alpha4_PacketMachineSpec(in, out, s)
}

// Convert_v1alpha4_PacketMachineSpec_To_v1alpha3_PacketMachineSpec handles the manual conversion steps necessary.
func Convert_v1alpha4_PacketMachineSpec_To_v1alpha3_PacketMachineSpec(in *v1alpha4.PacketMachineSpec, out *PacketMachineSpec, s apiconversion.Scope) error {
	out.SshKeys = *(*[]string)(unsafe.Pointer(&in.SSHKeys))
	return autoConvert_v1alpha4_PacketMachineSpec_To_v1alpha3_PacketMachineSpec(in, out, s)
}

func Convert_v1alpha4_PacketMachineStatus_To_v1alpha3_PacketMachineStatus(in *v1alpha4.PacketMachineStatus, out *PacketMachineStatus, s apiconversion.Scope) error {
	// v1alpha3 is missing Status.Conditions, but this is restored in PacketCluster.ConvertTo
	out.ErrorMessage = in.FailureMessage
	out.ErrorReason = in.FailureReason
	return autoConvert_v1alpha4_PacketMachineStatus_To_v1alpha3_PacketMachineStatus(in, out, s)
}

func Convert_v1alpha3_PacketMachineStatus_To_v1alpha4_PacketMachineStatus(in *PacketMachineStatus, out *v1alpha4.PacketMachineStatus, s apiconversion.Scope) error {
	out.FailureMessage = in.ErrorMessage
	out.FailureReason = in.ErrorReason
	return autoConvert_v1alpha3_PacketMachineStatus_To_v1alpha4_PacketMachineStatus(in, out, s)
}
