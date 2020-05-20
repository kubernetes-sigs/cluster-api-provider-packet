# Getting Started (with repo)

At the moment the best way to deploy a cluster requires to clone this repository
locally, check the [requirements)(../requirements.md) and follow the steps
listed here.

### Steps

To deploy a cluster via the cluster-api:

1. Create a project in Packet, using one of: the [API](https://www.packet.com/developers/api/), one of the many [SDKs](https://www.packet.com/developers/libraries/), the [CLI](https://github.com/packethost/packet-cli) or the [Web UI](https://app.packet.net).
1. Create an API key for the project save it
1. Deploy your bootstrap cluster and set the environment variable `KUBECONFIG` (optional)
1. Initialize the cluster (see below)
1. Generate your cluster yaml (see below)
1. Apply your cluster yaml (see below)

#### Initialize the Cluster

To initialize the cluster:

```
clusterctl --config=https://github.com/packethost/cluster-api-provider-packet/releases/latest/clusterctl.yaml init --infrastructure=packet
```

We are in the process of working with the core cluster-api team, so that you will not need the
`--config=` option, hopefully soon.

#### Generate Cluster yaml

To generate your cluster yaml:

1. Set the required environment variables:
   * `PACKET_PROJECT_ID` - Packet project ID
   * `PACKET_FACILITY` - The Packet facility where you wantto deploy the cluster. If not set, it will default to `ewr1`.
1. (Optional) Set the optional environment variables:
   * `CLUSTER_NAME` - The created cluster will have this name. If not set, it will generate one for you, see defaults below.
   * `NODE_OS` - The operating system to use for the node. If not set, see defaults below.
   * `SSH_KEY` - The path to an ssh public key to place on all of the machines. If not set, it will use whichever ssh keys are defined for your project.
   * `POD_CIDR` - The CIDR to use for your pods; if not set, see defaults below
   * `SERVICE_CIDR` - The CIDR to use for your services; if not set, see defaults below
   * `MASTER_NODE_TYPE` - The Packet node type to use for control plane nodes; if not set, see defaults below
   * `WORKER_NODE_TYPE` - The Packet node type to use for worker nodes; if not set, see defaults below
1. Run the cluster generation command:

```
clusterctl --config=https://github.com/packethost/cluster-api-provider-packet/releases/latest/clusterctl.yaml config cluster <cluster-name> > out/cluster.yaml
```

Note that the above command will make _all_ of the environment variables required. This is a limitation of
`clusterctl` that is in the process of being fixed. If you want to use the defaults, instead of running
the above `clusterctl` command, run:

```
make cluster
```

This will:

1. Generate a random cluster name
1. Set the defaults
1. Accept any of your overrides for those defaults
1. Generate the output
1. Tell you where it is an the `kubectl apply` command to run

##### Defaults

If you do not change the generated `yaml` files, it will use defaults. You can look in the [templates/cluster-template.yaml](./templates/cluster.yaml) file for details.

* service CIDR: `172.25.0.0/16`
* pod CIDR: `172.26.0.0/16`
* service domain: `cluster.local`
* cluster name: `test1-<random>`, where random is a random 5-character string containing the characters `a-z0-9`
* master node type: `t1.small`
* worker node type: `t1.small`
* worker and master OS type: `ubuntu_18_04`

#### Apply Your Cluster

```
kubectl apply -f cluster.yaml
```

Now wait for it to come up. You can check the status with any of the following commands:

```
kubectl get cluster
kubectl get machine
```

