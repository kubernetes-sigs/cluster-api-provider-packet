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

package migrator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base/testutils"
)

func TestMigrator_CheckPrerequisitesManagement(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	mgmtNamespace := util.RandomString(16)

	happyCluster := testutils.GenerateCluster("", "HappyCluster")
	sadWithOldCPEM := testutils.GenerateCluster("", "SadWihtOldCPEM")
	sadNoCCMOrCPEM := testutils.GenerateCluster("", "SadNoCPEM")

	clusters := []client.Object{
		happyCluster,
		sadWithOldCPEM,
		sadNoCCMOrCPEM,
	}

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: happyCluster.Namespace, Name: happyCluster.Name}: {
			testutils.GenerateDeployment(metav1.NamespaceSystem, CPEMDeploymentName, "cpem:"+CPEMMinVersion),
		},
		{Namespace: sadWithOldCPEM.Namespace, Name: sadWithOldCPEM.Name}: {
			testutils.GenerateDeployment(metav1.NamespaceSystem, CPEMDeploymentName, "cpem:3.0.0"),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusters...)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		TargetNamespace:      mgmtNamespace,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	m, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// If mgmt cluster is missing capp, then do not expect to be able to proceed
	g.Expect(m.CheckPrerequisites(ctx)).To(MatchError(ErrMissingCAPPDeployment))

	imageBaseName := "docker.io/packethost/cluster-api-provider-packet"
	cappDeployment := testutils.GenerateDeployment(mgmtNamespace, CAPPDeploymentName, imageBaseName+":v0.3.9")
	g.Expect(fakeEnv.MgmtClient.Create(ctx, cappDeployment.DeepCopy())).To(Succeed())

	// Verify that pre-requisites fail when CAPP is too old
	g.Expect(m.CheckPrerequisites(ctx)).To(MatchError(ErrCAPPTooOld))

	// Verify that pre-requisites get further with latest tag
	cappDeployment.Spec.Template.Spec.Containers[0].Image = imageBaseName
	g.Expect(fakeEnv.MgmtClient.Update(ctx, cappDeployment.DeepCopy())).To(Succeed())

	g.Expect(m.CheckPrerequisites(ctx)).To(Succeed())

	// Verify that pre-requisites get further with the minimum version tag
	cappDeployment.Spec.Template.Spec.Containers[0].Image = imageBaseName + ":v" + CAPPMinVersion
	g.Expect(fakeEnv.MgmtClient.Update(ctx, cappDeployment.DeepCopy())).To(Succeed())

	g.Expect(m.CheckPrerequisites(ctx)).To(Succeed())

	g.Expect(m.GetErrorFor(happyCluster)).NotTo(HaveOccurred())
	g.Expect(m.GetErrorFor(sadWithOldCPEM)).To(MatchError(ErrCPEMTooOld))
	g.Expect(m.GetErrorFor(sadNoCCMOrCPEM)).To(MatchError(ErrMissingCPEMDeployment))
}

