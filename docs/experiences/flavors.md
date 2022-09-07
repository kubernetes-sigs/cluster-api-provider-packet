# Flavors & Custom Templates

## Kube-VIP

### API Server VIP Management Choice

By default CPEM will be used to manage the EIP that serves as the VIP for the api-server. As of v0.6.0 you can choose to use kube-vip to manage the api-server VIP instead of CPEM.

### Choosing Kube-VIP

 To use kube-vip, when generating the template with `clusterctl`, pass in the `--flavor kube-vip` flag. For example, your `clusterctl generate` command might look like the following:

```sh
clusterctl generate cluster capi-quickstart \
  --kubernetes-version v1.24.0 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  --infrastructure packet \
  --flavor kube-vip
  > capi-quickstart.yaml
```

## Custom Templates

When using the `clusterctl` you can generate your own cluster spec from a
template.

This is what happens when you run:

```sh
$ clusterctl config cluster <cluster-name> --infrastructure packet >
out/cluster.yaml
```

The workflow triggered by that command summarized here:

1. Based on the `--infrastructure` option it goes to the right repository and it
   looks for the `latest` release. At the time I am writing it is
   [v0.3.0](gh-release-v030).
2. `clusterctl` lookup a file called `cluster-template.yaml` from the release artifacts
3. `clusterctl` uses `cluster-template.yaml` plus a set of environment variables
   that you can find described in the README.md to generate the cluster
   specification
4. With the command `> out/cluster.yaml` the generated spec gets moved to
   `./out/cluster.yaml` other than being printed to `stdout`.

This is good if you do not have particular needs or if you are trying capp.

The current `cluster-templates.yaml` uses `apt` and it depends upon `Ubuntu`.
But you can open and modify it to match your requirements.

## When should I modify the template?

Every time you feel like the default one is not enough, or if you need more
automation. Here a few examples:

1. ClusterAPI decided to leave the CNI configuration out from its workflow
   because there are too many of them. If you want to automate that part and
   let's suppose you want `flannel` you can add the following line to
   `postKubeadmCommands` for the `KubeadmControlPlane` resource:

   ```sh
   kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
   ```

1. If you want to use an operating system that is not Ubuntu you can change the
   `preKubeadmCommands` for the `KubeadmControlPlane` and the
   `KubeadmConfigTemplate` to use kubernetes binaries or a different package
   manager.

   If you want to change operating system you have to change the `OS` field
   for the `PacketMachineTemplate` resource.

[gh-release-v030]: https://github.com/kubernetes-sigs/cluster-api-provider-packet/releases/tag/v0.3.0
