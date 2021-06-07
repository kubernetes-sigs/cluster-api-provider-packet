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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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

	mgmtEnv := testutils.NewMgmtEnv(ctx, t, new(base.ToolConfig))

	res, err := mgmtEnv.Tool.GetClusters(ctx)
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

	mgmtEnv := testutils.NewMgmtEnv(ctx, t, new(base.ToolConfig), testClusters...)
	allClusters, err := mgmtEnv.Tool.GetClusters(ctx)
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

	mgmtEnv := testutils.NewMgmtEnv(
		ctx,
		t,
		&base.ToolConfig{WatchingNamespace: namespaceToFilterOn}, //nolint:exhaustivestruct
		testClusters...,
	)
	filteredClusters, err := mgmtEnv.Tool.GetClusters(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(filteredClusters).To(HaveLen(len(expectedClusterNames)))

	for _, c := range filteredClusters {
		g.Expect(c.Namespace).To(BeEquivalentTo(namespaceToFilterOn))
		g.Expect(c.Name).To(BeElementOf(expectedClusterNames))
	}
}

func TestTool_TestTypedNamespacedManagementGet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	testNamespace := fmt.Sprintf("test-%s", util.RandomString(6))
	testName := fmt.Sprintf("test-deployment-%s", util.RandomString(6))
	testLabels := map[string]string{"app": testName}

	initial := &appsv1.Deployment{ // nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ // nolint:exhaustivestruct
			Namespace: testNamespace,
			Name:      testName,
		},
		Spec: appsv1.DeploymentSpec{ // nolint:exhaustivestruct
			Selector: &metav1.LabelSelector{ // nolint:exhaustivestruct
				MatchLabels: testLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{ // nolint:exhaustivestruct
					Labels: testLabels,
				},
				Spec: corev1.PodSpec{ // nolint:exhaustivestruct
					Containers: []corev1.Container{
						{
							Name:  testName,
							Image: testName,
						},
					},
				},
			},
		},
	}

	mgmtEnv := testutils.NewMgmtEnv(ctx, t, new(base.ToolConfig))
	resourceKey, err := client.ObjectKeyFromObject(initial)
	g.Expect(err).NotTo(HaveOccurred())

	// Ensure that the resource doesn't already exist
	g.Expect(mgmtEnv.Tool.ManagementGet(
		ctx,
		resourceKey,
		new(appsv1.Deployment),
	)).To(MatchError(ContainSubstring("not found")))

	// Create the resource manually since we don't expose a wrapper for create
	mgmtClient, err := mgmtEnv.Tool.ManagementClient()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(mgmtClient.Create(ctx, initial.DeepCopy())).To(Succeed())

	// Ensure that the resource is returned
	res := new(appsv1.Deployment)
	g.Expect(mgmtEnv.Tool.ManagementGet(ctx, resourceKey, res)).To(Succeed())
	g.Expect(res).To(testutils.BeDerivativeOf(initial))
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
}

