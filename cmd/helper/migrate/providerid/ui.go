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
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/helper/migrate/providerid/migrator"
)

const (
	fps      = 60
	padding  = 2
	maxWidth = 80
)

func cleanQuit() tea.Msg {
	// This is to ensure that the buffers are flushed to stdout prior to exiting
	time.Sleep(time.Second)

	return tea.Quit()
}

func initialModel(ctx context.Context, kubeconfig *string) (model, error) {
	migrator, err := migrator.NewMigrator(ctx, kubeconfig)
	if err != nil {
		return model{}, fmt.Errorf("failed to initialize migration utility: %w", err)
	}

	prog, err := progress.NewModel(progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))
	if err != nil {
		return model{}, fmt.Errorf("failed to generate progress bar: %w", err)
	}

	return model{ //nolint:exhaustivestruct
		migrator: migrator,
		progress: prog,
	}, nil
}

func (m model) runMigration() tea.Msg {
	m.migrator.Run(context.TODO())

	return cleanQuit()
}

func (m model) checkPrerequisites() tea.Msg {
	return m.migrator.CheckPrerequisites(context.TODO()) //nolint: wrapcheck
}

type model struct {
	migrator *migrator.Migrator
	progress *progress.Model
	percent  float64
	err      error
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.Sequentially(m.checkPrerequisites, m.runMigration), tickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			cmd = cleanQuit
		}
	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - padding*2 - 4 //nolint: gomnd
		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}

		return m, nil
	case error:
		m.err = msg
		cmd = cleanQuit
	default:
		m.percent = m.migrator.CalculatePercentage()
		cmd = tickCmd()
	}

	return m, cmd
}

func (m model) View() string {
	// TODO: handle window size detection and text wrapping
	errorColor := lipgloss.Color("#dc322f")
	infoColor := lipgloss.Color("#859900")
	headingColor := lipgloss.Color("#268bd2")
	clusterStyle := lipgloss.NewStyle().Foreground(headingColor)
	outputHeadings := clusterStyle.Copy().PaddingLeft(4)                           //nolint:gomnd
	clusterOutputStyle := lipgloss.NewStyle().PaddingLeft(8).Foreground(infoColor) //nolint:gomnd
	clusterErrorStyle := lipgloss.NewStyle().PaddingLeft(4).Foreground(errorColor) //nolint:gomnd

	s := "CAPP ProviderID Migration\n\n"

	s += m.progress.View(m.percent) + "\n\n"

	if m.err != nil {
		s += lipgloss.NewStyle().Foreground(errorColor).Render(fmt.Sprintf("Error: %s", m.err.Error())) + "\n"
	}

	for i := range m.migrator.Clusters {
		c := m.migrator.Clusters[i]
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)
		s += clusterStyle.Render(fmt.Sprintf("Cluster %s:", outputKey)) + "\n"

		out := m.migrator.GetOutputFor(c)

		if len(out) > 0 {
			s += outputHeadings.Render("Output:") + "\n"
			s += clusterOutputStyle.Render(out) + "\n"
		}

		if err := m.migrator.GetErrorFor(c); err != nil {
			s += clusterErrorStyle.Render(fmt.Sprintf("Error: %s", err.Error())) + "\n"
		}
	}

	return s
}
