/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"flag"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/machine"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/machine/machineconfig"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
)

func main() {
	var err error

	machineSetupConfig := flag.String("config", "/etc/machineconfig/machine_configs.yaml", "path to the machine setup config")
	flag.Parse()

	// get our config file, create a getter for it, and pass it on
	getter, err := machineconfig.NewFileGetter(*machineSetupConfig)
	if err != nil {
		klog.Fatalf(err.Error())
	}

	// get a packet client
	client, err := packet.GetClient()
	if err != nil {
		klog.Fatalf("unable to get Packet client: %v", err)
	}

	machineActuator, err := machine.NewActuator(machine.ActuatorParams{
		MachineConfigGetter: getter,
		Client:              client,
	})
	if err != nil {
		klog.Fatalf(err.Error())
	}
	if err != nil {
		klog.Fatalf("Error creating cluster provisioner for packet : %v", err)
	}
	common.RegisterClusterProvisioner("packet", machineActuator)
	cmd.Execute()
}
