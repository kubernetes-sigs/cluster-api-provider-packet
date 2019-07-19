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
	"reflect"
	"time"

	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/ca"
	"github.com/packethost/cluster-api-provider-packet/pkg/cloud/packet/deployer"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	controllerError "sigs.k8s.io/cluster-api/pkg/controller/error"
)

// if the control plane is not ready, wait 15 seconds and try again
const waitForControlPlaneMachineDuration = 15 * time.Second

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
	// save the original status
	clusterCopy := cluster.DeepCopy()
	// ensure that we have a CA cert/key and save it
	if cert, _ := a.deployer.GetCA(cluster.Name); cert == nil {
		caCertAndKey, err := ca.GenerateCACertAndKey(cluster.Name, "")
		if err != nil {
			return fmt.Errorf("unable to generate CA cert and key: %v", err)
		}
		err = a.deployer.PutCA(cluster.Name, caCertAndKey)
		if err != nil {
			return fmt.Errorf("unable to save CA cert and key: %v", err)
		}
	}
	// ensure that we save the correct IP address for the cluster
	address, err := a.deployer.GetIP(cluster, nil)
	_, isNoMachine := err.(*deployer.MachineNotFound)
	_, isNoIP := err.(*deployer.MachineNoIP)
	switch {
	case err != nil && isNoMachine:
		return &controllerError.RequeueAfterError{RequeueAfter: waitForControlPlaneMachineDuration}
	case err != nil && isNoIP:
		return &controllerError.RequeueAfterError{RequeueAfter: waitForControlPlaneMachineDuration}
	case err != nil:
		return err
	case err == nil:
		cluster.Status.APIEndpoints = []clusterv1.APIEndpoint{
			{
				Host: address,
				Port: a.deployer.ControlPort,
			},
		}
	}

	var clusterClient client.ClusterInterface
	if a.clustersGetter != nil {
		clusterClient = a.clustersGetter.Clusters(cluster.Namespace)
	}
	if !reflect.DeepEqual(cluster.Status, clusterCopy.Status) {
		log.Printf("saving updated cluster status %s", cluster.Name)
		if _, err := clusterClient.UpdateStatus(cluster); err != nil {
			msg := fmt.Sprintf("failed to save updated cluster status %s: %v", cluster.Name, err)
			log.Printf(msg)
			return fmt.Errorf(msg)
		}
	}

	return nil
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
	log.Printf("Deleting cluster %v.", cluster.Name)
	// remove the CA cert key
	a.deployer.DeleteCA(cluster.Name)
	return nil
}
