.PHONY: deps test manager clusterctl run install deploy manifests generate fmt vet run

# BUILDARCH is the host architecture
# ARCH is the target architecture
# we need to keep track of them separately
BUILDARCH ?= $(shell uname -m)
BUILDOS ?= $(shell uname -s | tr A-Z a-z)

# canonicalized names for host architecture
ifeq ($(BUILDARCH),aarch64)
        BUILDARCH=arm64
endif
ifeq ($(BUILDARCH),x86_64)
        BUILDARCH=amd64
endif

# unless otherwise set, I am building for my own architecture, i.e. not cross-compiling
ARCH ?= $(BUILDARCH)

# canonicalized names for target architecture
ifeq ($(ARCH),aarch64)
        override ARCH=arm64
endif
ifeq ($(ARCH),x86_64)
    override ARCH=amd64
endif

# unless otherwise set, I am building for my own OS, i.e. not cross-compiling
OS ?= $(BUILDOS)


# Image URL to use all building/pushing image targets
IMG ?= packethost/cluster-api-provider-packet:latest
PROVIDERYAML ?= provider-components.yaml.template
CLUSTERCTL ?= bin/clusterctl-$(OS)-$(ARCH)
MANAGER ?= bin/manager-$(OS)-$(ARCH)
KUBECTL ?= kubectl

all: test manager clusterctl

# deps
deps:
	dep ensure

# Run tests
test: deps generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: $(MANAGER)
$(MANAGER): deps generate fmt vet
	GOOS=$(OS) GOARCH=$(ARCH) go build -o $@ github.com/packethost/cluster-api-provider-packet/cmd/manager

# Build clusterctl binary
clusterctl: $(CLUSTERCTL)
$(CLUSTERCTL): deps generate fmt vet
	GOOS=$(OS) GOARCH=$(ARCH) go build -o $@ github.com/packethost/cluster-api-provider-packet/cmd/clusterctl

# Run against the configured Kubernetes cluster in ~/.kube/config
run: deps generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	generate-yaml.sh
	cat out/packet/provider-components.yaml | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: $(PROVIDERYAML)
$(PROVIDERYAML):
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	$(KUBECTL) kustomize vendor/sigs.k8s.io/cluster-api/config/default/ > $(PROVIDERYAML)
	echo "---" >> $(PROVIDERYAML)
	$(KUBECTL) kustomize config/ >> $(PROVIDERYAML)


# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build: test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

image-name:
	@echo ${IMG}
