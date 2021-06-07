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

package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/migrate"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/upgrade"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	config := new(base.ToolConfig)

	rootCmd := &cobra.Command{ //nolint:exhaustivestruct
		Use:          "capp-helper",
		Short:        "Helper utilties for CAPP",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help() //nolint:wrapcheck
		},
	}

	rootCmd.PersistentFlags().StringVar(&config.Kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
	rootCmd.PersistentFlags().StringVar(&config.Context, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	rootCmd.PersistentFlags().StringVar(&config.TargetNamespace, "target-namespace", "cluster-api-provider-packet-system",
		"The namespace where cluster-api-provider-packet is deployed.")
	rootCmd.PersistentFlags().StringVar(&config.WatchingNamespace, "watching-namespace", metav1.NamespaceAll,
		"The namespace where cluster-api-provider-packet is deployed.")
	rootCmd.PersistentFlags().BoolVar(&config.DryRun, "dry-run", false, "Dry run.")

	rootCmd.AddCommand((&migrate.Command{ToolConfig: config}).Command())
	rootCmd.AddCommand((&upgrade.Command{ToolConfig: config}).Command())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
