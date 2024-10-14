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
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	apitypes "k8s.io/apimachinery/pkg/types"

	metal "github.com/equinix/equinix-sdk-go/services/metalv1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	clusterutilv1 "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-packet/internal/emlb"
	"sigs.k8s.io/cluster-api-provider-packet/internal/layer2"
	packet "sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet/scope"
	clog "sigs.k8s.io/cluster-api/util/log"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	force = true
)

const (
	networkConfigurationSuccessEvent = "network_configuration_success"
	networkConfigurationFailureEvent = "network_configuration_failure"
	eventPollingInterval             = 5 * time.Second
)

var (
	errMissingDevice = errors.New("machine does not exist")
	errFacilityMatch = errors.New("instance facility does not match machine facility")
	errMetroMatch    = errors.New("instance metro does not match machine metro")
)

// PacketMachineReconciler reconciles a PacketMachine object.
type PacketMachineReconciler struct {
	client.Client
	PacketClient *packet.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=packetmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machinesets;machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs;kubeadmconfigs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *PacketMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the PacketMachine instance.
	packetmachine := &infrav1.PacketMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, packetmachine); err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "PacketMachine resource not found or already deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Unable to fetch PacketMachine resource")
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of PacketMachine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, packetmachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := clusterutilv1.GetOwnerMachine(ctx, r.Client, packetmachine.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machine))
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := clusterutilv1.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, packetmachine) {
		log.Info("PacketMachine or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Fetch the Packet Cluster
	packetcluster := &infrav1.PacketCluster{}
	packetclusterNamespacedName := client.ObjectKey{
		Namespace: packetmachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, packetclusterNamespacedName, packetcluster); err != nil {
		log.Info("PacketCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Create the machine scope
	machineScope, err := scope.NewMachineScope(
		scope.MachineScopeParams{
			Client:        r.Client,
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
		if err := machineScope.Close(ctx); err != nil && rerr == nil {
			log.Error(err, "failed to patch packetmachine")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if packetmachine.ObjectMeta.DeletionTimestamp.IsZero() && !ctrlutil.ContainsFinalizer(packetmachine, infrav1.MachineFinalizer) {
		ctrlutil.AddFinalizer(packetmachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deleted machines
	if !packetmachine.ObjectMeta.DeletionTimestamp.IsZero() {
		err = r.reconcileDelete(ctx, machineScope)
		return ctrl.Result{}, err
	}
	return r.reconcile(ctx, machineScope)
}

func (r *PacketMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	clusterToPacketMachines, err := clusterutilv1.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.PacketMachineList{}, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to create mapper for Cluster to PacketMachines: %w", err)
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.PacketMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(clusterutilv1.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("PacketMachine"))),
		).
		Watches(
			&infrav1.PacketCluster{},
			handler.EnqueueRequestsFromMapFunc(r.PacketClusterToPacketMachines),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToPacketMachines),
			builder.WithPredicates(
				predicates.ClusterUnpausedAndInfrastructureReady(log),
			),
		).
		Watches(
			&ipamv1.IPAddressClaim{},
			handler.EnqueueRequestsFromMapFunc(ipAddressClaimToPacketMachine),
		).Complete(r)
	if err != nil {
		return fmt.Errorf("failed setting up with a controller manager: %w", err)
	}
	return nil
}

// PacketClusterToPacketMachines is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// of PacketMachines.
func (r *PacketMachineReconciler) PacketClusterToPacketMachines(ctx context.Context, o client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	result := []ctrl.Request{}
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

	cluster, err := clusterutilv1.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		log.Error(err, "owning cluster is not found, skipping mapping.")
		return result
	case err != nil:
		log.Error(err, "failed to get owning cluster")
		return result
	}

	labels := map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		log.Error(err, "failed to get Machines for Cluster")
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}

func ipAddressClaimToPacketMachine(_ context.Context, a client.Object) []reconcile.Request {
	ipAddressClaim, ok := a.(*ipamv1.IPAddressClaim)
	if !ok {
		return nil
	}

	requests := []reconcile.Request{}
	if clusterutilv1.HasOwner(ipAddressClaim.OwnerReferences, infrav1.GroupVersion.String(), []string{"PacketMachine"}) {
		for _, ref := range ipAddressClaim.OwnerReferences {
			if ref.Kind == "PacketMachine" {
				requests = append(requests, reconcile.Request{
					NamespacedName: apitypes.NamespacedName{
						Name:      ref.Name,
						Namespace: ipAddressClaim.Namespace,
					},
				})
				break
			}
		}
	}
	return requests
}

func (r *PacketMachineReconciler) reconcile(ctx context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) { //nolint:gocyclo,maintidx
	log := ctrl.LoggerFrom(ctx, "machine", machineScope.Machine.Name, "cluster", machineScope.Cluster.Name)
	log.Info("Reconciling PacketMachine")

	packetmachine := machineScope.PacketMachine
	// If the PacketMachine is in an error state, return early.
	if packetmachine.Status.FailureReason != nil || packetmachine.Status.FailureMessage != nil {
		log.Info("Error state detected, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if !machineScope.Cluster.Status.InfrastructureReady {
		log.Info("Cluster infrastructure is not ready yet")
		conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data secret is available and populated.
	if machineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Bootstrap data secret is not yet available")
		conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	deviceID := machineScope.GetDeviceID()
	var (
		dev                  *metal.Device
		addrs                []corev1.NodeAddress
		err                  error
		controlPlaneEndpoint *metal.IPReservation
		resp                 *http.Response
		ipAddrCfg			 []packet.IPAddressCfg
	)

	if deviceID != "" {
		// If we already have a device ID, then retrieve the device using the
		// device ID. This means that the Machine has already been created
		// and we successfully recorded the device ID.
		dev, resp, err = r.PacketClient.GetDevice(ctx, deviceID) //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
		if err != nil {
			if resp != nil {
				if resp.StatusCode == http.StatusNotFound {
					machineScope.SetFailureReason(capierrors.UpdateMachineError)
					machineScope.SetFailureMessage(fmt.Errorf("failed to find device: %w", err))
					log.Error(err, "unable to find device")
					conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
				} else if resp.StatusCode == http.StatusForbidden {
					machineScope.SetFailureReason(capierrors.UpdateMachineError)
					log.Error(err, "device failed to provision")
					machineScope.SetFailureMessage(fmt.Errorf("device failed to provision: %w", err))
					conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceProvisionFailedReason, clusterv1.ConditionSeverityError, err.Error())
				}
			}

			return ctrl.Result{}, err
		}
	}

	if dev == nil {
		// We don't yet have a device ID, check to see if we've already
		// created a device by using the tags that we assign to devices
		// on creation.
		dev, err = r.PacketClient.GetDeviceByTags(
			ctx,
			machineScope.PacketCluster.Spec.ProjectID,
			packet.DefaultCreateTags(machineScope.Namespace(), machineScope.Machine.Name, machineScope.Cluster.Name),
		)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if dev == nil {
		// We weren't able to find a device by either device ID or by tags,
		// so we need to create a new device.

		// Avoid a flickering condition between InstanceProvisionStarted and InstanceProvisionFailed if there's a persistent failure with createInstance
		if conditions.GetReason(machineScope.PacketMachine, infrav1.DeviceReadyCondition) != infrav1.InstanceProvisionFailedReason {
			conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceProvisionStartedReason, clusterv1.ConditionSeverityInfo, "")
			if patchErr := machineScope.PatchObject(ctx); patchErr != nil {
				log.Error(patchErr, "failed to patch conditions")
				return ctrl.Result{}, patchErr
			}
		}
		if len(machineScope.PacketMachine.Spec.NetworkPorts) > 0 {
			if err := r.ReconcileIPAddresses(ctx, machineScope.PacketMachine); err != nil {
				return ctrl.Result{}, err
			}
		}

		ipAddrCfg, err = getIPAddressCfg(ctx, r.Client, machineScope.PacketMachine)
		if err != nil {
			return ctrl.Result{}, err
		}

		routesCfg, err := getRoutesCfg(machineScope.PacketMachine)
		if err != nil {
			return ctrl.Result{}, err
		}

		createDeviceReq := packet.CreateDeviceRequest{
			MachineScope: machineScope,
			ExtraTags:    packet.DefaultCreateTags(machineScope.Namespace(), machineScope.Machine.Name, machineScope.Cluster.Name),
			IPAddresses:  ipAddrCfg,
			Routes:       routesCfg,
		}

		// when a node is a control plane node we need the elastic IP
		// to template out the kube-vip deployment
		if machineScope.IsControlPlane() {
			var controlPlaneEndpointAddress string
			var cpemLBConfig string
			var emlbID string
			switch machineScope.PacketCluster.Spec.VIPManager {
			case infrav1.CPEMID, infrav1.KUBEVIPID:
				controlPlaneEndpoint, _ = r.PacketClient.GetIPByClusterIdentifier(
					ctx,
					machineScope.Cluster.Namespace,
					machineScope.Cluster.Name,
					machineScope.PacketCluster.Spec.ProjectID)
				if len(controlPlaneEndpoint.Assignments) == 0 {
					a := corev1.NodeAddress{
						Type:    corev1.NodeExternalIP,
						Address: controlPlaneEndpoint.GetAddress(),
					}
					addrs = append(addrs, a)
				}
				controlPlaneEndpointAddress = controlPlaneEndpoint.GetAddress()
			case infrav1.EMLBVIPID:
				controlPlaneEndpointAddress = machineScope.Cluster.Spec.ControlPlaneEndpoint.Host
				cpemLBConfig = "emlb:///" + machineScope.PacketCluster.Spec.Metro
				emlbID = machineScope.PacketCluster.Annotations["equinix.com/loadbalancerID"]
			}
			createDeviceReq.ControlPlaneEndpoint = controlPlaneEndpointAddress
			createDeviceReq.CPEMLBConfig = cpemLBConfig
			createDeviceReq.EMLBID = emlbID
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
			return ctrl.Result{}, fmt.Errorf("failed to create machine %s: %w", machineScope.Name(), err)
		case err != nil && strings.Contains(err.Error(), " unexpected EOF"):
			// Do not treat unexpected EOF as fatal, provisioning likely is proceeding
		case err != nil:
			errs := fmt.Errorf("failed to create machine %s: %w", machineScope.Name(), err)
			machineScope.SetFailureReason(capierrors.CreateMachineError)
			machineScope.SetFailureMessage(errs)
			conditions.MarkFalse(machineScope.PacketMachine, infrav1.DeviceReadyCondition, infrav1.InstanceProvisionFailedReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{}, errs
		}
	}

	// we do not need to set this as equinixmetal://<id> because SetProviderID() does the formatting for us
	machineScope.SetProviderID(dev.GetId())
	machineScope.SetInstanceStatus(infrav1.PacketResourceStatus(dev.GetState()))

	if machineScope.PacketCluster.Spec.VIPManager == infrav1.KUBEVIPID {
		if err := r.PacketClient.EnsureNodeBGPEnabled(ctx, dev.GetId()); err != nil {
			// Do not treat an error enabling bgp on machine as fatal
			return ctrl.Result{RequeueAfter: time.Second * 20}, fmt.Errorf("failed to enable bgp on machine %s: %w", machineScope.Name(), err)
		}
	}

	deviceAddr := r.PacketClient.GetDeviceAddresses(dev)
	machineScope.SetAddresses(append(addrs,deviceAddr...))

	// Proceed to reconcile the PacketMachine state.
	var result reconcile.Result

	switch infrav1.PacketResourceStatus(dev.GetState()) {
	case infrav1.PacketResourceStatusNew, infrav1.PacketResourceStatusQueued, infrav1.PacketResourceStatusProvisioning:
		log.Info("Machine instance is pending", "instance-id", machineScope.ProviderID())
		machineScope.SetNotReady()
		result = ctrl.Result{RequeueAfter: 10 * time.Second}
	case infrav1.PacketResourceStatusRunning:
		log.Info("Machine instance is active", "instance-id", machineScope.ProviderID())

		switch {
		case machineScope.PacketCluster.Spec.VIPManager == infrav1.CPEMID:
			controlPlaneEndpoint, _ = r.PacketClient.GetIPByClusterIdentifier(
				ctx,
				machineScope.Cluster.Namespace,
				machineScope.Cluster.Name,
				machineScope.PacketCluster.Spec.ProjectID)
			if len(controlPlaneEndpoint.Assignments) == 0 && machineScope.IsControlPlane() {
				apiRequest := r.PacketClient.DevicesApi.CreateIPAssignment(ctx, *dev.Id).IPAssignmentInput(metal.IPAssignmentInput{
					Address: controlPlaneEndpoint.GetAddress(),
				})
				if _, _, err := apiRequest.Execute(); err != nil { //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
					log.Error(err, "err assigining elastic ip to control plane. retrying...")
					return ctrl.Result{RequeueAfter: time.Second * 20}, nil
				}
			}
		case machineScope.PacketCluster.Spec.VIPManager == infrav1.EMLBVIPID:
			if machineScope.IsControlPlane() {
				// Create new EMLB object
				lb := emlb.NewEMLB(r.PacketClient.GetConfig().DefaultHeader["X-Auth-Token"], machineScope.PacketCluster.Spec.ProjectID, machineScope.PacketCluster.Spec.Metro)

				if err := lb.ReconcileVIPOrigin(ctx, machineScope, deviceAddr); err != nil {
					return ctrl.Result{}, err
				}
			}
		}

		if machineScope.PacketMachine.Spec.NetworkPorts != nil {
			eventsList,resp,err := r.PacketClient.EventsApi.FindDeviceEvents(ctx, *dev.Id).Execute()
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get device events: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				return ctrl.Result{}, fmt.Errorf("failed to get device events: %w", err)
			}
			// check if the network configuration has been successful/failed by polling the /events endpoint
			// if the network configuration has been successful, we can set the device to ready else we need to requeue
			// we need to wait for either the network configuration to be successful or failed before we can proceed.
			if len(eventsList.Events) > 0 {
				if checkIfEventsContainNetworkConfigurationSuccess(eventsList) {
					conditions.MarkTrue(machineScope.PacketMachine, infrav1.Layer2NetworkConfigurationConditionSuccess)
				} else if checkIfEventsContainNetworkConfigurationFailure(eventsList) {
					conditions.MarkTrue(machineScope.PacketMachine, infrav1.Layer2NetworkConfigurationConditionFailed)
					return ctrl.Result{}, fmt.Errorf("failed to configure network on device")
				} else {
					// if the network configuration is still in progress, we need to requeue
					// user data scripts might take some time to complete.
					log.Info("waiting for layer2 network configurations to complete")
					return ctrl.Result{RequeueAfter: time.Second * 10}, nil
				}
			}
		}

		// once the network configuration has been successful, we can call the APIs to set the port configuration to layer2/bonded/bound VXLAN to port.
		// reconstruct ipAddrCfg as earlier it was done when device was created first. During later reconciliations, this might be nil and we need to reconstruct it.
		ipAddrCfg, err = getIPAddressCfg(ctx, r.Client, machineScope.PacketMachine)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(ipAddrCfg) > 0 {
			if err := r.reconcilePortConfigurations(ctx, *dev.Id, ipAddrCfg); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set port configuration: %w", err)
			}
		}

		machineScope.SetReady()
		conditions.MarkTrue(machineScope.PacketMachine, infrav1.DeviceReadyCondition)

		result = ctrl.Result{}
	default:
		machineScope.SetNotReady()
		log.Info("Equinix Metal device state is undefined", "state", dev.GetState(), "device-id", machineScope.ProviderID())
		machineScope.SetFailureReason(capierrors.UpdateMachineError)
		machineScope.SetFailureMessage(fmt.Errorf("instance status %q is unexpected", dev.GetState())) //nolint:goerr113
		conditions.MarkUnknown(machineScope.PacketMachine, infrav1.DeviceReadyCondition, "", "")

		result = ctrl.Result{}
	}

	// If Metro or Facility has changed in the spec, verify that the facility's metro is compatible with the requested spec change.
	deviceFacility := dev.Facility.Code
	deviceMetro := dev.Metro.Code

	if machineScope.PacketMachine.Spec.Facility != "" && machineScope.PacketMachine.Spec.Facility != *deviceFacility {
		return ctrl.Result{}, fmt.Errorf("%w: %s != %s", errFacilityMatch, machineScope.PacketMachine.Spec.Facility, *deviceFacility)
	}

	if machineScope.PacketMachine.Spec.Metro != "" && machineScope.PacketMachine.Spec.Metro != *deviceMetro {
		return ctrl.Result{}, fmt.Errorf("%w: %s != %s", errMetroMatch, machineScope.PacketMachine.Spec.Facility, *deviceMetro)
	}

	return result, nil
}

func (r *PacketMachineReconciler) reconcileDelete(ctx context.Context, machineScope *scope.MachineScope) error {
	log := ctrl.LoggerFrom(ctx, "machine", machineScope.Machine.Name, "cluster", machineScope.Cluster.Name)
	log.Info("Reconciling Delete PacketMachine")

	packetmachine := machineScope.PacketMachine
	deviceID := machineScope.GetDeviceID()

	var device *metal.Device

	if deviceID == "" {
		// If no device ID was recorded, check to see if there are any instances
		// that match by tags
		dev, err := r.PacketClient.GetDeviceByTags(
			ctx,
			machineScope.PacketCluster.Spec.ProjectID,
			packet.DefaultCreateTags(machineScope.Namespace(), machineScope.Machine.Name, machineScope.Cluster.Name),
		)
		if err != nil {
			return err
		}

		if dev == nil {
			log.Info("Server not found by tags, nothing left to do")
			ctrlutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
			return nil
		}

		device = dev
	} else {
		var resp *http.Response
		// Otherwise, try to retrieve the device by the providerID
		dev, resp, err := r.PacketClient.GetDevice(ctx, deviceID) //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
		if err != nil {
			if resp != nil {
				if resp.StatusCode == http.StatusNotFound {
					// When the server does not exist we do not have anything left to do.
					// Probably somebody manually deleted the server from the UI or via API.
					log.Info("Server not found by id, nothing left to do")
					ctrlutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
					return nil
				}

				if resp.StatusCode == http.StatusForbidden {
					// When a server fails to provision it will return a 403
					log.Info("Server appears to have failed provisioning, nothing left to do")
					ctrlutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
					return nil
				}
			}

			return fmt.Errorf("error retrieving machine status %s: %w", packetmachine.Name, err)
		}

		device = dev
	}

	// We should never get there but this is a safety check
	if device == nil {
		ctrlutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
		return fmt.Errorf("%w: %s", errMissingDevice, packetmachine.Name)
	}

	if machineScope.PacketCluster.Spec.VIPManager == infrav1.EMLBVIPID {
		if machineScope.IsControlPlane() {
			// Create new EMLB object
			lb := emlb.NewEMLB(r.PacketClient.GetConfig().DefaultHeader["X-Auth-Token"], machineScope.PacketCluster.Spec.ProjectID, packetmachine.Spec.Metro)

			if err := lb.DeleteLoadBalancerOrigin(ctx, machineScope); err != nil {
				return fmt.Errorf("failed to delete load balancer origin: %w", err)
			}
		}
	}

	apiRequest := r.PacketClient.DevicesApi.DeleteDevice(ctx, device.GetId()).ForceDelete(force)
	if _, err := apiRequest.Execute(); err != nil { //nolint:bodyclose // see https://github.com/timakin/bodyclose/issues/42
		return fmt.Errorf("failed to delete the machine: %w", err)
	}

	ctrlutil.RemoveFinalizer(packetmachine, infrav1.MachineFinalizer)
	return nil
}

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;create;patch;watch;list;update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch

// ReconcileIPAddresses reconciles ip addresses forpacket machine
func (r *PacketMachineReconciler) ReconcileIPAddresses(ctx context.Context, machine *infrav1.PacketMachine) error {

	log := ctrl.LoggerFrom(ctx)

	totalClaims, claimsCreated := 0, 0
	claimsFulfilled := 0

	var (
		claims  []conditions.Getter
		errList []error
	)

	for portIdx, port := range machine.Spec.NetworkPorts {
		for networkIdx, network := range port.Networks {
			totalClaims++

			ipAddrClaimName := packet.IPAddressClaimName(machine.Name, portIdx, networkIdx)

			ipAddrClaim := &ipamv1.IPAddressClaim{}
			ipAddrClaimKey := client.ObjectKey{
				Namespace: machine.Namespace,
				Name:      ipAddrClaimName,
			}

			log := log.WithValues("IPAddressClaim", klog.KRef(ipAddrClaimKey.Namespace, ipAddrClaimKey.Name))
			ctx := ctrl.LoggerInto(ctx, log)

			err := r.Client.Get(ctx, ipAddrClaimKey, ipAddrClaim)
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get IPAddressClaim %s err: %v", klog.KRef(ipAddrClaimKey.Namespace, ipAddrClaimKey.Name), err)
			}

			ipAddrClaim, created, err := createOrPatchIPAddressClaim(ctx, r.Client, machine, ipAddrClaimName, network.AddressFromPool)
			if err != nil {
				errList = append(errList, err)
				continue
			}

			if created {
				claimsCreated++
			}
			if ipAddrClaim.Status.AddressRef.Name != "" {
				claimsFulfilled++
			}

			if conditions.Has(ipAddrClaim, clusterv1.ReadyCondition) {
				claims = append(claims, ipAddrClaim)
			}
		}
	}

	if len(errList) > 0 {
		aggregatedErr := kerrors.NewAggregate(errList)
		conditions.MarkFalse(machine,
			infrav1.IPAddressClaimedCondition,
			infrav1.IPAddressClaimNotFoundReason,
			clusterv1.ConditionSeverityError,
			aggregatedErr.Error())
		return aggregatedErr
	}

	// Fallback logic to calculate the state of the IPAddressClaimed condition
	switch {
	case totalClaims == claimsFulfilled:
		conditions.MarkTrue(machine, infrav1.IPAddressClaimedCondition)
	case claimsFulfilled < totalClaims && claimsCreated > 0:
		conditions.MarkFalse(machine, infrav1.IPAddressClaimedCondition,
			infrav1.IPAddressClaimsBeingCreatedReason, clusterv1.ConditionSeverityInfo,
			"%d/%d claims being created", claimsCreated, totalClaims)
	case claimsFulfilled < totalClaims && claimsCreated == 0:
		conditions.MarkFalse(machine, infrav1.IPAddressClaimedCondition,
			infrav1.WaitingForIPAddressReason, clusterv1.ConditionSeverityInfo,
			"%d/%d claims being processed", totalClaims-claimsFulfilled, totalClaims)
	}

	return nil
}

func createOrPatchIPAddressClaim(ctx context.Context, client client.Client, machine *infrav1.PacketMachine, name string, poolRef corev1.TypedLocalObjectReference) (*ipamv1.IPAddressClaim, bool, error) {
	claim := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machine.Namespace,
		},
	}

	mutateFn := func() (err error) {
		claim.SetOwnerReferences(clusterutilv1.EnsureOwnerRef(
			claim.OwnerReferences,
			metav1.OwnerReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       "PacketMachine",
				Name:       machine.Name,
				UID:        machine.UID,
			}))

		ctrlutil.AddFinalizer(claim, infrav1.IPAddressClaimFinalizer)

		if claim.Labels == nil {
			claim.Labels = make(map[string]string)
		}
		claim.Labels[clusterv1.ClusterNameLabel] = machine.Labels[clusterv1.ClusterNameLabel]

		claim.Spec.PoolRef.APIGroup = poolRef.APIGroup
		claim.Spec.PoolRef.Kind = poolRef.Kind
		claim.Spec.PoolRef.Name = poolRef.Name
		return nil
	}

	log := ctrl.LoggerFrom(ctx)

	result, err := ctrlutil.CreateOrPatch(ctx, client, claim, mutateFn)
	if err != nil {
		return nil, false, fmt.Errorf("failed to CreateOrPatch IPAddressClaim, err: : %s", err)
	}
	switch result {
	case ctrlutil.OperationResultCreated:
		log.Info("Created IPAddressClaim")
		return claim, true, nil
	case ctrlutil.OperationResultUpdated:
		log.Info("Updated IPAddressClaim")
	case ctrlutil.OperationResultNone, ctrlutil.OperationResultUpdatedStatus, ctrlutil.OperationResultUpdatedStatusOnly:
		log.V(3).Info("No change required for IPAddressClaim", "operationResult", result)
	}
	return claim, false, nil
}

