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

// Provides the clusterapi types and reconciler for defining and managing clusters deployed on Equinix Metal devices.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cgrecord "k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/version"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/flags"
	"sigs.k8s.io/cluster-api/util/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-packet/controllers"
	packet "sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
	// +kubebuilder:scaffold:imports
)

var (
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	controllerName = "cluster-api-packet-controller-manager"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
}

var (
	enableLeaderElection        bool
	leaderElectionNamespace     string
	watchNamespace              string
	profilerAddress             string
	healthAddr                  string
	watchFilterValue            string
	webhookCertDir              string
	packetClusterConcurrency    int
	packetMachineConcurrency    int
	webhookPort                 int
	syncPeriod                  time.Duration
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	enableContentionProfiling   bool
	restConfigQPS               float32
	restConfigBurst             int
	tlsOptions                  = flags.TLSOptions{}
	diagnosticsOptions          = flags.DiagnosticsOptions{}
	logOptions                  = logs.NewOptions()
)

// Add RBAC for the authorized diagnostics endpoint.
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func main() {
	initFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	// Set log level 2 as default.
	if err := pflag.CommandLine.Set("v", "2"); err != nil {
		setupLog.Error(err, "failed to set log level: %v")
		os.Exit(1)
	}
	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	// Machine and cluster operations can create enough events to trigger the event recorder spam filter
	// Setting the burst size higher ensures all events will be recorded and submitted to the API
	broadcaster := cgrecord.NewBroadcasterWithCorrelatorOptions(cgrecord.CorrelatorOptions{
		BurstSize: 100,
	})

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent(controllerName)

	tlsOptionOverrides, err := flags.GetTLSOptionOverrideFuncs(tlsOptions)
	if err != nil {
		setupLog.Error(err, "unable to add TLS settings to the webhook server")
		os.Exit(1)
	}

	diagnosticsOpts := flags.GetDiagnosticsOptions(diagnosticsOptions)

	var watchNamespaces map[string]cache.Config
	if watchNamespace != "" {
		setupLog.Info("Watching cluster-api objects only in namespace for reconciliation", "namespace", watchNamespace)
		watchNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	if enableContentionProfiling {
		goruntime.SetBlockProfileRate(1)
	}

	ctrlOptions := ctrl.Options{
		Scheme:                     scheme,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "controller-leader-election-capp",
		LeaseDuration:              &leaderElectionLeaseDuration,
		RenewDeadline:              &leaderElectionRenewDeadline,
		RetryPeriod:                &leaderElectionRetryPeriod,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		HealthProbeBindAddress:     healthAddr,
		PprofBindAddress:           profilerAddress,
		Metrics:                    diagnosticsOpts,
		Cache: cache.Options{
			DefaultNamespaces: watchNamespaces,
			SyncPeriod:        &syncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:    webhookPort,
				CertDir: webhookCertDir,
				TLSOpts: tlsOptionOverrides,
			},
		),
		EventBroadcaster: broadcaster,
	}

	mgr, err := ctrl.NewManager(restConfig, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupReconcilers(ctx, mgr)
	setupWebhooks(mgr)

	if profilerAddress != "" {
		setupLog.Info(fmt.Sprintf("Profiler listening for requests at %s", profilerAddress))
		go func() {
			srv := http.Server{Addr: profilerAddress, ReadHeaderTimeout: 2 * time.Second}
			if err := srv.ListenAndServe(); err != nil {
				setupLog.Error(err, "problem running profiler server")
			}
		}()
	}

	// Initialize event recorder.
	record.InitFromRecorder(mgr.GetEventRecorderFor("packet-controller"))

	setupLog.Info("starting manager", "version", version.Get().String())
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
	// get a packet client
	client, err := packet.GetClient()
	if err != nil {
		setupLog.Error(err, "unable to get Packet client")
		os.Exit(1)
	}

	if err := (&controllers.PacketClusterReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
		PacketClient:     client,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: packetClusterConcurrency}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PacketCluster")
		os.Exit(1)
	}

	if err := (&controllers.PacketMachineReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
		PacketClient:     client,
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: packetMachineConcurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PacketMachine")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if err := (&infrav1.PacketCluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "PacketCluster")
		os.Exit(1)
	}
	if err := (&infrav1.PacketMachine{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "PacketMachine")
		os.Exit(1)
	}
	if err := (&infrav1.PacketMachineTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "PacketMachineTemplate")
		os.Exit(1)
	}
}

func initFlags(fs *pflag.FlagSet) {
	fs.BoolVar(
		&enableLeaderElection,
		"leader-elect",
		false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.",
	)

	fs.DurationVar(
		&leaderElectionLeaseDuration,
		"leader-elect-lease-duration",
		15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)",
	)

	fs.DurationVar(
		&leaderElectionRenewDeadline,
		"leader-elect-renew-deadline",
		10*time.Second,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)",
	)

	fs.DurationVar(
		&leaderElectionRetryPeriod,
		"leader-elect-retry-period",
		2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)",
	)

	fs.StringVar(
		&watchNamespace,
		"namespace",
		"",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.",
	)

	fs.StringVar(
		&leaderElectionNamespace,
		"leader-election-namespace",
		"",
		"Namespace that the controller performs leader election in. If unspecified, the controller will discover which namespace it is running in.",
	)

	fs.StringVar(
		&profilerAddress,
		"profiler-address",
		"",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)",
	)

	fs.BoolVar(
		&enableContentionProfiling,
		"contention-profiling",
		false,
		"Enable block profiling",
	)

	fs.StringVar(
		&watchFilterValue,
		"watch-filter",
		"",
		fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel),
	)

	fs.IntVar(&packetClusterConcurrency,
		"packetcluster-concurrency",
		10,
		"Number of PacketClusters to process simultaneously",
	)

	fs.IntVar(&packetMachineConcurrency,
		"packetmachine-concurrency",
		10,
		"Number of PacketMachines to process simultaneously",
	)

	fs.DurationVar(&syncPeriod,
		"sync-period",
		10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)",
	)

	fs.IntVar(&webhookPort,
		"webhook-port",
		9443,
		"Webhook Server port",
	)

	fs.StringVar(&webhookCertDir,
		"webhook-cert-dir",
		"/tmp/k8s-webhook-server/serving-certs",
		"Webhook Server Certificate Directory, is the directory that contains the server key and certificate",
	)

	fs.StringVar(&healthAddr,
		"health-addr",
		":9440",
		"The address the health endpoint binds to.",
	)

	flags.AddDiagnosticsOptions(fs,
		&diagnosticsOptions,
	)

	flags.AddTLSOptions(fs,
		&tlsOptions,
	)

	logsv1.AddFlags(logOptions, fs)
}
