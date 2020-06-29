module github.com/packethost/cluster-api-provider-packet

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/google/uuid v1.1.1
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/packethost/packngo v0.2.0
	github.com/pkg/errors v0.9.1
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200229041039-0a110f9eb7ab
	sigs.k8s.io/cluster-api v0.3.5
	sigs.k8s.io/controller-runtime v0.5.2
)

replace github.com/packethost/packngo => github.com/deitch/packngo v0.2.1-0.20200628082620-d644bb21e1f3
