# Copyright 2020 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# If you update this file, please follow
# https://www.thapaliya.com/en/writings/well-documented-makefiles/

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL:=/usr/bin/env bash

.DEFAULT_GOAL:=help

GOPATH  := $(shell go env GOPATH)
GOARCH  := $(shell go env GOARCH)
GOOS    := $(shell go env GOOS)
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

# Default timeout for starting/stopping the Kubebuilder test control plane
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?=60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?=60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled
# curl retries
CURL_RETRIES=3

# Directories.
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/bin)
BIN_DIR := $(abspath $(ROOT_DIR)/bin)
GO_INSTALL = ./scripts/go_install.sh

REPO_ROOT := $(shell git rev-parse --show-toplevel)
# Set --output-base for conversion-gen if we are not within GOPATH
ifneq ($(abspath $(REPO_ROOT)),$(shell go env GOPATH)/src/sigs.k8s.io/cluster-api-provider-packet)
	GEN_OUTPUT_BASE := --output-base=$(REPO_ROOT)
else
	export GOPATH := $(shell go env GOPATH)
endif

# Binaries.
CONTROLLER_GEN_VER := v0.6.2
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER)

CONVERSION_GEN_VER := v0.21.3
CONVERSION_GEN_BIN := conversion-gen
CONVERSION_GEN := $(TOOLS_BIN_DIR)/$(CONVERSION_GEN_BIN)-$(CONVERSION_GEN_VER)

ENVSUBST_VER := v1.2.0
ENVSUBST_BIN := envsubst
ENVSUBST := $(TOOLS_BIN_DIR)/$(ENVSUBST_BIN)

GOLANGCI_LINT_VER := v1.42.1
GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN)-$(GOLANGCI_LINT_VER)

KUSTOMIZE_VER := v3.9.1
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)-$(KUSTOMIZE_VER)

GINKGO_VER := v1.16.4
GINKGO_BIN := ginkgo
GINKGO := $(TOOLS_BIN_DIR)/$(GINKGO_BIN)-$(GINKGO_VER)

KUBECTL_VER := v1.20.4
KUBECTL_BIN := kubectl
KUBECTL := $(TOOLS_BIN_DIR)/$(KUBECTL_BIN)-$(KUBECTL_VER)

KIND_VER := v0.11.1
KIND_BIN := kind
KIND := $(TOOLS_BIN_DIR)/$(KIND_BIN)-$(KIND_VER)

TIMEOUT := $(shell command -v timeout || command -v gtimeout)

# Define Docker related variables. Releases should modify and double check these vars.
REGISTRY ?= ghcr.io
IMAGE_NAME ?= kubernetes-sigs/cluster-api-provider-packet
export CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)
export TAG ?= dev
export ARCH ?= amd64
ALL_ARCH = amd64 arm64

# Allow overriding manifest generation destination directory
MANIFEST_ROOT ?= config
CRD_ROOT ?= $(MANIFEST_ROOT)/crd/bases
WEBHOOK_ROOT ?= $(MANIFEST_ROOT)/webhook
RBAC_ROOT ?= $(MANIFEST_ROOT)/rbac

# Allow overriding the imagePullPolicy
PULL_POLICY ?= Always

# Hosts running SELinux need :z added to volume mounts
SELINUX_ENABLED := $(shell cat /sys/fs/selinux/enforce 2> /dev/null || echo 0)

ifeq ($(SELINUX_ENABLED),1)
  DOCKER_VOL_OPTS?=:z
endif

# Build time versioning details.
LDFLAGS := $(shell hack/version.sh)

GOLANG_VERSION := 1.16.8

## --------------------------------------
## Help
## --------------------------------------

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Testing
## --------------------------------------

.PHONY: test
test: ## Run tests
	source ./scripts/fetch_ext_bins.sh; fetch_tools; setup_envs; go test -v ./... -coverprofile cover.out

# Allow overriding the e2e configurations
GINKGO_NODES ?= 1
GINKGO_NOCOLOR ?= false
GINKGO_FOCUS ?= ""
GINKGO_SKIP ?= ""
GINKGO_FLAKE_ATTEMPTS ?= 2
ARTIFACTS ?= $(ROOT_DIR)/_artifacts
SKIP_CLEANUP ?= false
SKIP_CREATE_MGMT_CLUSTER ?= false
E2E_DIR ?= $(REPO_ROOT)/test/e2e
KUBETEST_CONF_PATH ?= $(abspath $(E2E_DIR)/data/kubetest/conformance.yaml)
E2E_CONF_FILE_SOURCE ?= $(E2E_DIR)/config/packet-ci.yaml
E2E_CONF_FILE ?= $(E2E_DIR)/config/packet-ci-envsubst.yaml
E2E_ARTIFACTS_DIR ?= ./out/e2e
E2E_ARTIFACTS_CONF_FILE_SOURCE ?= $(E2E_DIR)/config/packet-ci-actions.yaml
E2E_ARTIFACTS_CONF_FILE ?= $(E2E_ARTIFACTS_DIR)/config/e2e-config.yaml

