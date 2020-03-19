/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"flag"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/deployer"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/util"
	kclient "k8s.io/client-go/kubernetes"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	var (
		err           error
		kube          *kclient.Clientset
		secretsGetter clientv1.SecretsGetter
	)

	flag.Parse()

	// get a packet client
	client, err := packet.GetClient()
	if err != nil {
		klog.Fatalf("unable to get Packet client: %v", err)
	}

	cfg, _ := config.GetConfig()
	if cfg != nil {
		kube, _ = kclient.NewForConfig(cfg)
	}
	if kube != nil {
		secretsGetter = kube.CoreV1()
	}

	// get a deployer, which is needed at various stages
	deployer, err := deployer.New(deployer.Params{
		Port:          util.ControlPort,
		SecretsGetter: secretsGetter,
		Client:        client,
	})
	if err != nil {
		klog.Fatalf("unable to get deployer: %v", err)
	}

	common.RegisterClusterProvisioner("packet", deployer)
	cmd.Execute()
}
