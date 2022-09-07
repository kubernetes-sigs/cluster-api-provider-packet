# E2E

The e2e tests assumes that the stack is as close as possible with the one end
user will use in a real environment.

The cluster-api community serves a [framework](https://github.com/kubernetes-sigs/cluster-api/tree/master/test/framework) with utility functions to spin
up a ready to use KIND cluster with all the required dependencies like:

* Cluster API Components
* Cert Manager
* Kubeadm Bootstrap provider
* And so on.

The reference implementation we have comes from the cluster-api repository
itself [github.com/kubernetes-sigs/cluster-api/test/e2e](https://github.com/kubernetes-sigs/cluster-api/tree/master/test/e2e)

You can see our setup in [./test/e2e/suite_test.go](../../test/e2e/suite_test.go).

## Requirements

* Go
* Kind
* Docker
* [ginkgo](https://onsi.github.io/ginkgo/)

## Run e2e tests

### Running e2e smoke tests

These are quick running tests that are intended to provide a quick signal for PR blocking tests:

```sh
./scripts/ci-e2e-capi-smoketest.sh
```

### Running e2e conformance tests

These are tests that are intended to validate the conformance of clusters deployed by CAPP

```sh
./scripts/ci-e2e-capi-conformance.sh
```

### Running the rest of the e2e tests

The rest of the e2e tests (that do not fall under smoketest or conformance tests), can be run:

```sh
./scripts/ci-e2e-capi.sh
```

By default this will only run tests that do not require a published container image for CAPP.

To run the whole suite of tests, you will need to build and publish a container image, first:

```sh
export REGISTRY=<my_registry_host>
export IMAGE_NAME=<my_image_name>
export TAG=<my_tag>
export SKIP_IMAGE_BUILD=1
make docker-build-all docker-push-all
./scripts/ci-e2e-capi.sh
```
