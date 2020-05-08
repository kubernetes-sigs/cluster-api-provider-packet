# Building and Running

This document describes how to build and iterate upon the Packet infrastructure provider.

This is _not_ intended for regular users.

There are several stages:

1. Build and Deploy - build your components and deploy them
1. Iterate - once your components are deployed, make any necessary changes
1. Apply - apply your cluster yaml. This is covered in the main [README.md](../README.md)

## Build and Deploy

Building and deploying initially involves the following steps:

1. deploy a management cluster
1. build the CAPP manager binary
1. deploy the core cluster-api provider to the management cluster
1. generate Packet infrastructure provider in a managerless mode
1. deploy the Packet infrastructure provider to the management cluster
1. run the binary locally against your cluster

### Deploy a cluster

Now you need to deploy the management cluster. If you are reading this document, it is assumed you know how
to deploy a kubernetes cluster. Any compliant cluster will work, including
[official kubernetes](https://kubernetes.io), [k3s](https://k3s.io), [kind](https://github.com/kubernetes-sigs/kind)
and [k3d](https://github.com/rancher/k3d).

Once you have your cluster, ensure your `KUBECONFIG` environment variable is set correctly.

### Build it

To build the binary, you need to:

1. modify your CRDs in [config/crd](./config/crd) as needed
1. Run `make generate` to generate the `.go` files in [api/](./api)
1. Run `make manager` to generate the binary for your local OS/architecture. If you prefer to build for another, run `make manager OS=<os> ARCH=<arch>`, filling in `<os>` and `<arch>` as needed

You now should have a functional manager in [bin/](./bin/) named `manager-<os>-<arch>`.

### Deploy the core cluster-api provider

This can be done in one of three ways:

* Manually: This generally is not recommended, but is good for seeing the various parts that make up a manager cluster, understanding how they work together, and debugging issues.
  * apply the cert manager as `kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.14.1/cert-manager.yaml`
  * download the components from the [official cluster-api releases page](https://github.com/kubernetes-sigs/cluster-api/releases); you will need all of the `.yaml` files in a release. Then run `kubectl apply -f <dir>` to whatever directory you downloaded it. The order _does_ matter, and the CRDs have to exist, so you might need to
`kubectl apply` multiple times until it all is accepted.
* Make target: This just wraps the above manual steps: `make cluster-init`
* CLI: use the `clusterctl` binary from the [official cluster-api releases page](https://github.com/kubernetes-sigs/cluster-api/releases) to deploy to your cluster. This will download the yaml files, apply them and ensure that CRDs are in place before applying the rest.

### Generate the Packet infrastructure provider yaml

You need to generate the "managerless" version of the Packet cluster-api infrastructure provider.
This is _almost_ identical to the yaml that is deployed for a regular user, except that it does _not_
deploy the pod which contains the manager. This lets you develop and run it locally against your cluster.

We have created a simple way to generate it:

```sh
make managerless
```

This will generate the yaml you need in: [out/managerless/infrastructure-packet/0.3.0/infrastructure-components.yaml](./out/managerless/infrastructure-packet/0.3.0/infrastructure-components.yaml).
This odd path is so that it complies with the `clusterctl` requirements.

### Deploy the Packet infrastructure provider

You now can apply it to your cluster with:

```sh
kubectl apply -f out/managerless/infrastructure-packet/0.3.0/infrastructure-components.yaml
```

If you prefer to use the `clusterctl` binary, you just need to specify the path. The `make managerless`
will tell you how to run it, as `clusterctl --config=out/managerless/infrastructure-packet/clusterctl-0.3.0.yaml ...`

Your cluster is now ready for usage.

### Run your manager binary

Don't forget to set your `KUBECONFIG` environment variable, and then
run your manager binary against your cluster as `./bin/manager-<os>-<arch>`.

For example:

```sh
./bin/manager-darwin-amd64
```

At this point, you can change your binary

## Iterate

The process to apply changes depends upon if you are changing just your manager binary, or also
your CRDs.

### Manager Only

To make changes just to your manager binary, without changing your CRDs:

1. Stop running the manager binary
1. Make any changes
1. Rebuild the manager with `make manager`
1. Start your manager again with `./bin/manager-<os>-<arch>`

You do not need to reapply any components or restart your cluster.

### CRDs

If you are changing your CRDs, for example the spec on your cluster or machine, or any templates or
additional CRDs, you need to regenerate some components:

1. Stop running the manager binary
1. Make any changes to your CRD specs in [api/](./api/)
1. Regenerate with `make generate`
1. Rebuild the manager with `make manager`
1. Start your manager again with `./bin/manager-<os>-<arch>`

The core components do not need to be reapplied.

At this point, you can apply any actual cluster-api resources, such as `Cluster` or `Machine`.
See [README.md](./README.md) for details.


## Building

The following are the requirements for building:

* [go](https://golang.org), v1.13 or higher
* Make

To build all of the components:

```
make
```

This will leave you with:

* the controller binary as `bin/manager-<os>-<arch>` for the OS and architecture on which you are running
* the infrastructure provider yaml files for Packet in [out/release](./out/release)

You can build for a different OS or architecture by setting `OS` and `ARCH`. For example:

```
make OS=windows
make OS=linux ARCH=arm64
make ARCH=s390x
```

To build the OCI image for the controller:

```
make image
```

This will leave you with:

* a docker image whose name matches the one set as the default in the `Makefile`
* the release components in [out/release](./out/release)

You can change the name of the image to be built with `IMG=`, for example:

```
make image IMG=myname/img-provider
```

To see the name of the docker image that would be built, run:

```
make image-name
```

To push it out:

```
make push
```

To build individual components, call its target:

```
make manifests
make manager
```

As always with `make`, you can force the rebuilding of a component with `make -B <target>`.
