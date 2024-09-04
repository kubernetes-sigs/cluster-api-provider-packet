Motivation/Abstract
===================

By default all servers that are created on Equinix Metal via Cluster API have Layer 3 networking and there is no option when provisioning Equinix Metal cluster to specify type of networking for instances or to create additional L2 interfaces with specific local IP addresses and VLAN.

To solve this, CAPP should provide options to specify:

-   Network type (L2/L3/Hybrid)

-   Creating network interfaces with specific VLAN

-   IP address range for L2 interfaces

* * * * *

Limitations
===========

CAPP managed clusters running without internet connections would need to be able to pull images from a repository also in the layer2, or they would need a bastion host that acts as a gateway and NAT. This isn't supported today, so complete Layer2 will not be supported in the initial phases of the feature.

* * * * *

Background
==========

User stories
------------

As a 	**user of Cluster API provider Packet (CAPP)**
I want 	**to configure L2 interfaces and define my own IP Address range**
so that **machines are able to communicate over layer2 VLAN**


* * * * *

Goals
===============

In Phase 1 of integrating Layer2 support, the Cluster API Provider (CAPP) will focus on Bring Your Own (BYO) Infrastructure.
Key objectives for this phase include:
- Implementing Hybrid Bonded Mode and Hybrid Unbonded Modes to enhance Layer2 functionalities in CAPP.
- Enabling CAPP to attach network ports to specific VLANs or VXLANs.
- Allowing CAPP to configure Layer2 networking at the OS level on a metal node, including creating sub-interfaces, assigning IP addresses, and assigning VLANs.
- Ensuring CAPP can track the lifecycle of available IP addresses from a specified range, which may be arbitrarily defined or sourced from VRF


Non-Goals
===============

-   Complete layer2 will not be supported in the initial phases.

-   IPAM Provider will be supported in phase-2

Proposal Design/Approach
========================

* * * * *

**Understanding the context and problem space** : The problem space primarily revolves around the operating system (OS) and, to some extent, the cluster level. Specifically, it concerns how Cluster API (CAPP) clusters and machines are defined by IP addresses, networks, and gateways.
A critical aspect of this space is how CAPP provisions infrastructure, particularly network infrastructure. This includes VLANs, gateways, virtual circuits, and IP address ranges such as elastic IPs or IP reservations. Additionally, it involves the management of VRFs and the attachment of these network resources to nodes, ensuring that newly created nodes have ports in a ready state for these attachments. The default approach will be Layer2 networking in a hybrid-bonded mode, though other configurations may also be supported in the future.
This understanding forms the foundation for addressing the technical challenges in provisioning and managing network infrastructure with CAPP.
 
**Bring Your Own Infrastructure (BYOI)**:

The BYOI approach allows users to leverage their existing infrastructure, such as VLANs, VRFs, Metal Gateways, and similar components.
In this model, users specify the IP ranges to be assigned to metal nodes on VLAN-tagged interfaces. Importantly, CAPP is not responsible for creating or managing this infrastructure, it is assumed to already exist.
However, CAPP needs to be informed of the VLAN ID to attach the network port to the appropriate VLAN using the Equinix Metal (EM) API. This ensures that the network configuration aligns with the pre-existing infrastructure provided by the user.

### Custom Resource Changes:

PacketCluster and PacketMachineTemplate will be extended to support the new network configurations for Layer2 networking.
PacketCluster will have a new field called Networks, which will be a list of NetworkSpec objects. NetworkSpec objects define different networks that can be attached to the PacketCluster. Each NetworkSpec object will have the following fields:
- Name: Name of the network, e.g., "storage VLAN" (optional)
- Description: Description of the network, e.g., "Storage network for VMs" (optional)
- IPAddresses: IP address range for the cluster network, e.g, Virtual Routing and Forwarding (VRF) . This field will be a list of strings, where each string represents an IP address range.
- Assignment: Component responsible for allocating IP addresses to the machines, either cluster-api or dhcp.
- Gateway: Default gateway for the network (optional) (TODO: do we need this?)

