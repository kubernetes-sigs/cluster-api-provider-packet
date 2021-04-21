module sigs.k8s.io/cluster-api-provider-packet

go 1.13

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.2.0
	github.com/onsi/ginkgo v1.16.1
	github.com/onsi/gomega v1.11.0
	github.com/packethost/packngo v0.13.0
	github.com/pkg/errors v0.9.1
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog/v2 v2.4.0
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/cluster-api v0.3.16
	sigs.k8s.io/controller-runtime v0.8.3
)
