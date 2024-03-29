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
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
)

const (
	// ProviderIDPrefix will be appended to the beginning of Equinix Metal resource IDs to form the Kubernetes Provider ID.
	// NOTE: this format matches the 2 slashes format used in cloud-provider and cluster-autoscaler.
	ProviderIDPrefix = "equinixmetal://"
)

var (
	// ErrMissingClient is returned when a client is not provided to the MachineScope.
	ErrMissingClient = errors.New("client is required when creating a MachineScope")
	// ErrMissingCluster is returned when a cluster is not provided to the MachineScope.
	ErrMissingCluster = errors.New("cluster is required when creating a MachineScope")
	// ErrMissingMachine is returned when a machine is not provided to the MachineScope.
	ErrMissingMachine = errors.New("machine is required when creating a MachineScope")
	// ErrMissingPacketCluster is returned when a packetCluster is not provided to the MachineScope.
	ErrMissingPacketCluster = errors.New("packetCluster is required when creating a MachineScope")
	// ErrMissingPacketMachine is returned when a packetMachine is not provided to the MachineScope.
	ErrMissingPacketMachine = errors.New("packetMachine is required when creating a MachineScope")
	// ErrMissingBootstrapDataSecret is returned when the bootstrap data secret is not found.
	ErrMissingBootstrapDataSecret = errors.New("error retrieving bootstrap data: linked Machine's bootstrap.dataSecretName is nil")
	// ErrBootstrapDataMissingKey is returned when the bootstrap data secret does not contain the "value" key.
	ErrBootstrapDataMissingKey = errors.New("error retrieving bootstrap data: secret value key is missing")
)

// MachineScopeParams defines the input parameters used to create a new MachineScope.
type MachineScopeParams struct {
	Client        client.Client
	Cluster       *clusterv1.Cluster
	Machine       *clusterv1.Machine
	PacketCluster *infrav1.PacketCluster
	PacketMachine *infrav1.PacketMachine
}

// NewMachineScope creates a new MachineScope from the supplied parameters.
// This is meant to be called for each reconcile iteration
// both PacketClusterReconciler and PacketMachineReconciler.
func NewMachineScope(params MachineScopeParams) (*MachineScope, error) {
	if params.Client == nil {
		return nil, ErrMissingClient
	}
	if params.Machine == nil {
		return nil, ErrMissingMachine
	}
	if params.Cluster == nil {
		return nil, ErrMissingCluster
	}
	if params.PacketCluster == nil {
		return nil, ErrMissingPacketCluster
	}
	if params.PacketMachine == nil {
		return nil, ErrMissingPacketMachine
	}

	helper, err := patch.NewHelper(params.PacketMachine, params.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to init patch helper: %w", err)
	}
	return &MachineScope{
		client:        params.Client,
		patchHelper:   helper,
		Cluster:       params.Cluster,
		Machine:       params.Machine,
		PacketCluster: params.PacketCluster,
		PacketMachine: params.PacketMachine,
	}, nil
}

// MachineScope defines a scope defined around a machine and its cluster.
type MachineScope struct {
	client        client.Client
	patchHelper   *patch.Helper
	Cluster       *clusterv1.Cluster
	Machine       *clusterv1.Machine
	PacketCluster *infrav1.PacketCluster
	PacketMachine *infrav1.PacketMachine
}

// Close the MachineScope by updating the machine spec, machine status.
func (m *MachineScope) Close(ctx context.Context) error {
	return m.PatchObject(ctx)
}

// Name returns the PacketMachine name.
func (m *MachineScope) Name() string {
	return m.PacketMachine.Name
}

// Namespace returns the PacketMachine namespace.
func (m *MachineScope) Namespace() string {
	return m.PacketMachine.Namespace
}

// IsControlPlane returns true if the machine is a control plane.
func (m *MachineScope) IsControlPlane() bool {
	return util.IsControlPlaneMachine(m.Machine)
}