**PacketCluster**
```go
type PacketClusterSpec struct {
  ...
    // Networks is a list of network configurations for the PacketCluster
    Networks []NetworkSpec `json:"networks,omitempty"`
}

// +kubebuilder:validation:Enum=cluster-api,dhcp
type AssignmentType string

const (
    AssignmentClusterAPI AssignmentType = "cluster-api"
    AssignmentDHCP       AssignmentType = "dhcp"
)

// NetworkSpec defines the network configuration for a PacketCluster
type NetworkSpec struct {
    // Name of the network, e.g. "storage VLAN", is optional
    // +optional
    Name        string         `json:"name,omitempty"`
    // Description of the network, e.g. "Storage network", is optional
    // +optional
    Description string         `json:"description,omitempty"`
    // IpAddressRange for the cluster network for eg: VRF IP Ranges
    IPAddresses []string      `json:"ipAddresses,omitempty"`
    // Assignment is component responsible for allocating IP addresses to the machines, either cluster-api or dhcp
    Assignment  AssignmentType `json:"assignment,omitempty"`
    // Default gateway for the network
    // +optional
    Gateway     string        `json:"gateway,omitempty"`
}
```

The following example configures a network named "storage VLAN" with VXLAN ID 1000, IP address range 10.60.10.0/24, and a default gateway of
10.60.10.1. The IP addresses are assigned to the indvidual machines by the cluster-api component.

// TODO: Do we need to add the VLAN and gateway field in the NetworkSpec?

```yaml
kind: PacketCluster
metadata:
  name: example-packet-cluster
spec:
  template:
    spec:
      facility: ny5
      metro: ny
      plan: c3.small.x86
      billingCycle: hourly
      project: your-packet-project-id
      sshKeys:
        - ssh-rsa AAAAB3...your-public-key...
      operatingSystem: ubuntu_20_04
      networks:
        - name: Storage VLAN
          description: Storage network for VMs
          ipAddresses: ["10.60.0.0/16"]
          assignment: cluster-api
```

**PacketMachineTemplate**

To support enhanced layer2 networking capabilities, we propose adding a new Ports and Routes field under the spec of the *PacketMachineTemplate*. These fields will allow users to define various network port configurations and add static routes for an Equinix Metal Machine. Below is an outline of the proposed changes:

```go
// PacketMachineSpec defines the desired state of PacketMachine.
 type PacketMachineSpec struct {
   ..
   // List of Port Configurations on each Packet Machine
   // +optional
   Ports []Port `json:"ports"`
   // List of Routes to be configured on the Packet Machine
    // +optional
   Routes      []RouteSpec     `json:"routes,omitempty"`
}

type Port struct {
     // name of the port e.g bond0,eth0 and eth1 for 2 NIC servers.
     Name string `json:"name"`
     // port bonded or not - by default true
     Bonded bool `json:"bonded,omitempty"`
     // convert port to layer 2. is false by default on new devices. changes result in /ports/id/convert/layer-[2|3] API calls
     Layer2 bool `json:"layer2"`
     // IPAddress configurations associated with this port
     // These are typically IP Reservations carved out of VRF.
     IPAddresses []IPAddress `json:"ip_addresses,omitempty"`
}
// IPAddress represents an IP address configuration 
type IPAddress struct {
    // Addresses to reserve for these ports.
    // for eg: can be carved out of a VRF IP Range.
    Address string `json:"address"`
    // VLANs for EM API to find by vxlan, project, and metro match then attach to device. OS userdata template will also configure this VLAN on the bond device    
    VXLANID int `json:"vxlanId,omitempty"`
    // IP Address of the gateway
    Gateway string `gateway,omitempty`
    // Subnet Size for per machine
    // +optional
    SubnetSize string `json:"subnetSize,omitempty"`
}

// RouteSpec defines the static route configuration for a PacketMachine.
type RouteSpec struct {
    Destination string `json:"destination"`
    Gateway     string `json:"gateway"`
}
```

For example:
In the below example, we have defined two PacketMachineTemplates, each with a different IP address range and VLAN ID. The first template has an IP address range of 10.60.10.0/24 with a VXLAN ID of 1000, while the second template has an IP address range of 10.60.20.0/24 with a VXLAN ID of 1001. Both templates have a static route defined for the destination 10.60.0.0/24 with the gateway set to the respective gateway IP address.

Ref: https://deploy.equinix.com/developers/guides/connecting-vlans-via-vrf/

