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
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/migrate"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/upgrade"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	var kubeconfig string

	rootCmd := &cobra.Command{ //nolint:exhaustivestruct
		Use:          "capp-helper",
		Short:        "Helper utilties for CAPP",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help() //nolint:wrapcheck
		},
	}

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
	rootCmd.AddCommand((&migrate.Command{KubeConfig: &kubeconfig}).Command())
	rootCmd.AddCommand((&upgrade.Command{KubeConfig: &kubeconfig}).Command())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
