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
