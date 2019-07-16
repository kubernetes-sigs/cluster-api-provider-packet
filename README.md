# Packet cluster-api Provider

This is the official [cluster-api](https://github.com/kubernetes-sigs/cluster-api) provider for [Packet](https://packet.com).

## Using

### Requirements

To use the cluster-api to deploy a Kubernetes cluster to Packet, you need the following:

* A Packet API key
* A Packet project ID
* The `clusterctl` binary from this repository.
* A Kubernetes cluster - the "bootstrap cluster" - that will deploy and manage the cluster on Packet. 
* `kubectl` - not absolutely required, but hard to interact with a cluster without it

For the bootstrap cluster, any cluster is just fine for this, including [k3s](https://k3s.io), [k3d](https://github.com/rancher/k3d) and [kind](https://github.com/kubernetes-sigs/kind).

You have two choices for the bootstrap cluster:

* An existing cluster, which can be on your desktop, another Packet instance, or anywhere else that Kubernetes runs, as long as it has access to the Packet API.
* Rely on `clusterctl` to set a temporary one up for you using [kind](https://github.com/kubernetes-sigs/kind) on your local docker.

### Steps

To deploy a cluster:

1. Create a project in Packet, using one of: the [API](https://www.packet.com/developers/api/), one of the many [SDKs](https://www.packet.com/developers/libraries/), the [CLI](https://github.com/packethost/packet-cli) or the [Web UI](https://app.packet.net).
1. Create an API key for the project.
1. Set the required environment variables:
   * `PACKET_PROJECT_ID` - Packet project ID
   * `PACKET_API_KEY` - Packet API key
1. (Optional) Set the optional environment variables:
   *  `CLUSTER_NAME` - The created cluster will have this name. If not set, it will generate one for you.
   *  `FACILITY` - The Packet facility where you wantto deploy the cluster. If not set, it will default to `ewr1`.
   *  `SSH_KEY` - The path to an ssh public key to place on all of the machines. If not set, it will use whichever ssh keys are defined for your project.
1. Create the config files you need via `./generate-yaml.sh`. This will generate the following files in [out/packet](./out/packet):
   * `cluster.yaml`
   * `machines.yaml`
   * `provider-components.yaml`
   * `addons.yaml` 
1. If desired, edit the following files:
   * `cluster.yaml` - to change parameters or settings, including network CIDRs
   * `machines.yaml` - to change parameters or settings, including machine types and quantity
1. Run `clusterctl` with the appropriate command.

```sh
./bin/clusterctl create cluster \
    --provider packet \
    --bootstrap-type kind \
    -c ./out/packet/cluster.yaml \
    -m ./out/packet/machines.yaml \
    -p ./out/packet/provider-components.yaml \
    -a ./out/packet/addons.yaml
```

Run `clusterctl create cluster --help` for more options, for example to use an existing bootstrap cluster rather than creating a temporary one with [kind](https://github.com/kubernetes-sigs/kind).

`clusterctl` will do the folloiwng:

1. Connect to your bootstrap cluster either via:
   * creating a new one using [kind](https://github.com/kubernetes-sigs/kind)
   * connecting using the provided kubeconfig
1. Deploy the provider components in `provider-components.yaml`
1. Update the secret in the cluster with your credentials based on the environment variables set earlier
1. Create a master node on Packet, download the `kubeconfig` file
1. Connect to the master and deploy the controllers
1. Create worker nodes
1. Deploy add-on components, e.g. the [packet cloud-controller-manager](https://github.com/packethost/packet-ccm) and the [packet cloud storage interface provider](https://github.com/packethost/csi-packet)
1. If a new bootstrap cluster was created, terminate it


### Deploying Manually

If you _really_ want to deploy manually, rather than using `clusterctl`, do the following. This assumes that you have generated the yaml files as required.

1. Ensure you have a cluster running
1. Edit the `provider-components.yaml` to update the secret at the very end with the real values for the project ID and API key
1. Deploy the manager controller: `kubectl apply -f provider-components.yaml`
1. Deploy the cluster: `kubectl apply -f cluster.yaml`
1. Deploy the machines: `kubectl apply -f machines.yaml`
1. Deploy the addons: `kubectl apply -f addons.yaml`

Note that, unlike `clusterctl`, this method will not take care of the following:

* create a bootstrap cluster
* inject the actual secret values
* pivot the control from the bootstrap cluster to the newly started cluster
* remove the bootstrap cluster

## Components

The components deployed via the `yaml` files are the following:

* `cluster.yaml` - contains 
  * a single `Cluster` CRD which defines the new cluster to be deployed. Includes cluster-wide definitions, including cidr definitions for services and pods.
* `machines.yaml` - contains
  * one or more `Machine` CRDs, which cause the deployment of individual server instance to serve as Kubernetes master or worker nodes.
  * one or more `MachineDeployment` CRDs, which causes the deployment of a managed group of server instances.
* `addons.yaml` - contains
  * necessary `ServiceAccount`, `ClusterRole` and `ClusterRoleBinding` declarations
  * packet-ccm `Deployment`
  * csi-packet `Deployment` and 'DaemonSet`
* `provider-components.yaml` - contains
  * [Custom Resource Definitions (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for the cluster API resources
  * all of the necessary `ClusterRole`, `ClusterRoleBinding`, `ServiceAccount` to run the controllers
  * Packet-specific `manager` binary, in a `StatefulSet`, whose control loop manages the `Cluster` and `MachineDeployment` resources, and creates, updates or removes `Machine` resources
  * Cluster-API-generic `controller` binary, in a `StatefulSet`, whose control loop manages the `Machine` resources

As of this writing, the Packet cluster-api provider control plane supports only one master node. Thus, you should deploy a single control plane node as a `Machine`, and the worker nodes as a `MachineDeployment`. This is the default provided by `generate-yaml.sh`. Because the worker nodes are a `MachineDeployment`, the cluster-api manager keeps track of the count. If one disappears, it ensures that a new one is deployed to take its place.

In the future, we will add high-availability, enabling multiple masters and worker nodes.

## How It Works

The Packet cluster-api provider follows the standard design for cluster-api. It consists of two components:

* `manager` - the controller that runs in your bootstrap cluster, and watches for creation, update or deletion of the standard resources: `Cluster`, `Machine`, `MachineDeployment`. It then updates the actual resources in Packet.
* `clusterctl` - a convenient utility run from your local desktop or server and controls the new deployed cluster via the bootstrap cluster.

The actual machines are deployed using `kubeadm`. The deployment process uses the following process.

1. When a new `Cluster` is created:
   * create a new CA certificate and key, encrypt and save them in a bootstrap controller Kubernetes secret
   * create a new kubeadm token, encrypt and save in a bootstrap controller Kubernetes secret. This 
   * create an admin user certificate using the CA certificate and key, encrypt and save them in a bootstrap controller Kubernetes secret
2. When a new master `Machine` is created:
   * retrieve the CA certificate and key from the bootstrap controller Kubernetes secret
   * retrieve the kubeadm token from the bootstrap controller Kubernetes secret
   * launch a new server instance on Packet
   * set the `cloud-init` on the instance to run `kubeadm init`, passing it the CA certificate and key and kubeadm token
3. When a new worker `Machine` is created:
   * retrieve the kubeadm token from the Kubernetes secret
   * launch a new server instance on Packet
   * set the `cloud-init` on the instance to run `kubeadm join`, passing it the kubeadm token
4. When a user requests the kubeconfig via `clusterctl`, it retrieves it from the Kubernetes secret and passes it to the user


## Building

There are multiple components that can be built. For normal operation, you just need to download the `clusterctl` binary and yaml files and run them. This section describes how to build components.

The following are the requirements for building:

* [go](https://golang.org), v1.11 or higher
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/), v1.14 or higher (we are working on removing this requirement)
* Make

To build all of the components:

```
make
```

This will leave you with:

* the controller binary as `bin/manager` for the OS and architecture on which you are running
* the cluster control CLI binary as `bin/clusterctl` for the OS and architecture on which you are running
* the config file to deploy to your bootstrap cluster as `provider-components.yaml`

You can build for a different OS or architecture by setting `OS` and `ARCH`. For example:

```
make OS=windows
make OS=linux ARCH=arm64
make ARCH=s390x
```

To build the OCI image for the controller:

```
make docker-build
```

This will leave you with:

* a docker image whose name matches the one set as the default in the `Makefile`
* the config file to deploy to your bootstrap cluster as `provider-components.yaml`

You can change the name of the image to be built with `IMG=`, for example:

```
make docker-build IMG=myname/img-provider
```

To see the name of the docker image that would be built, run:

```
make image-name
```

To build individual components, call its target:

```
make manager
make clusterctl
make manifests
```

As always with `make`, you can force the rebuilding of a component with `make -B <target>`.

## Running locally

You can run the `manager` locally on your own laptop in order to ease and speed development, or even run it through a debugger. The steps are:

1. Create a kubernetes bootstrap cluster, e.g. kind
1. Set your `KUBECONFIG` to point to that cluster, e.g. `export KUBECONFIG=...`
1. Create a local OS/arch `manager` binary, essentially `make manager`. This will save it as `bin/manager-<os>-<arch>`, e.g. `bin/manager-linux-arm64` or `bin/manager-darwin-amd64`
1. Generate your yaml `./generate-yaml.sh`
1. Run the manager against the cluster with the local configs, `bin/manager-darwin-amd64 -config ./config/default/machine_configs.yaml -ca-cache ./out/cache.json`

In the above example:

* We are running on macOS (`darwin`) and an Intel x86_64 (`amd64`)
* We are using the default config file `./config/default/machine_configs.yaml`
* We are caching the CA keys and certs in `out/cache.json`

Caching the CA caches the certs **and the keys**. Only do this in test mode, or if you really are sure what you are doing. The purpose, in this case, is to allow you to stop and start the process, and pick up existing certs.

## References

* [kubeadm yaml api](https://godoc.org/k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta2)

