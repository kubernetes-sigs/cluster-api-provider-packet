# Packet cluster-api Provider

This is the official [cluster-api](https://github.com/kubernetes-sigs/cluster-api) provider for [Packet](https://packet.com).

## Using

To use the cluster-api, you need the following:

* An existing Kubernetes cluster - the "bootstrap cluster" - that will deploy and manage the cluster on Packet. This can be on your desktop, another Packet instance, or anywhere else that Kubernetes runs, as long as it has access to the Packet API. [k3s](https://k3s.io), [k3d](https://github.com/rancher/k3d) and [kind](https://github.com/kubernetes-sigs/kind) all are just fine for this.
* A Packet API key
* A Packet project ID

To deploy a cluster, you need to take a few steps:

1. Deploy the Packet cluster-api provider to your bootstrap cluster:

```
kubectl apply -f ./provider-components.yaml
```

1. Create the config files for your cluster: `Cluster`, one or more `Machine` to serve as cluster masters, and a `MachineDeployment` to serve as cluster workers
1. Deploy your cluster, either by applying the config files or using `clusterctl`

## How It Works

The Packet cluster-api provider follows the standard design for cluster-api. It consists of two components:

* `manager` - the controller that runs in your bootstrap cluster, and watches for creation, update or deletion of the standard resources: `Cluster`, `Machine`, `MachineDeployment`, `MachineSet`. It then updates the actual resources in Packet.
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

