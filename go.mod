module sigs.k8s.io/cluster-api-provider-packet

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/charmbracelet/bubbles v0.9.0
	github.com/charmbracelet/bubbletea v0.16.0
	github.com/charmbracelet/lipgloss v0.4.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.3.0 // indirect
	github.com/kube-vip/kube-vip v0.3.8
	github.com/mattn/go-isatty v0.0.13
	github.com/muesli/reflow v0.3.0
	github.com/onsi/gomega v1.16.0
	github.com/packethost/packngo v0.19.1
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/cloud-provider v0.22.2
	k8s.io/component-base v0.22.2
	k8s.io/component-helpers v0.21.4
	k8s.io/klog/v2 v2.10.0
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/cluster-api v1.0.0-rc.0
	sigs.k8s.io/controller-runtime v0.10.1
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/osrg/gobgp v2.0.0+incompatible => github.com/osrg/gobgp v0.0.0-20191101114856-a42a1a5f6bf0