func getIPAddressCfg(ctx context.Context, client client.Client, machine *infrav1.PacketMachine) ([]packet.IPAddressCfg, error) {
	log := ctrl.LoggerFrom(ctx)

	boundClaims, totalClaims := 0, 0
	ipaddrCfgs := []packet.IPAddressCfg{}

	for portIdx, port := range machine.Spec.NetworkPorts {
		for networkIdx, network := range port.Networks {
			totalClaims++

			ipAddrClaimName := packet.IPAddressClaimName(machine.Name, portIdx, networkIdx)

			log := log.WithValues("IPAddressClaim", klog.KRef(machine.Namespace, ipAddrClaimName))

			ctx := ctrl.LoggerInto(ctx, log)

			ipAddrClaim, err := getIPAddrClaim(ctx, client, ipAddrClaimName, machine.Namespace)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// it would be odd for this to occur, a findorcreate just happened in a previous step
					continue
				}
				return nil, fmt.Errorf("failed to get IPAddressClaim %s, err: %v", klog.KRef(machine.Namespace, ipAddrClaimName), err)
			}

			log.V(5).Info("Fetched IPAddressClaim")
			ipAddrName := ipAddrClaim.Status.AddressRef.Name
			if ipAddrName == "" {
				log.V(5).Info("IPAddress not yet bound to IPAddressClaim")
				continue
			}

			ipAddr := &ipamv1.IPAddress{}
			ipAddrKey := apitypes.NamespacedName{
				Namespace: machine.Namespace,
				Name:      ipAddrName,
			}
			if err := client.Get(ctx, ipAddrKey, ipAddr); err != nil {
				// because the ref was set on the claim, it is expected this error should not occur
				return nil, err
			}
			ipaddrCfgs = append(ipaddrCfgs, packet.IPAddressCfg{
				VXLAN:    network.VXLAN,
				Address:  ipAddr.Spec.Address,
				Netmask:  net.IP(net.CIDRMask(ipAddr.Spec.Prefix, 32)).String(),
				PortName: port.Name,
				Layer2:   port.Layer2,
				Bonded:   port.Bonded,
			})
			boundClaims++
		}
	}

	if boundClaims < totalClaims {
		log.Info("Waiting for ip address claims to be bound",
			"total claims", totalClaims,
			"claims bound", boundClaims)
		return nil, fmt.Errorf("waiting for IP address claims to be bound")
	}

	return ipaddrCfgs, nil
}

