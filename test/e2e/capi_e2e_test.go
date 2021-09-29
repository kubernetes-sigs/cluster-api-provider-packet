// +build e2e

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

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
)

var _ = Describe("[General] Running the Cluster API E2E tests", func() {
	ctx := context.TODO()

	// The following upstream tests are not implemented because we do not support the features
	// tested:
	// - capi_e2e.KCPAdoptionSpec
	// - capi_e2e.MachinePoolSpec

	// The following upstream tests are not implemented because they are subsets of
	// capi_e2e.ClusterUpgradeConformanceSpec:
	// - capi_e2e.MachineDeploymentScaleSpec
	// - capi_e2e.MachineDeploymentRolloutSpec
	// - capi_e2e.KCPUpgradeSpec w/ ControlPlaneMachineCount = 1

	Context("Running the mhc-remediation spec", func() {
		capi_e2e.MachineRemediationSpec(ctx, func() capi_e2e.MachineRemediationSpecInput {
			return capi_e2e.MachineRemediationSpecInput{
				E2EConfig:             e2eConfig,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapClusterProxy: bootstrapClusterProxy,
				ArtifactFolder:        artifactFolder,
				SkipCleanup:           skipCleanup,
			}
		})
	})

	Context("Running the node-drain-timeout spec", func() {
		capi_e2e.NodeDrainTimeoutSpec(ctx, func() capi_e2e.NodeDrainTimeoutSpecInput {
			return capi_e2e.NodeDrainTimeoutSpecInput{
				E2EConfig:             e2eConfig,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapClusterProxy: bootstrapClusterProxy,
				ArtifactFolder:        artifactFolder,
				SkipCleanup:           skipCleanup,
			}
		})
	})
})
