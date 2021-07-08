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
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	apiv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	apiv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha4 "sigs.k8s.io/cluster-api-provider-packet/api/v1alpha4"
)

// ConvertTo converts this PacketCluster to the Hub version (v1alpha4).
func (src *PacketCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.PacketCluster)

	if err := Convert_v1alpha3_PacketCluster_To_v1alpha4_PacketCluster(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data from annotations.
	restored := &v1alpha4.PacketCluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status.Conditions = restored.Status.Conditions.DeepCopy()

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha4) to this version.
func (dst *PacketCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.PacketCluster)

	if err := Convert_v1alpha4_PacketCluster_To_v1alpha3_PacketCluster(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts this PacketClusterList to the Hub version (v1alpha4).
func (src *PacketClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha4.PacketClusterList)
	return Convert_v1alpha3_PacketClusterList_To_v1alpha4_PacketClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1alpha4) to this version.
func (dst *PacketClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.PacketClusterList)
	return Convert_v1alpha4_PacketClusterList_To_v1alpha3_PacketClusterList(src, dst, nil)
}

func Convert_v1alpha4_PacketClusterStatus_To_v1alpha3_PacketClusterStatus(in *v1alpha4.PacketClusterStatus, out *PacketClusterStatus, s apiconversion.Scope) error {
	// v1alpha3 is missing Status.Conditions, but this is restored in PacketCluster.ConvertTo
	return autoConvert_v1alpha4_PacketClusterStatus_To_v1alpha3_PacketClusterStatus(in, out, s)
}

// Convert_v1alpha3_APIEndpoint_To_v1alpha4_APIEndpoint.
func Convert_v1alpha3_APIEndpoint_To_v1alpha4_APIEndpoint(in *apiv1alpha3.APIEndpoint, out *apiv1alpha4.APIEndpoint, s apiconversion.Scope) error {
	return apiv1alpha3.Convert_v1alpha3_APIEndpoint_To_v1alpha4_APIEndpoint(in, out, s)
}

// Convert_v1alpha4_APIEndpoint_To_v1alpha3_APIEndpoint.
func Convert_v1alpha4_APIEndpoint_To_v1alpha3_APIEndpoint(in *apiv1alpha4.APIEndpoint, out *apiv1alpha3.APIEndpoint, s apiconversion.Scope) error {
	return apiv1alpha3.Convert_v1alpha4_APIEndpoint_To_v1alpha3_APIEndpoint(in, out, s)
}
