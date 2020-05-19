.PHONY: vendor test manager clusterctl run install deploy manifests generate fmt vet run kubebuilder ci cd

GIT_VERSION ?= $(shell git log -1 --format="%h")
RELEASE_TAG := $(shell git describe --abbrev=0 --tags ${TAG_COMMIT} 2>/dev/null || true)

ifneq ($(shell git status --porcelain),)
	# next is used by GoReleaser as well when --spanshot is set
  RELEASE_TAG := $(RELEASE_TAG)-next
endif

KUBEBUILDER_VERSION ?= 2.3.1
# default install location for kubebuilder; can be placed elsewhere
KUBEBUILDER_DIR ?= /usr/local/kubebuilder
KUBEBUILDER ?= $(KUBEBUILDER_DIR)/bin/kubebuilder
CONTROLLER_GEN_VERSION ?= v0.3.0
CONTROLLER_GEN=$(GOBIN)/controller-gen

CERTMANAGER_URL ?= https://github.com/jetstack/cert-manager/releases/download/v0.14.1/cert-manager.yaml

REPO_URL ?= https://github.com/packethost/cluster-api-provider-packet

# BUILDARCH is the host architecture
# ARCH is the target architecture
# we need to keep track of them separately
BUILDARCH ?= $(shell uname -m)
BUILDOS ?= $(shell uname -s | tr A-Z a-z)

TEST_E2E_DIR := test/e2e
E2E_FOCUS := "functional tests"

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
IMG ?= packethost/cluster-api-provider-packet:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# where we store downloaded core
COREPATH ?= out/core
CORE_VERSION ?= v0.3.5
CORE_API ?= https://api.github.com/repos/kubernetes-sigs/cluster-api/releases
CORE_URL ?= https://github.com/kubernetes-sigs/cluster-api/releases/download/$(CORE_VERSION)

# useful function
word-dot = $(word $2,$(subst ., ,$1))

VERSION ?= $(RELEASE_TAG)
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
RELEASE_CLUSTER_TEMPLATE := $(RELEASE_DIR)/cluster-template.yaml
FULL_RELEASE_MANIFEST := $(FULL_RELEASE_DIR)/infrastructure-components.yaml
FULL_RELEASE_MANIFEST_URL := $(REPO_URL)/releases/$(RELEASE_VERSION)/infrastructure-components.yaml
FULL_RELEASE_CLUSTERCTLYAML := $(FULL_RELEASE_DIR)/clusterctl.yaml
RELEASE_CLUSTERCTLYAML := $(RELEASE_BASE)/clusterctl-$(RELEASE_VERSION).yaml

# managerless - for running manager locally for testing
MANAGERLESS_VERSION ?= $(RELEASE_VERSION)
MANAGERLESS_BASE := out/managerless/infrastructure-packet
MANAGERLESS_DIR := $(MANAGERLESS_BASE)/$(RELEASE_VERSION)
FULL_MANAGERLESS_DIR := $(realpath .)/$(MANAGERLESS_DIR)
MANAGERLESS_MANIFEST := $(MANAGERLESS_DIR)/infrastructure-components.yaml
MANAGERLESS_METADATA := $(MANAGERLESS_DIR)/metadata.yaml
MANAGERLESS_CLUSTER_TEMPLATE := $(MANAGERLESS_DIR)/cluster-template.yaml
FULL_MANAGERLESS_MANIFEST := $(FULL_MANAGERLESS_DIR)/infrastructure-components.yaml
MANAGERLESS_CLUSTERCTLYAML := $(MANAGERLESS_BASE)/clusterctl-$(MANAGERLESS_VERSION).yaml

# templates
METADATA_TEMPLATE ?= templates/metadata-template.yaml
CLUSTERCTL_TEMPLATE ?= templates/clusterctl-template.yaml
CLUSTER_TEMPLATE ?= templates/cluster-template.yaml


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
	mv /tmp/kubebuilder_$(KUBEBUILDER_VERSION)_$(BUILDOS)_$(BUILDARCH) $(KUBEBUILDER_DIR)

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Run e2e tests
e2e:
	# This is the name used inside the component.yaml for the container that runs the manager
	# The image gets loaded inside kind from ./test/e2e/config/packet-dev.yaml
	$(E2E_FLAGS) $(MAKE) -C test/e2e run

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
# version must be at least the given version
.PHONY: $(CONTROLLER_GEN)
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN):
	scripts/controller-gen.sh $@ $(CONTROLLER_GEN_VERSION)

## generate a cluster using clusterctl and setting defaults
cluster:
	./scripts/generate-cluster.sh

$(RELEASE_DIR) $(RELEASE_BASE):
	mkdir -p $@

$(MANAGERLESS_DIR) $(MANAGERLESS_BASE):
	mkdir -p $@

.PHONY: semver release-clusterctl release-manifests release $(RELEASE_CLUSTERCTLYAML) $(RELEASE_MANIFEST) $(RELEASE_METADATA) $(RELEASE_CLUSTER_TEMPLATE) $(FULL_RELEASE_CLUSTERCTLYAML)