```yaml
kind: PacketMachineTemplate
metadata:
  name: example-packet-machine-template
spec:
  template:
    spec:
      facility: ny5
      metro: ny
      plan: c3.small.x86
      billingCycle: hourly
      project: your-packet-project-id
      sshKeys:
        - ssh-rsa AAAAB3...your-public-key...
      operatingSystem: ubuntu_20_04
      ports:
        - name: bond0
          layer2: false
          ip_addresses:
            - address: "10.60.10.0/24"
              vxlanId: 1000
              gateway: "10.60.10.1"
              subnetSize: "/32"
      routes:
        - destination: "10.60.0.0/16"
          gateway: "10.60.10.1"

kind: PacketMachineTemplate
metadata:
  name: example-packet-machine-template-1
spec:
  template:
    spec:
      facility: ny5
      metro: ny
      plan: c3.small.x86
      billingCycle: hourly
      project: your-packet-project-id
      sshKeys:
        - ssh-rsa AAAAB3...your-public-key...
      operatingSystem: ubuntu_20_04
      ports:
        - name: bond0
          layer2: false
          ip_addresses:
            - address: "10.60.20.0/24"
              vxlanId: 1001
              gateway: "10.60.20.1"
              subnetSize: "/32"
      routes:
        - destination: "10.60.0.0/16"
          gateway: "10.60.20.1"
```

### APIs:

* * * * *

Following are some of the APIs provided by EM, that would be used:
1. **Convert the port to a layer2 port**:

    a. https://deploy.equinix.com/developers/api/metal/#tag/Ports/operation/convertLayer2
    b. Endpoint: https://api.equinix.com/metal/v1/ports/{id}/convert/layer-2
    c. Requied Params : vnid (VLAN ID) 

2. **Assign a port to a virtual network (VLAN)**:

    a. https://deploy.equinix.com/developers/api/metal/#tag/Ports/operation/assignPort

    b. Endpoint: https://api.equinix.com/metal/v1/ports/{id}/assign
Requied Params : vnid (VLAN ID)
	c. Type: POST
    d. Batch Mode
    ```
    curl -X POST \
    -H "Content-Type: application/json" \
    -H "X-Auth-Token: <API_TOKEN> " \
    "https://api.equinix.com/metal/v1/ports/{id}/vlan-assignments/batches" \
    -d '{
        "vlan_assignments": [
            {
                "vlan": "string",
                "state": "assigned"
            },
            {
                "vlan": "string",
                "state": "assigned"
            },
        ]
    }'
    ```

3. **Device Events API**:
 	a. Endpoint:  `https://api.equinix.com/metal/v1/devices/<id>/events`

4. **Remove port from the bond**
    a. Endpoint: 
    ```
    curl -X POST \
        -H "Content-Type: application/json" \
        -H "X-Auth-Token: <API_TOKEN>" \
        "https://api.equinix.com/metal/v1/ports/{id}/disbond" \
        -d '{
            "bulk_disable": false
        }'
    ```


### User-Data Script for Network Configuration
To configure the operating system (OS), create new sub-interfaces for handling VLAN-tagged traffic, and assign IP addresses to those sub-interfaces, a user-data script is required to run at the time of OS boot.
Below is the user-data script that would be used.

// TODO(rahuls): Add route configuration in the user-data script

