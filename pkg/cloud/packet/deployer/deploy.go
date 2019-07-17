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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/ca"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/cluster-api-provider-packet/pkg/tokens"
	"github.com/packethost/packngo"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/cert"
)

const (
	defaultTokenTTL = 10 * time.Minute
	adminUserName   = "kubernetes-admin"
)

// Deployer satisfies the ProviderDeployer(https://github.com/kubernetes-sigs/cluster-api/blob/master/cmd/clusterctl/clusterdeployer/clusterdeployer.go) interface.
type Deployer struct {
	client      *packet.PacketClient
	caCache     string
	ControlPort int
	// Certs map of clusterName to CA CertificateAuthority pointers. Each CertificateAuthority contains the Certificate and PrivateKey as its PEM-Encoded bytes
	Certs map[string]*cert.CertificateAuthority
}

// Params is used to create a new deployer.
type Params struct {
	Client  *packet.PacketClient
	Port    int
	CACache string
}

// New returns a new Deployer.
func New(params Params) (*Deployer, error) {
	d := Deployer{
		client:      params.Client,
		caCache:     params.CACache,
		ControlPort: params.Port,
		Certs:       map[string]*cert.CertificateAuthority{},
	}
	// start by loading cache, if needed
	err := d.readCache()
	if err != nil {
		return nil, fmt.Errorf("unable to read CA cache %s: %v", d.caCache, err)
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
		log.Printf("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)
		machineRef = string(machine.UID)
		device, err = d.client.GetDevice(machine)
	} else {
		log.Printf("Getting IP of any master machine for cluster %v.", cluster.Name)
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
	log.Printf("Getting KubeConfig for cluster %v.", cluster.Name)

	// use local var to allow for easier future changes
	userName := adminUserName

	// what we need to do:
	// 1. Get the CA cert and key
	// 2. Get the URL of the master
	// 3. Generate a client key + cert signed by CA
	// 4. Generate a kubeconfig with all of the above
	// 5. serialize to yaml

	// get the CA cert and key
	var (
		caCertKey *cert.CertificateAuthority
		err       error
		ok        bool
	)
	if caCertKey, ok = d.Certs[cluster.Name]; !ok {
		return "", fmt.Errorf("CA certs for cluster %s not initialized", cluster.Name)
	}

	// get the URL of the master
	server, err := d.ClusterURL(cluster)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve controlplane URL: %+v", err)
	}

	caCertPem, _ := pem.Decode(caCertKey.Certificate)
	caCert, err := x509.ParseCertificate(caCertPem.Bytes)
	if err != nil {
		return "", fmt.Errorf("error parsing CA certificate: %v", err)
	}
	caKeyPem, _ := pem.Decode(caCertKey.PrivateKey)
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
				CertificateAuthorityData: caCertKey.Certificate,
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

// readCache read the CAs that have been cached to a file
// if blank, returns nil
func (d *Deployer) readCache() error {
	if d.caCache == "" {
		return nil
	}
	// open for reading if it is there
	f, err := os.Open(d.caCache)
	switch {
	case err != nil && os.IsNotExist(err):
		return nil
	case err != nil:
		return fmt.Errorf("failed to open cache %s for reading: %v", d.caCache, err)
	}
	defer f.Close()
	// it exists, so read it
	decoder := json.NewDecoder(f)
	certs := map[string]*cert.CertificateAuthority{}
	if err := decoder.Decode(&certs); err != nil {
		return fmt.Errorf("error reading cache file %s: %v", d.caCache, err)
	}
	// save the data
	d.Certs = certs
	return nil
}

// writeCache write the CAs to a cache file
// if blank, returns nil
func (d *Deployer) writeCache() error {
	if d.caCache == "" {
		return nil
	}
	// create the file if it does not exist
	f, err := os.OpenFile(d.caCache, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open cache %s for writing: %v", d.caCache, err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	encoder.Encode(d.Certs)
	return nil
}

// GetCA get the CA cert pair for a cluster
func (d *Deployer) GetCA(name string) (*cert.CertificateAuthority, error) {
	c, ok := d.Certs[name]
	if !ok {
		return nil, nil
	}
	return c, nil
}

// PutCA put the CA cert pair for a cluster
func (d *Deployer) PutCA(name string, ca *cert.CertificateAuthority) error {
	d.Certs[name] = ca
	return d.writeCache()
}

// DeleteCA remove the CA cert pair for a cluster
func (d *Deployer) DeleteCA(name string) error {
	delete(d.Certs, name)
	return d.writeCache()
}
