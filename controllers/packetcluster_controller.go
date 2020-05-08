/*


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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrastructurev1alpha3 "github.com/packethost/cluster-api-provider-packet/api/v1alpha3"
	packet "github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/scope"
)

// PacketClusterReconciler reconciles a PacketCluster object
type PacketClusterReconciler struct {
	client.Client
	Log          logr.Logger
	Recorder     record.EventRecorder
	Scheme       *runtime.Scheme
	PacketClient *packet.PacketClient
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *PacketClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	logger := r.Log.WithValues("packetcluster", req.NamespacedName)

	// your logic here
	packetcluster := &infrastructurev1alpha3.PacketCluster{}
	if err := r.Get(ctx, req.NamespacedName, packetcluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger = logger.WithName(packetcluster.APIVersion)

	// Fetch the Machine.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, packetcluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if cluster == nil {
		logger.Info("OwenerCluster is not set yet. Requeuing...")
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: 2 * time.Second,
		}, nil
	}

	// Create the cluster scope
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Logger:        logger,
		Client:        r.Client,
		Cluster:       cluster,
		PacketCluster: packetcluster,
	})
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to create scope: %+v", err)
	}
	// Always close the scope when exiting this function so we can persist any PacketCluster changes.
	defer func() {
		if err := clusterScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	address, err := r.getIP(clusterScope.PacketCluster)
	if err == nil {
		clusterScope.PacketCluster.Status.Ready = true
		return ctrl.Result{}, nil
	}

	_, isNoMachine := err.(*MachineNotFound)
	_, isNoIP := err.(*MachineNoIP)
	switch {
	case err != nil && isNoMachine:
		logger.Info("Control plane device not found. Requeueing...")
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	case err != nil && isNoIP:
		logger.Info("Control plane device not found. Requeueing...")
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	case err != nil:
		logger.Error(err, "error getting a control plane ip")
		return ctrl.Result{}, err
	case err == nil:
		clusterScope.PacketCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: address,
			Port: 6443,
		}
		clusterScope.PacketCluster.Status.Ready = true
	}

	return ctrl.Result{}, nil
}

func (r *PacketClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha3.PacketCluster{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.ClusterToInfrastructureMapFunc(infrastructurev1alpha3.GroupVersion.WithKind("PacketCluster")),
			},
		).
		Complete(r)
}

func (r *PacketClusterReconciler) getIP(cluster *infrastructurev1alpha3.PacketCluster) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cannot get IP of machine in nil cluster")
	}
	tags := []string{
		packet.GenerateClusterTag(string(cluster.Name)),
		infrastructurev1alpha3.MasterTag,
	}
	device, err := r.PacketClient.GetDeviceByTags(cluster.Spec.ProjectID, tags)
	if err != nil {
		return "", fmt.Errorf("error retrieving machine: %v", err)
	}
	if device == nil {
		return "", &MachineNotFound{err: fmt.Sprintf("machine does not exist")}
	}
	if device.Network == nil || len(device.Network) == 0 || device.Network[0].Address == "" {
		return "", &MachineNoIP{err: "machine does not yet have an IP address"}
	}
	// TODO: validate that this address exists, so we don't hit nil pointer
	// TODO: check which address to return
	// TODO: check address format (cidr, subnet, etc.)
	return device.Network[0].Address, nil
}

// MachineNotFound error representing that the requested device was not yet found
type MachineNotFound struct {
	err string
}

func (e *MachineNotFound) Error() string {
	return e.err
}

// MachineNoIP error representing that the requested device does not have an IP yet assigned
type MachineNoIP struct {
	err string
}

func (e *MachineNoIP) Error() string {
	return e.err
}
