package machine

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"text/template"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// userdataParams for templates for the bootstrap scripts
type userdataParams struct {
	Machine        *clusterv1.Machine
	Cluster        *clusterv1.Cluster
	Token          string
	MasterEndpoint string
	PodCIDR        string
	ServiceCIDR    string
	CRPackage      string
	CACertificate  string
	CAPrivateKey   string
	Role           string
	Port           int
}

func parseUserdata(userdata, role string, cluster *clusterv1.Cluster, machine *clusterv1.Machine, image, token string, caCertificate, caKey []byte, port int, containerRuntime string) (string, error) {
	params := userdataParams{
		Cluster:       cluster,
		Machine:       machine,
		Token:         token,
		PodCIDR:       subnet(cluster.Spec.ClusterNetwork.Pods),
		ServiceCIDR:   subnet(cluster.Spec.ClusterNetwork.Services),
		CACertificate: base64.StdEncoding.EncodeToString(caCertificate),
		CAPrivateKey:  base64.StdEncoding.EncodeToString(caKey),
		Role:          role,
		Port:          port,
		CRPackage:     containerRuntime,
	}
	vars := masterEnvironmentVariables
	if role == "node" {
		params.MasterEndpoint = endpoint(cluster.Status.APIEndpoints[0])
		vars = nodeEnvironmentVariables
	}

	tmpl := template.Must(template.New(role).Parse(vars))
	b := &bytes.Buffer{}
	err := tmpl.Execute(b, params)
	if err != nil {
		return "", fmt.Errorf("failed to execute %s user-data template: %v", role, err)
	}
	// and now append the standard userdata
	b.Write([]byte(userdata))

	return b.String(), nil
}

// endpoint gets the endpoint as an "ip:port" string
func endpoint(apiEndpoint clusterv1.APIEndpoint) string {
	return fmt.Sprintf("%s:%d", apiEndpoint.Host, apiEndpoint.Port)
}

// subnet gets first IP of the subnet.
func subnet(netRange clusterv1.NetworkRanges) string {
	if len(netRange.CIDRBlocks) == 0 {
		return ""
	}
	return netRange.CIDRBlocks[0]
}

const (
	// masterEnvironmentVariables is the environment variables template for master instances.
	masterEnvironmentVariables = `#!/bin/bash
KUBELET_VERSION={{ .Machine.Spec.Versions.Kubelet }}
TOKEN={{ .Token }}
PORT={{ .Port }}
NAMESPACE={{ .Machine.ObjectMeta.Namespace }}
MACHINE=$NAMESPACE/{{ .Machine.ObjectMeta.Name }}
CONTROL_PLANE_VERSION={{ .Machine.Spec.Versions.ControlPlane }}
CLUSTER_DNS_DOMAIN={{ .Cluster.Spec.ClusterNetwork.ServiceDomain }}
POD_CIDR={{ .PodCIDR }}
SERVICE_CIDR={{ .ServiceCIDR }}
MASTER_CA_CERTIFICATE={{ .CACertificate }}
MASTER_CA_PRIVATE_KEY={{ .CAPrivateKey }}
ROLE={{ .Role }}
CR_PACKAGE={{ .CRPackage }}
`
	// nodeEnvironmentVariables is the environment variables template for worker instances.
	nodeEnvironmentVariables = `#!/bin/bash
KUBELET_VERSION={{ .Machine.Spec.Versions.Kubelet }}
MASTER={{ .MasterEndpoint }}
TOKEN={{ .Token }}
NAMESPACE={{ .Machine.ObjectMeta.Namespace }}
MACHINE=$NAMESPACE/{{ .Machine.ObjectMeta.Name }}
CLUSTER_DNS_DOMAIN={{ .Cluster.Spec.ClusterNetwork.ServiceDomain }}
POD_CIDR={{ .PodCIDR }}
SERVICE_CIDR={{ .ServiceCIDR }}
ROLE={{ .Role }}
CR_PACKAGE={{ .CRPackage }}
`
)
