/*
Copyright 2019 Packet Inc.

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
	"os"

	"github.com/packethost/cluster-api-provider-packet/pkg/apis"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/cluster"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/machine"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/actuators/machine/machineconfig"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/deployer"
	"k8s.io/klog"
	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	capicluster "sigs.k8s.io/cluster-api/pkg/controller/cluster"
	capimachine "sigs.k8s.io/cluster-api/pkg/controller/machine"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	controlPort = 6443
)

func main() {
	klog.InitFlags(nil)

	cfg := config.GetConfigOrDie()
	if cfg == nil {
		klog.Fatalf("GetConfigOrDie didn't die")
	}

	metricsAddr := flag.String("metrics-addr", ":8080", "The address the metric endpoint binds to.")

	machineSetupConfig := flag.String("config", "/etc/machineconfig/machine_configs.yaml", "path to the machine setup config")
	flag.Parse()

	log := logf.Log.WithName("packet-controller-manager")
	logf.SetLogger(logf.ZapLogger(false))
	entryLog := log.WithName("entrypoint")

	// Setup a Manager
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: *metricsAddr})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf(err.Error())
	}

	// get a packet client
	client, err := packet.GetClient()
	if err != nil {
		klog.Fatalf("unable to get Packet client: %v", err)
	}
	// get a deployer, which is needed at various stages
	deployer, err := deployer.New(deployer.Params{
		Client: client,
		Port:   controlPort,
	})
	if err != nil {
		klog.Fatalf(err.Error())
	}

	clusterInterface := cs.ClusterV1alpha1()
	clusterActuator, err := cluster.NewActuator(cluster.ActuatorParams{
		ClustersGetter: clusterInterface,
		Deployer:       deployer,
	})
	if err != nil {
		klog.Fatalf(err.Error())
	}

	// get our config file, create a getter for it, and pass it on
	getter, err := machineconfig.NewFileGetter(*machineSetupConfig)
	if err != nil {
		klog.Fatalf(err.Error())
	}

	machineActuator, err := machine.NewActuator(machine.ActuatorParams{
		MachinesGetter:      clusterInterface,
		MachineConfigGetter: getter,
		Client:              client,
		Deployer:            deployer,
	})
	if err != nil {
		klog.Fatalf(err.Error())
	}

	// Register our cluster deployer (the interface is in clusterctl and we define the Deployer interface on the actuator)
	common.RegisterClusterProvisioner("packet", clusterActuator)

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf(err.Error())
	}

	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf(err.Error())
	}

	capimachine.AddWithActuator(mgr, machineActuator)

	capicluster.AddWithActuator(mgr, clusterActuator)

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
