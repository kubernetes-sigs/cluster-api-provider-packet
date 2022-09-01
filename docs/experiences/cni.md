# CNI Notes

## Calico

### Install

When using the CAPI quickstart, follow the [Calico install instructions from Tigera](https://projectcalico.docs.tigera.io/getting-started/kubernetes/quickstart).

## Flannel

### Install

Follow the instructions at <https://github.com/flannel-io/flannel#deploying-flannel-manually> (ignoring the instruction to create a `flanneld` binary on each node).

When declaring your cluster, set the `POD_CIDR` to `10.244.0.0/16` which is the default `Network` (`net-conf.json`) for Flannel, or update the Flannel manifest to match the desired POD CIDR.
