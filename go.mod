module github.com/packethost/cluster-api-provider-packet

go 1.12

require (
	github.com/appscode/jsonpatch v2.0.0+incompatible // indirect
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20190702054246-869f871628b6 // indirect
	github.com/google/uuid v1.1.1
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/onsi/gomega v1.5.0
	github.com/packethost/packngo v0.0.0-20190507131943-1343be729ca2
	github.com/pkg/errors v0.8.1
	github.com/spf13/cobra v0.0.5 // indirect
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/zap v1.10.0 // indirect
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apimachinery v0.0.0-20190703205208-4cfb76a8bf76
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/cluster-bootstrap v0.0.0-20190223141759-fab9a0a63c55
	k8s.io/code-generator v0.0.0-20190717022600-77f3a1fe56bb
	k8s.io/klog v0.3.2
	sigs.k8s.io/cluster-api v0.1.7
	sigs.k8s.io/controller-runtime v0.1.12
	sigs.k8s.io/controller-tools v0.1.11
	sigs.k8s.io/yaml v1.1.0
)

replace (
	github.com/Sirupsen/logrus v1.0.5 => github.com/sirupsen/logrus v1.0.5
	github.com/Sirupsen/logrus v1.3.0 => github.com/Sirupsen/logrus v1.0.6
	github.com/Sirupsen/logrus v1.4.0 => github.com/sirupsen/logrus v1.0.6
	github.com/Sirupsen/logrus v1.4.2 => github.com/sirupsen/logrus v1.0.6
	github.com/appscode/jsonpatch v2.0.0+incompatible => github.com/gomodules/jsonpatch v0.0.0-20190108182946-7c0e3b262f30
	k8s.io/api => k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
)
