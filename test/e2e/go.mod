module sigs.k8s.io/cluster-api-provider-packet/test/e2e

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/docker/distribution v2.7.1+incompatible
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/packethost/packngo v0.19.1
	golang.org/x/crypto v0.0.0-20210503195802-e9a32991a82e
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/cluster-api v1.0.0-rc.0
	sigs.k8s.io/cluster-api-provider-packet v0.0.0-00010101000000-000000000000
	sigs.k8s.io/cluster-api/test v1.0.0-rc.0
	sigs.k8s.io/controller-runtime v0.10.1
)

replace (
	github.com/osrg/gobgp v2.0.0+incompatible => github.com/osrg/gobgp v0.0.0-20191101114856-a42a1a5f6bf0
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.0.0-rc.0
	sigs.k8s.io/cluster-api-provider-packet => ../../
)
