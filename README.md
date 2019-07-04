# Packet cluster-api Provider

This is the official [cluster-api](https://github.com/kubernetes-sigs/cluster-api) provider for [Packet](https://packet.com).

## Using

### Requirements

To use the cluster-api to deploy a Kubernetes cluster to Packet, you need the following:

* A Packet API key
* A Packet project ID
* The `clusterctl` binary from this repository.
* A Kubernetes cluster - the "bootstrap cluster" - that will deploy and manage the cluster on Packet. 

For the bootstrap cluster, any cluster is just fine for this, including [k3s](https://k3s.io), [k3d](https://github.com/rancher/k3d) and [kind](https://github.com/kubernetes-sigs/kind).

You have two choices for the bootstrap cluster:

* An existing cluster, which can be on your desktop, another Packet instance, or anywhere else that Kubernetes runs, as long as it has access to the Packet API.
* Rely on `clusterctl` to set a temporary one up for you using [kind](https://github.com/kubernetes-sigs/kind) on your local docker.

### Steps

To deploy a cluster:

1. Create the config files you need, specifically `cluster.yaml`, `machines.yaml`, `provider-components.yaml`, `addons.yaml`. Samples of these are provided by default in [examples/packet/](./examples/packet/). The simplest way to get started is to run `./generate-yaml.sh`, which will create those and place them in [out/packet/](./out/packet).
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
   * create a new one using [kind](https://github.com/kubernetes-sigs/kind)
   * connect using the provided kubeconfig
1. Deploy the `cluster-api-controller` and `packet-manager`
1. Create a master node on Packet, download the `kubeconfig` file
1. Connect to the master and deploy the controllers
1. Create worker nodes
1. Deploy add-on components, e.g. the [packet cloud-controller-manager](https://github.com/packethost/packet-ccm) and the [packet cloud storage interface provider](https://github.com/packethost/csi-packet)
1. If a new `kind` cluster was created, terminate the `kind` cluster


### Deploying Manually

If you want to deploy manually, rather than using `clusterctl`, do the following. This assumes that you have generated the yaml files as required.

1. Ensure you have a cluster running
1. Deploy the manager controller: `kubectl apply -f provider-components.yaml`
1. Deploy the cluster: `kubectl apply -f cluster.yaml`
1. Deploy the machines: `kubectl apply -f machines.yaml`
1. Deploy the addons: `kubectl apply -f addons.yaml`

Note that this method will _not_ pivot the control from the bootstrap cluster to the newly started cluster, unlike using `clustertctl`.

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
  * `manager` binary, in a `Deployment`, whose control loop ensures the necessary cluster and machines are deployed and running

As a general rule, you should deploy a single control plane node as a `Machine`, and the worker nodes as a `MachineDeployment`. Because the worker nodes are a `MachineDeployment`, the cluster-api manager keeps track of the cound. If one disappears, it ensures that a new one is deployed to take its place.



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

To build the provider binary, simply do:

```
make
```

This will leave you with:

* the controller binary as `bin/manager` for the OS and architecture on which you are running
* the cluster control CLI binary as `bin/clusterctl` for the OS and architecture on which you are running
* the config file to deploy to your bootstrap cluster as `provider-components.yaml`

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

