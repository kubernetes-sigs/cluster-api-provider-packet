package deployer

// MachineNotFound error representing that the requested device was not yet found
type MachineNotFound struct {
	err string
}

func (e *MachineNotFound) Error() string {
	return e.err
}

// MachineNoIP error representing that the requested device does not have an IP yet assigned
type MachineNoIP struct {
	err string
}

func (e *MachineNoIP) Error() string {
	return e.err
}
