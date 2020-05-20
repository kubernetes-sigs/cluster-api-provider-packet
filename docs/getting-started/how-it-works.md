# How It Works

The Packet cluster-api provider follows the standard design for cluster-api. It consists of two components:

* `manager` - the controller that runs in your bootstrap cluster, and watches for creation, update or deletion of the standard resources: `Cluster`, `Machine`, `MachineDeployment`. It then updates the actual resources in Packet.
* `clusterctl` - a convenient utility run from your local desktop or server and controls the new deployed cluster via the bootstrap cluster.

The actual machines are deployed using `kubeadm`. The deployment process uses the following process.

1. When a new `Cluster` is created:
   * if the appropriate `Secret` does not include a CA key/certificate pair, create one and save it in that `Secret`
2. When a new master `Machine` is created:
   * retrieve the CA certificate and key from the appropriate Kubernetes `Secret`
   * launch a new server instance on Packet
   * set the `cloud-init` on the instance to run `kubeadm init`, passing it the CA certificate and key
3. When a new worker `Machine` is created:
   * check if the cluster is ready, i.e. has a valid endpoint; if not, retry every 15 seconds
   * generate a new kubeadm bootstrap token and save it to the workload cluster
   * launch a new server instance on Packet
   * set the `cloud-init` on the instance to run `kubeadm join`, passing it the newly generated kubeadm token
4. When a user requests the kubeconfig via `clusterctl`, generate a new one using the CA key/certificate pair

