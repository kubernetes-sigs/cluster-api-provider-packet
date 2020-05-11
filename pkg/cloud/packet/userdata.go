package packet

const userData = `#!/bin/bash
set -e
set -x
(
ARCH=amd64
  
# Obtain server IP addresses.
METADATA="https://metadata.packet.net/2009-04-04/meta-data"
HOSTNAME=$(curl -s ${METADATA}/hostname)
PRIVATEIP=$(curl -s ${METADATA}/local-ipv4)
PUBLICIP=$(curl -s ${METADATA}/public-ipv4)

# TODO: this has to be removed, just temporary to see if master shows up
ROLE="master"

CA_CERT_DIR=/etc/kubernetes/pki
  
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl

curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF | sudo tee /etc/apt/sources.list.d/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF

apt-get update -y
apt-get install -y \
    ca-certificates \
    socat \
    jq \
    ebtables \
    apt-transport-https \
    cloud-utils \
    prips
  
# If something failed during package installation but one of docker/kubeadm/kubelet was already installed
# an apt-mark hold after the install won't do it, which is why we test here if the binaries exist and if
# yes put them on hold
set +e
which docker && apt-mark hold docker docker-ce
which kubelet && apt-mark hold kubelet
which kubeadm && apt-mark hold kubeadm
  
# When docker is started from within the apt installation it fails with a
# 'no sockets found via socket activation: make sure the service was started by systemd'
# Apparently the package is broken in a way that it gets started without its dependencies, manually starting
# it works fine though
which docker && systemctl start docker
set -e
  
function install_configure_docker () {
    # prevent docker from auto-starting
    echo "exit 101" > /usr/sbin/policy-rc.d
    chmod +x /usr/sbin/policy-rc.d
    trap "rm /usr/sbin/policy-rc.d" RETURN
  
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
    apt-key fingerprint 0EBFCD88

    sudo add-apt-repository \
        "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
	$(lsb_release -cs) \
	stable"

    apt-get install -y docker-ce docker-ce-cli containerd.io kubelet kubeadm kubectl
  
    echo 'DOCKER_OPTS="--iptables=false --ip-masq=false"' > /etc/default/docker
    systemctl daemon-reload
    systemctl enable docker
    systemctl start docker
}
install_configure_docker
  
# kubeadm uses 10th IP as DNS server
CLUSTER_DNS_SERVER=$(prips ${SERVICE_CIDR} | head -n 11 | tail -n 1)
  
function install_custom_ca () {
    if [ ! -n "$MASTER_CA_CERTIFICATE" ]; then
        return
    fi
    if [ ! -n "$MASTER_CA_PRIVATE_KEY" ]; then
        return
    fi
 
    echo "Installing custom certificate authority..."
 
    PKI_PATH=${CA_CERT_DIR}
    mkdir -p ${PKI_PATH}
    CA_CERT_PATH=${PKI_PATH}/ca.crt
    echo ${MASTER_CA_CERTIFICATE} | base64 -d > ${CA_CERT_PATH}
    chmod 0644 ${CA_CERT_PATH}
    CA_KEY_PATH=${PKI_PATH}/ca.key
    echo ${MASTER_CA_PRIVATE_KEY} | base64 -d > ${CA_KEY_PATH}
    chmod 0600 ${CA_KEY_PATH}
}
  
# running with swap is not supported
swapoff -a

if [ "$ROLE" = "master" ]; then
  # Set up kubeadm config file to pass parameters to kubeadm init.
  touch /etc/kubernetes/kubeadm_config.yaml
  cat > /etc/kubernetes/kubeadm_config.yaml <<EOF
apiVersion: kubeadm.k8s.io/v1beta2
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: 147.75.39.125
  bindPort: 6443
nodeRegistration:
  kubeletExtraArgs:
    cloud-provider: external
  taints:
  - effect: NoSchedule
    key: node-role.kubernetes.io/master
---
apiServer:
  extraArgs:
    cloud-provider: external
  timeoutForControlPlane: 4m0s
apiVersion: kubeadm.k8s.io/v1beta2
certificatesDir: /etc/kubernetes/pki
clusterName: kubernetes
controllerManager:
  extraArgs:
    cloud-provider: external
dns:
  type: CoreDNS
etcd:
  local:
    dataDir: /var/lib/etcd
imageRepository: k8s.gcr.io
kind: ClusterConfiguration
kubernetesVersion: v1.18.0
networking:
  dnsDomain: cluster.local
  podSubnet: 100.96.0.0/11
  serviceSubnet: 100.64.0.0/13
EOF
  
  install_custom_ca
  
  kubeadm init --config /etc/kubernetes/kubeadm_config.yaml
  # Apply Weave CNI
  for tries in $(seq 1 60); do
    kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')&env.IPALLOC_RANGE=${POD_CIDR}" && break
    sleep 1
  done

else

  touch /etc/kubernetes/kubeadm_config.yaml
  cat > /etc/kubernetes/kubeadm_config.yaml <<EOF
apiVersion: kubeadm.k8s.io/v1beta1
kind: JoinConfiguration
nodeRegistration:
  name: ${HOSTNAME}
  kubeletExtraArgs: 
    cloud-provider: external 
EOF

  export ENDPOINT=''
      if [ -n "$MASTER_PRIVATE" ]; then
          export ENDPOINT=$MASTER_PRIVATE
      else
          export ENDPOINT=$MASTER
      fi
  
      # to make it easier to debug going forward
      echo "running kubeadm join $(date)"
      kubeadm join --token "${TOKEN}" "${ENDPOINT}" --ignore-preflight-errors=all --discovery-token-unsafe-skip-ca-verification
    fi
  
  
    # Annotate node.
    for tries in $(seq 1 60); do
        kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate --overwrite node ${HOSTNAME} machine=${MACHINE} && break
        sleep 1
    done
  
    echo done.
    ) 2>&1 | tee /var/log/startup.log`
