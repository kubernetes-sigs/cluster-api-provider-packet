package layer2

import (
	corev1 "k8s.io/api/core/v1"
)

type IPAddressManagement interface {
	// CreateClusterIPAssignmentConfigMap creates a ConfigMap object for tracking IP address assignments per port and VLAN at the cluster level.
	CreateClusterIPAssignmentConfigMap(clusterName string) error
	// GetClusterIPAssignmentConfigMap returns the ConfigMap map object for tracking IP address assignments per port and VLAN at the cluster level.
	// It returns the ConfigMap object and a boolean indicating if the ConfigMap was found or not.
	GetClusterIPAssignmentConfigMap(clusterName string) (*corev1.ConfigMap, bool, error)
	// UpdateClusterIPAssignmentConfigMap updates the ConfigMap object.
	// The Data field of the ConfigMap looks like this:
	// 	Data:
	//   "da-1000": |
	//     {
	//       "machine-1": "10.60.10.2",
	//       "machine-2": "10.60.10.3",
	//       "machine-3": "10.60.10.4"
	//     }
	// The key is the combination of the <metro-vxlan> and the value is a JSON object with the machine name as the key and the IP address as the value.
	UpdateClusterIPAssignmentConfigMap(clusterName, metro, vxlan, machineID string) error

	// GetNextAvailableIPAddress returns the next available IP address for the given cluster, metro, and vxlan from the assignment range.
	// It queries the ConfigMap to get the list of assigned IP addresses and returns the next available IP address from the assignment range.
	// For example, if the assignment range is 10.60.10.2-10.60.10.8 and the IP addresses 10.60.10.2, 10.60.10.3 are already assigned, it will return
	// the next in the sequence.
	GetNextAvailableIPAddress(clusterName, metro, assignmentRange, vxlan string) (string, error)

	// GetIPAddressForMachine returns the IP address assigned to the machine.
	// It queries the ConfigMap to get the list of assigned IP addresses for <metro-vxlan> and returns the IP address assigned to the machine.
	GetIPAddressForMachine(clusterName, machineID, metro, vxlan string) (string, error)

	// ReleaseIPAddress releases the IP address assigned to the machine.
	// It queries the ConfigMap to get the list of assigned IP addresses for <metro-vxlan> and removes the IP address assigned to the machine.
	ReleaseIPAddress(clusterName, metro, vxlan, machineID, IPAddress string) error

	// DeleteClusterIPAssignmentConfigMap deletes the ConfigMap object for tracking IP address assignments per port and VLAN at the cluster level.
	DeleteClusterIPAssignmentConfigMap(clusterName string) error
}