func getRoutesCfg(machine *infrav1.PacketMachine) ([]layer2.RouteSpec, error) {
	routes := []layer2.RouteSpec{}

	for _, route := range machine.Spec.Routes {
		routes = append(routes, layer2.RouteSpec{
			Destination: route.Destination,
			Gateway:     route.Gateway,
		})
	}

	if len(routes) == 0 {
		return nil, nil
	}

	return routes, nil
}



func checkIfEventsContainNetworkConfigurationSuccess(eventsList *metal.EventList) bool {
	networkConfigurationSuccess := "network_configuration_success"

	for _, event := range eventsList.Events {
		if event.Body == nil {
            continue
        }

		if *event.Body == networkConfigurationSuccess {
			return true
		}
	}
	return false
}

func checkIfEventsContainNetworkConfigurationFailure(eventsList *metal.EventList) bool {
	var networkConfigurationFailure *string = new(string)
	*networkConfigurationFailure = "network_configuration_failure"

	for _, event := range eventsList.Events {
		if event.Body == networkConfigurationFailure {
			return true
		}
	}
	return false
}

// reconcilePortConfigurations manages port configurations for a given device
// It ensures that the ports are configured with the correct VXLAN, layer2, and bonding settings
func (r *PacketMachineReconciler) reconcilePortConfigurations(ctx context.Context, deviceID string, desiredConfigs []packet.IPAddressCfg) error {
	// Fetch the device details
	device, _, err := r.PacketClient.GetDevice(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("failed to get device %s: %w", deviceID, err)
	}

	for _, desiredConfig := range desiredConfigs {
		if err := r.reconcilePortConfig(ctx, device.NetworkPorts, desiredConfig); err != nil {
			return fmt.Errorf("failed to reconcile port %s: %w", desiredConfig.PortName, err)
		}
	}

	return nil
}