semver:
ifeq (,$(VERSION))
	$(error could not determine version to use from git tag, will not create artifacts)
endif


manifest: semver release-manifests release-clusterctl release-cluster-template

release:
	goreleaser release --rm-dist --snapshot --skip-publish --debug

release/publish:
	goreleaser release --rm-dist

release-manifests: semver $(RELEASE_MANIFEST) $(RELEASE_METADATA) $(RELEASE_CLUSTER_TEMPLATE)
$(RELEASE_MANIFEST): $(RELEASE_DIR) ## Builds the manifests to publish with a release
	kustomize build config/default > $@

$(RELEASE_METADATA): semver $(RELEASE_DIR) $(METADATA_TEMPLATE)
	cat $(METADATA_TEMPLATE) | sed 's/MAJOR/$(VERSION_MAJOR)/g' | sed 's/MINOR/$(VERSION_MINOR)/g' | sed 's/CONTRACT/$(VERSION_CONTRACT)/g' > $@

release-cluster-template: semver $(RELEASE_CLUSTER_TEMPLATE)
$(RELEASE_CLUSTER_TEMPLATE): $(RELEASE_DIR)
	cp $(CLUSTER_TEMPLATE) $@

release-clusterctl: semver $(RELEASE_CLUSTERCTLYAML) $(FULL_RELEASE_CLUSTERCTLYAML)
$(RELEASE_CLUSTERCTLYAML): $(RELEASE_BASE)
	cat $(CLUSTERCTL_TEMPLATE) | sed 's%URL%$(FULL_RELEASE_MANIFEST)%g' > $@

$(FULL_RELEASE_CLUSTERCTLYAML): $(RELEASE_DIR)
	cat $(CLUSTERCTL_TEMPLATE) | sed 's%URL%$(FULL_RELEASE_MANIFEST_URL)%g' > $@

.PHONY: managerless-clusterctl managerless-manifests managerless $(MANAGERLESS_CLUSTERCTLYAML) $(MANAGERLESS_MANIFEST) $(MANAGERLESS_METADATA) $(MANAGERLESS_CLUSTER_TEMPLATE)
managerless: semver managerless-manifests managerless-clusterctl managerless-cluster-template
managerless-manifests: semver $(MANAGERLESS_MANIFEST) $(MANAGERLESS_METADATA)
$(MANAGERLESS_MANIFEST): $(MANAGERLESS_DIR)
	kustomize build config/managerless > $@

$(MANAGERLESS_METADATA): semver $(MANAGERLESS_DIR) $(METADATA_TEMPLATE)
	cat $(METADATA_TEMPLATE) | sed 's/MAJOR/$(VERSION_MAJOR)/g' | sed 's/MINOR/$(VERSION_MINOR)/g' | sed 's/CONTRACT/$(VERSION_CONTRACT)/g' > $@

managerless-cluster-template: semver $(MANAGERLESS_CLUSTER_TEMPLATE)
$(MANAGERLESS_CLUSTER_TEMPLATE): $(MANAGERLESS_DIR)
	cp $(CLUSTER_TEMPLATE) $@

managerless-clusterctl: semver $(MANAGERLESS_CLUSTERCTLYAML)
$(MANAGERLESS_CLUSTERCTLYAML): $(MANAGERLESS_BASE)
	@cat $(CLUSTERCTL_TEMPLATE) | sed 's%URL%$(FULL_MANAGERLESS_MANIFEST)%g' > $@
	@echo "managerless ready, command-line is:"
	@echo "	clusterctl --config=$@ <commands>"

$(COREPATH):
	mkdir -p $@

$(COREPATH)/%:
	curl -s -L -o $@ $(CORE_URL)/$*

core: $(COREPATH)
	# download from core
	@$(eval YAMLS := $(shell curl -s -L $(CORE_API) | jq -r '[.[] | select(.tag_name == "$(CORE_VERSION)").assets[] | select(.name | contains("yaml")) | .name] | join(" ")'))
	@if [ -n "$(YAMLS)" ]; then $(MAKE) $(addprefix $(COREPATH)/,$(YAMLS)); fi

# the standard way to initialize a cluster. If you are using an actually released version,
# then you can just do "clusterctl init --infrastructure=packet" without any of this
cluster-init: managerless release
	clusterctl init
	clusterctl init --config=$(MANAGERLESS_CLUSTERCTLYAML) --infrastructure=packet

# this is just for those who really want to see the manual steps
cluster-init-manual: core managerless release
	kubectl apply --validate=false -f $(CERTMANAGER_URL)
	# because of dependencies, this is allowed to fail once or twice
	kubectl apply -f $(COREPATH) || kubectl apply -f $(COREPATH) || kubectl apply -f $(COREPATH)
	kubectl apply -f $(FULL_MANAGERLESS_MANIFEST)
