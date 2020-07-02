[KubeadmControlPlane](kubeadmcontrolplane-book) is a CRD provided by the cluster-api kubeadm bootstrapped.
It manages the control planes lifecycle.

We use this CRD when deploying a Kubernetes Cluster in High Availability because
you can use the field `replicas` to specify how many control plans you need.

It is a good idea to use it even when you have only one control plane because it
manages Kubernetes updates.

[kubeadmcontrolplane-book]: https://cluster-api.sigs.k8s.io/developer/architecture/controllers/control-plane.html