// reconcilePortConfig handles the configuration for a single port
func (r *PacketMachineReconciler) reconcilePortConfig(ctx context.Context, networkPorts []metal.Port, desiredConfig packet.IPAddressCfg) error {
	log := ctrl.LoggerFrom(ctx)

	portID, err := getMetalPortID(desiredConfig.PortName, networkPorts)
	if err != nil {
		return fmt.Errorf("failed to get port ID for %s: %w", desiredConfig.PortName, err)
	}

	if err := r.reconcileVXLAN(ctx, *portID, desiredConfig); err != nil {
		return err
	}

	if err := r.reconcileLayer2AndBonding(ctx, *portID, desiredConfig); err != nil {
		return err
	}

	log.Info("Port configuration reconciled successfully", "port", desiredConfig.PortName)
	return nil
}

// reconcileVXLAN ensures the port is assigned to the correct VXLAN
func (r *PacketMachineReconciler) reconcileVXLAN(ctx context.Context, portID string, desiredConfig packet.IPAddressCfg) error {
	log := ctrl.LoggerFrom(ctx)

	currentAssignments, err := r.getCurrentVXLANAssignments(ctx, portID)
	if err != nil {
		return err
	}

	desiredVXLAN := int32(desiredConfig.VXLAN)
	desiredVXLANStr := strconv.Itoa(desiredConfig.VXLAN)
	desiredStateExists := false

	for _, currentVXLAN := range currentAssignments {
		if currentVXLAN == desiredVXLAN {
			desiredStateExists = true
			continue
		}
		if err := r.unassignVXLAN(ctx, portID, currentVXLAN); err != nil {
			return err
		}
	}

	if !desiredStateExists {
		if err := r.assignVXLAN(ctx, portID, desiredVXLANStr); err != nil {
			return err
		}
		log.Info("Port assigned to VXLAN", "port", desiredConfig.PortName, "vxlan", desiredVXLAN)
	} else {
		log.Info("Port already assigned to desired VXLAN", "port", desiredConfig.PortName, "vxlan", desiredVXLAN)
	}

	return nil
}

