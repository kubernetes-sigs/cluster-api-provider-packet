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
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	packet "sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"

	infrastructurev1alpha3 "sigs.k8s.io/cluster-api-provider-packet/api/v1alpha3"
	"sigs.k8s.io/cluster-api-provider-packet/controllers"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrastructurev1alpha3.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {

	var (
		metricsAddr          string
		healthAddr           string
		enableLeaderElection bool
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")

	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.StringVar(&healthAddr,
		"health-addr",
		":9440",
		"The address the health endpoint binds to.",
	)

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Machine and cluster operations can create enough events to trigger the event recorder spam filter
	// Setting the burst size higher ensures all events will be recorded and submitted to the API
	broadcaster := record.NewBroadcasterWithCorrelatorOptions(record.CorrelatorOptions{
		BurstSize: 100,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		EventBroadcaster:       broadcaster,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "cad3ba79.cluster.x-k8s.io",
		HealthProbeBindAddress: healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// get a packet client
	client, err := packet.GetClient()
	if err != nil {
		setupLog.Error(err, "unable to get Packet client")
		os.Exit(1)
	}

	if err = (&controllers.PacketClusterReconciler{
		Client:       mgr.GetClient(),
		Log:          ctrl.Log.WithName("controllers").WithName("PacketCluster"),
		Recorder:     mgr.GetEventRecorderFor("packetcluster-controller"),
		PacketClient: client,
		Scheme:       mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PacketCluster")
		os.Exit(1)
	}
	if err = (&controllers.PacketMachineReconciler{
		Client:       mgr.GetClient(),
		Log:          ctrl.Log.WithName("controllers").WithName("PacketMachine"),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("packetmachine-controller"),
		PacketClient: client,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PacketMachine")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
