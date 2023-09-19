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
	"net/url"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "sigs.k8s.io/cluster-api-provider-packet/api/v1beta1"
)

const (
	providerIDPrefix           = "equinixmetal"
	deprecatedProviderIDPrefix = "packet"
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

	workloadClientGetter remote.ClusterClientGetter
}

// NewMachineScope creates a new MachineScope from the supplied parameters.
// This is meant to be called for each reconcile iteration
// both PacketClusterReconciler and PacketMachineReconciler.
func NewMachineScope(ctx context.Context, params MachineScopeParams) (*MachineScope, error) {
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

	providerIDPrefix, err := getProviderIDPrefix(ctx, params.Client, params.workloadClientGetter,
		params.Cluster, params.Machine, params.PacketMachine)
	if err != nil {
		return nil, err
	}

	helper, err := patch.NewHelper(params.PacketMachine, params.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to init patch helper: %w", err)
	}
	return &MachineScope{
		client:           params.Client,
		patchHelper:      helper,
		providerIDPrefix: providerIDPrefix,

		Cluster:       params.Cluster,
		Machine:       params.Machine,
		PacketCluster: params.PacketCluster,
		PacketMachine: params.PacketMachine,
	}, nil
}

// MachineScope defines a scope defined around a machine and its cluster.
type MachineScope struct {
	client           client.Client
	patchHelper      *patch.Helper
	providerIDPrefix string

	Cluster       *clusterv1.Cluster
	Machine       *clusterv1.Machine
	PacketCluster *infrav1.PacketCluster
	PacketMachine *infrav1.PacketMachine
}