// reconcileLayer2AndBonding ensures the port is set to the correct layer2 and bonding configuration
func (r *PacketMachineReconciler) reconcileLayer2AndBonding(ctx context.Context, portID string, desiredConfig packet.IPAddressCfg) error {
	port, _, err := r.PacketClient.PortsApi.FindPortById(ctx, portID).Execute()
	if err != nil {
		return fmt.Errorf("failed to get port %s: %w", portID, err)
	}
	if port == nil {
		return fmt.Errorf("port %s not found", portID)
	}

	if desiredConfig.Layer2 {
		if err := r.setLayer2(ctx, portID, port, desiredConfig.PortName); err != nil {
			return err
		}
	}

	if desiredConfig.Bonded {
		if err := r.setBonding(ctx, portID, port, desiredConfig.PortName); err != nil {
			return err
		}
	}

	return nil
}

// getCurrentVXLANAssignments fetches the current VXLAN assignments for a port
func (r *PacketMachineReconciler) getCurrentVXLANAssignments(ctx context.Context, portID string) ([]int32, error) {
	vlanAssignList, resp, err := r.PacketClient.PortsApi.FindPortVlanAssignments(ctx, portID).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to get port VLAN assignments: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get port VLAN assignments: unexpected status code %d", resp.StatusCode)
	}
	return getPortVXLANAssignments(vlanAssignList), nil
}

