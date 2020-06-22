# Packet cluster-api Provider

This is the official [cluster-api](https://github.com/kubernetes-sigs/cluster-api) provider for [Packet](https://packet.com). It implements cluster-api provider version v1alpha3.

## Using

The following section describes how to use the cluster-api provider for packet (CAPP) as a regular user.
You do _not_ need to clone this repository, or install any special tools, other than the standard
`kubectl` and `clusterctl`; see below.

* To build CAPP and to deploy individual components, see [docs/BUILD.md](./docs/BUILD.md).
* To build CAPP and to cut a proper release, see [docs/RELEASE.md](./docs/RELEASE.md).

### Requirements

To use the cluster-api to deploy a Kubernetes cluster to Packet, you need the following:

* A Packet API key
* A Packet project ID
* The `clusterctl` binary from the [official cluster-api provider releases page](https://github.com/kubernetes-sigs/cluster-api/releases)
* A Kubernetes cluster - the "bootstrap cluster" - that will deploy and manage the cluster on Packet.
* `kubectl` - not absolutely required, but it is hard to interact with a cluster without it!

For the bootstrap cluster, any compliant cluster will work, including
[official kubernetes](https://kubernetes.io), [k3s](https://k3s.io), [kind](https://github.com/kubernetes-sigs/kind)
and [k3d](https://github.com/rancher/k3d).

Once you have your cluster, ensure your `KUBECONFIG` environment variable is set correctly.

You have two choices for the bootstrap cluster:

* An existing cluster, which can be on your desktop, another Packet instance, or anywhere else that Kubernetes runs, as long as it has access to the Packet API.
* Rely on `clusterctl` to create a temporary one up for you using [kind](https://github.com/kubernetes-sigs/kind) on your local docker.

### Steps

To deploy a cluster via the cluster-api:

1. Create a project in Packet, using one of: the [API](https://www.packet.com/developers/api/), one of the many [SDKs](https://www.packet.com/developers/libraries/), the [CLI](https://github.com/packethost/packet-cli) or the [Web UI](https://app.packet.net).
1. Create an API key for the project save it
1. Deploy your bootstrap cluster and set the environment variable `KUBECONFIG` (optional)
1. Initialize the cluster (see below)
1. Generate your cluster yaml (see below)
1. Apply your cluster yaml (see below)

#### Initialize the Cluster

To initialize the cluster, you need to provider it with the path to the config file.

1. Download the clusterctl config file for your release.
1. Use the config file.

```
clusterctl init --infrastructure=packet
```

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
   * `WORKER_MACHINE_COUNT` - The number of worker machines to deploy; if not set, cluster-api itself (not the Packet implementation) defaults to 0 workers.
1. Run the cluster generation command:

```
clusterctl config cluster <cluster-name> > out/cluster.yaml
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

## How It Works

The Packet cluster-api provider follows the standard design for cluster-api. It consists of two components:

* `manager` - the controller that runs in your bootstrap cluster, and watches for creation, update or deletion of the standard resources: `Cluster`, `Machine`, `MachineDeployment`. It then updates the actual resources in Packet.
* `clusterctl` - a convenient utility run from your local desktop or server and controls the new deployed cluster via the bootstrap cluster.

The actual machines are deployed using `kubeadm`. The deployment process uses the following process.

1. When a new `Cluster` is created:
   * if the appropriate `Secret` does not include a CA key/certificate pair, create one and save it in that `Secret`
2. When a new master `Machine` is created:
   * retrieve the CA certificate and key from the appropriate Kubernetes `Secret`
   * launch a new server instance on Packet
   * set the `cloud-init` on the instance to run `kubeadm init`, passing it the CA certificate and key
3. When a new worker `Machine` is created:
   * check if the cluster is ready, i.e. has a valid endpoint; if not, retry every 15 seconds
   * generate a new kubeadm bootstrap token and save it to the workload cluster
   * launch a new server instance on Packet
   * set the `cloud-init` on the instance to run `kubeadm join`, passing it the newly generated kubeadm token
4. When a user requests the kubeconfig via `clusterctl`, generate a new one using the CA key/certificate pair

## Supported node OS and Versions

CAPP (Cluster API Provider for Packet) supports Ubuntu 18.04 and Kubernetes 1.14.3. To extend it to work with different combinations, you only need to edit the file [config/default/machine_configs.yaml](./config/default/machine_configs.yaml).

In this file, each list entry represents a combination of OS and Kubernetes version supported. Each entry is composed of the following parts:

* `machineParams`: list of the combination of OS image, e.g. `ubuntu_18_04`, and Kubernetes versions, both control plane and kubelet, to install. Also includes the container runtime to install.
* `userdata`: the actual userdata that will be run on server instance startup.

When trying to install a new machine, the logic is as follows:

1. Take the requested image and kubernetes versions.
1. Match those to an entry in `machineParams`. If it matches, use this `userdata`.

Important notes:

* There can be multiple `machineParams` entries for each `userdata`, enabling one userdata script to be used for more than one combination of OS and Kubernetes versions.
* There are versions both for `controlPlane` and `kubelet`. `master` servers will match both `controlPlane` and `kubelet`; worker nodes will have no `controlPlane` entry.
* The `containerRuntime` is installed as is. The value of `containerRuntime` will be passed to the userdata script as `${CR_PACKAGE}`, to be installed as desired.

## References

* [kubeadm yaml api](https://godoc.org/k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta2)

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- Chat with us on [Slack](http://slack.k8s.io/) in the [#cluster-api][#cluster-api slack] channel
- Subscribe to the [SIG Cluster Lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) Google Group for access to documents and calendars

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
[#cluster-api slack]: https://kubernetes.slack.com/archives/C8TSNPY4T
