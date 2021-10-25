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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/packethost/packngo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1alpha4"
	packet "sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet/scope"
)

const (
	force = true
)

var ErrMissingDevice = errors.New("machine does not exist")

// PacketMachineReconciler reconciles a PacketMachine object
type PacketMachineReconciler struct {
	client.Client
	WatchFilterValue string
	PacketClient     *packet.Client
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs;kubeadmconfigs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *PacketMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	packetmachine := &infrav1.PacketMachine{}
	if err := r.Get(ctx, req.NamespacedName, packetmachine); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PacketMachine resource not found or already deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Unable to fetch PacketMachine resource")
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, packetmachine.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, machine) {
		log.Info("PacketMachine or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	packetcluster := &infrav1.PacketCluster{}
	packetclusterNamespacedName := client.ObjectKey{
		Namespace: packetmachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Get(ctx, packetclusterNamespacedName, packetcluster); err != nil {
		log.Info("PacketCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Create the machine scope
	machineScope, err := scope.NewMachineScope(ctx, scope.MachineScopeParams{
		Client:        r.Client,
		Logger:        log,
		Cluster:       cluster,
		Machine:       machine,
		PacketCluster: packetcluster,
		PacketMachine: packetmachine,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create scope: %w", err)
	}

	// Always close the scope when exiting this function so we can persist any PacketMachine changes.
	defer func() {
		if err := machineScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle deleted machines
	if !packetmachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machineScope)
	}

	return r.reconcile(ctx, machineScope)
}

func (r *PacketMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	clusterToObjectFunc, err := util.ClusterToObjectsMapper(r.Client, &infrav1.PacketMachineList{}, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to create mapper for Cluster to PacketMachines: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1.PacketMachine{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue)).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("PacketMachine"))),
		).
		Watches(
			&source.Kind{Type: &infrav1.PacketCluster{}},
			handler.EnqueueRequestsFromMapFunc(r.PacketClusterToPacketMachines(ctx)),
		).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(clusterToObjectFunc),
			builder.WithPredicates(predicates.ClusterUnpausedAndInfrastructureReady(log)),
		).
		Complete(r)
}

// PacketClusterToPacketMachines is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// of PacketMachines.
func (r *PacketMachineReconciler) PacketClusterToPacketMachines(ctx context.Context) handler.MapFunc {
	log := ctrl.LoggerFrom(ctx)
	return func(o client.Object) []ctrl.Request {
		c, ok := o.(*infrav1.PacketCluster)
		if !ok {
			log.Error(fmt.Errorf("expected a PacketCluster but got a %T", o), "failed to get PacketMachine for PacketCluster") //nolint:goerr113
			return nil
		}

		log = log.WithValues("PacketCluster", c.Name, "Namespace", c.Namespace)

		// Don't handle deleted PacketClusters
		if !c.ObjectMeta.DeletionTimestamp.IsZero() {
			log.V(4).Info("PacketCluster has a deletion timestamp, skipping mapping.")
			return nil
		}

		cluster, err := util.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)
		switch {
		case apierrors.IsNotFound(err) || cluster == nil:
			log.Error(err, "owning cluster is not found, skipping mapping.")
			return nil
		case err != nil:
			log.Error(err, "failed to get owning cluster")
			return nil
		}

		machines, err := collections.GetFilteredMachinesForCluster(ctx, r.Client, cluster)
		if err != nil {
			log.Error(err, "failed to get Machines for Cluster")
			return nil
		}

		var result []ctrl.Request

		for _, m := range machines.UnsortedList() {
			if m.Spec.InfrastructureRef.Name == "" {
				continue
			}
			name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.InfrastructureRef.Name}
			result = append(result, ctrl.Request{NamespacedName: name})
		}

		return result
	}
}

