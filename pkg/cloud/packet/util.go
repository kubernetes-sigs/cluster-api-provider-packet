/*
Copyright 2020 The Kubernetes Authors.

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

package packet

import (
	"fmt"
)

const (
	machineUIDTag = "capp:machine-uid"
	clusterIDTag  = "capp:cluster-id"
	namespaceTag  = "capp:namespace"
)

// GenerateMachineNameTag generates a tag for a machine.
func GenerateMachineNameTag(name string) string {
	return fmt.Sprintf("%s:%s", machineUIDTag, name)
}

// GenerateClusterTag generates a tag for a cluster.
func GenerateClusterTag(clusterName string) string {
	return fmt.Sprintf("%s:%s", clusterIDTag, clusterName)
}

// GenerateNamespaceTag generates a tag for a namespace.
func GenerateNamespaceTag(namespace string) string {
	return fmt.Sprintf("%s:%s", namespaceTag, namespace)
}

// ItemsInList checks if all items are in the list.
func ItemsInList(list []string, items []string) bool {
	// convert the items against which we are mapping into a map
	itemMap := map[string]bool{}
	for _, elm := range items {
		itemMap[elm] = false
	}
	// every one that is matched goes from false to true in the map
	for _, elm := range list {
		if _, ok := itemMap[elm]; ok {
			itemMap[elm] = true
		}
	}
	// go through the map; if any is false, return false, else all matched so return true
	for _, v := range itemMap {
		if !v {
			return false
		}
	}
	return true
}

// DefaultCreateTags returns the default tags for an Equinix Metal resource.
func DefaultCreateTags(namespace, name, clusterName string) []string {
	return []string{
		GenerateClusterTag(clusterName),
		GenerateMachineNameTag(name),
		GenerateNamespaceTag(namespace),
	}
}

func IPAddressClaimName(machineName string, portIndex, networkIndex int) string {
	return fmt.Sprintf("%s-port-%d-network-%d", machineName, portIndex, networkIndex)
}