$(E2E_CONF_FILE): $(ENVSUBST) $(E2E_CONF_FILE_SOURCE)
	mkdir -p $(shell dirname $(E2E_CONF_FILE))
	$(ENVSUBST) < $(E2E_CONF_FILE_SOURCE) > $(E2E_CONF_FILE)

$(E2E_ARTIFACTS_DIR):
	mkdir -p $(E2E_ARTIFACTS_DIR)/

.PHONY: e2e-artifacts
e2e-artifacts: clean-e2e-artifacts $(E2E_ARTIFACTS_DIR) $(ENVSUBST) $(KUSTOMIZE) $(GINKGO) ## Build e2e test artifacts
	$(MAKE) set-manifest-image MANIFEST_IMG=$(REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(TAG)
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent
	$(MAKE) release-manifests release-metadata e2e-test-templates $(E2E_ARTIFACTS_CONF_FILE) RELEASE_DIR=$(E2E_ARTIFACTS_DIR) TEST_TEMPLATES_TARGET_DIR=$(E2E_ARTIFACTS_DIR)/data E2E_CONF_FILE_SOURCE=$(E2E_ARTIFACTS_CONF_FILE_SOURCE) E2E_CONF_FILE=$(E2E_ARTIFACTS_CONF_FILE)
	cp -r templates/addons $(E2E_ARTIFACTS_DIR)/
	mkdir -p $(E2E_ARTIFACTS_DIR)/data/kubetest
	cp test/e2e/data/kubetest/conformance.yaml $(E2E_ARTIFACTS_DIR)/data/kubetest/
	mkdir -p $(E2E_ARTIFACTS_DIR)/data/shared/v1alpha4
	cp test/e2e/data/shared/v1alpha4/metadata.yaml $(E2E_ARTIFACTS_DIR)/data/shared/v1alpha4/
	cd test/e2e; $(GINKGO) build -tags=e2e ./
	mv test/e2e/e2e.test $(E2E_ARTIFACTS_DIR)/

.PHONY: clean-e2e-artifacts
clean-e2e-artifacts: ## Remove the release folder
	rm -rf $(E2E_ARTIFACTS_DIR)

.PHONY: run-e2e-tests
run-e2e-tests: $(KUBECTL) $(KUSTOMIZE) $(KIND) $(GINKGO) $(E2E_CONF_FILE) e2e-test-templates $(if $(SKIP_IMAGE_BUILD),,e2e-image) ## Run the e2e tests
	cd test/e2e; time $(GINKGO) -v -trace -progress -v -tags=e2e \
		--randomizeAllSpecs -race $(GINKGO_ADDITIONAL_ARGS) \
		-focus=$(GINKGO_FOCUS) -skip=$(GINKGO_SKIP) \
		-nodes=$(GINKGO_NODES) --noColor=$(GINKGO_NOCOLOR) \
		--flakeAttempts=$(GINKGO_FLAKE_ATTEMPTS) ./ -- \
		-e2e.artifacts-folder="$(ARTIFACTS)" \
		-e2e.config="$(E2E_CONF_FILE)" \
		-e2e.skip-resource-cleanup=$(SKIP_CLEANUP) \
		-e2e.use-existing-cluster=$(SKIP_CREATE_MGMT_CLUSTER)

.PHONY: test-e2e-conformance
test-e2e-conformance:
	$(MAKE) run-e2e-tests GINKGO_FOCUS="'\[Conformance\]'"

.PHONY: test-e2e-management-upgrade
test-e2e-management-upgrade:
	$(MAKE) run-e2e-tests GINKGO_FOCUS="'\[Management Upgrade\]'"

.PHONY: test-e2e-workload-upgrade
test-e2e-workload-upgrade:
	$(MAKE) run-e2e-tests GINKGO_FOCUS="'\[Workload Upgrade\]'"

.PHONY: test-e2e-quickstart
test-e2e-quickstart:
	$(MAKE) run-e2e-tests GINKGO_FOCUS="'\[QuickStart\]'"

.PHONY: test-e2e-local
test-e2e-local:
	$(MAKE) run-e2e-tests GINKGO_SKIP="'\[QuickStart\]|\[Conformance\]|\[Needs Published Image\]|\[Management Upgrade\]|\[Workload Upgrade\]'"

.PHONY: test-e2e-ci
test-e2e-ci:
	$(MAKE) run-e2e-tests GINKGO_SKIP="'\[QuickStart\]|\[Conformance\]|\[Management Upgrade\]|\[Workload Upgrade\]'"

## --------------------------------------
## E2E Test Templates
## --------------------------------------

TEST_TEMPLATES_TARGET_DIR ?= $(REPO_ROOT)/test/e2e/data

.PHONY: e2e-test-templates
e2e-test-templates: $(KUSTOMIZE) e2e-test-templates-v1alpha3 e2e-test-templates-v1alpha4 ## Generate cluster templates for all versions

e2e-test-templates-v1alpha3: $(KUSTOMIZE) ## Generate cluster templates for v1alpha3
	mkdir -p $(TEST_TEMPLATES_TARGET_DIR)/v1alpha3/
	$(KUSTOMIZE) build $(REPO_ROOT)/test/e2e/data/v1alpha3/cluster-template-packet-ccm --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha3/cluster-template-packet-ccm.yaml
	$(KUSTOMIZE) build $(REPO_ROOT)/test/e2e/data/v1alpha3/cluster-template-cpem --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha3/cluster-template-cpem.yaml

e2e-test-templates-v1alpha4: $(KUSTOMIZE) ## Generate cluster templates for v1alpha4
	mkdir -p $(TEST_TEMPLATES_TARGET_DIR)/v1alpha4/
	$(KUSTOMIZE) build $(REPO_ROOT)/templates/experimental-crs-cni --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha4/cluster-template.yaml
	$(KUSTOMIZE) build $(REPO_ROOT)/test/e2e/data/v1alpha4/cluster-template-kcp-scale-in --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha4/cluster-template-kcp-scale-in.yaml
	$(KUSTOMIZE) build $(REPO_ROOT)/test/e2e/data/v1alpha4/cluster-template-node-drain --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha4/cluster-template-node-drain.yaml
	$(KUSTOMIZE) build $(REPO_ROOT)/test/e2e/data/v1alpha4/cluster-template-md-remediation --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha4/cluster-template-md-remediation.yaml
	$(KUSTOMIZE) build $(REPO_ROOT)/test/e2e/data/v1alpha4/cluster-template-kcp-remediation --load_restrictor none > $(TEST_TEMPLATES_TARGET_DIR)/v1alpha4/cluster-template-kcp-remediation.yaml

## --------------------------------------
## Tooling Binaries
## --------------------------------------

$(ENVSUBST): ## Build envsubst from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/a8m/envsubst/cmd/envsubst $(ENVSUBST_BIN) $(ENVSUBST_VER)

$(GOLANGCI_LINT): ## Build golangci-lint from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/golangci/golangci-lint/cmd/golangci-lint $(GOLANGCI_LINT_BIN) $(GOLANGCI_LINT_VER)

$(KUSTOMIZE): ## Build kustomize from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) sigs.k8s.io/kustomize/kustomize/v3 $(KUSTOMIZE_BIN) $(KUSTOMIZE_VER)

$(CONTROLLER_GEN): ## Build controller-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) sigs.k8s.io/controller-tools/cmd/controller-gen $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)

$(CONVERSION_GEN): ## Build conversion-gen.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) k8s.io/code-generator/cmd/conversion-gen $(CONVERSION_GEN_BIN) $(CONVERSION_GEN_VER)

$(GINKGO): ## Build ginkgo.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/onsi/ginkgo/ginkgo $(GINKGO_BIN) $(GINKGO_VER)

$(KUBECTL): ## Build kubectl
	mkdir -p $(TOOLS_BIN_DIR)
	rm -f "$(KUBECTL)*"
	curl --retry $(CURL_RETRIES) -fsL https://dl.k8s.io/release/$(KUBECTL_VER)/bin/$(GOOS)/$(GOARCH)/kubectl -o $(KUBECTL)
	ln -sf "$(KUBECTL)" "$(TOOLS_BIN_DIR)/$(KUBECTL_BIN)"
	chmod +x "$(TOOLS_BIN_DIR)/$(KUBECTL_BIN)" "$(KUBECTL)"

$(KIND): ## Build kind
	mkdir -p $(TOOLS_BIN_DIR)
	rm -f "$(KIND)*"
	curl --retry $(CURL_RETRIES) -fsL https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VER}/kind-${GOOS}-${GOARCH} -o ${KIND}
	ln -sf "$(KIND)" "$(TOOLS_BIN_DIR)/$(KIND_BIN)"
	chmod +x "$(TOOLS_BIN_DIR)/$(KIND_BIN)" "$(KIND)"