func (r *PacketMachineReconciler) reconcile(ctx context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) { //nolint:gocyclo
	machineScope.Info("Reconciling PacketMachine")

	packetmachine := machineScope.PacketMachine

	// If the PacketMachine is in an error state, return early.
	if packetmachine.Status.FailureReason != nil || packetmachine.Status.FailureMessage != nil {
		machineScope.Info("Error state detected, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// If the PacketMachine doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(packetmachine, infrav1.MachineFinalizer)
	if err := machineScope.PatchObject(ctx); err != nil {
		machineScope.Error(err, "unable to patch object")
	}

	if !machineScope.Cluster.Status.InfrastructureReady {
		machineScope.Info("Cluster infrastructure is not ready yet")
		conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data secret is available and populated.
	if machineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		machineScope.Info("Bootstrap data secret is not yet available")
		conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	providerID := machineScope.GetInstanceID()
	var (
		dev                  *packngo.Device
		addrs                []corev1.NodeAddress
		err                  error
		controlPlaneEndpoint packngo.IPAddressReservation
	)

	if providerID != "" {
		// If we already have a providerID, then retrieve the device using the
		// providerID. This means that the Machine has already been created
		// and we successfully recorded the providerID.
		dev, err = r.PacketClient.GetDevice(providerID)
		if err != nil {
			var perr *packngo.ErrorResponse
			if errors.As(err, &perr) && perr.Response != nil {
				if perr.Response.StatusCode == http.StatusNotFound {
					machineScope.SetFailureReason(capierrors.UpdateMachineError)
					machineScope.SetFailureMessage(fmt.Errorf("failed to find device: %w", err))
					machineScope.Error(err, "unable to find device")
					conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
				} else if perr.Response.StatusCode == http.StatusForbidden {
					machineScope.SetFailureReason(capierrors.UpdateMachineError)
					machineScope.Error(err, "device failed to provision")
					machineScope.SetFailureMessage(fmt.Errorf("device failed to provision: %w", err))
					conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceProvisionFailedReason, clusterv1.ConditionSeverityError, err.Error())
				}
			}

			return ctrl.Result{}, err
		}
	}

	if dev == nil {
		// We don't yet have a providerID, check to see if we've already
		// created a device by using the tags that we assign to devices
		// on creation.
		dev, err = r.PacketClient.GetDeviceByTags(
			machineScope.PacketCluster.Spec.ProjectID,
			packet.DefaultCreateTags(machineScope.Namespace(), machineScope.Machine.Name, machineScope.Cluster.Name),
		)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if dev == nil {
		// We weren't able to find a device by either providerID or by tags,
		// so we need to create a new device.

		// Avoid a flickering condition between InstanceProvisionStarted and InstanceProvisionFailed if there's a persistent failure with createInstance
		if conditions.GetReason(machineScope.PacketMachine, infrav1.DeviceReadyCondition) != infrav1.InstanceProvisionFailedReason {
			conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceProvisionStartedReason, clusterv1.ConditionSeverityInfo, "")
			if patchErr := machineScope.PatchObject(ctx); err != nil {
				machineScope.Error(patchErr, "failed to patch conditions")
				return ctrl.Result{}, patchErr
			}
		}

		createDeviceReq := packet.CreateDeviceRequest{
			MachineScope: machineScope,
			ExtraTags:    packet.DefaultCreateTags(machineScope.Namespace(), machineScope.Machine.Name, machineScope.Cluster.Name),
		}

		// TODO: see if this can be removed with kube-vip in place
		// when the node is a control plan we should check if the elastic ip
		// for this cluster is not assigned. If it is free we can prepare the
		// current node to use it.
		if machineScope.IsControlPlane() {
			controlPlaneEndpoint, _ = r.PacketClient.GetIPByClusterIdentifier(
				machineScope.Cluster.Namespace,
				machineScope.Cluster.Name,
				machineScope.PacketCluster.Spec.ProjectID)
			if len(controlPlaneEndpoint.Assignments) == 0 {
				a := corev1.NodeAddress{
					Type:    corev1.NodeExternalIP,
					Address: controlPlaneEndpoint.Address,
				}
				addrs = append(addrs, a)
			}
			createDeviceReq.ControlPlaneEndpoint = controlPlaneEndpoint.Address
		}

		dev, err = r.PacketClient.NewDevice(ctx, createDeviceReq)

		switch {
		// TODO: find a better way than parsing the error messages for this.
		case err != nil && strings.Contains(err.Error(), " no available hardware reservations "):
			// Do not treat an error indicating there are no hardware reservations available as fatal
			return ctrl.Result{}, fmt.Errorf("failed to create machine %s: %w", machineScope.Name(), err)
		case err != nil && strings.Contains(err.Error(), "Server is not provisionable"):
			// Do not treat an error indicating that reserved hardware is not provisionable as fatal
			// This occurs when reserved hardware is in the process of being deprovisioned
		case err != nil && strings.Contains(err.Error(), " unexpected EOF"):
			// Do not treat unexpected EOF as fatal, provisioning likely is proceeding
			return ctrl.Result{}, fmt.Errorf("failed to create machine %s: %w", machineScope.Name(), err)
		case err != nil:
			errs := fmt.Errorf("failed to create machine %s: %w", machineScope.Name(), err)
			machineScope.SetFailureReason(capierrors.CreateMachineError)
			machineScope.SetFailureMessage(errs)
			conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceProvisionFailedReason, clusterv1.ConditionSeverityError, err.Error())
			return ctrl.Result{}, errs
		}
	}

	// we do not need to set this as equinixmetal://<id> because SetProviderID() does the formatting for us
	machineScope.SetProviderID(dev.ID)
	machineScope.SetInstanceStatus(infrav1.PacketResourceStatus(dev.State))

	deviceAddr := r.PacketClient.GetDeviceAddresses(dev)
	machineScope.SetAddresses(append(addrs, deviceAddr...))

	// Proceed to reconcile the PacketMachine state.
	var result reconcile.Result

	switch infrav1.PacketResourceStatus(dev.State) {
	case infrav1.PacketResourceStatusNew, infrav1.PacketResourceStatusQueued, infrav1.PacketResourceStatusProvisioning:
		machineScope.Info("Machine instance is pending", "instance-id", machineScope.GetInstanceID())
		machineScope.SetNotReady()
		conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceNotReadyReason, clusterv1.ConditionSeverityWarning, "")
		result = ctrl.Result{RequeueAfter: 10 * time.Second}
	case infrav1.PacketResourceStatusRunning:
		machineScope.Info("Machine instance is active", "instance-id", machineScope.GetInstanceID())

		// This logic is here because an elastic ip can be assigned only an
		// active node. It needs to be a control plane and the IP should not be
		// assigned to anything at this point.
		// TODO: see if this can be removed with kube-vip in place
		controlPlaneEndpoint, _ = r.PacketClient.GetIPByClusterIdentifier(
			machineScope.Cluster.Namespace,
			machineScope.Cluster.Name,
			machineScope.PacketCluster.Spec.ProjectID)
		if len(controlPlaneEndpoint.Assignments) == 0 && machineScope.IsControlPlane() {
			if _, _, err := r.PacketClient.DeviceIPs.Assign(dev.ID, &packngo.AddressStruct{
				Address: controlPlaneEndpoint.Address,
			}); err != nil {
				machineScope.Error(err, "err assigining elastic ip to control plane. retrying...")
				return ctrl.Result{RequeueAfter: time.Second * 20}, nil
			}
		}
		machineScope.SetReady()
		conditions.MarkTrue(machineScope.PacketMachine, infrav1.DeviceReadyCondition)
		result = ctrl.Result{}
	default:
		machineScope.SetNotReady()
		machineScope.Info("Equinix Metal device state is undefined", "state", dev.State, "device-id", machineScope.GetInstanceID())
		machineScope.SetFailureReason(capierrors.UpdateMachineError)
		machineScope.SetFailureMessage(fmt.Errorf("instance status %q is unexpected", dev.State)) //nolint:goerr113
		conditions.MarkUnknown(machineScope.PacketMachine, infrav1.DeviceReadyCondition, "", "")
		result = ctrl.Result{}
	}

	return result, nil
}

