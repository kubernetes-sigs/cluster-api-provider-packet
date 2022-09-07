# Building and Running

This document describes how to build and iterate upon the Equinix Metal (formerly Packet) infrastructure provider.

This is _not_ intended for regular users.

We recommend following the upstream [Developing Cluster API with Tilt](https://cluster-api.sigs.k8s.io/developer/tilt.html) guide for iterative test and development of CAPP.

## Example workflow

1. `git clone kubernetes-sigs/cluster-api-provider-packet`

1. `git clone kubernetes-sigs/cluster-api`

1. Move to the cluster-api-provider-packet directory

    ```sh
    cd cluster-api-provider-packet
    ```

1. `git checkout "branch-you're-testing"`

1. Move to the cluster-api directory you checked out earlier:

    ```sh
    cd ../cluster-api
    ```

1. Install tilt and kind

    ```sh
    brew install tilt
    brew install kind
    ```

1. Create the tilt-settings.json file in the cluster-api folder.

    ```sh
    touch cluster-api/tilt-settings.json 
    ```

1. Copy the following into that file, updating the <> sections with relevant info:

    ```json
    {
        "default_registry": "ghcr.io/<your github username>",
        "provider_repos": ["../cluster-api-provider-packet"],
        "enable_providers": ["packet","kubeadm-bootstrap","kubeadm-control-plane"],
        "kustomize_substitutions": {
            "PACKET_API_KEY": "<API_KEY>",
            "EXP_CLUSTER_RESOURCE_SET": "true",
            "EXP_MACHINE_POOL": "true",
            "CLUSTER_TOPOLOGY": "true"
        }
    }
    ```

1. Create a cluster.
   1. Change to the directory where you checked out both projects

        ```sh
        cd ~
        ```

   1. Run the kind  install for capd script included in the cluster-api repository:

        ```sh
        cluster-api/hack/kind-install-for-capd.sh
        ```

   1. Navigate to the cluster-api directory and run:

        ```sh
        tilt up
        ```

   1. Get another terminal window

        ```sh
        cd cluster-api-provider-packet
        ```

1. You now have a choice:

   * clusterctl

        ```sh
        clusterctl generate cluster my-cluster --kubernetes-version=1.23.6 --control-plane-machine-count=1 --worker-machine-count=1 --from templates/cluster-template-kube-vip.yaml > test-kube-vip.yaml
        ```

      1. Set your kubernetes context to the cluster created in kind

            ```sh
            kubectl apply -f test-kube-vip.yaml
            ```

   * e2e testing
      1. Run your e2e tests.

            ```sh
            make test-e2e-local
            ```