func testLifecycle(ctx context.Context, t *testing.T, initial, patchInput controllerutil.Object) {
	g := NewWithT(t)
	resourceType := reflect.TypeOf(initial).Elem()
	mgmtEnv := testutils.NewMgmtEnv(ctx, t, new(base.ToolConfig))
	cluster := &clusterv1.Cluster{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: "test",
			Name:      "test-cluster",
		},
	}
	resourceKey, err := client.ObjectKeyFromObject(initial)
	g.Expect(err).NotTo(HaveOccurred())

	// Ensure that the resource doesn't already exist
	g.Expect(mgmtEnv.Tool.WorkloadGet(
		ctx,
		cluster,
		resourceKey,
		reflect.New(resourceType).Interface().(controllerutil.Object),
	)).To(MatchError(ContainSubstring("not found")))

	preCreate, _ := initial.DeepCopyObject().(controllerutil.Object)
	preCreateOutput := mgmtEnv.Tool.GetOutputFor(cluster)

	// verify dry-run deletion of non-existing resource acts as expected
	mgmtEnv.ToolConfig.DryRun = true
	g.Expect(mgmtEnv.Tool.WorkloadDelete(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("not found")))

	// verify deletion of non-existing resource acts as expected
	mgmtEnv.ToolConfig.DryRun = false
	g.Expect(mgmtEnv.Tool.WorkloadDelete(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("not found")))

	// verify dry-run patch of non-existing resource acts as expected
	mgmtEnv.ToolConfig.DryRun = true
	g.Expect(mgmtEnv.Tool.WorkloadPatch(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object), client.Merge)).
		To(MatchError(ContainSubstring("not found")))

	// verify patch of non-existing resource acts as expected
	mgmtEnv.ToolConfig.DryRun = false
	g.Expect(mgmtEnv.Tool.WorkloadPatch(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object), client.Merge)).
		To(MatchError(ContainSubstring("not found")))

	// verify dry-run create
	mgmtEnv.ToolConfig.DryRun = true
	postDryCreate, _ := preCreate.DeepCopyObject().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadCreate(ctx, cluster, postDryCreate)).To(Succeed())
	g.Expect(postDryCreate).To(testutils.BeDerivativeOf(preCreate))

	postDryCreateOutput := mgmtEnv.Tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryCreateOutput, preCreateOutput))

	// Ensure that the resource still doesn't exist
	g.Expect(mgmtEnv.Tool.WorkloadGet(
		ctx,
		cluster,
		resourceKey,
		reflect.New(resourceType).Interface().(controllerutil.Object),
	)).To(MatchError(ContainSubstring("not found")))

	// verify real create
	mgmtEnv.ToolConfig.DryRun = false
	postCreate, _ := preCreate.DeepCopyObject().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadCreate(ctx, cluster, postCreate)).To(Succeed())
	g.Expect(postCreate).To(testutils.BeDerivativeOf(preCreate))

	postCreateOutput := mgmtEnv.Tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postCreateOutput, postDryCreateOutput))

	// Ensure that the resource now exists
	actualPostCreate, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadGet(ctx, cluster, resourceKey, actualPostCreate)).To(Succeed())
	g.Expect(actualPostCreate).To(testutils.BeDerivativeOf(preCreate))

	// verify dry run create of an already existing resource fails
	mgmtEnv.ToolConfig.DryRun = true
	g.Expect(mgmtEnv.Tool.WorkloadCreate(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("already exists")))

	// verify create of an already existing resource fails
	mgmtEnv.ToolConfig.DryRun = false
	g.Expect(mgmtEnv.Tool.WorkloadCreate(ctx, cluster, preCreate.DeepCopyObject().(controllerutil.Object))).
		To(MatchError(ContainSubstring("already exists")))

	prePatch, _ := preCreate.DeepCopyObject().(controllerutil.Object)

	// verify dry-run patch
	mgmtEnv.ToolConfig.DryRun = true
	postDryPatch, _ := patchInput.DeepCopyObject().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadPatch(ctx, cluster, postDryPatch, client.Merge)).To(Succeed())
	g.Expect(postDryPatch).To(testutils.BeDerivativeOf(patchInput))

	postDryPatchOutput := mgmtEnv.Tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryPatchOutput, postCreateOutput))

	// ensure that the resource is the same as when we started
	actualPostDryPatch, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadGet(ctx, cluster, resourceKey, actualPostDryPatch)).To(Succeed())
	g.Expect(actualPostDryPatch).To(testutils.BeDerivativeOf(prePatch))

	// verify real patch
	mgmtEnv.ToolConfig.DryRun = false
	postPatch, _ := patchInput.DeepCopyObject().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadPatch(ctx, cluster, postPatch, client.Merge)).To(Succeed())
	g.Expect(postPatch).To(testutils.BeDerivativeOf(patchInput))

	postPatchOutput := mgmtEnv.Tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postPatchOutput, postDryPatchOutput))

	// ensure that the resource is the same as when we started
	actualPostPatch, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadGet(ctx, cluster, resourceKey, actualPostPatch)).To(Succeed())
	g.Expect(actualPostPatch).To(testutils.BeDerivativeOf(patchInput))

	preDelete, _ := postPatch.DeepCopyObject().(controllerutil.Object)
	preDelete.SetCreationTimestamp(metav1.NewTime(time.Time{}))

	// verify dry-run delete
	mgmtEnv.ToolConfig.DryRun = true
	g.Expect(mgmtEnv.Tool.WorkloadDelete(ctx, cluster, preDelete.DeepCopyObject().(controllerutil.Object))).To(Succeed())

	postDryDeleteOutput := mgmtEnv.Tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryDeleteOutput, postPatchOutput))

	// ensure that the resource is the same as when we started
	actualPostDryDelete, _ := reflect.New(resourceType).Interface().(controllerutil.Object)
	g.Expect(mgmtEnv.Tool.WorkloadGet(ctx, cluster, resourceKey, actualPostDryDelete)).To(Succeed())
	g.Expect(actualPostDryDelete).To(testutils.BeDerivativeOf(preDelete))

	// verify real delete
	mgmtEnv.ToolConfig.DryRun = false
	g.Expect(mgmtEnv.Tool.WorkloadDelete(ctx, cluster, preDelete.DeepCopyObject().(controllerutil.Object))).To(Succeed())

	postDeleteOutput := mgmtEnv.Tool.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postDeleteOutput, postDryDeleteOutput))

	// ensure that the resource no longer exists
	g.Expect(mgmtEnv.Tool.WorkloadGet(
		ctx,
		cluster,
		resourceKey,
		reflect.New(resourceType).Interface().(controllerutil.Object),
	)).To(MatchError(ContainSubstring("not found")))
}
