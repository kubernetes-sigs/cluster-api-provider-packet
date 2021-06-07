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
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base/testutils"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type migratorEnv struct {
	mgmtEnv  *testutils.MgmtEnv
	migrator *Migrator
}

func (m *migratorEnv) DryRun(dryRun bool) {
	m.mgmtEnv.DryRun(dryRun)
}

func newMigratorEnv(ctx context.Context, t *testing.T) *migratorEnv {
	t.Helper()
	mgmtEnv := testutils.NewMgmtEnv(ctx, t, new(base.ToolConfig))
	migratorEnv := new(migratorEnv)
	migratorEnv.mgmtEnv = mgmtEnv
	migratorEnv.migrator = &Migrator{ //nolint:exhaustivestruct
		Tool: mgmtEnv.Tool,
	}

	return migratorEnv
}

func TestMigrator_migrateNode(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	migratorEnv := newMigratorEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")
	oldPrefix := "packet://"
	newPrefix := "equinixmetal://"
	nodeName := fmt.Sprintf("test-node-%s", util.RandomString(6))
	nodeKey := client.ObjectKey{Name: nodeName} //nolint:exhaustivestruct
	providerIDSuffix := "my-provider-id"
	initialNode := &corev1.Node{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Name: nodeName,
		},
		Spec: corev1.NodeSpec{ //nolint:exhaustivestruct
			ProviderID: oldPrefix + providerIDSuffix,
		},
	}
	g.Expect(migratorEnv.migrator.WorkloadCreate(ctx, cluster, initialNode.DeepCopy())).To(Succeed())

	expectedNode := initialNode.DeepCopy()
	expectedNode.Spec.ProviderID = newPrefix + providerIDSuffix

	// Test Dry Run Migrate
	migratorEnv.DryRun(true)

	postDryRunNode := initialNode.DeepCopy()
	preDryRunOutput := migratorEnv.migrator.GetOutputFor(cluster)
	g.Expect(migratorEnv.migrator.migrateNode(ctx, postDryRunNode, cluster)).To(Succeed())
	postDryRunOutput := migratorEnv.migrator.GetOutputFor(cluster)

	g.Expect(postDryRunNode).To(testutils.BeDerivativeOf(expectedNode))

	postDryRunOutputDiff := strings.TrimPrefix(postDryRunOutput, preDryRunOutput)
	postDryRunOutputLines := strings.Split(postDryRunOutputDiff, "\n")
	g.Expect(postDryRunOutputLines).To(HaveLen(3))
	testutils.VerifySuccessOutputDryRun(t, postDryRunOutputLines[0])
	testutils.VerifySuccessOutputDryRun(t, postDryRunOutputLines[1])

	actualPostDryRunNode := new(corev1.Node)
	g.Expect(migratorEnv.migrator.WorkloadGet(ctx, cluster, nodeKey, actualPostDryRunNode)).To(Succeed())
	g.Expect(actualPostDryRunNode).To(testutils.BeDerivativeOf(initialNode))

	// Test Migrate
	migratorEnv.DryRun(false)

	postMigrateNode := initialNode.DeepCopy()
	preMigrateOutput := migratorEnv.migrator.GetOutputFor(cluster)
	g.Expect(migratorEnv.migrator.migrateNode(ctx, postMigrateNode, cluster)).To(Succeed())
	postMigrateOutput := migratorEnv.migrator.GetOutputFor(cluster)

	g.Expect(postMigrateNode).To(testutils.BeDerivativeOf(expectedNode))

	postMigrateOutputDiff := strings.TrimPrefix(postMigrateOutput, preMigrateOutput)
	postMigrateOutputLines := strings.Split(postMigrateOutputDiff, "\n")
	g.Expect(postMigrateOutputLines).To(HaveLen(3))
	testutils.VerifySuccessOutputChanged(t, postMigrateOutputLines[0])
	testutils.VerifySuccessOutputChanged(t, postMigrateOutputLines[1])

	actualPostMigrateNode := new(corev1.Node)
	g.Expect(migratorEnv.migrator.WorkloadGet(ctx, cluster, nodeKey, actualPostMigrateNode)).To(Succeed())
	g.Expect(actualPostMigrateNode).To(testutils.BeDerivativeOf(expectedNode))
}

func TestMigrator_migrateNodeAlreadyMigrated(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := context.TODO()
	migratorEnv := newMigratorEnv(ctx, t)
	cluster := testutils.GenerateCluster("", "")

	newPrefix := "equinixmetal://"

	nodeName := fmt.Sprintf("test-node-%s", util.RandomString(6))
	nodeKey := client.ObjectKey{Name: nodeName} //nolint:exhaustivestruct
	providerIDSuffix := "my-provider-id"
	expectedNode := &corev1.Node{ //nolint:exhaustivestruct
		ObjectMeta: metav1.ObjectMeta{ //nolint:exhaustivestruct
			Name: nodeName,
		},
		Spec: corev1.NodeSpec{ //nolint:exhaustivestruct
			ProviderID: newPrefix + providerIDSuffix,
		},
	}
	g.Expect(migratorEnv.migrator.WorkloadCreate(ctx, cluster, expectedNode.DeepCopy())).To(Succeed())

	// Test Already Migrated
	preAlreadyMigrateOutput := migratorEnv.migrator.GetOutputFor(cluster)
	g.Expect(migratorEnv.migrator.migrateNode(ctx, expectedNode.DeepCopy(), cluster)).To(Succeed())
	postAlreadyMigrateOutput := migratorEnv.migrator.GetOutputFor(cluster)

	postAlreadyMigrateOutputDiff := strings.TrimPrefix(postAlreadyMigrateOutput, preAlreadyMigrateOutput)
	testutils.VerifySuccessOutputUnchanged(t, postAlreadyMigrateOutputDiff)

	actualPostAlreadyMigrateNode := new(corev1.Node)
	g.Expect(migratorEnv.migrator.WorkloadGet(ctx, cluster, nodeKey, actualPostAlreadyMigrateNode)).To(Succeed())
	g.Expect(actualPostAlreadyMigrateNode).To(testutils.BeDerivativeOf(expectedNode))
}
