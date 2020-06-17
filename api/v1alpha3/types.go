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

package v1alpha3

// PacketResourceStatus describes the status of a Packet resource.
type PacketResourceStatus string

var (
	// PacketResourceStatusNew represents a Packet resource requested.
	// The Packet infrastucture uses a queue to avoid any abuse. So a resource
	// does not get created straigh away but it can wait for a bit in a queue.
	PacketResourceStatusNew = PacketResourceStatus("new")
	// PacketResourceStatusQueued represents a device waiting for his turn to be provisioned.
	// Time in queue depends on how many creation requests you already issued, or
	// from how many resources waiting to be deleted we have for you.
	PacketResourceStatusQueued = PacketResourceStatus("queued")
	// PacketResourceStatusProvisioning represents a resource that got dequeued
	// and it is activelly processed by a worker.
	PacketResourceStatusProvisioning = PacketResourceStatus("provisioning")
	// PacketResourceStatusRunning represents a Packet resource already provisioned and in a active state.
	PacketResourceStatusRunning = PacketResourceStatus("active")
	// PacketResourceStatusErrored represents a Packet resource in a errored state.
	PacketResourceStatusErrored = PacketResourceStatus("errored")
	// PacketResourceStatusOff represents a Packet resource in off state.
	PacketResourceStatusOff = PacketResourceStatus("off")
)

// Tags defines a slice of tags.
type Tags []string

// PacketMachineTemplateResource describes the data needed to create am PacketMachine from a template
type PacketMachineTemplateResource struct {
	// Spec is the specification of the desired behavior of the machine.
	Spec PacketMachineSpec `json:"spec"`
}
