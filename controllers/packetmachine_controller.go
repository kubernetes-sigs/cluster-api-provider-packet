/*
Copyright Packet Inc 2020

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
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/packethost/packngo"
	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	packet "github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/scope"

	infrastructurev1alpha3 "github.com/packethost/cluster-api-provider-packet/api/v1alpha3"
)

const (
	providerName = "packet"
)

// PacketMachineReconciler reconciles a PacketMachine object
type PacketMachineReconciler struct {
	client.Client
	Log          logr.Logger
	Recorder     record.EventRecorder
	Scheme       *runtime.Scheme
	PacketClient *packet.PacketClient
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch

func (r *PacketMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	logger := r.Log.WithValues("packetmachine", req.NamespacedName)

	// your logic here
	packetmachine := &infrastructurev1alpha3.PacketMachine{}
	if err := r.Get(ctx, req.NamespacedName, packetmachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger = logger.WithName(packetmachine.APIVersion)

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, packetmachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	packetcluster := &infrastructurev1alpha3.PacketCluster{}
	packetclusterNamespacedName := client.ObjectKey{
		Namespace: packetmachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Get(ctx, packetclusterNamespacedName, packetcluster); err != nil {
		logger.Info("PacketCluster is not available yet")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("packetcluster", packetcluster.Name)

	// Create the cluster scope
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:        r.Client,
		Logger:        logger,
		Cluster:       cluster,
		PacketCluster: packetcluster,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create the machine scope
	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Logger:        logger,
		Client:        r.Client,
		Cluster:       cluster,
		Machine:       machine,
		PacketCluster: packetcluster,
		PacketMachine: packetmachine,
	})
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to create scope: %+v", err)
	}

	// Always close the scope when exiting this function so we can persist any PacketMachine changes.
	defer func() {
		if err := machineScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle deleted machines
	if !packetmachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machineScope, clusterScope, logger)
	}

	return r.reconcile(ctx, machineScope, clusterScope, logger)
}

func (r *PacketMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha3.PacketMachine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(infrastructurev1alpha3.GroupVersion.WithKind("PacketMachine")),
			},
		).
		Complete(r)
}

func (r *PacketMachineReconciler) reconcile(ctx context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Reconciling PacketMachine")
	packetmachine := machineScope.PacketMachine
	// If the PacketMachine is in an error state, return early.
	if packetmachine.Status.ErrorReason != nil || packetmachine.Status.ErrorMessage != nil {
		machineScope.Info("Error state detected, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// If the PacketMachine doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(packetmachine, infrastructurev1alpha3.MachineFinalizer)

	if !machineScope.IsControlPlane() {
		if !machineScope.Cluster.Status.InfrastructureReady {
			machineScope.Info("Cluster infrastructure is not ready yet")
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}

		// Make sure bootstrap data secret is available and populated.
		if machineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
			machineScope.Info("Bootstrap data secret is not yet available")
			return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
		}
	}

	providerID := machineScope.GetInstanceID()
	var (
		dev *packngo.Device
		err error
	)
	// if we have no provider ID, then we are creating
	if providerID != "" {
		dev, err = r.PacketClient.GetDevice(providerID)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	if dev == nil {
		// generate a unique UID that will survive pivot, i.e. is not tied to the cluster itself
		mUID := uuid.New().String()
		tags := []string{
			packet.GenerateMachineTag(mUID),
			packet.GenerateClusterTag(clusterScope.Name()),
		}
		if machineScope.IsControlPlane() {
			tags = append(tags, infrastructurev1alpha3.MasterTag)
		} else {
			tags = append(tags, infrastructurev1alpha3.WorkerTag)
		}

		name := machineScope.Name()
		dev, err = r.PacketClient.NewDevice(name, clusterScope.PacketCluster.Spec.ProjectID, machineScope.PacketMachine.Spec, tags)
		if err != nil {
			errs := fmt.Errorf("failed to create machine %s: %v", name, err)
			machineScope.SetErrorReason(capierrors.CreateMachineError)
			machineScope.SetErrorMessage(errs)
			return ctrl.Result{}, errs
		}
	}

	// we do not need to set this as packet://<id> because SetProviderID() does the formatting for us
	machineScope.SetProviderID(dev.ID)
	machineScope.SetInstanceStatus(infrastructurev1alpha3.PacketResourceStatus(dev.State))

	addrs, err := r.PacketClient.GetDeviceAddresses(dev)
	if err != nil {
		machineScope.SetErrorMessage(errors.New("failed to getting device addresses"))
		return ctrl.Result{}, err
	}
	machineScope.SetAddresses(addrs)

	// Proceed to reconcile the PacketMachine state.
	var result = ctrl.Result{}

	switch infrastructurev1alpha3.PacketResourceStatus(dev.State) {
	case infrastructurev1alpha3.PacketResourceStatusNew, infrastructurev1alpha3.PacketResourceStatusQueued, infrastructurev1alpha3.PacketResourceStatusProvisioning:
		machineScope.Info("Machine instance is pending", "instance-id", machineScope.GetInstanceID())
		result = ctrl.Result{RequeueAfter: 10 * time.Second}
	case infrastructurev1alpha3.PacketResourceStatusRunning:
		machineScope.Info("Machine instance is active", "instance-id", machineScope.GetInstanceID())
		machineScope.SetReady()
		result = ctrl.Result{}
	default:
		machineScope.SetErrorReason(capierrors.UpdateMachineError)
		machineScope.SetErrorMessage(errors.Errorf("Instance status %q is unexpected", dev.State))
		result = ctrl.Result{}
	}

	return result, nil
}

func (r *PacketMachineReconciler) reconcileDelete(ctx context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Deleting machine")
	packetmachine := machineScope.PacketMachine
	providerID := machineScope.GetInstanceID()
	if providerID == "" {
		logger.Info("no provider ID provided, nothing to delete")
		controllerutil.RemoveFinalizer(packetmachine, infrastructurev1alpha3.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	device, err := r.PacketClient.GetDevice(providerID)
	if err != nil {
		if err.(*packngo.ErrorResponse).Response != nil && err.(*packngo.ErrorResponse).Response.StatusCode == http.StatusNotFound {
			// When the server does not exist we do not have anything left to do.
			// Probably somebody manually deleted the server from the UI or via API.
			logger.Info("Server not found, nothing left to do")
			controllerutil.RemoveFinalizer(packetmachine, infrastructurev1alpha3.MachineFinalizer)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("error retrieving machine status %s: %v", packetmachine.Name, err)
	}

	// We should never get there but this is a safetly check
	if device == nil {
		controllerutil.RemoveFinalizer(packetmachine, infrastructurev1alpha3.MachineFinalizer)
		return ctrl.Result{}, fmt.Errorf("machine does not exist: %s", packetmachine.Name)
	}

	_, err = r.PacketClient.Devices.Delete(device.ID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete the machine: %v", err)
	}

	controllerutil.RemoveFinalizer(packetmachine, infrastructurev1alpha3.MachineFinalizer)
	return ctrl.Result{}, nil
}
