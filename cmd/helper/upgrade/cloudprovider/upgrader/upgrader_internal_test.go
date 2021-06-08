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

package upgrader

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base/testutils"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO: Add tests for CCM v2.0.0 compared to v1.1.0
// TODO: Do we need tests for CPEM >= v3.1.0 && < v3.1.0?

func TestUpgrader_patchOrCreateUnstructured(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	expectedData := map[string]interface{}{
		"color": base64.StdEncoding.EncodeToString([]byte("red")),
	}
	expectedResource := new(unstructured.Unstructured)
	expectedResource.SetUnstructuredContent(map[string]interface{}{"data": expectedData})
	expectedResource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	expectedResource.SetNamespace(fmt.Sprintf("test-%s", util.RandomString(6)))
	expectedResource.SetName(fmt.Sprintf("test-secret-%s", util.RandomString(6)))

	preMutatedData := map[string]string{
		"color": base64.StdEncoding.EncodeToString([]byte("purple")),
	}
	preMutatedResource := expectedResource.DeepCopy()
	g.Expect(unstructured.SetNestedStringMap(preMutatedResource.UnstructuredContent(), preMutatedData, "data")).
		To(Succeed())

	clusterWithoutResource := testutils.GenerateCluster("", "withoutResource")
	clusterWithResource := testutils.GenerateCluster("", "withResource")
	clusterWithResourceDiff := testutils.GenerateCluster("", "withResourceDiff")

	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: clusterWithResource.Namespace, Name: clusterWithResource.Name}: {
			expectedResource.DeepCopy(),
		},
		{Namespace: clusterWithResourceDiff.Namespace, Name: clusterWithResourceDiff.Name}: {
			preMutatedResource.DeepCopy(),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithResource,
		clusterWithResourceDiff, clusterWithoutResource)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Create
	preCreateOutput := u.GetOutputFor(clusterWithoutResource)
	g.Expect(u.patchOrCreateUnstructured(ctx, clusterWithoutResource, expectedResource.DeepCopy())).To(Succeed())
	postCreateOutput := u.GetOutputFor(clusterWithoutResource)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postCreateOutput, preCreateOutput))

	expectedResourceKey, err := client.ObjectKeyFromObject(expectedResource)
	g.Expect(err).NotTo(HaveOccurred())

	actualPostCreate := expectedResource.NewEmptyInstance()
	g.Expect(u.WorkloadGet(ctx, clusterWithoutResource, expectedResourceKey, actualPostCreate)).To(Succeed())
	g.Expect(actualPostCreate).To(testutils.BeDerivativeOf(expectedResource))

	// Test Noop on unchanged
	preNoopOutput := u.GetOutputFor(clusterWithResource)
	g.Expect(u.patchOrCreateUnstructured(ctx, clusterWithResource, expectedResource.DeepCopy())).To(Succeed())
	postNoopOutput := u.GetOutputFor(clusterWithResource)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postNoopOutput, preNoopOutput))

	actualNoop := expectedResource.NewEmptyInstance()
	g.Expect(u.WorkloadGet(ctx, clusterWithResource, expectedResourceKey, actualNoop)).To(Succeed())
	g.Expect(actualNoop).To(testutils.BeDerivativeOf(expectedResource))

	// Test Modify
	preMutateOutput := u.GetOutputFor(clusterWithResourceDiff)
	g.Expect(u.patchOrCreateUnstructured(ctx, clusterWithResourceDiff, expectedResource.DeepCopy())).To(Succeed())
	postMutateOutput := u.GetOutputFor(clusterWithResourceDiff)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMutateOutput, preMutateOutput))

	actualMutate := expectedResource.NewEmptyInstance()
	g.Expect(u.WorkloadGet(ctx, clusterWithResourceDiff, expectedResourceKey, actualMutate)).To(Succeed())
	g.Expect(actualMutate).To(testutils.BeDerivativeOf(expectedResource))
	g.Expect(actualMutate).NotTo(testutils.BeDerivativeOf(preMutatedResource))
}

