# Equinix Metal cluster-api Provider

[![GitHub release](https://img.shields.io/github/release/kubernetes-sigs/cluster-api-provider-packet/all.svg?style=flat-square)](https://github.com/kubernetes-sigs/cluster-api-provider-packet/releases)
[![Continuous Integration](https://github.com/kubernetes-sigs/cluster-api-provider-packet/actions/workflows/ci.yaml/badge.svg)](https://github.com/kubernetes-sigs/cluster-api-provider-packet/actions/workflows/ci.yaml)
[![GoDoc](https://godoc.org/sigs.k8s.io/cluster-api-provider-packet?status.svg)](https://pkg.go.dev/sigs.k8s.io/cluster-api-provider-packet?tab=overview)
[![Go Report Card](https://goreportcard.com/badge/sigs.k8s.io/cluster-api-provider-packet)](https://goreportcard.com/report/sigs.k8s.io/cluster-api-provider-packet)
[![Docker Pulls](https://img.shields.io/docker/pulls/packethost/cluster-api-provider-packet.svg)](https://hub.docker.com/r/packethost/cluster-api-provider-packet/)

This is the official [cluster-api](https://github.com/kubernetes-sigs/cluster-api) provider for [Equinix Metal](https://metal.equinix.com/), formerly known as Packet. It implements cluster-api provider version v1beta1.

![Packetbot works hard to keep Kubernetes cluster in a good shape](./docs/banner.png)

## Ugrading from v0.3.X to v1.1.X

* **IMPORTANT** - Before you upgrade, please note that multi-tenancy support has changed in versions after v0.3.X
  * We no longer support running multiple instances of the provider in the same management cluster. Typically this was done to enable multiple credentials for managing devices in more than one project.
  * If you currently have a management cluster with multiple instances of the provider, it's recommended you use clusterctl move to migrate them to another cluster before upgrading.
  * [See more information about `clusterctl move` here](https://cluster-api.sigs.k8s.io/clusterctl/commands/move.html)

* Upgrade your clusterctl to version 1.1.3 or later.
* Backup your clusterapi objects from your management cluster by using the `clusterctl backup` comamnd.

```bash
clusterctl backup --directory /path/to/backup/directory/
```

* More details are available [here](https://cluster-api.sigs.k8s.io/clusterctl/commands/upgrade.html).
* The next step is to run `clusterctl upgrade plan`, and you should see something like this:

```bash
Latest release available for the v1beta1 API Version of Cluster API (contract):

NAME                    NAMESPACE                            TYPE                     CURRENT VERSION   NEXT VERSION
bootstrap-kubeadm       capi-kubeadm-bootstrap-system        BootstrapProvider        v0.3.25           v1.1.2
control-plane-kubeadm   capi-kubeadm-control-plane-system    ControlPlaneProvider     v0.3.25           v1.1.2
cluster-api             capi-system                          CoreProvider             v0.3.25           v1.1.2
infrastructure-packet   cluster-api-provider-packet-system   InfrastructureProvider   v0.3.11           v0.5.0

You can now apply the upgrade by executing the following command:

clusterctl upgrade apply --contract v1beta1
```

* Go ahead and run `clusterctl upgrade apply --contract v1beta1`
* After this, if you'd like to co ntinue and upgrade kubernetes, it's a normal upgrade flow where you upgrade the control plane by editing the machinetemplates and kubeadmcontrolplane and the workers by editing the machinesets and machinedeployments. Full details [here](https://cluster-api.sigs.k8s.io/tasks/upgrading-clusters.html). Below is a very basic example upgrade of a small cluster:

```bash
kubectl get PacketMachineTemplate example-control-plane -o yaml > example-control-plane.yaml
# Using a text editor, edit the spec.version field to the new kubernetes version
kubectl apply -f example-control-plane.yaml
kubectl get machineDeployment example-worker-a -o yaml > example-worker-a.yaml
# Using a text editor, edit the spec.template.spec.version to the new kubernetes version
kubectl apply -f example-worker-a.yaml
```

## Using

The following section describes how to use the cluster-api provider for packet (CAPP) as a regular user.
You do _not_ need to clone this repository, or install any special tools, other than the standard
`kubectl` and `clusterctl`; see below.

* To build CAPP and to deploy individual components, see [docs/BUILD.md](./docs/BUILD.md).
* To build CAPP and to cut a proper release, see [docs/RELEASE.md](./docs/RELEASE.md).

### Requirements

To use the cluster-api to deploy a Kubernetes cluster to Equinix Metal, you need the following:

* A Equinix Metal API key
* A Equinix Metal project ID
* The `clusterctl` binary from the [official cluster-api provider releases page](https://github.com/kubernetes-sigs/cluster-api/releases)
* A Kubernetes cluster - the "bootstrap cluster" - that will deploy and manage the cluster on Equinix Metal.
* `kubectl` - not absolutely required, but it is hard to interact with a cluster without it!

For the bootstrap cluster, any compliant cluster will work, including
[official kubernetes](https://kubernetes.io), [k3s](https://k3s.io), [kind](https://github.com/kubernetes-sigs/kind)
and [k3d](https://github.com/rancher/k3d).

Once you have your cluster, ensure your `KUBECONFIG` environment variable is set correctly.

### Getting Started

You can follow the [Cluster API Quick Start Guide](https://cluster-api.sigs.k8s.io/user/quick-start.html), selecting the 'Equinix Metal' tabs.

#### Defaults

If you do not change the generated `yaml` files, it will use defaults. You can look in the [templates/cluster-template.yaml](./templates/cluster-template.yaml) file for details.

* SERVICE_CIDR: `172.26.0.0/16`
* POD_CIDR: `192.168.0.0/16`
* NODE_OS: `ubuntu_18_04`

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

* Chat with us on [Slack](http://slack.k8s.io/) in the [#cluster-api-provider-packet][#cluster-api-provider-packet slack] channel
* Subscribe to the [SIG Cluster Lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) Google Group for access to documents and calendars

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
[#cluster-api-provider-packet slack]: https://kubernetes.slack.com/archives/C8TSNPY4T
