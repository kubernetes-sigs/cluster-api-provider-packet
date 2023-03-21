# PacketMachine CRD

PacketMachine is the name of the resource that identifies a
[Device](https://metal.equinix.com/developers/api/devices/#devices-createdevice) on Packet.

This is an example of it:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PacketMachine
metadata:
  name: "qa-controlplane-0"
spec:
  os: "ubuntu_22_04"
  billingCycle: hourly
  machineType: "c3.small.x86"
  sshKeys:
  - "Your SSH public key"
  tags: []
```

It is a [Kubernetes Custom Resource Definition (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) as everything
else in the cluster-api land.

The reported fields in the example are the most common one but you can see the
full list of supported parameters as part of the OpenAPI definition available
[here](../../config/crd/bases/infrastructure.cluster.x-k8s.io_packetclusters.yaml)
searching for `kind: PacketMachine`.

The `PacketMachine`, `PacketCluster`, and `PacketMachineTemplate` CRD specs are also documented at [docs.crds.dev](https://doc.crds.dev/github.com/kubernetes-sigs/cluster-api-provider-packet).