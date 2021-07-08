module sigs.k8s.io/cluster-api-provider-packet

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.3.0 // indirect
	github.com/onsi/gomega v1.16.0
	github.com/packethost/packngo v0.19.1
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20210503195802-e9a32991a82e // indirect
	golang.org/x/net v0.0.0-20210505024714-0287a6fb4125 // indirect
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56 // indirect
	k8s.io/api v0.21.5
	k8s.io/apiextensions-apiserver v0.21.5 // indirect
	k8s.io/apimachinery v0.21.5
	k8s.io/client-go v0.21.5
	k8s.io/component-base v0.21.5
	k8s.io/klog/v2 v2.10.0
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	sigs.k8s.io/cluster-api v0.4.3
	sigs.k8s.io/controller-runtime v0.9.7
)

replace github.com/osrg/gobgp v2.0.0+incompatible => github.com/osrg/gobgp v0.0.0-20191101114856-a42a1a5f6bf0
