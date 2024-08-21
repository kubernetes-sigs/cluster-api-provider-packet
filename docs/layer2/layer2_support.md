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
- Allowing CAPP to configure Layer2 networking at the OS level on a metal node, including creating sub-interfaces and assigning IP addresses.
- Ensuring CAPP can track the lifecycle of available IP addresses from VRF Range.


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
**PacketMachineTemplate**

To support enhanced layer2 networking capabilities, we propose adding a new Ports field under the spec of the *PacketMachineTemplate*. This field will allow users to define various network port configurations for an Equinix Metal Machine. Below is an outline of the proposed changes:

```go
// PacketMachineSpec defines the desired state of PacketMachine.
 type PacketMachineSpec struct {
   ..
   // List of Port Configurations on each Packet Machine
   // +optional
   Ports []Port `json:"ports"`
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
    // IPAddressReservation to reserve for these cluster nodes.
    // for eg: can be carved out of a VRF IP Range.
    IPAddressReservation string `json:"ipAddressReservation"`	
    // VLANs for EM API to find by vxlan, project, and metro match then attach to device. OS userdata template will also configure this VLAN on the bond device    
    VXLANIDs []string `json:"vxlan_ids,omitempty"`
    // UUID of VLANs to which this port should be assigned.
    // Either VXLANID or VLANID should be provided.
    VLANIDs []string  `vlan_ids,omitempty`
    // IP Address of the gateway
    Gateway string `gateway,omitempty`
}
```

For example:
The following example configures the bond0 port of each node in a cluster to a hybrid bonded mode, attaches vxlan_id with ID 1000 and assigns each node an IP address from range "192.168.2.0/24" with gateway 192.168.2.1

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
       -  name: bond0
          layer2: false
          ip_addresses:
            - ipAddressReservation: "192.168.2.0/24"
              vxlan_ids: [1000]
              gateway: "192.168.2.1"
```

The following example configures the eth1 port of each node in a cluster to a hybrid unbonded mode, removed the port from the bond, converts the port into a layer mode i.e attaches vxlan_id with ID 1001 and assigns each node an IP address from range "10.50.10.0/24" with gateway 10.50.10.1

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
       - eth1:
          bonded: false
          layer2: true
          ip_addresses:
            - ipAddressReservation: "10.50.10.0/24"
              vxlan_ids: [1001]
              gateway: "10.50.10.1"

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
Below is the user-data script that would be used (WIP)

```sh

```
### Layer 2 Networking Setup by the CAPP Operator
When provisioning a metal node with Layer 2 networking, the Cluster API Provider (CAPP) Operator will perform the following steps:
1. **Create a ConfigMap for IP Address Management**: The operator will create a new ConfigMap named <cluster_name-port_name> for each port to manage IP addresses. This ConfigMap is critical for tracking and allocating IP addresses as detailed in the *IP Address Management* section.
2. **Select an Available IP Address**: CAPP will select an available IP address from the ConfigMap to be assigned to the machine, node, or server being provisioned.
3. **Generate User-Data Script**: Using Go templates, CAPP will substitute the necessary variables in the user-data script, such as port name, IP address, gateway, and VXLAN. These values are provided by the user through the custom resource definition.
4. **Submit Device Creation Request**: CAPP will then submit a request to create the device, incorporating the generated user-data script for OS and network configuration.
5. **Verify Network Configuration**: After the machine or device is successfully provisioned, CAPP will poll the device events API to check whether the network configuration was successful. If not, it will handle the failure or timeout as needed.
6. **Perform Post-Provisioning Network Operations**: Once the device is provisioned and the network configuration from the user-data script is in place, CAPP will make calls to the /ports API to perform additional operations. These include assigning the VLAN to the port, converting the port to Layer 2 if required, and other necessary configurations.

### Explanation of send_user_state_event Function
The send_user_state_event function in the script is responsible for sending status updates to the user_state_url fetched from Equinix Metadata API. The Metadata API is a service available on every Equinix Metal server instance that allows the server to access and share various data about itself. Here’s how the function works:
1. **Retrieve the user_state_url**: The script fetches the user_state_url from the Equinix Metadata API. This URL is used to send custom user state events that report on the progress or status of the server's configuration.
2. **Prepare the Event Data**: The function constructs a JSON payload containing the state, code, and message. The jq tool is used to create this JSON object dynamically, based on the input parameters.
3. **Send the Event**: The constructed JSON data is then sent to the user_state_url via a POST request. This allows the system to log the state of the network configuration process (e.g., "running," "succeeded," or "failed") along with an appropriate status code and message.
This approach enables tracking of the server's state during the boot process, particularly for critical operations like network configuration.


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
  cidr: 192.168.2.0/24
  allocations: |
    {
     "machine1": "192.168.2.2", 
     "machine2": "192.168.2.3" 
    }
```

In the example above, capp-ip-allocations ConfigMap in the cluster-api-provider-packet-system namespace tracks IP allocations. The cidr field specifies the IP range, while the allocations field is a JSON object mapping machine names to their allocated IP addresses.