// unassignVXLAN removes a VXLAN assignment from a port
func (r *PacketMachineReconciler) unassignVXLAN(ctx context.Context, portID string, vxlan int32) error {
	vxlanStr := strconv.Itoa(int(vxlan))
	_, resp, err := r.PacketClient.PortsApi.UnassignPort(ctx, portID).PortAssignInput(metal.PortAssignInput{
		Vnid: &vxlanStr,
	}).Execute()
	if err != nil {
		return fmt.Errorf("failed to unassign port from VXLAN %d: %w", vxlan, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to unassign port from VXLAN %d: unexpected status code %d", vxlan, resp.StatusCode)
	}
	return nil
}

// assignVXLAN assigns a VXLAN to a port
func (r *PacketMachineReconciler) assignVXLAN(ctx context.Context, portID, vxlanStr string) error {
	_, resp, err := r.PacketClient.PortsApi.AssignPort(ctx, portID).PortAssignInput(metal.PortAssignInput{
		Vnid: &vxlanStr,
	}).Execute()
	if err != nil {
		return fmt.Errorf("failed to assign port to VXLAN %s: %w", vxlanStr, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to assign port to VXLAN %s: unexpected status code %d", vxlanStr, resp.StatusCode)
	}
	return nil
}

// setLayer2 configures the port for layer2 if not already set
func (r *PacketMachineReconciler) setLayer2(ctx context.Context, portID string, port *metal.Port, portName string) error {
	log := ctrl.LoggerFrom(ctx)

	if strings.Contains(string(*port.NetworkType), "layer2") {
		log.Info("Port is already set to layer2", "port", portName)
		return nil
	}

	_, resp, err := r.PacketClient.PortsApi.ConvertLayer2(ctx, portID).Execute()
	if err != nil {
		return fmt.Errorf("failed to set port to layer2: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set port to layer2: unexpected status code %d", resp.StatusCode)
	}
	log.Info("Port set to layer2", "port", portName)
	return nil
}

// setBonding configures the port for bonding if not already set
func (r *PacketMachineReconciler) setBonding(ctx context.Context, portID string, port *metal.Port, portName string) error {
	log := ctrl.LoggerFrom(ctx)

	if port.Data == nil {
		return fmt.Errorf("failed to get port data to check bonded status")
	}
	if *port.Data.Bonded {
		log.Info("Port is already bonded", "port", portName)
		return nil
	}

	_, resp, err := r.PacketClient.PortsApi.BondPort(ctx, portID).Execute()
	if err != nil {
		return fmt.Errorf("failed to set port to bonded: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set port to bonded: unexpected status code %d", resp.StatusCode)
	}
	log.Info("Port set to bonded", "port", portName)
	return nil
}
// getPortVXLANAssignments returns all current VXLAN assignments for a port
func getPortVXLANAssignments(vlanAssignList *metal.PortVlanAssignmentList) []int32 {
    var assignments []int32
    for _, vlanAssign := range vlanAssignList.VlanAssignments {
        if vlanAssign.Vlan != nil {
            assignments = append(assignments, *vlanAssign.Vlan)
        }
    }
    return assignments
}

func getMetalPortID(portName string, networkPorts []metal.Port) (*string, error) {
	for _, port := range networkPorts {
		if *port.Name == portName {
			return port.Id, nil
		}
	}
	return nil, fmt.Errorf("failed to find port %s", portName)
}

func getIPAddrClaim(ctx context.Context, client client.Client, ipAddrClaimName, namespace string) (*ipamv1.IPAddressClaim, error) {
	log := ctrl.LoggerFrom(ctx)

	ipAddrClaim := &ipamv1.IPAddressClaim{}
	ipAddrClaimKey := apitypes.NamespacedName{
		Namespace: namespace,
		Name:      ipAddrClaimName,
	}

	log.V(5).Info("Fetching IPAddressClaim", "IPAddressClaim", klog.KRef(ipAddrClaimKey.Namespace, ipAddrClaimKey.Name))
	if err := client.Get(ctx, ipAddrClaimKey, ipAddrClaim); err != nil {
		return nil, err
	}
	return ipAddrClaim, nil
}