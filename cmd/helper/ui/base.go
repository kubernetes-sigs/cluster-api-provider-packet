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

package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

const (
	fps      = 60
	padding  = 2
	maxWidth = 80
)

type Tool interface {
	CalculatePercentage() float64
	GetClusters(context.Context) ([]*clusterv1.Cluster, error)
	GetOutputFor(*clusterv1.Cluster) string
	GetErrorFor(*clusterv1.Cluster) error
	Run(context.Context)
	CheckPrerequisites(context.Context) error
}

func NewModel(title string, tool Tool) (Model, error) {
	prog, err := progress.NewModel(progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))
	if err != nil {
		return Model{}, fmt.Errorf("failed to generate progress bar: %w", err)
	}

	return Model{
		title:    title,
		Tool:     tool,
		progress: prog,
	}, nil
}

type Model struct {
	title    string
	Tool     Tool
	progress *progress.Model
	percent  float64
	err      error
	height   int
	width    int
}

type TickMsg time.Time

func TickCmd() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m Model) runTool() tea.Msg {
	m.Tool.Run(context.TODO())

	return cleanQuit()
}

func (m Model) checkPrerequisites() tea.Msg {
	return m.Tool.CheckPrerequisites(context.TODO()) //nolint: wrapcheck
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.Sequentially(m.checkPrerequisites, m.runTool), TickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			cmd = cleanQuit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - padding*2 - 4 //nolint: gomnd

		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}

		return m, nil
	case error:
		m.err = msg
		cmd = cleanQuit
	default:
		m.percent = m.Tool.CalculatePercentage()
		cmd = TickCmd()
	}

	return m, cmd
}

func (m Model) View() string {
	errorColor := lipgloss.Color("#dc322f")
	infoColor := lipgloss.Color("#859900")
	headingColor := lipgloss.Color("#268bd2")
	clusterStyle := lipgloss.NewStyle().Foreground(headingColor)
	outputHeadings := clusterStyle.Copy().PaddingLeft(4)                           //nolint:gomnd
	clusterOutputStyle := lipgloss.NewStyle().PaddingLeft(8).Foreground(infoColor) //nolint:gomnd
	clusterErrorStyle := lipgloss.NewStyle().PaddingLeft(4).Foreground(errorColor) //nolint:gomnd

	s := m.title + "\n\n"

	s += m.progress.View(m.percent) + "\n\n"

	if m.err != nil {
		s += lipgloss.NewStyle().Foreground(errorColor).Render(fmt.Sprintf("Error: %s", m.err.Error())) + "\n"
	}

	clusters, err := m.Tool.GetClusters(context.TODO())
	if err != nil {
		s += lipgloss.NewStyle().Foreground(errorColor).Render(fmt.Sprintf("Error: %s", err.Error())) + "\n"

		return wordwrap.String(s, m.width)
	}

	for i := range clusters {
		c := clusters[i]
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)
		s += clusterStyle.Render(fmt.Sprintf("Cluster %s:", outputKey)) + "\n"

		out := m.Tool.GetOutputFor(c)

		if len(out) > 0 {
			s += outputHeadings.Render("Output:") + "\n"
			s += clusterOutputStyle.Render(out) + "\n"
		}

		if err := m.Tool.GetErrorFor(c); err != nil {
			s += clusterErrorStyle.Render(fmt.Sprintf("Error: %s", err.Error())) + "\n"
		}
	}

	return wordwrap.String(s, m.width)
}

func cleanQuit() tea.Msg {
	// This is to ensure that the buffers are flushed to stdout prior to exiting
	time.Sleep(time.Second)

	return tea.Quit()
}
