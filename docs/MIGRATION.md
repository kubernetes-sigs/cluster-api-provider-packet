# Migration Notes

## Ugrading from CAPI v0.3.X to v1.1.X

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
