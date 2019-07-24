.PHONY: vendor test manager clusterctl run install deploy manifests generate fmt vet run kubebuilder ci cd

KUBEBUILDER_VERSION ?= 2.0.0-beta.0
KUBEBUILDER ?= /usr/local/kubebuilder/bin/kubebuilder

GIT_VERSION?=$(shell git log -1 --format="%h")
RELEASE_TAG ?= $(shell git tag --points-at HEAD)

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
BUILD_IMAGE ?= packethost/cluster-api-provider-packet
BUILD_IMAGE_TAG ?= $(BUILD_IMAGE):latest
PUSH_IMAGE_TAG ?= $(BUILD_IMAGE):$(IMAGETAG)
PROVIDERYAML ?= provider-components.yaml.template
CLUSTERCTL ?= bin/clusterctl-$(OS)-$(ARCH)
MANAGER ?= bin/manager-$(OS)-$(ARCH)
KUBECTL ?= kubectl

GO ?= GO111MODULE=on CGO_ENABLED=0 go

all: test manager clusterctl

# vendor
vendor:
	$(GO) mod vendor
	./hack/update-vendor.sh

# 2 separate targets: ci-test does everything locally, does not need docker; ci includes ci-test and building the image
ci-test: fmt vet test
ci: ci-test image

imagetag:
ifndef IMAGETAG
	$(error IMAGETAG is undefined - run using make <target> IMAGETAG=X.Y.Z)
endif

tag-image: imagetag
	docker tag $(BUILD_IMAGE_TAG) $(PUSH_IMAGE_TAG)

confirm:
ifndef CONFIRM
	$(error CONFIRM is undefined - run using make <target> CONFIRM=true)
endif

cd: confirm
	$(MAKE) tag-image push IMAGETAG=$(GIT_VERSION)

# needed kubebuilder for tests
kubebuilder: $(KUBEBUILDER)
$(KUBEBUILDER):
	curl -sL https://go.kubebuilder.io/dl/$(KUBEBUILDER_VERSION)/$(BUILDOS)/$(BUILDARCH) | tar -xz -C /tmp/
	# move to a long-term location and put it on your path
	# (you'll need to set the KUBEBUILDER_ASSETS env var if you put it somewhere else)
	mv /tmp/kubebuilder_$(KUBEBUILDER_VERSION)_$(BUILDOS)_$(BUILDARCH) /usr/local/kubebuilder


# Run tests
test: vendor generate fmt vet manifests $(KUBEBUILDER)
	$(GO) test -mod=vendor ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: $(MANAGER)
$(MANAGER): vendor generate fmt vet
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -mod=vendor -o $@ github.com/packethost/cluster-api-provider-packet/cmd/manager

# Build clusterctl binary
clusterctl: $(CLUSTERCTL)
$(CLUSTERCTL): vendor generate fmt vet
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -mod=vendor -o $@ github.com/packethost/cluster-api-provider-packet/cmd/clusterctl

# Run against the configured Kubernetes cluster in ~/.kube/config
run: vendor generate fmt vet
	$(GO) run -mod=vendor ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests $(CLUSTERCTL)
	generate-yaml.sh
	$(CLUSTERCTL) create cluster --provider packet --bootstrap-type kind -c out/packet/cluster.yaml -m out/packet/machines.yaml -a out/packet/addons.yaml -p out/packet/provider-components.yaml --v=10

# Generate manifests e.g. CRD, RBAC etc.
manifests: $(PROVIDERYAML)
$(PROVIDERYAML):
	# which image do we patch in? BUILD_IMAGE_TAG or PUSH_IMAGE_TAG? Depends on if it is set
ifdef IMAGETAG
	$(eval PATCH_IMAGE_TAG := $(PUSH_IMAGE_TAG))
else
	$(eval PATCH_IMAGE_TAG := $(BUILD_IMAGE_TAG))
endif
	# generate
	$(GO) run -mod=vendor vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	# patch the particular image tag we will want to deploy
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@PATCH_ME_IMAGE@image: '"$(PATCH_IMAGE_TAG)"'@' ./config/default/manager_image_patch.yaml
	# create the manifests
	$(KUBECTL) kustomize vendor/sigs.k8s.io/cluster-api/config/default/ > $(PROVIDERYAML)
	echo "---" >> $(PROVIDERYAML)
	$(KUBECTL) kustomize config/ >> $(PROVIDERYAML)


# Run go fmt against code
fmt:
	$(GO) fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	$(GO) vet -mod=vendor ./pkg/... ./cmd/...

# Generate code
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	$(GO) generate -mod=vendor ./pkg/... ./cmd/...

# Build the docker image
image: docker-build
docker-build:
	docker build -t $(BUILD_IMAGE_TAG) .

# Push the docker image
push:
	docker push $(PUSH_IMAGE_TAG)

image-tag:
	@echo $(PUSH_IMAGE_TAG)
