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
	"fmt"
	"log"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	"github.com/packethost/packngo"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// Deployer satisfies the ProviderDeployer(https://github.com/kubernetes-sigs/cluster-api/blob/master/cmd/clusterctl/clusterdeployer/clusterdeployer.go) interface.
type Deployer struct {
	client      *packet.PacketClient
	ControlPort int
}

// Params is used to create a new deployer.
type Params struct {
	Client *packet.PacketClient
	Port   int
}

// New returns a new Deployer.
func New(params Params) *Deployer {
	return &Deployer{
		client:      params.Client,
		ControlPort: params.Port,
	}
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
		return "", fmt.Errorf("machine does not exist: %s", machineRef)
	}
	// TODO: validate that this address exists, so we don't hit nil pointer
	// TODO: check which address to return
	// TODO: check address format (cidr, subnet, etc.)
	return device.Network[0].Address, nil
}

// GetKubeConfig gets a kubeconfig from the master.
func (d *Deployer) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cannot get kubeconfig for nil cluster")
	}
	if master == nil {
		return "", fmt.Errorf("cannot get kubeconfig for nil master")
	}
	log.Printf("Getting IP of machine %v for cluster %v.", master.Name, cluster.Name)
	return "", fmt.Errorf("TODO: Not yet implemented")
}

func (d *Deployer) CoreV1Client(cluster *clusterv1.Cluster) (corev1.CoreV1Interface, error) {
	controlPlaneDNSName, err := d.GetIP(cluster, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve controlplane (GetIP): %+v", err)
	}

	controlPlaneURL := fmt.Sprintf("https://%s:%d", controlPlaneDNSName, d.ControlPort)

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
