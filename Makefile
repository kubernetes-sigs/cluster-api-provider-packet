.PHONY: deps test manager run install deploy manifests generate fmt vet run

# Image URL to use all building/pushing image targets
IMG ?= packethost/cluster-api-provider-packet:latest

all: test manager clusterctl

# deps
deps:
	dep ensure

# Run tests
test: deps generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: deps generate fmt vet
	go build -o bin/manager github.com/packethost/cluster-api-provider-packet/cmd/manager

# Build clusterctl binary
clusterctl: deps generate fmt vet
	go build -o bin/clusterctl github.com/packethost/cluster-api-provider-packet/cmd/clusterctl

# Run against the configured Kubernetes cluster in ~/.kube/config
run: deps generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cat provider-components.yaml | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	kustomize build config/default/ > provider-components.yaml
	echo "---" >> provider-components.yaml
	kustomize build vendor/sigs.k8s.io/cluster-api/config/default/ >> provider-components.yaml


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
