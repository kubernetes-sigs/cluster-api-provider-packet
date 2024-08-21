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

As a  cluster administrator

- I want  to configure L2 interfaces and define my own IP Address range so that  machines are able to communicate over layer2 VLAN.

* * * * *

Goals (phase-1)
===============

-   For the phase-1 of supporting Layer2 functionalities in CAPP, it will add support for [Hybrid Bonded](https://deploy.equinix.com/developers/docs/metal/layer2-networking/hybrid-bonded-mode/) and [Hybrid Unbonded](https://deploy.equinix.com/developers/docs/metal/layer2-networking/hybrid-unbonded-mode/) mode.

-   CAPP will be able to attach ports to specified VLAN.

-   CAPP will be able to create a new VLAN if required in the specified project and metro.

-   CAPP will be able to create VRF (Virtual Routing & Forwarding) and IP Reservations as specified in the spec.

-   CAPP will be able to configure networking at the OS level of a metal node.

Non-Goals
===============

-   Complete layer2 will not be supported in the initial phases.

-   IPAM Provider will be supported in phase-2

Proposal Design/Approach
========================

* * * * *

### PacketCluster

The idea is to introduce options to specify VLANs and VRFs configurations as a part of ClusterSpec.

```go
type  PacketClusterSpec struct {
  ...

// VLANConfig represents the configuration for layer2 VLAN.
 VLANConfig *VLANConfig            `json:"vlanConfig,omitempty"`

// VRFConfig represents the configuration for Virtual  Router and Forwarding (VRF)
 VRFConfig *VRFConfig              `json:"vrfConfig,omitempty"`
}
```

// VLANConfig represents the configuration for an existing or new VLAN
```go
type VLANConfig struct {
 // VLANID is the ID of the VLAN. It can be an existing VLAN or a new one to be created.
  VLANID string `json:"vlanID"`

 // Description of the VLAN
 // +optional
  Description string `json:"description,omitempty"`

 // CreateNew indicates whether a new VLAN needs to be created
 // +optional
  CreateNew bool `json:"createNew,omitempty"`
}
```

-   For an existing VLAN
```json
{
"vlanID": "existing-vlan-id",
"description": "Existing VLAN",
"createNew": false
}
```

-   For a new VLAN:

```json
{
"vlanID": "new-vlan-id",
"description": "New VLAN to be created",
"createNew": true
}
```

* * * * *


// VRFConfig represents the configuration for an existing or new VRF

```go
type VRFConfig struct {
 // Name of the VRF. It can be an existing VRF or a new one to be created.
  Name string `json:"name,omitempty"`

 // Description of the VRF
 // +optional
  Description string `json:"description,omitempty"`

 // IP ranges that are allowed to access and communicate with this virtual router.
  AllowedIPRanges []string `json:"allowedIPRanges,omitempty"`

 // CreateNew indicates whether a new VRF needs to be created\
 // +optional
  CreateNew bool `json:"createNew,omitempty"`
}
```

- For an existing VRF:

```json
{
"name": "Existing VRF",
"description": "Existing VRF description",
"allowedIPRanges": ["192.168.1.0/21"],
"createNew": false
}
```

- For a new VRF:
```json
{
"name": "New VRF",
"description": "New VRF description",
  "allowedIPRanges": ["10.0.0.0/24"],
"createNew": true
}
```

Users can specify different options for CAPP Controllers to create a new VLAN/VRF or to provide existing configuration details. Below is an example YAML configuration illustrating this:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1\
kind: PacketCluster
metadata:
  name: capi-quickstart
  namespace: default
spec:
  metro: da
  projectID:  7f713f3e-fe14-4386-a2fb-f22c29ba685e\
  vipManager: CPEM
  vlanConfig:
 "vlanID": "existing-vlan-id",
"description": "Existing VLAN",
"createNew": false
  vrfConfig: 
    "name": "New VRF",
    "description": "New VRF description",
    "allowedIPRanges": ["10.0.0.0/24"],
    "createNew": true
```

In the above example, vlanConfig and vrfConfig are used to create a new VLAN and a new VRF with specific configurations, respectively.

### Infrastructure Setup and Readiness

* * * * *

The PacketCluster Controller is responsible for setting up infrastructure. Once the setup is successfully completed, it marks itself as "Ready" and sets the condition NetworkInfrastructureReadyCondition to true. The controller will continue to follow this setup process and call Equinix Metal APIs to create or get VLANs and VRFs as needed before marking the infrastructure as Ready.

This approach also helps avoid potential race conditions between the execution of user-data scripts and the application of port configurations at the API level. By ensuring the network configurations are established through the controller before any user-data scripts run, we can maintain a consistent and reliable infrastructure setup process.

### APIs

* * * * *

1.  Create a new VLAN

```
curl -X POST
-H "Content-Type: application/json"
-H "X-Auth-Token: <API_TOKEN>"
"https://api.equinix.com/metal/v1/projects/{id}/virtual-networks"
-d '{
 "vxlan": <integer>,
 "description": "<string>",
 "metro": "<string>"
 }'
