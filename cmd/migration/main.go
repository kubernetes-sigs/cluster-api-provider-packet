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
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cluster-api-provider-packet/cmd/migration/util"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

var (
	kubeconfig             string                 //nolint:gochecknoglobals
	migrationOutputBuffers *util.OutputBuffers    //nolint:gochecknoglobals
	migrationOutput        *util.OutputCollection //nolint:gochecknoglobals
	migrationErrors        *util.ErrorCollection  //nolint:gochecknoglobals
	clusters               []clusterv1.Cluster    //nolint:gochecknoglobals
	buffMutex              sync.Mutex             //nolint:gochecknoglobals
)

func copyBuffers() {
	buffMutex.Lock()
	defer buffMutex.Unlock()

	for _, c := range clusters {
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)

		out, err := ioutil.ReadAll(migrationOutputBuffers.Get(outputKey))
		if err != nil {
			continue
		}

		migrationOutput.Append(outputKey, string(out))
	}
}

func runMigration() tea.Msg {
	mgmtClient, err := util.GetManagementClient(kubeconfig)
	if err != nil {
		return fatalErr(fmt.Errorf("failed to create client for management cluster: %w", err))
	}

	clusterList := &clusterv1.ClusterList{} //nolint:exhaustivestruct
	if err := mgmtClient.List(context.TODO(), clusterList); err != nil {
		return fatalErr(fmt.Errorf("failed to list workload clusters in management cluster: %w", err))
	}

	var wg sync.WaitGroup

	clusters = clusterList.Items
	migrationOutputBuffers, migrationErrors = util.RunMigration(context.TODO(), mgmtClient, clusterList.Items, &wg)
	migrationOutput = util.NewOutputCollection(len(clusters))

	wg.Wait()

	return cleanQuit()
}

type model struct {
	spinner spinner.Model
	err     error
}

type fatalErr error

func cleanQuit() tea.Msg {
	copyBuffers()

	// This is to ensure that the buffers are flushed to stdout prior to exiting
	time.Sleep(time.Second)

	return tea.Quit()
}

func initialModel() model {
	s := spinner.NewModel()
	s.Spinner = spinner.Pulse
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return model{spinner: s} //nolint:exhaustivestruct
}

func (m model) Init() tea.Cmd {
	return tea.Batch(runMigration, spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			cmd = cleanQuit
		}
	case fatalErr:
		m.err = msg
		cmd = cleanQuit
	default:
		copyBuffers()

		m.spinner, cmd = m.spinner.Update(msg)
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

	s := "CAPP Migration\n\n"

	s += fmt.Sprintf("%s Running...\n\n", m.spinner.View())

	if m.err != nil {
		s += fmt.Sprintf("Error: %v\n", m.err)
	}

	for _, c := range clusters {
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)
		s += clusterStyle.Render(fmt.Sprintf("Cluster %s:", outputKey)) + "\n"

		out := migrationOutput.Get(outputKey)

		if len(out) > 0 {
			s += outputHeadings.Render("Output:") + "\n"
			s += clusterOutputStyle.Render(out) + "\n"
		}

		if err, ok := migrationErrors.Load(outputKey); ok {
			s += clusterErrorStyle.Render(fmt.Sprintf("Error: %s", err.Error())) + "\n"
		}
	}

	return s
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	var showHelp bool

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flag.StringVar(&kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
	flag.BoolVar(&showHelp, "h", false, "show help")
	flag.Parse()

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	m := initialModel()

	p := tea.NewProgram(m)
	if err := p.Start(); err != nil {
		fmt.Println("Error starting Bubble Tea program:", err) //nolint: forbidigo
		os.Exit(1)
	}
}