func TestUpgrader_patchOrCreateUnstructuredDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	expectedData := map[string]interface{}{
		"color": base64.StdEncoding.EncodeToString([]byte("red")),
	}
	expectedResource := new(unstructured.Unstructured)
	expectedResource.SetUnstructuredContent(map[string]interface{}{"data": expectedData})
	expectedResource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	expectedResource.SetNamespace(fmt.Sprintf("test-%s", util.RandomString(6)))
	expectedResource.SetName(fmt.Sprintf("test-secret-%s", util.RandomString(6)))

	preMutatedData := map[string]string{
		"color": base64.StdEncoding.EncodeToString([]byte("purple")),
	}
	preMutatedResource := expectedResource.DeepCopy()
	g.Expect(unstructured.SetNestedStringMap(preMutatedResource.UnstructuredContent(), preMutatedData, "data")).
		To(Succeed())

	clusterWithoutResource := testutils.GenerateCluster("", "withoutresource")
	clusterWithResourceDiff := testutils.GenerateCluster("", "withdiff")

	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: clusterWithResourceDiff.Namespace, Name: clusterWithResourceDiff.Name}: {
			preMutatedResource.DeepCopy(),
		},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources,
		clusterWithResourceDiff, clusterWithoutResource)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run Create
	preDryRunOutput := u.GetOutputFor(clusterWithoutResource)
	g.Expect(u.patchOrCreateUnstructured(ctx, clusterWithoutResource, expectedResource.DeepCopy())).To(Succeed())
	postDryRunOutput := u.GetOutputFor(clusterWithoutResource)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	expectedResourceKey, err := client.ObjectKeyFromObject(expectedResource)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(u.WorkloadGet(ctx, clusterWithoutResource, expectedResourceKey, expectedResource.NewEmptyInstance())).
		To(MatchError(ContainSubstring("not found")))

	// Test Dry Run Modify
	preDryRunMutateOutput := u.GetOutputFor(clusterWithResourceDiff)
	g.Expect(u.patchOrCreateUnstructured(ctx, clusterWithResourceDiff, expectedResource.DeepCopy())).To(Succeed())
	postDryRunMutateOutput := u.GetOutputFor(clusterWithResourceDiff)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunMutateOutput, preDryRunMutateOutput))

	actualDryRunMutate := expectedResource.NewEmptyInstance()
	g.Expect(u.WorkloadGet(ctx, clusterWithResourceDiff, expectedResourceKey, actualDryRunMutate)).To(Succeed())
	g.Expect(actualDryRunMutate).To(testutils.BeDerivativeOf(preMutatedResource))
	g.Expect(actualDryRunMutate).NotTo(testutils.BeDerivativeOf(expectedResource))
}

func TestUpgrader_removeCCMDeployment(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	clusterWithoutCCM := testutils.GenerateCluster("", "withoutCCM")
	clusterWithCCM := testutils.GenerateCluster("", "withCCM")
	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: clusterWithCCM.Namespace, Name: clusterWithCCM.Name}: {
			testutils.GenerateDeployment(metav1.NamespaceSystem, oldDeploymentName, "test"),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithoutCCM, clusterWithCCM)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	preRemoveOutput := u.GetOutputFor(clusterWithoutCCM)
	g.Expect(u.removeCCMDeployment(ctx, clusterWithoutCCM)).To(Succeed())
	postRemoveOutput := u.GetOutputFor(clusterWithoutCCM)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	oldDeploymentKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldDeploymentName}
	g.Expect(u.WorkloadGet(ctx, clusterWithoutCCM, oldDeploymentKey, new(appsv1.Deployment))).
		To(MatchError(ContainSubstring("not found")))

	preRemoveOutput = u.GetOutputFor(clusterWithCCM)
	g.Expect(u.removeCCMDeployment(ctx, clusterWithCCM)).To(Succeed())
	postRemoveOutput = u.GetOutputFor(clusterWithCCM)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	g.Expect(u.WorkloadGet(ctx, clusterWithCCM, oldDeploymentKey, new(appsv1.Deployment))).
		To(MatchError(ContainSubstring("not found")))
}

