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
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base/testutils"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type upgradeEnv struct {
	mgmtEnv  *testutils.MgmtEnv
	upgrader *Upgrader
}

func (u *upgradeEnv) DryRun(dryRun bool) {
	u.mgmtEnv.DryRun(dryRun)
}

func newUpgradeEnv(ctx context.Context, t *testing.T) *upgradeEnv {
	t.Helper()
	mgmtEnv := testutils.NewMgmtEnv(ctx, t, new(base.ToolConfig))
	upgradeEnv := new(upgradeEnv)
	upgradeEnv.mgmtEnv = mgmtEnv
	upgradeEnv.upgrader = &Upgrader{ //nolint:exhaustivestruct
		Tool: mgmtEnv.Tool,
	}

	return upgradeEnv
}

func TestUpgrader_patchOrCreateUnstructured(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")
	initialData := map[string]interface{}{
		"color": base64.StdEncoding.EncodeToString([]byte("red")),
	}
	testResource := new(unstructured.Unstructured)
	testResource.SetUnstructuredContent(map[string]interface{}{"data": initialData})
	testResource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	testResource.SetNamespace(fmt.Sprintf("test-%s", util.RandomString(6)))
	testResource.SetName(fmt.Sprintf("test-secret-%s", util.RandomString(6)))

	// Test Dry Run Create
	upgradeEnv.DryRun(true)
	preDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.patchOrCreateUnstructured(ctx, cluster, testResource.DeepCopy())).To(Succeed())
	postDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	testResourceKey, err := client.ObjectKeyFromObject(testResource)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, testResourceKey, testResource.NewEmptyInstance())).
		To(MatchError(ContainSubstring("not found")))

	// Test Create
	upgradeEnv.DryRun(false)
	preCreateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.patchOrCreateUnstructured(ctx, cluster, testResource.DeepCopy())).To(Succeed())
	postCreateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postCreateOutput, preCreateOutput))

	actualPostCreate := testResource.NewEmptyInstance()
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, testResourceKey, actualPostCreate)).To(Succeed())
	g.Expect(actualPostCreate).To(testutils.BeDerivativeOf(testResource))

	// Test Noop on unchanged
	preNoopOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.patchOrCreateUnstructured(ctx, cluster, testResource.DeepCopy())).To(Succeed())
	postNoopOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postNoopOutput, preNoopOutput))

	actualNoop := testResource.NewEmptyInstance()
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, testResourceKey, actualNoop)).To(Succeed())
	g.Expect(actualNoop).To(testutils.BeDerivativeOf(testResource))

	mutatedData := map[string]string{
		"color": base64.StdEncoding.EncodeToString([]byte("purple")),
	}
	mutatedResource := testResource.DeepCopy()
	g.Expect(unstructured.SetNestedStringMap(mutatedResource.UnstructuredContent(), mutatedData, "data")).To(Succeed())

	// Test Dry Run Modify
	upgradeEnv.DryRun(true)
	preDryRunMutateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.patchOrCreateUnstructured(ctx, cluster, mutatedResource.DeepCopy())).To(Succeed())
	postDryRunMutateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunMutateOutput, preDryRunMutateOutput))

	actualDryRunMutate := testResource.NewEmptyInstance()
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, testResourceKey, actualDryRunMutate)).To(Succeed())
	g.Expect(actualNoop).To(testutils.BeDerivativeOf(testResource))
	g.Expect(actualNoop).NotTo(testutils.BeDerivativeOf(mutatedResource))

	// Test Modify
	upgradeEnv.DryRun(false)
	preMutateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.patchOrCreateUnstructured(ctx, cluster, mutatedResource.DeepCopy())).To(Succeed())
	postMutateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMutateOutput, preMutateOutput))

	actualMutate := testResource.NewEmptyInstance()
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, testResourceKey, actualMutate)).To(Succeed())
	g.Expect(actualMutate).To(testutils.BeDerivativeOf(mutatedResource))
	g.Expect(actualMutate).NotTo(testutils.BeDerivativeOf(testResource))
}

func TestUpgrader_removeCCMDeploymentNotFound(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")

	preRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeCCMDeployment(ctx, cluster)).To(Succeed())
	postRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))
}

func TestUpgrader_removeCCMDeploymentSuccess(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")
	testLabels := map[string]string{"app": "test"}
	oldDeployment := &appsv1.Deployment{ // nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ // nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      oldDeploymentName,
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
							Name:  "test",
							Image: "test",
						},
					},
				},
			},
		},
	}

	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, oldDeployment.DeepCopy())).To(Succeed())

	// Test Dry Run
	upgradeEnv.DryRun(true)
	preDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeCCMDeployment(ctx, cluster)).To(Succeed())
	postDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Ensure the oldSecret still exists
	oldDeploymentKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldDeploymentName}
	actualOldDeployment := new(appsv1.Deployment)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldDeploymentKey, actualOldDeployment)).To(Succeed())
	g.Expect(actualOldDeployment).To(testutils.BeDerivativeOf(oldDeployment))

	// Test actual run
	upgradeEnv.DryRun(false)
	preRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeCCMDeployment(ctx, cluster)).To(Succeed())
	postRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	// Ensure the oldSecret no longer exists
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldDeploymentKey, new(appsv1.Deployment))).
		To(MatchError(ContainSubstring("not found")))
}