## --------------------------------------
## Linting
## --------------------------------------

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Lint codebase
	$(GOLANGCI_LINT) run -v --fast=false

## --------------------------------------
## Generate
## --------------------------------------

.PHONY: modules
modules: ## Runs go mod to ensure proper vendoring.
	go mod tidy
	cd test/e2e; go mod tidy

.PHONY: generate
generate: ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests
	$(MAKE) generate-templates

.PHONY: generate-templates
generate-templates: $(KUSTOMIZE) ## Generate cluster templates
	$(KUSTOMIZE) build templates/experimental-crs-cni --load_restrictor none > templates/cluster-template-crs-cni.yaml

.PHONY: generate-go
generate-go: $(CONTROLLER_GEN) $(CONVERSION_GEN) ## Runs Go related generate targets
	$(CONTROLLER_GEN) \
		paths=./api/... \
		object:headerFile=./hack/boilerplate.go.txt
	$(CONVERSION_GEN) \
		--input-dirs=./api/v1alpha3 \
		--build-tag=ignore_autogenerated_core_v1alpha3 \
		--extra-peer-dirs=sigs.k8s.io/cluster-api/api/v1alpha3 \
		--output-file-base=zz_generated.conversion \
		--go-header-file=./hack/boilerplate.go.txt $(GEN_OUTPUT_BASE)
	go generate ./...

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./api/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=$(CRD_ROOT) \
		output:webhook:dir=$(WEBHOOK_ROOT) \
		webhook
	$(CONTROLLER_GEN) \
		paths=./controllers/... \
		output:rbac:dir=$(RBAC_ROOT) \
		rbac:roleName=manager-role

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-build
docker-build: ## Build the docker image for controller-manager
	docker build --pull --build-arg ARCH=$(ARCH) --build-arg LDFLAGS="$(LDFLAGS)" . -t $(CONTROLLER_IMG)-$(ARCH):$(TAG)
	MANIFEST_IMG=$(CONTROLLER_IMG)-$(ARCH) MANIFEST_TAG=$(TAG) $(MAKE) set-manifest-image
	$(MAKE) set-manifest-pull-policy

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push $(CONTROLLER_IMG)-$(ARCH):$(TAG)

