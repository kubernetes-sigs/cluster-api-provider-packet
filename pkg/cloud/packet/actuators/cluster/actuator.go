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

package cluster

import (
	"fmt"
	"log"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/ca"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/deployer"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

// Add RBAC rules to access cluster-api resources
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;update

// Actuator is responsible for performing cluster reconciliation
type Actuator struct {
	clustersGetter client.ClustersGetter
	deployer       *deployer.Deployer
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	ClustersGetter client.ClustersGetter
	Deployer       *deployer.Deployer
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		clustersGetter: params.ClustersGetter,
		deployer:       params.Deployer,
	}, nil
}

// Reconcile reconciles a cluster and is invoked by the Cluster Controller
func (a *Actuator) Reconcile(cluster *clusterv1.Cluster) error {
	log.Printf("Reconciling cluster %v.", cluster.Name)
	// ensure that we have a CA cert/key and save it
	if _, ok := a.deployer.Certs[cluster.Name]; !ok {
		caCertAndKey, err := ca.GenerateSelfSignedCertAndKey(cluster.Name, "")
		if err != nil {
			return fmt.Errorf("unable to generate CA cert and key: %v", err)
		}
		a.deployer.Certs[cluster.Name] = caCertAndKey
	}
	return nil
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
	log.Printf("Deleting cluster %v.", cluster.Name)
	// remove the CA cert key
	delete(a.deployer.Certs, cluster.Name)
	return nil
}