func TestUpgrader_removeOldCCMSecretSkippedCSI(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
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
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, oldSecret.DeepCopy())).To(Succeed())

	testLabels := map[string]string{"app": "test"}
	csi := &appsv1.StatefulSet{ // nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ // nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      csiStatefulSetName,
		},
		Spec: appsv1.StatefulSetSpec{ // nolint:exhaustivestruct
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
							Name:  "test",
							Image: "test",
						},
					},
				},
			},
		},
	}
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, csi.DeepCopy())).To(Succeed())

	preRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeOldCCMSecret(ctx, cluster)).To(Succeed())
	postRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputSkipped(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	// Ensure the oldSecret still exists
	oldSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	actualOldSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))
}

func TestUpgrader_removeOldCCMSecretNotFound(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")

	preRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeOldCCMSecret(ctx, cluster)).To(Succeed())
	postRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))
}

func TestUpgrader_removeOldCCMSecretSuccess(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
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
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, oldSecret.DeepCopy())).To(Succeed())

	// Test Dry Run
	upgradeEnv.DryRun(true)
	preDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeOldCCMSecret(ctx, cluster)).To(Succeed())
	postDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Ensure the oldSecret still exists
	oldSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	actualOldSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))

	// Test actual run
	upgradeEnv.DryRun(false)
	preRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.removeOldCCMSecret(ctx, cluster)).To(Succeed())
	postRemoveOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postRemoveOutput, preRemoveOutput))

	// Ensure the oldSecret no longer exists
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldSecretKey, new(corev1.Secret))).
		To(MatchError(ContainSubstring("not found")))
}

func TestUpgrader_migrateSecretOldSecretNotFound(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")

	g.Expect(upgradeEnv.upgrader.migrateSecret(ctx, cluster)).To(MatchError(ContainSubstring("not found")))
}

func TestUpgrader_migrateSecretOnlyNewSecretAlreadyExists(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")
	newSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      newSecretName,
		},
		Data: map[string][]byte{
			"key": []byte("test"),
		},
	}
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, newSecret.DeepCopy())).To(Succeed())

	preMigrateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.migrateSecret(ctx, cluster)).To(Succeed())
	postMigrateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	// Verify that the value of the secret after migration isn't mutated
	newSecretKey, err := client.ObjectKeyFromObject(newSecret)
	g.Expect(err).NotTo(HaveOccurred())

	actualNewSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, newSecretKey, actualNewSecret)).To(Succeed())
	g.Expect(actualNewSecret).To(testutils.BeDerivativeOf(newSecret))
}

func TestUpgrader_migrateSecretBothSecretsAlreadyExist(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
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
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, oldSecret.DeepCopy())).To(Succeed())

	newSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      newSecretName,
		},
		Data: map[string][]byte{
			"for": []byte("new-cpem"),
		},
	}
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, newSecret.DeepCopy())).To(Succeed())

	preMigrateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.migrateSecret(ctx, cluster)).To(Succeed())
	postMigrateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	// Verify that running migrateSecret did not mutate the value of the existing secret
	newSecretKey, err := client.ObjectKeyFromObject(newSecret)
	g.Expect(err).NotTo(HaveOccurred())

	actualNewSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, newSecretKey, actualNewSecret)).To(Succeed())
	g.Expect(actualNewSecret).To(testutils.BeDerivativeOf(newSecret))

	// Verify that the old secret is still present and not mutated
	oldSecretKey, err := client.ObjectKeyFromObject(oldSecret)
	g.Expect(err).NotTo(HaveOccurred())

	actualOldSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))
}

func TestUpgrader_migrateSecretSuccessCreate(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	upgradeEnv := newUpgradeEnv(ctx, t)
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
	g.Expect(upgradeEnv.upgrader.WorkloadCreate(ctx, cluster, oldSecret.DeepCopy())).To(Succeed())

	// Test Dry Run
	upgradeEnv.DryRun(true)
	preDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.migrateSecret(ctx, cluster)).To(Succeed())
	postDryRunOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Verify that the new secret does not exist
	newSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: newSecretName}
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, newSecretKey, new(corev1.Secret))).
		To(MatchError(ContainSubstring("not found")))

	// Ensure the oldSecret still exists
	oldSecretKey := client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: oldSecretName}
	actualOldSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))

	// Test actual run
	upgradeEnv.DryRun(false)
	preMigrateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	g.Expect(upgradeEnv.upgrader.migrateSecret(ctx, cluster)).To(Succeed())
	postMigrateOutput := upgradeEnv.upgrader.GetOutputFor(cluster)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	// Verify that running migrateSecret created the new secret and it has the same data as the old secret
	actualNewSecret := new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, newSecretKey, actualNewSecret)).To(Succeed())
	g.Expect(actualNewSecret.Data).To(BeEquivalentTo(oldSecret.Data))

	// Verify that the old secret is still present and not mutated
	oldSecretKey, err := client.ObjectKeyFromObject(oldSecret)
	g.Expect(err).NotTo(HaveOccurred())

	actualOldSecret = new(corev1.Secret)
	g.Expect(upgradeEnv.upgrader.WorkloadGet(ctx, cluster, oldSecretKey, actualOldSecret)).To(Succeed())
	g.Expect(actualOldSecret).To(testutils.BeDerivativeOf(oldSecret))
}
