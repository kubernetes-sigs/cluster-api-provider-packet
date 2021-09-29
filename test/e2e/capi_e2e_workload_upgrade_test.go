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
	"k8s.io/utils/pointer"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

var _ = Describe("[Workload Upgrade] Running the Cluster API E2E Workload Cluster Upgrade tests", func() {
	ctx := context.TODO()

	// The following upstream tests are not implemented because they are subsets of
	// capi_e2e.ClusterUpgradeConformanceSpec:
	// - capi_e2e.MachineDeploymentScaleSpec
	// - capi_e2e.MachineDeploymentRolloutSpec
	// - capi_e2e.KCPUpgradeSpec w/ ControlPlaneMachineCount = 1

	Context("Running the cluster-upgrade spec", func() {
		capi_e2e.ClusterUpgradeConformanceSpec(ctx, func() capi_e2e.ClusterUpgradeConformanceSpecInput {
			return capi_e2e.ClusterUpgradeConformanceSpecInput{
				E2EConfig:             e2eConfig,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapClusterProxy: bootstrapClusterProxy,
				ArtifactFolder:        artifactFolder,
				SkipCleanup:           skipCleanup,
				SkipConformanceTests:  true,
				Flavor:                pointer.String(clusterctl.DefaultFlavor),
			}
		})
	})

	Context("Running the kcp-upgrade spec with HA control plane", func() {
		capi_e2e.KCPUpgradeSpec(ctx, func() capi_e2e.KCPUpgradeSpecInput {
			return capi_e2e.KCPUpgradeSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				ControlPlaneMachineCount: 3,
				Flavor:                   clusterctl.DefaultFlavor,
			}
		})
	})

	Context("Running the kcp-upgrade spec with HA control plane and scale-in", func() {
		capi_e2e.KCPUpgradeSpec(ctx, func() capi_e2e.KCPUpgradeSpecInput {
			return capi_e2e.KCPUpgradeSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				SkipCleanup:              skipCleanup,
				ControlPlaneMachineCount: 3,
				Flavor:                   "kcp-scale-in",
			}
		})
	})
})
