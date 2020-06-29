The PacketCluster is the CRD that contains information about where to place the
Kubernetes cluster: facility and project.

We do not support cross facilities or multi projects cluster. If you need so my
suggestion is to look for what it is called [federation](k8s-federation).

## Topology

Each cluster we create leverages at least two Packet features: Device and ElasticIP.

Kubernetes node is a Device and each cluster has an
[ElasticIP](elastic-ip-packet) that is tagged with the name of the cluster.

The ElasticIP guarantees a stable endpoint even when the control plane(s) are
recycling during a Kubernetes version update or an outages.

## ElasticIP lifecycle

Every cluster has its own ElasticIP. It is tagged with the name of the cluster and
it does not get removed when a cluster is terminated. You have to remove it manually.

This is a safety feature in this way you can re-assign the IP to another
cluster with the same name.

## FAQ

**Does cluster-api work with only Ubuntu/Debian?**

Currently the cluster-template only supports Ubuntu because it uses `apt` and it
does a couple of assumptions around networking. This does not mean that you
can't use `cluster-api-provier-packet` with other templates, but you will have
to make your own cluster specification in order to make the installation process
to work as you want. We have an open issue about this: ["Figure out where we
stand about operating systems"](os-issue).

[k8s-federation]: https://kubernetes.io/blog/2018/12/12/kubernetes-federation-evolution/
[elastic-ip-packet]: https://www.packet.com/developers/docs/network/basic/elastic-ips/
[os-issue]: https://github.com/kubernetes-sigs/cluster-api-provider-packet/issues/118
