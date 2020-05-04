.PHONY: vendor test manager clusterctl run install deploy manifests generate fmt vet run kubebuilder ci cd

#KUBEBUILDER_VERSION ?= 2.3.0
# because of a known bug in 2.3.0, notably the one fixed in https://github.com/kubernetes-sigs/kubebuilder/pull/1417
# we will use master until 2.3.1 (or 2.4.0) comes out
KUBEBUILDER_VERSION ?= master
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
MANAGER ?= bin/manager-$(OS)-$(ARCH)
KUBECTL ?= kubectl

GO ?= GO111MODULE=on CGO_ENABLED=0 go


# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# useful function
word-dot = $(word $2,$(subst ., ,$1))

VERSION ?= 0.3.0
VERSION_CONTRACT ?= v1alpha3
VERSION_MAJOR ?= $(call word-dot,$(VERSION),1)
VERSION_MINOR ?= $(call word-dot,$(VERSION),2)

# actual releases
RELEASE_VERSION ?= $(VERSION)
RELEASE_BASE := out/release/infrastructure-packet
RELEASE_DIR := $(RELEASE_BASE)/$(RELEASE_VERSION)
FULL_RELEASE_DIR := $(realpath .)/$(RELEASE_DIR)
RELEASE_MANIFEST := $(RELEASE_DIR)/infrastructure-components.yaml
RELEASE_METADATA := $(RELEASE_DIR)/metadata.yaml
FULL_RELEASE_MANIFEST := $(FULL_RELEASE_DIR)/infrastructure-components.yaml
RELEASE_CLUSTERCTLYAML := $(RELEASE_BASE)/clusterctl-$(RELEASE_VERSION).yaml

# managerless - for running manager locally for testing
MANAGERLESS_VERSION ?= $(RELEASE_VERSION)
MANAGERLESS_BASE := out/managerless/infrastructure-packet
MANAGERLESS_DIR := $(MANAGERLESS_BASE)/$(RELEASE_VERSION)
FULL_MANAGERLESS_DIR := $(realpath .)/$(MANAGERLESS_DIR)
MANAGERLESS_MANIFEST := $(MANAGERLESS_DIR)/infrastructure-components.yaml
MANAGERLESS_METADATA := $(MANAGERLESS_DIR)/metadata.yaml
FULL_MANAGERLESS_MANIFEST := $(FULL_MANAGERLESS_DIR)/infrastructure-components.yaml
MANAGERLESS_CLUSTERCTLYAML := $(MANAGERLESS_BASE)/clusterctl-$(MANAGERLESS_VERSION).yaml

# templates
METADATA_TEMPLATE ?= templates/metadata-template.yaml
CLUSTERCTL_TEMPLATE ?= templates/clusterctl-template.yaml


all: manager

# 2 separate targets: ci-test does everything locally, does not need docker; ci includes ci-test and building the image
ci: test image

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
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: $(MANAGER)
$(MANAGER): generate fmt vet
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -o $@ .

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
image: test
	docker build . -t ${IMG}

# Push the docker image
push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

examples:
	./generate-examples.sh

$(RELEASE_DIR) $(RELEASE_BASE):
	mkdir -p $@

$(MANAGERLESS_DIR) $(MANAGERLESS_BASE):
	mkdir -p $@

.PHONY: release-clusterctl release-manifests release $(RELEASE_CLUSTERCTLYAML) $(RELEASE_MANIFEST)
release: release-manifests release-clusterctl
release-manifests: $(RELEASE_MANIFEST) $(RELEASE_METADATA)
$(RELEASE_MANIFEST): $(RELEASE_DIR) ## Builds the manifests to publish with a release
	kustomize build config/default > $@

$(RELEASE_METADATA): $(RELEASE_DIR) $(METADATA_TEMPLATE)
	cat $(METADATA_TEMPLATE) | sed 's/MAJOR/$(VERSION_MAJOR)/g' | sed 's/MINOR/$(VERSION_MINOR)g' | sed 's/CONTRACT/$(VERSION_CONTRACT)/g' > $@

release-clusterctl: $(RELEASE_CLUSTERCTLYAML)
$(RELEASE_CLUSTERCTLYAML): $(RELEASE_BASE)
	cat $(CLUSTERCTL_TEMPLATE) | sed 's%URL%$(FULL_RELEASE_MANIFEST)%g' > $@

.PHONY: managerless-clusterctl managerless-manifests managerless $(MANAGERLESS_CLUSTERCTLYAML) $(MANAGERLESS_MANIFEST)
managerless: managerless-manifests managerless-clusterctl
managerless-manifests: $(MANAGERLESS_MANIFEST) $(MANAGERLESS_METADATA)
$(MANAGERLESS_MANIFEST): $(MANAGERLESS_DIR)
	kustomize build config/managerless > $@

$(MANAGERLESS_METADATA): $(MANAGERLESS_DIR) $(METADATA_TEMPLATE)
	cat $(METADATA_TEMPLATE) | sed 's/MAJOR/$(VERSION_MAJOR)/g' | sed 's/MINOR/$(VERSION_MINOR)/g' | sed 's/CONTRACT/$(VERSION_CONTRACT)/g' > $@

managerless-clusterctl: $(MANAGERLESS_CLUSTERCTLYAML)
$(MANAGERLESS_CLUSTERCTLYAML): $(MANAGERLESS_BASE)
	@cat $(CLUSTERCTL_TEMPLATE) | sed 's%URL%$(FULL_MANAGERLESS_MANIFEST)%g' > $@
	@echo "managerless ready, command-line is:"
	@echo "	clusterctl --config=$@ <commands>"