func (r *PacketMachineReconciler) reconcileDelete(_ context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) {
	machineScope.Info("Reconciling Delete PacketMachine")

	packetmachine := machineScope.PacketMachine
	providerID := machineScope.GetInstanceID()

	var device *packngo.Device

	if providerID == "" {
		// If no providerID was recorded, check to see if there are any instances
		// that match by tags
		dev, err := r.PacketClient.GetDeviceByTags(
			machineScope.PacketCluster.Spec.ProjectID,
			packet.DefaultCreateTags(machineScope.Namespace(), machineScope.Machine.Name, machineScope.Cluster.Name),
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		if dev == nil {
			machineScope.Info("Server not found by tags, nothing left to do")
			controllerutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
			return ctrl.Result{}, nil
		}

		device = dev
	} else {
		// Otherwise, try to retrieve the device by the providerID
		dev, err := r.PacketClient.GetDevice(providerID)
		if err != nil {
			var errResp *packngo.ErrorResponse
			if errors.As(err, &errResp) && errResp.Response != nil {
				if errResp.Response.StatusCode == http.StatusNotFound {
					// When the server does not exist we do not have anything left to do.
					// Probably somebody manually deleted the server from the UI or via API.
					machineScope.Info("Server not found by id, nothing left to do")
					controllerutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
					return ctrl.Result{}, nil
				}

				if errResp.Response.StatusCode == http.StatusForbidden {
					// When a server fails to provision it will return a 403
					machineScope.Info("Server appears to have failed provisioning, nothing left to do")
					controllerutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
					return ctrl.Result{}, nil
				}
			}

			return ctrl.Result{}, fmt.Errorf("error retrieving machine status %s: %w", packetmachine.Name, err)
		}

		device = dev
	}

	// We should never get there but this is a safetly check
	if device == nil {
		controllerutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, fmt.Errorf("%w: %s", ErrMissingDevice, packetmachine.Name)
	}

	if _, err := r.PacketClient.Devices.Delete(device.ID, force); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete the machine: %w", err)
	}

	controllerutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
	return ctrl.Result{}, nil
}
