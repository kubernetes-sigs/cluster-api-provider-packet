# Supported node OS and Versions

CAPP (Cluster API Provider for Packet) supports Ubuntu 18.04 and Kubernetes 1.14.3. To extend it to work with different combinations, you only need to edit the file [config/default/machine_configs.yaml](./config/default/machine_configs.yaml).

In this file, each list entry represents a combination of OS and Kubernetes version supported. Each entry is composed of the following parts:

* `machineParams`: list of the combination of OS image, e.g. `ubuntu_18_04`, and Kubernetes versions, both control plane and kubelet, to install. Also includes the container runtime to install.
* `userdata`: the actual userdata that will be run on server instance startup.

When trying to install a new machine, the logic is as follows:

1. Take the requested image and kubernetes versions.
1. Match those to an entry in `machineParams`. If it matches, use this `userdata`.

Important notes:

* There can be multiple `machineParams` entries for each `userdata`, enabling one userdata script to be used for more than one combination of OS and Kubernetes versions.
* There are versions both for `controlPlane` and `kubelet`. `master` servers will match both `controlPlane` and `kubelet`; worker nodes will have no `controlPlane` entry.
* The `containerRuntime` is installed as is. The value of `containerRuntime` will be passed to the userdata script as `${CR_PACKAGE}`, to be installed as desired.
