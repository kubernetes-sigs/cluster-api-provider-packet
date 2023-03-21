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

package controllers

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
	packet "sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet/scope"
)

// PacketClusterReconciler reconciles a PacketCluster object
type PacketClusterReconciler struct {
	client.Client
	WatchFilterValue string
	PacketClient     *packet.Client
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *PacketClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	packetcluster := &infrav1.PacketCluster{}
	if err := r.Get(ctx, req.NamespacedName, packetcluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PacketCluster resource not found or already deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "unable to fetch PacketCluster resource")
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, packetcluster.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner cluster")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, packetcluster) {
		log.Info("PacketCluster or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Create the cluster scope
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:        r.Client,
		Cluster:       cluster,
		PacketCluster: packetcluster,
	})
	if err != nil {
		log.Error(err, "failed to create scope")
		return ctrl.Result{}, err
	}
	// Always close the scope when exiting this function so we can persist any PacketCluster changes.
	defer func() {
		if err := clusterScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle deleted clusters
	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, clusterScope)
	}

	return r.reconcileNormal(ctx, clusterScope)
}

func (r *PacketClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("cluster", clusterScope.Cluster.Name)
	log.Info("Reconciling PacketCluster")

	packetCluster := clusterScope.PacketCluster

	ipReserv, err := r.PacketClient.GetIPByClusterIdentifier(clusterScope.Namespace(), clusterScope.Name(), packetCluster.Spec.ProjectID)
	switch {
	case errors.Is(err, packet.ErrControlPlanEndpointNotFound):
		// There is not an ElasticIP with the right tags, at this point we can create one
		ip, err := r.PacketClient.CreateIP(clusterScope.Namespace(), clusterScope.Name(), packetCluster.Spec.ProjectID, packetCluster.Spec.Facility)
		if err != nil {
			log.Error(err, "error reserving an ip")
			return ctrl.Result{}, err
		}
		clusterScope.PacketCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ip.To4().String(),
			Port: 6443,
		}
	case err != nil:
		log.Error(err, "error getting cluster IP")
		return ctrl.Result{}, err
	default:
		// If there is an ElasticIP with the right tag just use it again
		clusterScope.PacketCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: ipReserv.Address,
			Port: 6443,
		}
	}

	if clusterScope.PacketCluster.Spec.VIPManager == "KUBE_VIP" {
		if err := r.PacketClient.EnableProjectBGP(packetCluster.Spec.ProjectID); err != nil {
			log.Error(err, "error enabling bgp for project")
			return ctrl.Result{}, err
		}
	}

	clusterScope.PacketCluster.Status.Ready = true
	conditions.MarkTrue(packetCluster, infrav1.NetworkInfrastructureReadyCondition)

	return ctrl.Result{}, nil
}

func (r *PacketClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	// Initially I created this handler to remove an elastic IP when a cluster
	// gets delete, but it does not sound like a good idea.  It is better to
	// leave to the users the ability to decide if they want to keep and resign
	// the IP or if they do not need it anymore
	return ctrl.Result{}, nil
}

func (r *PacketClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1.PacketCluster{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(log)).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("PacketCluster"), mgr.GetClient(), &infrav1.PacketCluster{})),
			builder.WithPredicates(predicates.ClusterUpdateUnpaused(log)),
		).
		Complete(r)
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
