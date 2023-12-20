# Equinix Metal cluster-api Provider

[![GitHub release](https://img.shields.io/github/release/kubernetes-sigs/cluster-api-provider-packet/all.svg?style=flat-square)](https://github.com/kubernetes-sigs/cluster-api-provider-packet/releases)
[![Continuous Integration](https://github.com/kubernetes-sigs/cluster-api-provider-packet/actions/workflows/ci.yaml/badge.svg)](https://github.com/kubernetes-sigs/cluster-api-provider-packet/actions/workflows/ci.yaml)
[![GoDoc](https://godoc.org/sigs.k8s.io/cluster-api-provider-packet?status.svg)](https://pkg.go.dev/sigs.k8s.io/cluster-api-provider-packet?tab=overview)
[![Go Report Card](https://goreportcard.com/badge/sigs.k8s.io/cluster-api-provider-packet)](https://goreportcard.com/report/sigs.k8s.io/cluster-api-provider-packet)
[![Docker Pulls](https://img.shields.io/docker/pulls/packethost/cluster-api-provider-packet.svg)](https://hub.docker.com/r/packethost/cluster-api-provider-packet/)

This is the official [cluster-api](https://github.com/kubernetes-sigs/cluster-api) provider for [Equinix Metal](https://metal.equinix.com/), formerly known as Packet. It implements cluster-api provider version v1beta1.

![Packetbot works hard to keep Kubernetes cluster in a good shape](./docs/banner.png)

## Upgrading to v0.8.X

**IMPORTANT** We removed support for the _very_ old packet-ccm cloud provider in this release, please migrate to [Cloud Provider Equinix Metal](https://github.com/kubernetes-sigs/cloud-provider-equinix-metal) before upgrading.

- Now based on CAPI 1.6, please see [Cluster API release notes](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.6.0) for kubernetes version compatibility and relevant upgrade notes.
- The API version v1alpha3 has been completely removed in this release. Realistically, this was not used by anyone, but if you were using it, at this point it's likely easier to deploy a fresh cluster than to try to upgrade.
- We're deprecating --metrics-bind-addr and defaulting to secure communications for the metric server. Please see more info on the [upstream Cluster API PR](https://github.com/kubernetes-sigs/cluster-api/pull/9264).
- We've changed the tags applied to devices in the Equinix Metal API to start with "capp" instead of "cluster-api-provider-packet". This was done to enable longer cluster and machine names within the 80 character limit of the Equinix Metal API. If you have any automation that relies on the old tags, you'll need to update it.
- Pursuant to the above, if you have a cluster that is likely to add new nodes WHILE you are upgrading the Cluster API Provider Packet component, add the `cluster.x-k8s.io/paused` annotation to your cluster object. This will pause remediation. Then remember to remove the annotation after the upgrade.

## Ugrading to v0.7.X

**IMPORTANT** Before you upgrade, please note that Facilities have been deprecated as of version v0.7.0

- Newly generated cluster yaml files will use Metro by default.
- Facility is still usable, but should be moved away from as soon as you can
- See here for more info on the facility deprecation: [Bye Facilities, Hello (again) Metros](https://feedback.equinixmetal.com/changelog/bye-facilities-hello-again-metros)
- If you would like to upgrade your existing clusters from using facilities to using metros, please work with your Equinix support team to figure out the best course of action. We can also provide some support via our [community Slack](https://slack.equinixmetal.com/) and the [Equinix Helix community site](https://community.equinix.com/).
- The basic process will be to upgrade to v0.7.0, then replace `facility: sv15` with `metro: sv` (insert your correct metro instead of sv, for more information check out our [Metros documentation](https://deploy.equinix.com/developers/docs/metal/locations/metros/)) in your existing PacketCluster and PacketMachineTemplate objects.
  - For example, to update a PacketCluster object from facility `sv15` to metro `sv`
    - `kubectl patch packetclusters my-cluster --type='json' -p '[{"op":"remove","path":"/spec/facility"},{"op":"add","path":"/spec/metro","value":"sv"}]'`
  - To update a PacketMachineTemplate object from facility `sv15` to metro `sv` **PLEASE NOTE** Most people do not set the facility on their PacketMachineTemplate objects, so you may not need to do this step.
    - `kubectl patch packetmachinetemplate my-cluster-control-plane --type='json' -p '[{"op":"remove","path":"/spec/template/spec/facility"},{"op":"add","path":"/spec/template/spec/metro","value":"sv"}]'`
- The expectation is that if the devices are already in the correct metros you've specified, no disruption will happen to clusters or their devices, however, **as with any breaking change you should verify this outside of production before you upgrade.**

## Requirements

To use the cluster-api to deploy a Kubernetes cluster to Equinix Metal, you need the following:

- A Equinix Metal API key
- A Equinix Metal project ID
- The `clusterctl` binary from the [official cluster-api provider releases page](https://github.com/kubernetes-sigs/cluster-api/releases)
- A Kubernetes cluster - the "bootstrap cluster" - that will deploy and manage the cluster on Equinix Metal.
- `kubectl` - not absolutely required, but it is hard to interact with a cluster without it!

For the bootstrap cluster, any compliant cluster will work, including
[official kubernetes](https://kubernetes.io), [k3s](https://k3s.io), [kind](https://github.com/kubernetes-sigs/kind)
and [k3d](https://github.com/rancher/k3d).

Once you have your cluster, ensure your `KUBECONFIG` environment variable is set correctly.

## Getting Started

You should then follow the [Cluster API Quick Start Guide](https://cluster-api.sigs.k8s.io/user/quick-start.html), selecting the 'Equinix Metal' tabs where offered.

### Defaults

If you do not change the generated `yaml` files, it will use defaults. You can look in the [templates/cluster-template.yaml](./templates/cluster-template.yaml) file for details.

- `CPEM_VERSION` (defaults to `v3.7.0`)
- `KUBE_VIP_VERSION` (defaults to `v0.6.4`)
- `NODE_OS` (defaults to `ubuntu_20_04`)
- `POD_CIDR` (defaults to `192.168.0.0/16`)
- `SERVICE_CIDR` (defaults to `172.26.0.0/16`)

### Reserved Hardware

If you'd like to use reserved instances for your cluster, you need to edit your cluster yaml and add a hardwareReservationID field to your PacketMachineTemplates. That field can contain either a comma-separated list of hardware reservation IDs you'd like to use (which will cause it to ignore the facility and machineType you've specified), or just "next-available" to let the controller pick one that's available (that matches the machineType and facility you've specified). Here's an example:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PacketMachineTemplate
metadata:
  name: my-cluster-control-plane
  namespace: default
spec:
  template:
    spec:
      billingCycle: hourly
      machineType: c3.small.x86
      os: ubuntu_20_04
      sshKeys:
        - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDvMgVEubPLztrvVKgNPnRe9sZSjAqaYj9nmCkgr4PdK username@computer
      tags: []
      #If you want to specify the exact machines to use, provide a comma separated list of UUIDs
      hardwareReservationID: "b537c5aa-2ef3-11ed-a261-0242ac120002,b537c5aa-2ef3-11ed-a261-0242ac120002"
      #Or let the controller pick from available reserved hardware in the project that matches machineType and facility with `next-available`
      #hardwareReservationID: "next-available"
```

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

Equinix has a [cluster-api guide](https://metal.equinix.com/developers/guides/kubernetes-cluster-api/)

You can reach the maintainers of this project at:

- Chat with us on [Slack](http://slack.k8s.io/) in the [#cluster-api-provider-packet](https://kubernetes.slack.com/archives/C8TSNPY4T) channel
- Subscribe to the [SIG Cluster Lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) Google Group for access to documents and calendars

## Development and Customizations

The following section describes how to use the cluster-api provider for packet (CAPP) as a regular user.
You do _not_ need to clone this repository, or install any special tools, other than the standard
`kubectl` and `clusterctl`; see below.

- To build CAPP and to deploy individual components, see [docs/BUILD.md](./docs/BUILD.md).
- To build CAPP and to cut a proper release, see [docs/RELEASE.md](./docs/RELEASE.md).

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
