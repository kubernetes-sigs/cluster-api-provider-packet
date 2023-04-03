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
	allErrs := field.ErrorList{}

	// If both Metro and Facility are set, ignore Facility, we'll leave this to
	// the controller to deal with - the facility will need to reside in the
	// metro.

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("PacketMachine").GroupKind(), m.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (m *PacketMachine) ValidateUpdate(old runtime.Object) error {
	machineLog.Info("validate update", "name", m.Name)
	var allErrs field.ErrorList

	// Must have only one of Metro or Facility specified
	if len(m.Spec.Facility) > 0 && len(m.Spec.Metro) > 0 {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "Metro"),
				m.Spec.Metro, "field and Facility field are mutually exclusive"),
		)
	}

	newPacketMachine, err := runtime.DefaultUnstructuredConverter.ToUnstructured(m)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(nil, errors.Wrap(err,
				"failed to convert new PacketMachine to unstructured object")))
	}

	oldPacketMachine, err := runtime.DefaultUnstructuredConverter.ToUnstructured(old)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(nil, errors.Wrap(err,
				"failed to convert old PacketMachine to unstructured object")))
	}

	newPacketMachineSpec, _ := newPacketMachine["spec"].(map[string]interface{})
	oldPacketMachineSpec, _ := oldPacketMachine["spec"].(map[string]interface{})

	// allow changes to providerID
	delete(oldPacketMachineSpec, "providerID")
	delete(newPacketMachineSpec, "providerID")

	// allow changes to tags
	delete(oldPacketMachineSpec, "tags")
	delete(newPacketMachineSpec, "tags")

	// allow changes to facility
	delete(oldPacketMachineSpec, "facility")
	delete(newPacketMachineSpec, "facility")

	// allow changes to metro
	delete(oldPacketMachineSpec, "metro")
	delete(newPacketMachineSpec, "metro")

	if !reflect.DeepEqual(oldPacketMachineSpec, newPacketMachineSpec) {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec"),
				m.Spec, "cannot be modified"),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("PacketMachine").GroupKind(), m.Name, allErrs)
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