func TestMigrator_modifyCPEMConfig(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	oldConfig := map[string]string{"apiKey": "API_KEY", "projectID": "PROJECT_ID", "eipTag": "cluster-api-provider-packet:cluster-id:CLUSTER_NAME"}
	oldMarshaledConfig, err := json.Marshal(oldConfig)
	g.Expect(err).NotTo(HaveOccurred())

	oldSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      CPEMSecretName,
		},
		Data: map[string][]byte{
			CPEMConfigKey: []byte(base64.StdEncoding.EncodeToString(oldMarshaledConfig)),
		},
	}

	expectedConfig := map[string]string{"apiKey": "API_KEY", "projectID": "PROJECT_ID", "eipTag": ""}
	expectedMarshaledConfig, err := json.Marshal(expectedConfig)
	g.Expect(err).NotTo(HaveOccurred())

	expectedSecret := oldSecret.DeepCopy()
	expectedSecret.Data[CPEMConfigKey] = []byte(base64.StdEncoding.EncodeToString(expectedMarshaledConfig))

	clusterWithMissingSecret := testutils.GenerateCluster("", "missingSecret")
	clusterWithOldCPEMSecretConfig := testutils.GenerateCluster("", "withOldCPEMSecret")
	clusterWithUpdatedCPEMSecretConfig := testutils.GenerateCluster("", "withUpdatedCPEMSecret")

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: clusterWithOldCPEMSecretConfig.Namespace, Name: clusterWithOldCPEMSecretConfig.Name}: {
			oldSecret.DeepCopy(),
		},
		{Namespace: clusterWithUpdatedCPEMSecretConfig.Namespace, Name: clusterWithUpdatedCPEMSecretConfig.Name}: {
			expectedSecret.DeepCopy(),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithMissingSecret,
		clusterWithOldCPEMSecretConfig, clusterWithUpdatedCPEMSecretConfig)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Cluster missing CPEM secret should fail completely
	g.Expect(modifyCPEMConfig(ctx, toolConfig.Logger, u.Tool, clusterWithMissingSecret)).
		To(MatchError(ContainSubstring("not found")))

	// Cluster with already updated CPEM Config should succeed and be a noop
	preMigrateOutput := u.GetOutputFor(clusterWithUpdatedCPEMSecretConfig)
	g.Expect(modifyCPEMConfig(ctx, toolConfig.Logger, u.Tool, clusterWithUpdatedCPEMSecretConfig)).To(Succeed())
	postMigrateOutput := u.GetOutputFor(clusterWithUpdatedCPEMSecretConfig)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	secretKey := client.ObjectKeyFromObject(expectedSecret)
	actualSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, clusterWithUpdatedCPEMSecretConfig, secretKey, actualSecret)).To(Succeed())

	actualMarshaledConfig, err := base64.StdEncoding.DecodeString(string(actualSecret.Data[CPEMConfigKey]))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(actualMarshaledConfig).To(MatchJSON(expectedMarshaledConfig))

	// Cluster with old CPEM Secret should succeed and update the secret
	preMigrateOutput = u.GetOutputFor(clusterWithOldCPEMSecretConfig)
	g.Expect(modifyCPEMConfig(ctx, toolConfig.Logger, u.Tool, clusterWithOldCPEMSecretConfig)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithOldCPEMSecretConfig)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	g.Expect(u.WorkloadGet(ctx, clusterWithOldCPEMSecretConfig, secretKey, actualSecret)).To(Succeed())

	actualMarshaledConfig, err = base64.StdEncoding.DecodeString(string(actualSecret.Data[CPEMConfigKey]))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(actualMarshaledConfig).To(MatchJSON(expectedMarshaledConfig))
}

func TestMigrator_modifyCPEMConfigDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	cluster := testutils.GenerateCluster("", "")

	oldConfig := map[string]string{"apiKey": "API_KEY", "projectID": "PROJECT_ID", "eipTag": "cluster-api-provider-packet:cluster-id:CLUSTER_NAME"}
	oldMarshaledConfig, err := json.Marshal(oldConfig)
	g.Expect(err).NotTo(HaveOccurred())

	oldSecret := &corev1.Secret{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      CPEMSecretName,
		},
		Data: map[string][]byte{
			CPEMConfigKey: []byte(base64.StdEncoding.EncodeToString(oldMarshaledConfig)),
		},
	}

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: cluster.Namespace, Name: cluster.Name}: {
			oldSecret.DeepCopy(),
		},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run
	preDryRunOutput := u.GetOutputFor(cluster)
	g.Expect(modifyCPEMConfig(ctx, toolConfig.Logger, u.Tool, cluster)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Verify that the secret was not changed
	secretKey := client.ObjectKeyFromObject(oldSecret)
	actualSecret := new(corev1.Secret)
	g.Expect(u.WorkloadGet(ctx, cluster, secretKey, actualSecret)).To(Succeed())
	g.Expect(actualSecret).To(testutils.BeDerivativeOf(oldSecret))
}

func TestMigrator_rolloutCPEMDeployment(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	cpemDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      CPEMDeploymentName,
		},
	}

	existingRolloutTime := time.Now().AddDate(0, 0, -1)
	rolledOutCPEMDeployment := cpemDeployment.DeepCopy()
	rolledOutCPEMDeployment.SetAnnotations(
		map[string]string{
			DeploymentRolloutAnnotation: existingRolloutTime.Format(time.RFC3339),
		},
	)

	clusterWithMissingDeployment := testutils.GenerateCluster("", "missingDeployment")
	clusterWithNonRolledOutDeployment := testutils.GenerateCluster("", "withNonRolledOutDeployment")
	clusterWithRolledOutCPEMDeployment := testutils.GenerateCluster("", "withRolledOutCPEMDeployment")

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: clusterWithNonRolledOutDeployment.Namespace, Name: clusterWithNonRolledOutDeployment.Name}: {
			cpemDeployment.DeepCopy(),
		},
		{Namespace: clusterWithRolledOutCPEMDeployment.Namespace, Name: clusterWithRolledOutCPEMDeployment.Name}: {
			rolledOutCPEMDeployment.DeepCopy(),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithMissingDeployment,
		clusterWithNonRolledOutDeployment, clusterWithRolledOutCPEMDeployment)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Cluster missing CPEM Deployment should fail
	g.Expect(rolloutCPEMDeployment(ctx, toolConfig.Logger, u.Tool, clusterWithMissingDeployment)).
		To(MatchError(ContainSubstring("not found")))

	// Cluster with rolled out deployment should be a noop
	preMigrateOutput := u.GetOutputFor(clusterWithRolledOutCPEMDeployment)
	g.Expect(rolloutCPEMDeployment(ctx, toolConfig.Logger, u.Tool, clusterWithRolledOutCPEMDeployment)).To(Succeed())
	postMigrateOutput := u.GetOutputFor(clusterWithRolledOutCPEMDeployment)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	deploymentKey := client.ObjectKeyFromObject(cpemDeployment)
	actualDeployment := new(appsv1.Deployment)
	g.Expect(u.WorkloadGet(ctx, clusterWithRolledOutCPEMDeployment, deploymentKey, actualDeployment)).To(Succeed())
	g.Expect(actualDeployment).To(testutils.BeDerivativeOf(rolledOutCPEMDeployment))
	g.Expect(actualDeployment.GetAnnotations()).To(HaveKey(DeploymentRolloutAnnotation))
	actualRolloutTime, err := time.Parse(time.RFC3339, actualDeployment.GetAnnotations()[DeploymentRolloutAnnotation])
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(actualRolloutTime.Equal(existingRolloutTime))

	// Cluster with deployment that hasn't been rolled out yet, should rollout deployment
	preMigrateOutput = u.GetOutputFor(clusterWithNonRolledOutDeployment)
	g.Expect(rolloutCPEMDeployment(ctx, toolConfig.Logger, u.Tool, clusterWithNonRolledOutDeployment)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithNonRolledOutDeployment)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	actualDeployment = new(appsv1.Deployment)
	g.Expect(u.WorkloadGet(ctx, clusterWithNonRolledOutDeployment, deploymentKey, actualDeployment)).To(Succeed())
	g.Expect(actualDeployment).To(testutils.BeDerivativeOf(cpemDeployment))
	g.Expect(actualDeployment.GetAnnotations()).To(HaveKey(DeploymentRolloutAnnotation))
	actualRolloutTime, err = time.Parse(time.RFC3339, actualDeployment.GetAnnotations()[DeploymentRolloutAnnotation])
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(actualRolloutTime.After(existingRolloutTime))
}

func TestMigrator_rolloutCPEMDeploymentDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	cluster := testutils.GenerateCluster("", "")

	cpemDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Namespace: metav1.NamespaceSystem,
			Name:      CPEMDeploymentName,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"test": "test"},
				},
				Spec: corev1.PodSpec{
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

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: cluster.Namespace, Name: cluster.Name}: {
			cpemDeployment.DeepCopy(),
		},
	}

	testEnv := testutils.NewTestEnv(ctx, t, workloadResources, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run
	preDryRunOutput := u.GetOutputFor(cluster)
	g.Expect(rolloutCPEMDeployment(ctx, toolConfig.Logger, u.Tool, cluster)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Verify that the deployment was not changed
	deploymentKey := client.ObjectKeyFromObject(cpemDeployment)
	actualDeployment := new(appsv1.Deployment)
	g.Expect(u.WorkloadGet(ctx, cluster, deploymentKey, actualDeployment)).To(Succeed())
	g.Expect(actualDeployment).To(testutils.BeDerivativeOf(cpemDeployment))
	g.Expect(actualDeployment.GetAnnotations()).NotTo(HaveKey(DeploymentRolloutAnnotation))
}

func TestMigrator_addKubeVIPRBAC(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	clusterWithoutKubeVIPRBAC := testutils.GenerateCluster("", "withoutKubeVIPRBAC")
	clusterWithKubeVIPRBAC := testutils.GenerateCluster("", "withKubeVIPRBAC")

	expectedServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      "kube-vip",
		},
	}

	expectedClusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:kube-vip-role",
			Annotations: map[string]string{
				"rbac.authorization.kubernetes.io/autoupdate": "true",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "services/status", "nodes"},
				Verbs:     []string{"list", "get", "watch", "update"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"list", "get", "watch", "update", "create"},
			},
		},
	}

	expectedClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:kube-vip-binding",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     expectedClusterRole.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      expectedServiceAccount.Name,
				Namespace: expectedServiceAccount.Namespace,
			},
		},
	}

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: clusterWithKubeVIPRBAC.Namespace, Name: clusterWithKubeVIPRBAC.Name}: {
			expectedServiceAccount.DeepCopy(),
			expectedClusterRole.DeepCopy(),
			expectedClusterRoleBinding.DeepCopy(),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithoutKubeVIPRBAC, clusterWithKubeVIPRBAC)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	preMigrateOutput := u.GetOutputFor(clusterWithoutKubeVIPRBAC)
	g.Expect(addKubeVIPRBAC(ctx, toolConfig.Logger, u.Tool, clusterWithoutKubeVIPRBAC)).To(Succeed())
	postMigrateOutput := u.GetOutputFor(clusterWithoutKubeVIPRBAC)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	actualServiceAccount := new(corev1.ServiceAccount)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIPRBAC, client.ObjectKeyFromObject(expectedServiceAccount), actualServiceAccount)).To(Succeed())
	g.Expect(actualServiceAccount).To(testutils.BeDerivativeOf(expectedServiceAccount))

	actualClusterRole := new(rbacv1.ClusterRole)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIPRBAC, client.ObjectKeyFromObject(expectedClusterRole), actualClusterRole)).To(Succeed())
	g.Expect(actualClusterRole).To(testutils.BeDerivativeOf(expectedClusterRole))

	actualClusterRoleBinding := new(rbacv1.ClusterRoleBinding)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIPRBAC, client.ObjectKeyFromObject(expectedClusterRoleBinding), actualClusterRoleBinding)).To(Succeed())
	g.Expect(actualClusterRoleBinding).To(testutils.BeDerivativeOf(expectedClusterRoleBinding))

	preMigrateOutput = u.GetOutputFor(clusterWithKubeVIPRBAC)
	g.Expect(addKubeVIPRBAC(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPRBAC)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithKubeVIPRBAC)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	actualServiceAccount = new(corev1.ServiceAccount)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIPRBAC, client.ObjectKeyFromObject(expectedServiceAccount), actualServiceAccount)).To(Succeed())
	g.Expect(actualServiceAccount).To(testutils.BeDerivativeOf(expectedServiceAccount))

	actualClusterRole = new(rbacv1.ClusterRole)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIPRBAC, client.ObjectKeyFromObject(expectedClusterRole), actualClusterRole)).To(Succeed())
	g.Expect(actualClusterRole).To(testutils.BeDerivativeOf(expectedClusterRole))

	actualClusterRoleBinding = new(rbacv1.ClusterRoleBinding)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIPRBAC, client.ObjectKeyFromObject(expectedClusterRoleBinding), actualClusterRoleBinding)).To(Succeed())
	g.Expect(actualClusterRoleBinding).To(testutils.BeDerivativeOf(expectedClusterRoleBinding))
}

func TestMigrator_addKubeVIPRBACDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	cluster := testutils.GenerateCluster("", "")
	testEnv := testutils.NewTestEnv(ctx, t, map[client.ObjectKey][]client.Object{}, cluster)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run
	preDryRunOutput := u.GetOutputFor(cluster)
	g.Expect(addKubeVIPRBAC(ctx, toolConfig.Logger, u.Tool, cluster)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(cluster)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Verify that the rbac resources are not found
	g.Expect(u.WorkloadGet(ctx, cluster, client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kube-vip"}, new(corev1.ServiceAccount))).To(MatchError(ContainSubstring("not found")))
	g.Expect(u.WorkloadGet(ctx, cluster, client.ObjectKey{Name: "system:kube-vip-role"}, new(rbacv1.ClusterRole))).To(MatchError(ContainSubstring("not found")))
	g.Expect(u.WorkloadGet(ctx, cluster, client.ObjectKey{Name: "system:kube-vip-binding"}, new(rbacv1.ClusterRoleBinding))).To(MatchError(ContainSubstring("not found")))
}

func TestMigrator_addKubeVIPDaemonSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	clusterWithoutKubeVIP := testutils.GenerateCluster("", "withoutKubeVIP")
	clusterWithKubeVIP := testutils.GenerateCluster("", "withKubeVIP")
	clusterWithParallelKubeVIP := testutils.GenerateCluster("", "withParallelKubeVIP")
	clusterWithKubeVIPServices := testutils.GenerateCluster("", "withKubeVIPServices")
	clusterWithKubeVIPIncompatibleImage := testutils.GenerateCluster("", "withKubeVIPIncompatibleImage")
	clusterWithKubeVIPIncompatibleNoMetal := testutils.GenerateCluster("", "withKubeVIPIncompatibleMetal")
	clusterWithKubeVIPIncompatibleNoMetalAuth := testutils.GenerateCluster("", "withKubeVIPIncompatibleMetalAuth")
	clusterWithKubeVIPIncompatibleNoMetalProject := testutils.GenerateCluster("", "withKubeVIPIncompatibleMetalProject")
	clusterWithKubeVIPIncompatibleAddress := testutils.GenerateCluster("", "withKubeVIPIncompatibleAddress")
	clusterWithKubeVIPIncompatiblePort := testutils.GenerateCluster("", "withKubeVIPIncompatiblePort")
	clusterWithKubeVIPIncompatibleTolerations := testutils.GenerateCluster("", "withKubeVIPIncompatibleTolerations")
	clusterWithKubeVIPIncompatibleAffinity := testutils.GenerateCluster("", "withKubeVIPIncompatibleAffinity")

	existingDaemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      KubeVIPDaemonSetName,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kube-vip",
							Image: "ghcr.io/kube-vip/kube-vip:v0.3.8",
							Args:  []string{"manager"},
							Env: []corev1.EnvVar{
								{Name: "vip_arp", Value: "true"},
								{Name: "vip_interface", Value: "bond0"},
								{Name: "port", Value: "6443"},
								{Name: "cp_enable", Value: "true"},
								{Name: "cp_namespace", Value: "kube-system"},
								{Name: "vip_ddns", Value: "false"},
								{Name: "vip_leaderelection", Value: "true"},
								{Name: "vip_leaseduration", Value: "5"},
								{Name: "vip_renewdeadline", Value: "3"},
								{Name: "vip_retryperiod", Value: "1"},
								{Name: KubeVIPProviderConfigEnvVar, Value: "/etc/cloud-sa/cloud-sa.json"},
								{Name: "vip_packet", Value: "true"},
								{Name: "vip_packetproject"},
								{Name: "vip_packetprojectid"},
								{Name: "PACKET_AUTH_TOKEN"},
								{Name: "address", Value: "192.168.0.1"},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Effect:   corev1.TaintEffectNoSchedule,
							Operator: corev1.TolerationOpExists,
						},
						{
							Effect:   corev1.TaintEffectNoExecute,
							Operator: corev1.TolerationOpExists,
						},
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-role.kubernetes.io/master",
												Operator: corev1.NodeSelectorOpExists,
											},
										},
									},
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "node-role.kubernetes.io/control-plane",
												Operator: corev1.NodeSelectorOpExists,
											},
										},
									},
								},
							},
						},
					},
					HostNetwork: true,
				},
			},
		},
	}

	existingParallelDaemonSet := existingDaemonSet.DeepCopy()
	existingParallelDaemonSet.Name = KubeVIPOverrideDaemonSetName
	existingParallelDaemonSet.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"name": KubeVIPOverrideDaemonSetName,
		},
	}
	existingParallelDaemonSet.Spec.Template.ObjectMeta.Labels = map[string]string{
		"name": KubeVIPOverrideDaemonSetName,
	}

	existingServicesDaemonSet := existingDaemonSet.DeepCopy()
	existingServicesDaemonSet.Spec.Template.Spec.Containers[0].Env = make([]corev1.EnvVar, 0, len(existingDaemonSet.Spec.Template.Spec.Containers[0].Env))
	for _, e := range existingDaemonSet.Spec.Template.Spec.Containers[0].Env {
		if e.Name != "cp_enable" {
			existingServicesDaemonSet.Spec.Template.Spec.Containers[0].Env = append(existingServicesDaemonSet.Spec.Template.Spec.Containers[0].Env, e)
		}
	}
	existingServicesDaemonSet.Spec.Template.Spec.Containers[0].Env = append(existingServicesDaemonSet.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "svc_enable", Value: "true"})
	existingServicesDaemonSet.Spec.Template.Spec.Tolerations = nil
	existingServicesDaemonSet.Spec.Template.Spec.Affinity = nil

	existingIncompatibleImageDaemonSet := existingDaemonSet.DeepCopy()
	existingIncompatibleImageDaemonSet.Spec.Template.Spec.Containers[0].Image = "ghcr.io/kube-vip/kube-vip:v0.3.0"

	existingIncompatibleNoMetalDaemonSet := existingDaemonSet.DeepCopy()
	existingIncompatibleNoMetalDaemonSet.Spec.Template.Spec.Containers[0].Env = make([]corev1.EnvVar, 0, len(existingDaemonSet.Spec.Template.Spec.Containers[0].Env))
	for _, e := range existingDaemonSet.Spec.Template.Spec.Containers[0].Env {
		if e.Name != "vip_packet" {
			existingIncompatibleNoMetalDaemonSet.Spec.Template.Spec.Containers[0].Env = append(existingIncompatibleNoMetalDaemonSet.Spec.Template.Spec.Containers[0].Env, e)
		}
	}

	existingIncompatibleNoMetalDaemonSetAuth := existingDaemonSet.DeepCopy()
	existingIncompatibleNoMetalDaemonSetAuth.Spec.Template.Spec.Containers[0].Env = make([]corev1.EnvVar, 0, len(existingDaemonSet.Spec.Template.Spec.Containers[0].Env))
	for _, e := range existingDaemonSet.Spec.Template.Spec.Containers[0].Env {
		if e.Name != KubeVIPProviderConfigEnvVar {
			existingIncompatibleNoMetalDaemonSetAuth.Spec.Template.Spec.Containers[0].Env = append(existingIncompatibleNoMetalDaemonSetAuth.Spec.Template.Spec.Containers[0].Env, e)
		}
	}
	existingIncompatibleNoMetalDaemonSetAuth.Spec.Template.Spec.Containers[0].Env = append(existingIncompatibleNoMetalDaemonSetAuth.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "vip_packetprojectid", Value: "xxxxxxxxxx"})

	existingIncompatibleNoMetalDaemonSetProject := existingDaemonSet.DeepCopy()
	existingIncompatibleNoMetalDaemonSetProject.Spec.Template.Spec.Containers[0].Env = make([]corev1.EnvVar, 0, len(existingDaemonSet.Spec.Template.Spec.Containers[0].Env))
	for _, e := range existingDaemonSet.Spec.Template.Spec.Containers[0].Env {
		if e.Name != KubeVIPProviderConfigEnvVar {
			existingIncompatibleNoMetalDaemonSetProject.Spec.Template.Spec.Containers[0].Env = append(existingIncompatibleNoMetalDaemonSetProject.Spec.Template.Spec.Containers[0].Env, e)
		}
	}
	existingIncompatibleNoMetalDaemonSetProject.Spec.Template.Spec.Containers[0].Env = append(existingIncompatibleNoMetalDaemonSetProject.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "PACKET_AUTH_TOKEN", Value: "xxxxxxxxxx"})

	existingIncompatibleAddressDaemonSet := existingDaemonSet.DeepCopy()
	for i := range existingIncompatibleAddressDaemonSet.Spec.Template.Spec.Containers[0].Env {
		if existingIncompatibleAddressDaemonSet.Spec.Template.Spec.Containers[0].Env[i].Name == "address" {
			existingIncompatibleAddressDaemonSet.Spec.Template.Spec.Containers[0].Env[i].Value = "172.16.0.1"
		}
	}

	existingIncompatiblePortDaemonSet := existingDaemonSet.DeepCopy()
	for i := range existingIncompatiblePortDaemonSet.Spec.Template.Spec.Containers[0].Env {
		if existingIncompatiblePortDaemonSet.Spec.Template.Spec.Containers[0].Env[i].Name == "port" {
			existingIncompatiblePortDaemonSet.Spec.Template.Spec.Containers[0].Env[i].Value = "106443"
		}
	}

	existingIncompatibleTolerationsDaemonSet := existingDaemonSet.DeepCopy()
	existingIncompatibleTolerationsDaemonSet.Spec.Template.Spec.Tolerations = nil

	existingIncompatibleAffinityDaemonSet := existingDaemonSet.DeepCopy()
	existingIncompatibleAffinityDaemonSet.Spec.Template.Spec.Affinity = nil

	workloadResources := map[client.ObjectKey][]client.Object{
		{Namespace: clusterWithKubeVIP.Namespace, Name: clusterWithKubeVIP.Name}: {
			existingDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithParallelKubeVIP.Namespace, Name: clusterWithParallelKubeVIP.Name}: {
			existingParallelDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPServices.Namespace, Name: clusterWithKubeVIPServices.Name}: {
			existingServicesDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleImage.Namespace, Name: clusterWithKubeVIPIncompatibleImage.Name}: {
			existingIncompatibleImageDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleNoMetal.Namespace, Name: clusterWithKubeVIPIncompatibleNoMetal.Name}: {
			existingIncompatibleNoMetalDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleNoMetalAuth.Namespace, Name: clusterWithKubeVIPIncompatibleNoMetalAuth.Name}: {
			existingIncompatibleNoMetalDaemonSetAuth.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleNoMetalProject.Namespace, Name: clusterWithKubeVIPIncompatibleNoMetalProject.Name}: {
			existingIncompatibleNoMetalDaemonSetProject.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleAddress.Namespace, Name: clusterWithKubeVIPIncompatibleAddress.Name}: {
			existingIncompatibleAddressDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatiblePort.Namespace, Name: clusterWithKubeVIPIncompatiblePort.Name}: {
			existingIncompatiblePortDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleTolerations.Namespace, Name: clusterWithKubeVIPIncompatibleTolerations.Name}: {
			existingIncompatibleTolerationsDaemonSet.DeepCopy(),
		},
		{Namespace: clusterWithKubeVIPIncompatibleAffinity.Namespace, Name: clusterWithKubeVIPIncompatibleAffinity.Name}: {
			existingIncompatibleAffinityDaemonSet.DeepCopy(),
		},
	}

	fakeEnv := testutils.NewFakeEnv(ctx, t, workloadResources, clusterWithoutKubeVIP, clusterWithKubeVIP)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		MgmtClient:           fakeEnv.MgmtClient,
		WorkloadClientGetter: fakeEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test no existing kube-vip deployment
	preMigrateOutput := u.GetOutputFor(clusterWithoutKubeVIP)
	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithoutKubeVIP)).To(Succeed())
	postMigrateOutput := u.GetOutputFor(clusterWithoutKubeVIP)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	actualDSKey := client.ObjectKeyFromObject(existingDaemonSet)
	actualDS := new(appsv1.DaemonSet)
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIP, actualDSKey, actualDS)).To(Succeed())
	g.Expect(actualDS).To(testutils.BeDerivativeOf(existingDaemonSet))

	// Test pre-existing parallel kube-vip deployment that is sufficient
	preMigrateOutput = u.GetOutputFor(clusterWithParallelKubeVIP)
	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithParallelKubeVIP)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithParallelKubeVIP)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	parallelDSKey := client.ObjectKeyFromObject(existingParallelDaemonSet)
	parallelDS := new(appsv1.DaemonSet)
	g.Expect(u.WorkloadGet(ctx, clusterWithParallelKubeVIP, parallelDSKey, parallelDS)).To(Succeed())
	g.Expect(parallelDS).To(testutils.BeDerivativeOf(existingParallelDaemonSet))

	// Test pre-existing kube-vip deployment that is sufficient
	preMigrateOutput = u.GetOutputFor(clusterWithKubeVIP)
	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIP)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithKubeVIP)
	testutils.VerifySuccessOutputUnchanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	g.Expect(u.WorkloadGet(ctx, clusterWithKubeVIP, actualDSKey, actualDS)).To(Succeed())
	g.Expect(actualDS).To(testutils.BeDerivativeOf(existingDaemonSet))

	// Test pre-existing kube-vip deployment that is not configured for control plane
	preMigrateOutput = u.GetOutputFor(clusterWithKubeVIPServices)
	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPServices)).To(Succeed())
	postMigrateOutput = u.GetOutputFor(clusterWithKubeVIPServices)
	testutils.VerifySuccessOutputChanged(t, strings.TrimPrefix(postMigrateOutput, preMigrateOutput))

	// verify that we haven't modified the existing daemonset
	g.Expect(u.WorkloadGet(ctx, clusterWithKubeVIPServices, actualDSKey, actualDS)).To(Succeed())
	g.Expect(actualDS).To(testutils.BeDerivativeOf(existingServicesDaemonSet))

	// verify that the parallel daemonset we created matches what we expect
	expectedDS := existingDaemonSet.DeepCopy()
	expectedDS.Name = KubeVIPOverrideDaemonSetName
	g.Expect(u.WorkloadGet(ctx, clusterWithKubeVIPServices, parallelDSKey, parallelDS)).To(Succeed())
	g.Expect(parallelDS).To(testutils.BeDerivativeOf(expectedDS))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleImage)).To(MatchError(ErrKubeVIPTooOld))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleNoMetal)).To(MatchError(ErrKubeVIPIncompatible))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleNoMetalAuth)).To(MatchError(ErrKubeVIPIncompatible))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleNoMetalProject)).To(MatchError(ErrKubeVIPIncompatible))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleAddress)).To(MatchError(ErrKubeVIPIncompatible))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatiblePort)).To(MatchError(ErrKubeVIPIncompatible))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleTolerations)).To(MatchError(ErrKubeVIPIncompatible))

	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithKubeVIPIncompatibleAffinity)).To(MatchError(ErrKubeVIPIncompatible))
}

