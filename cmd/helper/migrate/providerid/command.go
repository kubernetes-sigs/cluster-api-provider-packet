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

package providerid

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/base"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/migrate/providerid/migrator"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/ui"
)

type Command struct {
	ToolConfig *base.ToolConfig
}

func (c *Command) Command() *cobra.Command {
	return &cobra.Command{ //nolint:exhaustivestruct
		Use:   "providerid",
		Short: "ProviderID migration utility for CAPP",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.RunE()
		},
	}
}

func (c *Command) RunE() error {
	migrator, err := migrator.New(context.TODO(), c.ToolConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize migration utility: %w", err)
	}

	m, err := ui.NewModel("CAPP ProviderID Migration", migrator)
	if err != nil {
		return err
	}

	p := tea.NewProgram(m)

	if err := p.Start(); err != nil {
		return fmt.Errorf("failed to start UI: %w", err)
	}

	return nil
}