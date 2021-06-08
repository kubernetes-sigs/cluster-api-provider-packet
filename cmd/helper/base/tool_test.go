/*
Copyright 2021 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package base_test

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base/testutils"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// TODO: unstructured namespaced lifecycle
// TODO: unstructured non-namespaced lifecycle
// TODO: are test cases needed for gvk failure and getWorkloadClient failure???
// TODO: tests with targetnamespace/watchingnamespace
// TODO: maybe tests for setting kubeconfig/context, but will require a live client instead of fake client
// TODO: tests for WorkloadList
// TODO: tests for HasError/GetErrorFor/AddErrorFor
// TODO: tests for GetOutputFor/GetBufferFor/flushing of buffers to output

func TestTool_TestGetClustersNone(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	fakeEnv := testutils.NewFakeEnv(ctx, t, nil)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	tool := &base.Tool{}
	tool.Configure(toolConfig)

	res, err := tool.GetClusters(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(BeEmpty())
}

func TestTool_TestGetClustersAll(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	// generate a random number of namespaces between 3 and 10
	numNamespaces := 3 + rand.Intn(8) //nolint:gosec

	var testClusters []runtime.Object

	for i := 0; i < numNamespaces; i++ {
		namespace := util.RandomString(6)

		// generate a random number of clusters between 1 and 10
		for j := 0; j < rand.Intn(10)+1; j++ { //nolint:gosec
			testClusters = append(testClusters, testutils.GenerateCluster(namespace, ""))
		}
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, nil, testClusters...)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	tool := &base.Tool{}
	tool.Configure(toolConfig)

	allClusters, err := tool.GetClusters(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(allClusters).To(HaveLen(len(testClusters)))
}

func TestTool_TestGetClustersFiltered(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	// generate a random number of namespaces between 3 and 10
	numNamespaces := 3 + rand.Intn(8) //nolint:gosec
	namespaces := make([]string, 0, numNamespaces)

	var testClusters []runtime.Object

	for i := 0; i < numNamespaces; i++ {
		namespace := util.RandomString(6)
		namespaces = append(namespaces, namespace)

		// generate a random number of clusters between 1 and 10
		for j := 0; j < rand.Intn(10)+1; j++ { //nolint:gosec
			testClusters = append(testClusters, testutils.GenerateCluster(namespace, ""))
		}
	}

	namespaceToFilterOn := namespaces[rand.Intn(len(namespaces))] //nolint:gosec

	var expectedClusterNames []string

	for _, c := range testClusters {
		cluster, _ := c.(controllerutil.Object)
		if cluster.GetNamespace() == namespaceToFilterOn {
			expectedClusterNames = append(expectedClusterNames, cluster.GetName())
		}
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, nil, testClusters...)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		WatchingNamespace:    namespaceToFilterOn,
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	tool := &base.Tool{}
	tool.Configure(toolConfig)

	filteredClusters, err := tool.GetClusters(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(filteredClusters).To(HaveLen(len(expectedClusterNames)))

	for _, c := range filteredClusters {
		g.Expect(c.Namespace).To(BeEquivalentTo(namespaceToFilterOn))
		g.Expect(c.Name).To(BeElementOf(expectedClusterNames))
	}
}

func TestTool_ManagementGet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	testNamespace := fmt.Sprintf("test-%s", util.RandomString(6))
	typedNamespacedResourceName := fmt.Sprintf("test-deployment-%s", util.RandomString(6))
	typedNamespacedResource := testutils.GenerateDeployment(testNamespace, typedNamespacedResourceName, "test")

	typedResourceName := fmt.Sprintf("test-node-%s", util.RandomString(6))
	typedResource := testutils.GenerateNode(typedResourceName, "")

	managementResources := []runtime.Object{
		typedNamespacedResource,
		typedResource,
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, nil, managementResources...)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	tool := &base.Tool{}
	tool.Configure(toolConfig)

	typedNamespacedResourceKey, err := client.ObjectKeyFromObject(typedNamespacedResource)
	g.Expect(err).NotTo(HaveOccurred())

	// Ensure that the resource is returned
	typedNamespacedRes := new(appsv1.Deployment)
	g.Expect(tool.ManagementGet(ctx, typedNamespacedResourceKey, typedNamespacedRes)).To(Succeed())
	g.Expect(typedNamespacedRes).To(testutils.BeDerivativeOf(typedNamespacedResource))

	typedResourceKey, err := client.ObjectKeyFromObject(typedResource)
	g.Expect(err).NotTo(HaveOccurred())

	// Ensure that the resource is returned
	typedRes := new(corev1.Node)
	g.Expect(tool.ManagementGet(ctx, typedResourceKey, typedRes)).To(Succeed())
	g.Expect(typedRes).To(testutils.BeDerivativeOf(typedResource))
}

func TestTool_TestTypedNamespacedWorkloadLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.TODO()
	testNamespace := fmt.Sprintf("test-%s", util.RandomString(6))
	testName := fmt.Sprintf("test-secret-%s", util.RandomString(6))
	initial := &corev1.Secret{ // nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ // nolint:exhaustivestruct
			Namespace: testNamespace,
			Name:      testName,
		},
		Data: map[string][]byte{
			"color": []byte("yellow"),
			"shape": []byte("square"),
		},
	}
	patchInput := initial.DeepCopy()
	patchInput.Data["size"] = []byte("large")

	testLifecycle(ctx, t, initial, patchInput)
}

func TestTool_TestTypedNonNamespacedWorkloadLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.TODO()
	testName := fmt.Sprintf("test-node-%s", util.RandomString(6))
	initial := &corev1.Node{ // nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ // nolint:exhaustivestruct
			Name: testName,
		},
		Spec: corev1.NodeSpec{}, // nolint:exhaustivestruct
	}
	patchInput := initial.DeepCopy()
	patchInput.Spec.Unschedulable = true

	testLifecycle(ctx, t, initial, patchInput)
	testLifecycleDry(ctx, t, initial, patchInput)
}

func testLifecycle(ctx context.Context, t *testing.T, initial, patchInput controllerutil.Object) {
	g := NewWithT(t)
	resourceType := reflect.TypeOf(initial).Elem()
	cluster := testutils.GenerateCluster("", "")
	resourceKey, err := client.ObjectKeyFromObject(initial)
	g.Expect(err).NotTo(HaveOccurred())

	testEnv := testutils.NewTestEnv(ctx, t, nil, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
	}
	tool := &base.Tool{}
	tool.Configure(toolConfig)

	// Ensure that the resource doesn't already exist
	g.Expect(tool.WorkloadGet(
		ctx,
		cluster,
		resourceKey,
		reflect.New(resourceType).Interface().(controllerutil.Object),
	)).To(MatchError(ContainSubstring("not found")))

	preCreate, _ := initial.DeepCopyObject().(controllerutil.Object)
	preCreateOutput := tool.GetOutputFor(cluster)

	// verify deletion of non-existing resource acts as expected
	g.Expect(tool.WorkloadDelete(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("not found")))

	// verify patch of non-existing resource acts as expected
	g.Expect(tool.WorkloadPatch(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object), client.Merge)).
		To(MatchError(ContainSubstring("not found")))

	// verify real create
	postCreate, _ := preCreate.DeepCopyObject().(controllerutil.Object)
	g.Expect(tool.WorkloadCreate(ctx, cluster, postCreate)).To(Succeed())
	g.Expect(postCreate).To(testutils.BeDerivativeOf(preCreate))

	postCreateOutput := tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postCreateOutput, preCreateOutput))

	// Ensure that the resource now exists
	actualPostCreate, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(tool.WorkloadGet(ctx, cluster, resourceKey, actualPostCreate)).To(Succeed())
	g.Expect(actualPostCreate).To(testutils.BeDerivativeOf(preCreate))

	// verify create of an already existing resource fails
	g.Expect(tool.WorkloadCreate(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("already exists")))

	// verify real patch
	postPatch, _ := patchInput.DeepCopyObject().(controllerutil.Object)
	g.Expect(tool.WorkloadPatch(ctx, cluster, postPatch, client.Merge)).To(Succeed())
	g.Expect(postPatch).To(testutils.BeDerivativeOf(patchInput))

	postPatchOutput := tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postPatchOutput, postCreateOutput))

	// ensure that the resource is the same as when we started
	actualPostPatch, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(tool.WorkloadGet(ctx, cluster, resourceKey, actualPostPatch)).To(Succeed())
	g.Expect(actualPostPatch).To(testutils.BeDerivativeOf(patchInput))

	preDelete, _ := postPatch.DeepCopyObject().(controllerutil.Object)
	preDelete.SetCreationTimestamp(metav1.NewTime(time.Time{}))

	// verify real delete
	g.Expect(tool.WorkloadDelete(ctx, cluster, preDelete.DeepCopyObject().(controllerutil.Object))).To(Succeed())

	postDeleteOutput := tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postDeleteOutput, postPatchOutput))

	// ensure that the resource no longer exists
	g.Expect(tool.WorkloadGet(
		ctx,
		cluster,
		resourceKey,
		reflect.New(resourceType).Interface().(controllerutil.Object),
	)).To(MatchError(ContainSubstring("not found")))
}

func testLifecycleDry(ctx context.Context, t *testing.T, initial, patchInput controllerutil.Object) {
	g := NewWithT(t)
	resourceType := reflect.TypeOf(initial).Elem()
	clusterWith := testutils.GenerateCluster("", "with")
	clusterWithKey, err := client.ObjectKeyFromObject(clusterWith)
	g.Expect(err).NotTo(HaveOccurred())

	clusterWithout := testutils.GenerateCluster("", "without")
	resourceKey, err := client.ObjectKeyFromObject(initial)
	g.Expect(err).NotTo(HaveOccurred())

	workloadResources := map[client.ObjectKey][]runtime.Object{
		clusterWithKey: {initial},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources, clusterWith, clusterWithout)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
	}
	tool := &base.Tool{}
	tool.Configure(toolConfig)

	// verify dry-run deletion of non-existing resource acts as expected
	g.Expect(tool.WorkloadDelete(ctx, clusterWithout, initial.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("not found")))

	// verify dry-run patch of non-existing resource acts as expected
	g.Expect(tool.WorkloadPatch(ctx, clusterWithout, initial.DeepCopyObject().(controllerutil.Object), client.Merge)).
		To(MatchError(ContainSubstring("not found")))

	// verify dry-run create
	preCreateOutput := tool.GetOutputFor(clusterWithout)
	postDryCreate, _ := initial.DeepCopyObject().(controllerutil.Object)
	g.Expect(tool.WorkloadCreate(ctx, clusterWithout, postDryCreate)).To(Succeed())
	postCreateOutput := tool.GetOutputFor(clusterWithout)

	g.Expect(postDryCreate).To(testutils.BeDerivativeOf(initial))
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postCreateOutput, preCreateOutput))

	// Ensure that the resource still doesn't exist
	g.Expect(tool.WorkloadGet(
		ctx,
		clusterWithout,
		resourceKey,
		reflect.New(resourceType).Interface().(controllerutil.Object),
	)).To(MatchError(ContainSubstring("not found")))

	// verify dry run create of an already existing resource fails
	g.Expect(tool.WorkloadCreate(ctx, clusterWith, initial.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("already exists")))

	// verify dry-run patch
	toolConfig.DryRun = true
	preDryPatchOutput := tool.GetOutputFor(clusterWith)
	postDryPatch, _ := patchInput.DeepCopyObject().(controllerutil.Object)
	g.Expect(tool.WorkloadPatch(ctx, clusterWith, postDryPatch, client.Merge)).To(Succeed())
	postDryPatchOutput := tool.GetOutputFor(clusterWith)

	g.Expect(postDryPatch).To(testutils.BeDerivativeOf(patchInput))
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryPatchOutput, preDryPatchOutput))

	// ensure that the resource is the same as when we started
	actualPostDryPatch, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(tool.WorkloadGet(ctx, clusterWith, resourceKey, actualPostDryPatch)).To(Succeed())
	g.Expect(actualPostDryPatch).To(testutils.BeDerivativeOf(initial))

	// verify dry-run delete
	preDryDeleteOutput := tool.GetOutputFor(clusterWith)
	g.Expect(tool.WorkloadDelete(ctx, clusterWith, initial.DeepCopyObject().(controllerutil.Object))).To(Succeed())
	postDryDeleteOutput := tool.GetOutputFor(clusterWith)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryDeleteOutput, preDryDeleteOutput))

	// ensure that the resource is the same as when we started
	actualPostDryDelete, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(tool.WorkloadGet(ctx, clusterWith, resourceKey, actualPostDryDelete)).To(Succeed())
	g.Expect(actualPostDryDelete).To(testutils.BeDerivativeOf(initial))
}