```sh
#cloud-config
package_update: true
package_upgrade: true
packages:
  - jq
  - vlan

write_files:
  - path: /var/lib/capi_network_settings/final_configuration.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      set -euo pipefail

      echo "Running final configuration commands"
      apt-get update -qq
      apt-get install -y -qq jq vlan

      modprobe 8021q
      echo "8021q" >> /etc/modules

      # Generate the network configuration and append it to /etc/network/interfaces for each VLAN-tagged sub-interface.
      cat <<EOL >> /etc/network/interfaces
{{ range .VLANs }}
      auto {{ .PortName }}.{{ .ID }}
      iface {{ .PortName }}.{{ .ID }} inet static
        pre-up sleep 5
        address {{ .IPAddress }}
        netmask {{ .Netmask }}
      {{- if .Gateway }}
        gateway {{ .Gateway }}
      {{- end }}
        vlan-raw-device {{ .PortName }}
{{ end }}
      EOL

      echo "VLAN configuration appended to /etc/network/interfaces."

      # Function to send user state events
      url="$(curl -sf https://metadata.platformequinix.com/metadata | jq -r .user_state_url)"
      send_user_state_event() {
        local state="$1"
        local code="$2"
        local message="$3"
        local data

        data=$(jq -n --arg state "$state" --arg code "$code" --arg message "$message" \
               '{state: $state, code: ($code | tonumber), message: $message}')

        curl -s -X POST -d "$data" "$url" || echo "Failed to send user state event"
      }

      send_user_state_event running 1000 "Configuring Network"

      systemctl restart networking
      # Verify network configuration
      verification_failed=false
{{ range .VLANs }}
      if ip addr show {{ .PortName }}.{{ .ID }} | grep -q {{ .IPAddress }}; then
        echo "Configuration for VLAN {{ .ID }} on {{ .PortName }} with IP {{ .IPAddress }} successful"
      else
        echo "Configuration for VLAN {{ .ID }} on {{ .PortName }} with IP {{ .IPAddress }} failed" >&2
        verification_failed=true
      fi
{{ end }}

      if [ "$verification_failed" = true ]; then
        send_user_state_event failed 1002 "Network configuration failed"
        exit 1
      else
        send_user_state_event succeeded 1001 "Network configuration successful"
      fi
    
  - path: /var/lib/capi_network_settings/initial_configuration.sh
    permissions: '0755'
    content: |
      #!/bin/bash
      set -eu
      
      # Fetch metadata from Equinix Metal
      metadata=$(curl https://metadata.platformequinix.com/metadata)
      
      # Extract MAC addresses for eth0 and eth1
      mac_eth0=$(echo "$metadata" | jq -r '.network.interfaces[] | select(.name == "eth0") | .mac')
      mac_eth1=$(echo "$metadata" | jq -r '.network.interfaces[] | select(.name == "eth1") | .mac')
      
      # Check if MAC addresses were successfully extracted
      if [ -z "$mac_eth0" ] || [ -z "$mac_eth1" ]; then
        echo "Error: Failed to extract MAC addresses" >&2
        exit 1
      fi
      
      # Display extracted MAC addresses
      echo "Extracted MAC addresses - eth0: $mac_eth0, eth1: $mac_eth1"
      
      # Function to find network interface by MAC address
      find_interface_by_mac() {
        local mac="$1"
        for iface in $(ls /sys/class/net/); do
          iface_mac=$(ethtool -P "$iface" 2>/dev/null | awk '{print $NF}')
          if [ "$iface_mac" == "$mac" ]; then
            echo "$iface"
            return
          fi
        done
        echo "Interface not found for MAC $mac" >&2
        return 1
      }
      
      # Find interfaces for eth0 and eth1 MAC addresses
      iface_eth0=$(find_interface_by_mac "$mac_eth0")
      iface_eth1=$(find_interface_by_mac "$mac_eth1")
      
      # Check and replace eth0 in /tmp/final_configuration.sh
      if grep -q "eth0" /tmp/final_configuration.sh; then
        sed -i "s/eth0/${iface_eth0}/g" /tmp/final_configuration.sh
        echo "Replaced eth0 with ${iface_eth0} in /tmp/final_configuration.sh"
      else
        echo "No occurrences of eth0 found in /tmp/final_configuration.sh. No changes made."
      fi
      
      # Check and replace eth1 in /tmp/final_configuration.sh
      if grep -q "eth1" /tmp/final_configuration.sh; then
        sed -i "s/eth1/${iface_eth1}/g" /tmp/final_configuration.sh
        echo "Replaced eth1 with ${iface_eth1} in /tmp/final_configuration.sh"
      else
        echo "No occurrences of eth1 found in /tmp/final_configuration.sh. No changes made."
      fi

runcmd:
  - /var/lib/capi_network_settings/initial_configuration.sh
  - /var/lib/capi_network_settings/final_configuration.sh
```

The CAPP will use go-templates to substitute the placeholders with appropriate values given by the user.