```

2.  Create a new VRF
```
curl -X POST
-H "Content-Type: application/json"
-H "X-Auth-Token: <API_TOKEN>"
"https://api.equinix.com/metal/v1/projects/{id}/vrfs"
-d '{
 "name": "<string>",
 "description": "<string>",
 "metro": "<metro_slug>",
 "ip_ranges": [
 "<cidr_address>"
    ],
 "local_asn": <integer>
}'
```

### PacketMachineTemplate


* * * * *

The idea is to introduce a new field Ports under spec which will contain different networking specifications of networking ports for a Equinix Metal Machine. This could look like the following:

```go
// PacketMachineSpec defines the desired state of  PacketMachine.
type  PacketMachineSpec struct {
  ..
// List of Port  Configurations on each Packet  Machine\
  // +optional\
 Ports []Port `json:"ports"`
}
```

```go
type Port struct {
 // name of the port e.g bond0
    Name string `json:"name"`
 // port bonded or not - by default true
    Bonded bool `json:"bonded,omitempty"`
 // convert port to layer 2. is false by default on new devices. changes result in /ports/id/convert/layer-[2|3] API calls\
    Layer2 bool `json:"layer2"`

    IPAddresses []IPAddress `json:"ip_addresses,omitempty"`
}
// IPAddress represents an IP address configuration\
type IPAddress struct {
    Type string `json:"type"`

    // VRF used to create the IP Address Reservation for this cluster\
    VRFName string `json:"vrfName,omitempty"`

    // IPAddressReservation to reserve for these cluster nodes.
    IPAddressReservation string `json:"ipAddressReservation"`

    // VLAN Id to join
    VLANId string `json:"vlanId,omitempty"`
}
```

Example:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PacketMachineTemplate\
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
        -  ssh-rsa  AAAAB3...your-public-key...
        operatingSystem: ubuntu_20_04
        ports: # network_ports in the API naming\
        - bond0:
            bonded: true  # default\
            layer2: false
            ip_addresses:
            - type: vrf
              vrfName: "vrf-01"
              ipAddressReservation: "192.168.2.0/24"
              vlanId: 1000
        - eth2: # unbonded eth ports can use most of the same attributes bond ports can use\
            layer2: true
```

The CAPP Operators while provisioning the metal node would:

1.  Call the Equinix Metal API to attach Ports to the VLAN 

```
curl -X POST
-H "Content-Type: application/json"
-H "X-Auth-Token: <API_TOKEN> "
"https://api.equinix.com/metal/v1/ports/{id}/assign"
-d '{
    "vnid": "<vlan_ID>"
    }'
```

1.  Once all the ports are attached to the respective VLANs, CAPP needs to configure the networking on the server's operating system.

2.  It will then GET the VRF and update it to reserve the IPAddressReservation.

```
curl -X POST
-H 'Content-Type: application/json'
-H "X-Auth-Token: <API_TOKEN>"
"https://api.equinix.com/metal/v1/projects/{id}/ips"
-d '{
    "cidr": <integer>,
    "network": "<ip_address>",
    "type": "vrf",
    "vrf_id": "<UUID>"
}'
```

1.  For each Machine, CAPP needs to pick an IP Address from the above range and assign it at the OS Level. In Phase 1, it will be done by CAPP itself (explained below) and later in phase-2, an IPAM Provider will be implemented as per CAPI standards.

2.  Once it receives an IP Address that is non-conflicting, It will create subinterfaces to handle the tagged traffic over the VLAN and assign the IP Address on that sub-interface. Hybrid Bonded mode does not support untagged VLAN traffic or setting a Native VLAN.

3.  In order to configure networking on the OS, user-data and cloud-config format could be utilized as shown below in the example.

Example:

```sh
#cloud-config\
write_files:\
  - path: /etc/network/interfaces

    append: true\
    content: |

      auto bond0.${VLAN_ID_0}\
        iface bond0.${VLAN_ID_0} inet static

        pre-up sleep 5\
address 192.168.2.2\
netmask 255.255.255.0\
gateway 192.168.2.1\
        vlan-raw-device bond0\
runcmd:\
  - systemctl restart networking
```



### IP Address Management:

* * * * *

In Phase-1, the Cluster API Provider Packet (CAPP) will manage IP allotment to individual machines using Kubernetes ConfigMaps. This approach allows for tracking allocations and assigning available IP addresses dynamically.

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

