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

package migrate

import (
	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/migrate/kubevip"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/migrate/providerid"
)

type Command struct {
	ToolConfig *base.ToolConfig
}

func (c *Command) Command() *cobra.Command {
	migrateCmd := &cobra.Command{ //nolint:exhaustivestruct
		Use:   "migrate",
		Short: "Migration utilities for CAPP",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help() //nolint:wrapcheck
		},
	}

	migrateCmd.AddCommand((&providerid.Command{ToolConfig: c.ToolConfig}).Command())
	migrateCmd.AddCommand((&kubevip.Command{ToolConfig: c.ToolConfig}).Command())

	return migrateCmd
}