.PHONY: e2e-image
e2e-image:
	docker build --build-arg $(GOPROXY) --tag=${REGISTRY}/${IMAGE_NAME}:${TAG} .

## --------------------------------------
## Docker — All ARCH
## --------------------------------------

.PHONY: docker-build-all ## Build all the architecture docker images
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH))

docker-build-%:
	$(MAKE) ARCH=$* docker-build

.PHONY: docker-push-all ## Push all the architecture docker images
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))
	$(MAKE) docker-push-manifest

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push-manifest
docker-push-manifest: ## Push the fat manifest docker image.
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	docker manifest create --amend $(CONTROLLER_IMG):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(CONTROLLER_IMG)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} ${CONTROLLER_IMG}:${TAG} ${CONTROLLER_IMG}-$${arch}:${TAG}; done
	docker manifest push --purge ${CONTROLLER_IMG}:${TAG}
	MANIFEST_IMG=$(CONTROLLER_IMG) MANIFEST_TAG=$(TAG) $(MAKE) set-manifest-image
	$(MAKE) set-manifest-pull-policy

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for default resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' ./config/default/manager_image_patch.yaml

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for default resource)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' ./config/default/manager_pull_policy.yaml

## --------------------------------------
## Release
## --------------------------------------

RELEASE_TAG := $(shell git describe --abbrev=0 2>/dev/null)
RELEASE_DIR ?= out/release

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: release
release: clean-release
	$(MAKE) set-manifest-image MANIFEST_IMG=$(REGISTRY)/$(IMAGE_NAME) MANIFEST_TAG=$(TAG)
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent
	$(MAKE) release-manifests
	$(MAKE) release-metadata
	$(MAKE) release-templates

.PHONY: release-manifests
release-manifests: $(KUSTOMIZE) $(RELEASE_DIR) ## Builds the manifests to publish with a release
	$(KUSTOMIZE) build config/default > $(RELEASE_DIR)/infrastructure-components.yaml

.PHONY: release-metadata
release-metadata: $(RELEASE_DIR)
	cp metadata.yaml $(RELEASE_DIR)/metadata.yaml

# TODO: envsubst to include CNI and/or CPEM resources
.PHONY: release-templates
release-templates: $(RELEASE_DIR)
	cp templates/cluster-template*.yaml $(RELEASE_DIR)/

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

.PHONY: clean
clean: clean-bin clean-temporary clean-release ## Remove all generated files

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf bin
	rm -rf hack/tools/bin

.PHONY: clean-temporary
clean-temporary: ## Remove all temporary files and folders
	rm -f minikube.kubeconfig
	rm -f kubeconfig

.PHONY: clean-release
clean-release: ## Remove the release folder
	rm -rf $(RELEASE_DIR)

.PHONY: verify
verify: verify-boilerplate verify-modules verify-gen

.PHONY: verify-boilerplate
verify-boilerplate:
	./hack/verify-boilerplate.sh

.PHONY: verify-modules
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod hack/tools/go.mod hack/tools/go.sum); then \
		echo "go module files are out of date"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate
	@if !(git diff --quiet HEAD); then \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi
