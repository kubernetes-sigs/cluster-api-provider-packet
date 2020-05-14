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

## Current situation

At the moment we only have the environment setup done. The cluster-api team
provides a set of written tests that we should import and run to verify our
implementation. But all of them assume support for `KubeadmControlPlane` and
this is something we do not support yet.

## Requirements

* Go
* Kind
* Docker
* [ginkgo](https://onsi.github.io/ginkgo/)

## Run e2e tests

Currently we decided to run those tests manually via:

```
make e2e
```
