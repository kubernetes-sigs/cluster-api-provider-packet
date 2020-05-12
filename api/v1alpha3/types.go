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
