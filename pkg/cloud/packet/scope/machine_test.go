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

package scope

import (
	"testing"

	. "github.com/onsi/gomega"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
)

func TestNewMachineScopeNoClient(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(
		MachineScopeParams{
			Cluster:       new(clusterv1.Cluster),
			Machine:       new(clusterv1.Machine),
			PacketCluster: new(infrav1.PacketCluster),
			PacketMachine: new(infrav1.PacketMachine),
		})
	g.Expect(err).To(MatchError(ErrMissingClient))
}

func TestNewMachineScopeNoCluster(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(
		MachineScopeParams{
			Client:        fake.NewClientBuilder().Build(),
			Machine:       new(clusterv1.Machine),
			PacketCluster: new(infrav1.PacketCluster),
			PacketMachine: new(infrav1.PacketMachine),
		})
	g.Expect(err).To(MatchError(ErrMissingCluster))
}

func TestNewMachineScopeNoMachine(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(
		MachineScopeParams{
			Client:        fake.NewClientBuilder().Build(),
			Cluster:       new(clusterv1.Cluster),
			PacketCluster: new(infrav1.PacketCluster),
			PacketMachine: new(infrav1.PacketMachine),
		})
	g.Expect(err).To(MatchError(ErrMissingMachine))
}

func TestNewMachineScopeNoPacketCluster(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(
		MachineScopeParams{
			Client:        fake.NewClientBuilder().Build(),
			Cluster:       new(clusterv1.Cluster),
			Machine:       new(clusterv1.Machine),
			PacketMachine: new(infrav1.PacketMachine),
		})
	g.Expect(err).To(MatchError(ErrMissingPacketCluster))
}

func TestNewMachineScopeNoPacketMachine(t *testing.T) {
	g := NewWithT(t)

	_, err := NewMachineScope(
		MachineScopeParams{
			Client:        fake.NewClientBuilder().Build(),
			Cluster:       new(clusterv1.Cluster),
			Machine:       new(clusterv1.Machine),
			PacketCluster: new(infrav1.PacketCluster),
		})
	g.Expect(err).To(MatchError(ErrMissingPacketMachine))
}
