package packet

import (
	"fmt"
)

const (
	MachineUIDTag = "cluster-api-provider-packet:machine-uid"
	clusterIDTag  = "cluster-api-provider-packet:cluster-id"
	AnnotationUID = "cluster.k8s.io/machine-uid"
)

func GenerateMachineTag(ID string) string {
	return fmt.Sprintf("%s:%s", MachineUIDTag, ID)
}
func GenerateClusterTag(ID string) string {
	return fmt.Sprintf("%s:%s", clusterIDTag, ID)
}

// ItemsInList checks if all items are in the list
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
