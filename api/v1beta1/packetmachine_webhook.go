/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

import (
	"reflect"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var machineLog = logf.Log.WithName("packetmachine-resource")

func (m *PacketMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-packetmachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=packetmachines,versions=v1beta1,name=validation.packetmachine.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-packetmachine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=packetmachines,versions=v1beta1,name=default.packetmachine.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (m *PacketMachine) ValidateCreate() error {
	machineLog.Info("validate create", "name", m.Name)

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (m *PacketMachine) ValidateUpdate(old runtime.Object) error {
	newPacketMachine, err := runtime.DefaultUnstructuredConverter.ToUnstructured(m)
	if err != nil {
		return apierrors.NewInvalid(GroupVersion.WithKind("PacketMachine").GroupKind(), m.Name, field.ErrorList{
			field.InternalError(nil, errors.Wrap(err, "failed to convert new PacketMachine to unstructured object")),
		})
	}
	oldPacketMachine, err := runtime.DefaultUnstructuredConverter.ToUnstructured(old)
	if err != nil {
		return apierrors.NewInvalid(GroupVersion.WithKind("PacketMachine").GroupKind(), m.Name, field.ErrorList{
			field.InternalError(nil, errors.Wrap(err, "failed to convert old PacketMachine to unstructured object")),
		})
	}

	newPacketMachineSpec, _ := newPacketMachine["spec"].(map[string]interface{})
	oldPacketMachineSpec, _ := oldPacketMachine["spec"].(map[string]interface{})

	// allow changes to providerID
	delete(oldPacketMachineSpec, "providerID")
	delete(newPacketMachineSpec, "providerID")

	// allow changes to tags
	delete(oldPacketMachineSpec, "tags")
	delete(newPacketMachineSpec, "tags")

	if !reflect.DeepEqual(oldPacketMachineSpec, newPacketMachineSpec) {
		return apierrors.NewInvalid(GroupVersion.WithKind("PacketMachine").GroupKind(), m.Name, field.ErrorList{
			field.Forbidden(field.NewPath("spec"), "cannot be modified"),
		})
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (m *PacketMachine) ValidateDelete() error {
	machineLog.Info("validate delete", "name", m.Name)

	return nil
}

// Default implements webhookutil.defaulter so a webhook will be registered for the type.
func (m *PacketMachine) Default() {
	machineLog.Info("default", "name", m.Name)
}