// Close the MachineScope by updating the machine spec, machine status.
func (m *MachineScope) Close() error {
	return m.PatchObject(context.TODO())
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

// GetProviderID returns the DOMachine providerID from the spec.
func (m *MachineScope) GetProviderID() string {
	if m.PacketMachine.Spec.ProviderID != nil {
		return *m.PacketMachine.Spec.ProviderID
	}
	return ""
}

// SetProviderID sets the DOMachine providerID in spec from device id.
func (m *MachineScope) SetProviderID(deviceID string) {
	pid := fmt.Sprintf("%s://%s", m.providerIDPrefix, deviceID)
	m.PacketMachine.Spec.ProviderID = ptr.To(pid)
}

// GetInstanceID returns the DOMachine droplet instance id by parsing Spec.ProviderID.
func (m *MachineScope) GetInstanceID() string {
	parsed, err := noderefutil.NewProviderID(m.GetProviderID())
	if err != nil {
		return ""
	}
	return parsed.ID()
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

// getProviderIDPrefix attempts to determine what providerID prefix should be used for this PacketMachine based on the following precedence:
// - If the PacketMachine already has a providerID defined, use the prefix from that providerID
// - If the workload cluster is already responding, attempt to determine the prefix to use based on the cloud provider deployed
// - If the bootstrap provider being used is the KubeadmConfig bootstrap provider, attempt to determine the prefix to use based on the bootstrap configuration
// - Otherwise, default to using "equinixmetal" as the prefix
// This order is used, because the underlying Node providerIDs are immutable, so we should treat the PacketMachine providerIDs similarly.
// When the providerID has not already been set, we should always prioritize setting the providerID prefix to match the deployed version
// of the cloud-provider, if possible. This ensures that we set the providerID in a way that will match what is configured on the node by
// the cloud-provider. However, when the workload cluster is first being bootstrapped, we cannot access the (yet to be provisioned) api server
// to query the actively deployed cloud-provider, so we need to attempt to determine which cloud-provider will be deployed through the
// bootstrapping configuration. If we cannot determine the providerID through any of those means, then we should assume that
// cloud-provider-equinix-metal will be used and default to "equinixmetal" as the prefix.
func getProviderIDPrefix(ctx context.Context, mgmtClient client.Client, workloadClientGetter remote.ClusterClientGetter, cluster *clusterv1.Cluster, machine *clusterv1.Machine, packetMachine *infrav1.PacketMachine) (string, error) {
	// Use existing prefix if already defined
	if existingPrefix := providerIDPrefixFromPacketMachine(packetMachine); existingPrefix != "" {
		return existingPrefix, nil
	}

	// Try to determine the appropriate prefix from any known cloud-provider deployments
	fromDeployments, err := providerIDFromCloudProviderDeployments(ctx, mgmtClient, workloadClientGetter, cluster)
	if err != nil {
		return "", err
	}
	if fromDeployments != "" {
		return fromDeployments, nil
	}

	// Try to determine the appropriate prefix from the KubeadmConfig if configured to use the KubeadmConfig bootstrapper
	if machine.Spec.Bootstrap.ConfigRef != nil && machine.Spec.Bootstrap.ConfigRef.Kind == "KubeadmConfig" {
		fromKubeadmConfig, err := providerIDFromKubeadmConfig(ctx, mgmtClient, packetMachine.Namespace, machine.Spec.Bootstrap.ConfigRef.Name)
		if err != nil {
			return "", err
		}
		if fromKubeadmConfig != "" {
			return fromKubeadmConfig, nil
		}
	}

	return providerIDPrefix, nil
}

// providerIDFromKubeadmConfig attempts to determine the appropriate providerID prefix to use based on the configuration
// of the referenced KubeadmConfig resource. It uses the following precedence:
//   - If an explicit providerID is being configured for the kubelet through extra arguments in the InitConfiguration, use it
//   - If an explicit providerID is being configured for the kubelet through extra arguments in the JoinConfiguration, use it
//     (InitConfiguration and JoinConfiguration are mutually exclusive options)
//   - If the PostKubeadmCommands are deploying packet-ccm, use "packet"
//   - If the PostKubeadmCommands are deploying cloud-provider-equinix-metal, use "equinixmetal"
//   - Otherwise, return ""
func providerIDFromKubeadmConfig(ctx context.Context, mgmtClient client.Client, namespace, name string) (string, error) {
	kubeadmConfig := new(bootstrapv1.KubeadmConfig)
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}

	if err := mgmtClient.Get(ctx, key, kubeadmConfig); err != nil {
		return "", fmt.Errorf("failed to get bootstrap resource: %w", err)
	}

	// providerid being set explicitly in the InitConfiguration
	if kubeadmConfig.Spec.InitConfiguration != nil && kubeadmConfig.Spec.InitConfiguration.NodeRegistration.KubeletExtraArgs != nil {
		if value, ok := kubeadmConfig.Spec.InitConfiguration.NodeRegistration.KubeletExtraArgs["provider-id"]; ok {
			if parsed, err := noderefutil.NewProviderID(value); err == nil {
				return parsed.CloudProvider(), nil
			}
		}
	}

	// providerid being set explicitly in the JoinConfiguration
	if kubeadmConfig.Spec.JoinConfiguration != nil && kubeadmConfig.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs != nil {
		if value, ok := kubeadmConfig.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs["provider-id"]; ok {
			if parsed, err := noderefutil.NewProviderID(value); err == nil {
				return parsed.CloudProvider(), nil
			}
		}
	}

	// inspect postkubeadmcommands
	for i := range kubeadmConfig.Spec.PostKubeadmCommands {
		if strings.Contains(kubeadmConfig.Spec.PostKubeadmCommands[i], "github.com/packethost/packet-ccm") {
			return deprecatedProviderIDPrefix, nil
		}

		if strings.Contains(kubeadmConfig.Spec.PostKubeadmCommands[i], "github.com/equinix/cloud-provider-equinix-metal") {
			return providerIDPrefix, nil
		}
	}

	return "", nil
}

// providerIDFromCloudProviderDeployments attempts to introspect the workload cluster (if already available) and determine which providerID
// prefix should be used based on the deployed cloud provider. If it detects packet-ccm, then it should return "packet", if it detects
// cloud-provider-equinix-metal, then it should return "equinixmetal", otherwise it should return "".
func providerIDFromCloudProviderDeployments(ctx context.Context, mgmtClient client.Client, workloadClientGetter remote.ClusterClientGetter, cluster *clusterv1.Cluster) (string, error) {
	if workloadClientGetter == nil {
		workloadClientGetter = remote.NewClusterClient
	}

	workloadClient, err := workloadClientGetter(ctx, "capp", mgmtClient, client.ObjectKeyFromObject(cluster))
	if err != nil {
		// Generating the workload client can throw a URL error if the
		// apiserver is not yet responding, so we need to swallow
		// a timeout error.
		var urlError *url.Error
		switch {
		case errors.As(err, &urlError):
			if urlError.Timeout() {
				return "", nil
			}

			return "", fmt.Errorf("failed to get workload cluster client: %w", err)
		default:
			return "", fmt.Errorf("failed to get workload cluster client: %w", err)
		}
	}

	err = hasWorkloadDeployment(ctx, workloadClient, metav1.NamespaceSystem, "cloud-provider-equinix-metal")
	switch {
	case err == nil:
		// CPEM is deployed, use equinixmetal
		return providerIDPrefix, nil
	case err != nil:
		// This is needed because apierrors doesn't handle wrapped errors
		// in the v0.17.17, can use client.IgnoreNotFound with later versions
		// of kubernetes dependencies.
		var apiError *apierrors.StatusError
		if errors.As(err, &apiError) {
			if !apierrors.IsNotFound(apiError) {
				return "", err
			}
		} else {
			return "", err
		}
	}

	err = hasWorkloadDeployment(ctx, workloadClient, metav1.NamespaceSystem, "packet-cloud-controller-manager")
	switch {
	case err == nil:
		// packet-ccm is deployed, use packet
		return deprecatedProviderIDPrefix, nil
	case err != nil:
		// This is needed because apierrors doesn't handle wrapped errors
		// in the v0.17.17, can use client.IgnoreNotFound with later versions
		// of kubernetes dependencies.
		var apiError *apierrors.StatusError
		if errors.As(err, &apiError) {
			if !apierrors.IsNotFound(apiError) {
				return "", err
			}
		} else {
			return "", err
		}
	}

	return "", nil
}

func hasWorkloadDeployment(ctx context.Context, workloadClient client.Client, namespace, name string) error {
	deployment := new(appsv1.Deployment)
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}

	if err := workloadClient.Get(ctx, key, deployment); err != nil {
		return fmt.Errorf("failed to query workload cluster for %s: %w", key.String(), err)
	}

	return nil
}

func providerIDPrefixFromPacketMachine(packetMachine *infrav1.PacketMachine) string {
	preexistingProviderID := ptr.Deref(packetMachine.Spec.ProviderID, "")
	if preexistingProviderID != "" {
		if parsed, err := noderefutil.NewProviderID(preexistingProviderID); err == nil {
			return parsed.CloudProvider()
		}
	}

	return ""
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