func TestUpgrader_removeCCMDeploymentDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	cluster := testutils.GenerateCluster("", "")
	oldDeployment := testutils.GenerateDeployment(metav1.NamespaceSystem, oldDeploymentName, "test")
	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: cluster.Namespace, Name: cluster.Name}: {
			oldDeployment,
		},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run
	preDryRunOutput := u.GetOutputFor(cluster)
	g.Expect(u.removeCCMDeployment(ctx, cluster)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Ensure the oldSecret still exists
	oldDeploymentKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldDeploymentName}
	actualOldDeployment := new(appsv1.Deployment)
	g.Expect(u.WorkloadGet(ctx, cluster, oldDeploymentKey, actualOldDeployment)).To(Succeed())
	g.Expect(actualOldDeployment).To(testutils.BeDerivativeOf(oldDeployment))
}

func TestUpgrader_removeCCMSecret(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	oldSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      oldSecretName,
		},
		Data: map[string][]byte{
			"for": []byte("old-ccm"),
		},
	}

	clusterWithoutCCMSecret := testutils.GenerateCluster("", "withoutCCMSecret")
	clusterWithCCMSecret := testutils.GenerateCluster("", "withCCMSecret")
	clusterWithCCMSecretAndCSI := testutils.GenerateCluster("", "withCCMSecretAndCSI")
	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: clusterWithCCMSecret.Namespace, Name: clusterWithCCMSecret.Name}: {
			oldSecret.DeepCopy(),
		},
		{Namespace: clusterWithCCMSecretAndCSI.Namespace, Name: clusterWithCCMSecretAndCSI.Name}: {
			oldSecret.DeepCopy(),
			testutils.GenerateStatefulSet(metav1.NamespaceSystem, csiStatefulSetName, "test"),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithoutCCMSecret,
		clusterWithCCMSecret, clusterWithCCMSecretAndCSI)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	preRemoveOutput := u.GetOutputFor(clusterWithoutCCMSecret)
	g.Expect(u.removeOldCCMSecret(ctx, clusterWithoutCCMSecret)).To(Succeed())
	postRemoveOutput := u.GetOutputFor(clusterWithoutCCMSecret)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	oldSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	g.Expect(u.WorkloadGet(ctx, clusterWithoutCCMSecret, oldSecretKey, new(corev1.Secret))).
		To(MatchError(ContainSubstring("not found")))

	preRemoveOutput = u.GetOutputFor(clusterWithCCMSecret)
	g.Expect(u.removeOldCCMSecret(ctx, clusterWithCCMSecret)).To(Succeed())
	postRemoveOutput = u.GetOutputFor(clusterWithCCMSecret)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	g.Expect(u.WorkloadGet(ctx, clusterWithCCMSecret, oldSecretKey, new(corev1.Secret))).
		To(MatchError(ContainSubstring("not found")))

	preRemoveOutput = u.GetOutputFor(clusterWithCCMSecretAndCSI)
	g.Expect(u.removeOldCCMSecret(ctx, clusterWithCCMSecretAndCSI)).To(Succeed())
	postRemoveOutput = u.GetOutputFor(clusterWithCCMSecretAndCSI)
	testutils.VerifySuccessOutputSkipped(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	actualSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, clusterWithCCMSecretAndCSI, oldSecretKey, actualSecret)).To(Succeed())
	g.Expect(actualSecret).To(testutils.BeDerivativeOf(oldSecret))
}

func TestUpgrader_removeOldCCMSecretDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	cluster := testutils.GenerateCluster("", "")

	oldSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      oldSecretName,
		},
		Data: map[string][]byte{
			"for": []byte("old-ccm"),
		},
	}

	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: cluster.Namespace, Name: cluster.Name}: {
			oldSecret.DeepCopy(),
		},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run
	preDryRunOutput := u.GetOutputFor(cluster)
	g.Expect(u.removeOldCCMSecret(ctx, cluster)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Ensure the oldSecret still exists
	oldSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	actualOldSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))
}