// Role returns the machine role from the labels.
func (m *MachineScope) Role() string {
	if util.IsControlPlaneMachine(m.Machine) {
		return infrav1.ControlPlaneTag
	}
	return infrav1.WorkerTag
}

// GetDeviceID returns the PacketMachine device ID by parsing the scope's providerID.
func (m *MachineScope) GetDeviceID() string {
	return parseProviderID(m.ProviderID())
}

// ProviderID returns the PacketMachine providerID from the spec.
func (m *MachineScope) ProviderID() string {
	return ptr.Deref(m.PacketMachine.Spec.ProviderID, "")
}

// SetProviderID sets the PacketMachine providerID in spec from device id.
func (m *MachineScope) SetProviderID(deviceID string) {
	pid := fmt.Sprintf("%s%s", ProviderIDPrefix, deviceID)
	m.PacketMachine.Spec.ProviderID = ptr.To(pid)
}

// GetInstanceStatus returns the PacketMachine device instance status from the status.
func (m *MachineScope) GetInstanceStatus() *infrav1.PacketResourceStatus {
	return m.PacketMachine.Status.InstanceStatus
}

// SetInstanceStatus sets the PacketMachine device id.
func (m *MachineScope) SetInstanceStatus(v infrav1.PacketResourceStatus) {
	m.PacketMachine.Status.InstanceStatus = &v
}

// SetReady sets the PacketMachine Ready Status.
func (m *MachineScope) SetReady() {
	m.PacketMachine.Status.Ready = true
}

// SetNotReady sets the PacketMachine Ready Status.
func (m *MachineScope) SetNotReady() {
	m.PacketMachine.Status.Ready = false
}

// SetFailureMessage sets the PacketMachine status error message.
func (m *MachineScope) SetFailureMessage(v error) {
	m.PacketMachine.Status.FailureMessage = ptr.To(v.Error())
}

// SetFailureReason sets the PacketMachine status error reason.
func (m *MachineScope) SetFailureReason(v capierrors.MachineStatusError) {
	m.PacketMachine.Status.FailureReason = &v
}

// SetAddresses sets the address status.
func (m *MachineScope) SetAddresses(addrs []corev1.NodeAddress) {
	m.PacketMachine.Status.Addresses = addrs
}

// Tags returns Tags from the scope's PacketMachine. The returned value will never be nil.
func (m *MachineScope) Tags() infrav1.Tags {
	if m.PacketMachine.Spec.Tags == nil {
		m.PacketMachine.Spec.Tags = infrav1.Tags{}
	}

	return m.PacketMachine.Spec.Tags.DeepCopy()
}

// GetRawBootstrapData returns the bootstrap data from the secret in the Machine's bootstrap.dataSecretName.
func (m *MachineScope) GetRawBootstrapData(ctx context.Context) ([]byte, error) {
	if m.Machine.Spec.Bootstrap.DataSecretName == nil {
		return nil, ErrMissingBootstrapDataSecret
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: m.Namespace(), Name: *m.Machine.Spec.Bootstrap.DataSecretName}
	if err := m.client.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("failed to retrieve bootstrap data secret for PacketMachine %s/%s: %w", m.Namespace(), m.Name(), err)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return nil, ErrBootstrapDataMissingKey
	}

	return value, nil
}

// PatchObject persists the machine spec and status.
func (m *MachineScope) PatchObject(ctx context.Context) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding during the deletion process).
	applicableConditions := []clusterv1.ConditionType{
		infrav1.DeviceReadyCondition,
	}

	conditions.SetSummary(m.PacketMachine,
		conditions.WithConditions(applicableConditions...),
		conditions.WithStepCounterIf(m.PacketMachine.ObjectMeta.DeletionTimestamp.IsZero()),
		conditions.WithStepCounter(),
	)

	return m.patchHelper.Patch(
		ctx,
		m.PacketMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.DeviceReadyCondition,
		}})
}

// ParseProviderID parses a string to a PacketMachine Provider ID, first removing the "equinixmetal://" prefix.
func parseProviderID(id string) string {
	return strings.TrimPrefix(id, ProviderIDPrefix)
}
