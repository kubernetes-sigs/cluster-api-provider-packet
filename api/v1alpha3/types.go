package v1alpha3

// PacketResourceStatus describes the status of a Packet resource.
type PacketResourceStatus string

var (
	// PacketResourceStatus is the string representing a Packet resource just created.
	PacketResourceStatusNew = PacketResourceStatus("new")
	// PacketResourceStatusQueued is the string representing a Packet resource that is waiting in a queue to be created.
	PacketResourceStatusQueued = PacketResourceStatus("queued")
	// PacketResourceStatusProvisioning is the string reresenting a Packet resource that is in provisioning
	PacketResourceStatusProvisioning = PacketResourceStatus("provisioning")
	// PacketResourceStatusRunning is the string representing a Packet resource already provisioned and in a active state.
	PacketResourceStatusRunning = PacketResourceStatus("active")
	// PacketResourceStatusErrored is the string representing a Packet resource in a errored state.
	PacketResourceStatusErrored = PacketResourceStatus("errored")
	// PacketResourceStatusOff is the string representing a Packet resource in off state.
	PacketResourceStatusOff = PacketResourceStatus("off")
)

// Tags defines a slice of tags.
type Tags []string
