/*
Copyright 2019 Packet Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployer

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/ca"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/cluster-api-provider-packet/pkg/tokens"
	"github.com/packethost/packngo"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	defaultTokenTTL = 30 * time.Minute
	adminUserName   = "kubernetes-admin"
)

// Deployer satisfies the ProviderDeployer(https://github.com/kubernetes-sigs/cluster-api/blob/master/cmd/clusterctl/clusterdeployer/clusterdeployer.go) interface.
type Deployer struct {
	client        *packet.PacketClient
	secretsGetter corev1.SecretsGetter
	ControlPort   int
}

// Params is used to create a new deployer.
type Params struct {
	Client        *packet.PacketClient
	SecretsGetter corev1.SecretsGetter
	Port          int
}

// New returns a new Deployer.
func New(params Params) (*Deployer, error) {
	d := Deployer{
		client:        params.Client,
		secretsGetter: params.SecretsGetter,
		ControlPort:   params.Port,
	}
	return &d, nil
}

// GetIP returns IP address of the machine in the cluster. If no machine is given, find the IP for the cluster itself, i.e. the master
func (d *Deployer) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cannot get IP of machine in nil cluster")
	}
	var (
		device     *packngo.Device
		err        error
		machineRef string
	)
	if machine != nil {
		klog.Infof("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)
		// if there are no annotations, or the annotation we want does not exist, nothing to do
		if machine.Annotations == nil {
			return "", fmt.Errorf("No annotations with machine UID for %s", machine.Name)
		}
		var (
			mUID string
			ok   bool
		)
		if mUID, ok = machine.Annotations[util.AnnotationUID]; !ok {
			return "", fmt.Errorf("No UID annotation %s on machine %s", util.AnnotationUID, machine.Name)
		}
		machineRef = mUID
		device, err = d.client.GetDevice(machine)
	} else {
		klog.Infof("Getting IP of any master machine for cluster %v.", cluster.Name)
		machineRef = fmt.Sprintf("master for cluster %s", cluster.Name)
		tags := []string{
			util.GenerateClusterTag(string(cluster.Name)),
			util.MasterTag,
		}
		c, err := util.ClusterProviderFromProviderConfig(cluster.Spec.ProviderSpec)
		if err != nil {
			return "", fmt.Errorf("unable to unpack cluster provider: %v", err)
		}
		device, err = d.client.GetDeviceByTags(c.ProjectID, tags)
	}
	if err != nil {
		return "", fmt.Errorf("error retrieving machine status %s: %v", machineRef, err)
	}
	if device == nil {
		return "", &MachineNotFound{err: fmt.Sprintf("machine does not exist: %s", machineRef)}
	}
	if device.Network == nil || len(device.Network) == 0 || device.Network[0].Address == "" {
		return "", &MachineNoIP{err: fmt.Sprintf("machine does not yet have an IP address: %s", machineRef)}
	}
	// TODO: validate that this address exists, so we don't hit nil pointer
	// TODO: check which address to return
	// TODO: check address format (cidr, subnet, etc.)
	return device.Network[0].Address, nil
}

// GetKubeConfig returns the kubeconfig after the bootstrap process is complete. In other words,
//  it is the kubeconfig to the master of the workload cluster
func (d *Deployer) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cannot get kubeconfig for nil cluster")
	}
	klog.Infof("Getting KubeConfig for cluster %v.", cluster.Name)

	// use local var to allow for easier future changes
	userName := adminUserName

	// what we need to do:
	// 1. Get the CA cert and key
	// 2. Get the URL of the master
	// 3. Generate a client key + cert signed by CA
	// 4. Generate a kubeconfig with all of the above
	// 5. serialize to yaml

	var secretsClient corev1.SecretInterface

	if d.secretsGetter != nil {
		secretsClient = d.secretsGetter.Secrets(util.CAPPNamespace)
	}
	// ensure that we have a CA cert/key and save it
	caKeyBytes, caCertBytes, err := util.GetCAFromSecret(secretsClient, cluster)
	if err != nil {
		return "", fmt.Errorf("unable to get CA from secret: %v", err)
	}

	// get the URL of the master
	server, err := d.ClusterURL(cluster)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve controlplane URL: %+v", err)
	}

	caCertPem, _ := pem.Decode(caCertBytes)
	caCert, err := x509.ParseCertificate(caCertPem.Bytes)
	if err != nil {
		return "", fmt.Errorf("error parsing CA certificate: %v", err)
	}
	caKeyPem, _ := pem.Decode(caKeyBytes)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyPem.Bytes)
	if err != nil {
		return "", fmt.Errorf("error parsing CA key: %v", err)
	}

	// generate a client key + cert signed by CA
	clientCert, clientKey, err := ca.GenerateClient(userName, caCert, caKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate a client cert and key: %+v", err)
	}

	// generate a kubeconfig with all of the above
	clusterName := cluster.Name
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)

	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   server,
				CertificateAuthorityData: caCertBytes,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			userName: {
				ClientKeyData:         ca.PemEncodeKey(clientKey),
				ClientCertificateData: ca.PemEncodeCert(clientCert),
			},
		},
		CurrentContext: contextName,
	}

	// serialize to yaml
	yaml, err := clientcmd.Write(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("failed to serialize config to yaml: %v", err)
	}

	return string(yaml), nil
}

func (d *Deployer) ClusterURL(cluster *clusterv1.Cluster) (string, error) {
	controlPlaneDNSName, err := d.GetIP(cluster, nil)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve controlplane (GetIP): %+v", err)
	}

	return fmt.Sprintf("https://%s:%d", controlPlaneDNSName, d.ControlPort), nil
}

func (d *Deployer) CoreV1Client(cluster *clusterv1.Cluster) (corev1.CoreV1Interface, error) {
	controlPlaneURL, err := d.ClusterURL(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve controlplane URL: %+v", err)
	}

	kubeConfig, err := d.GetKubeConfig(cluster, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve kubeconfig for cluster %q: %v", cluster.Name, err)
	}

	clientConfig, err := clientcmd.BuildConfigFromKubeconfigGetter(controlPlaneURL, func() (*clientcmdapi.Config, error) {
		return clientcmd.Load([]byte(kubeConfig))
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get client config for cluster at %q: %v", controlPlaneURL, err)
	}

	return corev1.NewForConfig(clientConfig)
}

func (d *Deployer) NewBootstrapToken(cluster *clusterv1.Cluster) (string, error) {
	coreClient, err := d.CoreV1Client(cluster)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve corev1 client: %v", err)
	}
	// generate a new bootstrap token, then save it as valid
	token, err := tokens.NewBootstrap(coreClient, defaultTokenTTL)
	if err != nil {
		return "", fmt.Errorf("failed to create or save new bootstrap token: %v", err)
	}
	return token, nil
}
