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

	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

const (
	fps        = 60
	padding    = 2
	maxWidth   = 80
	errorColor = lipgloss.Color("#dc322f")
)

type Tool interface {
	CalculatePercentage() float64
	GetClusters(context.Context) ([]*clusterv1.Cluster, error)
	GetOutputFor(*clusterv1.Cluster) string
	GetErrorFor(*clusterv1.Cluster) error
	Run(context.Context)
	CheckPrerequisites(context.Context) error
}

func NewModel(title string, tool Tool, tuiEnabled bool) (Model, error) {
	prog, err := progress.NewModel(progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))
	if err != nil {
		return Model{}, fmt.Errorf("failed to generate progress bar: %w", err)
	}

	p := paginator.NewModel()

	return Model{ //nolint:exhaustivestruct
		title:      title,
		Tool:       tool,
		progress:   prog,
		paginator:  p,
		tuiEnabled: tuiEnabled,
		viewport:   viewport.Model{}, //nolint:exhaustivestruct
	}, nil
}

type Model struct {
	title      string
	Tool       Tool
	progress   *progress.Model
	paginator  paginator.Model
	viewport   viewport.Model
	percent    float64
	err        error
	height     int
	width      int
	finished   bool
	tuiEnabled bool
}

type progressTick time.Time

type finishedMsg string

func progressTickCmd() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg {
		return progressTick(t)
	})
}

func (m *Model) runTool() tea.Msg {
	m.Tool.Run(context.TODO())

	return finishedMsg("")
}

func (m *Model) checkPrerequisites() tea.Msg {
	return m.Tool.CheckPrerequisites(context.TODO()) //nolint: wrapcheck
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.Sequentially(m.checkPrerequisites, m.runTool), progressTickCmd())
}

func (m *Model) updateViewport() {
	infoColor := lipgloss.Color("#859900")
	headingColor := lipgloss.Color("#268bd2")
	clusterStyle := lipgloss.NewStyle().Foreground(headingColor)
	outputHeadings := clusterStyle.Copy().PaddingLeft(4)                           //nolint:gomnd
	clusterOutputStyle := lipgloss.NewStyle().PaddingLeft(8).Foreground(infoColor) //nolint:gomnd
	clusterErrorStyle := lipgloss.NewStyle().PaddingLeft(4).Foreground(errorColor) //nolint:gomnd

	body := ""

	clusters, err := m.Tool.GetClusters(context.TODO())
	if err != nil {
		body += lipgloss.NewStyle().Foreground(errorColor).Render(fmt.Sprintf("Error: %s", err.Error())) + "\n"
	}

	start, end := m.paginator.GetSliceBounds(len(clusters))

	for _, c := range clusters[start:end] {
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)
		body += clusterStyle.Render(fmt.Sprintf("Cluster %s:", outputKey)) + "\n"

		out := m.Tool.GetOutputFor(c)

		if len(out) > 0 {
			body += outputHeadings.Render("Output:") + "\n"
			body += clusterOutputStyle.Render(out) + "\n"
		}

		if err := m.Tool.GetErrorFor(c); err != nil {
			body += clusterErrorStyle.Render(fmt.Sprintf("Error: %s", err.Error())) + "\n"
		}
	}

	body = wordwrap.String(body, m.viewport.Width)
	m.viewport.SetContent(body)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case finishedMsg:
		if !m.tuiEnabled {
			// if the TUI is disabled, we want to quit immediately instead of waiting
			// for the user to exit
			cmd = tea.Quit
		}

		m.finished = true
	case error:
		m.err = msg

		if m.err != nil {
			m.finished = true
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 9           //nolint: gomnd
		m.progress.Width = msg.Width - padding*2 - 4 //nolint: gomnd

		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}

		return m, nil
	case progressTick:
		m.updateTick()

		cmd = progressTickCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			cmd = tea.Quit

		case "q", "esc":
			if m.finished {
				cmd = tea.Quit
			}

		default:
			m.updatePaginatorAndViewPort(msg)
		}
	}

	m.updateViewport()

	return m, cmd
}

func (m *Model) updateTick() {
	m.percent = m.Tool.CalculatePercentage()
	clusters, _ := m.Tool.GetClusters(context.TODO())
	m.paginator.SetTotalPages(len(clusters))
}

func (m *Model) updatePaginatorAndViewPort(msg tea.KeyMsg) { //nolint:cyclop
	pageBefore := m.paginator.Page //nolint:ifshort

	// previous page: a, left, shift+tab
	// next page: d, right, tab
	// up one line: w, up
	// down one line: s, down
	// up one page: PageUp
	// down one page: PageDown
	// go to top: Home
	// go to end: End

	switch msg.Type { //nolint: exhaustive
	case tea.KeyTab, tea.KeyRight:
		m.paginator.NextPage()
	case tea.KeyShiftTab, tea.KeyLeft:
		m.paginator.PrevPage()
	case tea.KeyHome:
		m.viewport.GotoTop()
	case tea.KeyEnd:
		m.viewport.GotoBottom()
	case tea.KeyPgUp:
		m.viewport.ViewUp()
	case tea.KeyPgDown:
		m.viewport.ViewDown()
	case tea.KeyUp:
		m.viewport.LineUp(1)
	case tea.KeyDown:
		m.viewport.LineDown(1)
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "w":
			m.viewport.LineUp(1)
		case "a":
			m.paginator.PrevPage()
		case "s":
			m.viewport.LineDown(1)
		case "d":
			m.paginator.NextPage()
		}
	}

	// If we've changed pages, update the viewport and reset the view to the end
	if m.paginator.Page != pageBefore {
		m.updateViewport()
		m.viewport.GotoBottom()
	}
}

func (m Model) View() string {
	footerStyle := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center)

	header := fmt.Sprintf("%s\n\n%s\n\n", m.title, m.progress.View(m.percent))

	if m.err != nil {
		header += lipgloss.NewStyle().Foreground(errorColor).Render(fmt.Sprintf("Error: %s", m.err.Error())) + "\n"
	} else {
		header += "\n"
	}

	scrollPercent := fmt.Sprintf("%3.f%%\n", m.viewport.ScrollPercent()*100)
	if m.viewport.AtBottom() && m.viewport.AtTop() {
		// Do not show scroll percentage if everything fits on a single page
		scrollPercent = "\n"
	}

	// TODO: if m.finished is true, output quit instructions
	// TODO: add instructions for navigation
	footer := footerStyle.Render(scrollPercent + m.paginator.View() + "\n")

	return header + m.viewport.View() + "\n" + footer
}