func TestUpgrader_migrateSecret(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	oldSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      oldSecretName,
		},
		Data: map[string][]byte{
			"key": []byte("Test"),
		},
	}

	newSecret := oldSecret.DeepCopy()
	newSecret.Name = newSecretName

	clusterWithNeitherSecret := testutils.GenerateCluster("", "neitherSecret")
	clusterWithCCMSecret := testutils.GenerateCluster("", "withCCMSecret")
	clusterWithCPEMSecret := testutils.GenerateCluster("", "withCPEMSecret")
	clusterWithBothSecrets := testutils.GenerateCluster("", "withCPEMSecret")

	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: clusterWithCCMSecret.Namespace, Name: clusterWithCCMSecret.Name}: {
			oldSecret.DeepCopy(),
		},
		{Namespace: clusterWithCPEMSecret.Namespace, Name: clusterWithCPEMSecret.Name}: {
			newSecret.DeepCopy(),
		},
		{Namespace: clusterWithBothSecrets.Namespace, Name: clusterWithBothSecrets.Name}: {
			oldSecret.DeepCopy(),
			newSecret.DeepCopy(),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithNeitherSecret,
		clusterWithCCMSecret, clusterWithCPEMSecret, clusterWithBothSecrets)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	preMigrateOutput := u.GetOutputFor(clusterWithBothSecrets)
	g.Expect(u.migrateSecret(ctx, clusterWithBothSecrets)).To(Succeed())
	postMigrateOutput := u.GetOutputFor(clusterWithBothSecrets)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	newSecretKey, err := client.ObjectKeyFromObject(newSecret)
	g.Expect(err).NotTo(HaveOccurred())

	actualNewSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, clusterWithBothSecrets, newSecretKey, actualNewSecret)).To(Succeed())
	g.Expect(actualNewSecret).To(testutils.BeDerivativeOf(newSecret))

	oldSecretKey, err := client.ObjectKeyFromObject(oldSecret)
	g.Expect(err).NotTo(HaveOccurred())

	actualOldSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, clusterWithBothSecrets, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))

	preMigrateOutput = u.GetOutputFor(clusterWithCPEMSecret)
	g.Expect(u.migrateSecret(ctx, clusterWithCPEMSecret)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithCPEMSecret)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	actualNewSecret = new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, clusterWithCPEMSecret, newSecretKey, actualNewSecret)).To(Succeed())
	g.Expect(actualNewSecret).To(testutils.BeDerivativeOf(newSecret))

	g.Expect(u.WorkloadGet(ctx, clusterWithCPEMSecret, oldSecretKey, new(corev1.Secret))).
		To(MatchError(ContainSubstring("not found")))

	preMigrateOutput = u.GetOutputFor(clusterWithCCMSecret)
	g.Expect(u.migrateSecret(ctx, clusterWithCCMSecret)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithCCMSecret)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	g.Expect(u.WorkloadGet(ctx, clusterWithCCMSecret, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))

	g.Expect(u.WorkloadGet(ctx, clusterWithCCMSecret, newSecretKey, actualNewSecret)).To(Succeed())
	g.Expect(actualNewSecret).To(testutils.BeDerivativeOf(newSecret))

	g.Expect(u.migrateSecret(ctx, clusterWithNeitherSecret)).
		To(MatchError(ContainSubstring("not found")))
}

func TestUpgrader_migrateSecretDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	cluster := testutils.GenerateCluster("", "")

	oldSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      oldSecretName,
		},
		Data: map[string][]byte{
			"for": []byte("old-ccm"),
		},
	}

	workloadResources := map[client.ObjectKey][]runtime.Object{
		{Namespace: cluster.Namespace, Name: cluster.Name}: {
			oldSecret.DeepCopy(),
		},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run
	preDryRunOutput := u.GetOutputFor(cluster)
	g.Expect(u.migrateSecret(ctx, cluster)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Verify that the new secret does not exist
	newSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: newSecretName}
	g.Expect(u.WorkloadGet(ctx, cluster, newSecretKey, new(corev1.Secret))).
		To(MatchError(ContainSubstring("not found")))

	// Ensure the oldSecret still exists
	oldSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	actualOldSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))
}