### Layer 2 Networking Setup by the CAPP Operator
When provisioning a metal node with Layer 2 networking, the Cluster API Provider (CAPP) Operator will perform the following steps:
1. **Create a ConfigMap for IP Address Management**: The PacketMachine operator will create a new ConfigMap named <cluster_name-ip-allocations> This ConfigMap is critical for tracking and allocating IP addresses as detailed in the *IP Address Management* section.
2. **Select an Available IP Address**: CAPP will select an available IP address from the ConfigMap to be assigned to the machine, node, or server being provisioned.
3. **Generate User-Data Script**: Using Go templates, CAPP will substitute the necessary variables in the user-data script, such as port name, IP address, gateway, and VXLAN. These values are provided by the user through the custom resource definition.
4. **Submit Device Creation Request**: CAPP will then submit a request to create the device, incorporating the generated user-data script for OS and network configuration.
5. **Verify Network Configuration**: After the machine or device is successfully provisioned, CAPP will poll the device events API to check whether the network configuration was successful. If not, it will handle the failure or timeout as needed.
6. **Perform Post-Provisioning Network Operations**: Once the device is provisioned and the network configuration from the user-data script is in place, CAPP will make calls to the /ports API to perform additional operations. These include assigning the VLAN to the port, converting the port to Layer 2 if required, and other necessary configurations.

### Explanation of send_user_state_event Function
The send_user_state_event function in the script is responsible for sending status updates to the user_state_url fetched from Equinix Metadata API. The Metadata API is a service available on every Equinix Metal server instance (while in Layer3 or Hybrid mode) that allows the server to access and share various data about itself. Here’s how the function works:
1. **Retrieve the user_state_url**: The script fetches the user_state_url from the Equinix Metadata API. This URL is used to send custom user state events that report on the progress or status of the server's configuration.
2. **Prepare the Event Data**: The function constructs a JSON payload containing the state, code, and message. The jq tool is used to create this JSON object dynamically, based on the input parameters.
3. **Send the Event**: The constructed JSON data is then sent to the user_state_url via a POST request. This allows the system to log the state of the network configuration process (e.g., "running," "succeeded," or "failed") along with an appropriate status code and message.
This approach enables tracking of the server's state during the boot process, particularly for critical operations like network configuration.

### Note: 
1. If you want to connect the VLANs that are in different metros, you need to enable [Backend Transfer](https://deploy.equinix.com/developers/docs/metal/networking/backend-transfer/)

2. If you want to use your own IP address range, and the VLANs are in the same metro, you can use the VRF Metal Gateway to connect the VLANs and allow CAPP to assign IP addresses from the specified range.

### IP Address Management:

* * * * *

In Phase-1, the Cluster API Provider Packet (CAPP) will manage IP allotment to individual machines using Kubernetes Configmaps. This approach allows for tracking allocations and assigning available IP addresses dynamically.

Example:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: capp-ip-allocations
  namespace: cluster-api-provider-packet-system
Data:
  "da-1000-10.60.10.0/24": |
    {
      "machine-1": "10.60.10.2",
      "machine-2": "10.60.10.3",
      "machine-3": "10.60.10.4"
    }
  "da-1001-10.60.20.0/24": |
    {
      "machine-1": "10.60.20.2",
      "machine-2": "10.60.20.3",
      "machine-3": "10.60.20.4"
    }
```

1. In the example above, capp-ip-allocations ConfigMap in the cluster-api-provider-packet-system namespace tracks IP allocations.
2. When a new machine is provisioned, CAPP will select an available IP address from the ConfigMap based on the specified IP range and assign it to the machine. The ConfigMap is updated to reflect the allocation, ensuring that IP addresses are not reused or double-assigned.
3. The configmap entry is named using the format "metro-vlan-id-ip-range", where vlan-id is the VLAN ID and ip-range is the IP address range. The value is a JSON object with machine names as keys and IP addresses as values.
4. When a machine is deleted or decommissioned, CAPP will remove the corresponding entry from the ConfigMap, making the IP address available for future allocations.
5. The lifecycle of ConfigMap is tied to the PacketCluster, ensuring that IP address allocations are managed consistently across the cluster.
The ConfigMap approach provides a simple and effective way to manage IP address allocations within a Kubernetes cluster, ensuring that IP addresses are tracked and assigned correctly.
6. The lifecycle management of IP addresses is crucial for maintaining network integrity and avoiding conflicts or address exhaustion. By using ConfigMaps to track IP allocations, CAPP can efficiently manage IP addresses across multiple machines and ensure that each machine is assigned a unique and available IP address.