func TestMigrator_addKubeVIPDaemonSetDry(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()

	clusterWithoutKubeVIP := testutils.GenerateCluster("", "without-kube-vip")

	testEnv := testutils.NewTestEnv(ctx, t, nil, clusterWithoutKubeVIP)
	toolConfig := &base.ToolConfig{ //nolint:exhaustivestruct
		DryRun:               true,
		RestConfig:           testEnv.RestConfig,
		WorkloadClientGetter: testEnv.WorkloadClientGetter,
		Logger:               klogr.New(),
	}
	u, err := New(ctx, toolConfig)
	g.Expect(err).NotTo(HaveOccurred())

	// Test Dry Run Create
	preDryRunOutput := u.GetOutputFor(clusterWithoutKubeVIP)
	g.Expect(addKubeVIPDaemonSet(ctx, toolConfig.Logger, u.Tool, clusterWithoutKubeVIP)).To(Succeed())
	postDryRunOutput := u.GetOutputFor(clusterWithoutKubeVIP)
	testutils.VerifySuccessOutputDryRun(t, strings.TrimPrefix(postDryRunOutput, preDryRunOutput))

	// Verify that the daemonset is not found
	g.Expect(u.WorkloadGet(ctx, clusterWithoutKubeVIP, client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: KubeVIPDaemonSetName}, new(corev1.Secret))).To(MatchError(ContainSubstring("not found")))
}
