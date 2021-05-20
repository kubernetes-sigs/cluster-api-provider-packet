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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	kubeconfig   string            //nolint:gochecknoglobals
	migrationCmd = &cobra.Command{ //nolint:exhaustivestruct,gochecknoglobals
		Use:          "migration",
		SilenceUsage: true,
		Short:        "migration is used to handle migration tasks for cluster-api-provider-packet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigration(context.TODO())
		},
	}
)

func getManagementClient(kubeconfig string) (client.Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{} //nolint:exhaustivestruct
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client configuration for management cluster: %w", err)
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	return client.New(config, client.Options{Scheme: scheme}) //nolint:exhaustivestruct,wrapcheck
}

func runMigration(ctx context.Context) error {
	mgmtClient, err := getManagementClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create client for management cluster: %w", err)
	}

	clusterList := &clusterv1.ClusterList{} //nolint:exhaustivestruct
	if err := mgmtClient.List(ctx, clusterList); err != nil {
		return fmt.Errorf("failed to list workload clusters in management cluster: %w", err)
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	migrationOutput := make(map[string]*bytes.Buffer, len(clusterList.Items))
	migrationErrors := make(map[string]error, len(clusterList.Items))

	for i, c := range clusterList.Items {
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)

		clusterKey, err := client.ObjectKeyFromObject(&clusterList.Items[i])
		if err != nil {
			mu.Lock()
			migrationErrors[outputKey] = fmt.Errorf("failed to create object key: %w", err)
			mu.Unlock()

			continue
		}

		var buf bytes.Buffer

		mu.Lock()
		migrationOutput[outputKey] = &buf
		mu.Unlock()
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := migrateWorkloadCluster(context.TODO(), clusterKey, mgmtClient, &buf); err != nil {
				mu.Lock()
				migrationErrors[outputKey] = err
				mu.Unlock()

				return
			}
		}()
	}

	wg.Wait()

	if outputResults(clusterList.Items, migrationOutput, migrationErrors) {
		return fmt.Errorf("failed to run migration to completion") //nolint:goerr113
	}

	return nil
}

func outputResults(
	clusters []clusterv1.Cluster,
	migrationOutput map[string]*bytes.Buffer,
	migrationErrors map[string]error,
) bool {
	doc := strings.Builder{}
	physicalWidth, _, _ := term.GetSize(int(os.Stdout.Fd()))
	errorColor := lipgloss.Color("#dc322f")
	infoColor := lipgloss.Color("#859900")
	headingColor := lipgloss.Color("#268bd2")
	docStyle := lipgloss.NewStyle().Padding(1, 2, 1, 2)
	clusterStyle := lipgloss.NewStyle().Foreground(headingColor)
	outputHeadings := clusterStyle.Copy().PaddingLeft(4)                           //nolint:gomnd
	clusterOutputStyle := lipgloss.NewStyle().PaddingLeft(8).Foreground(infoColor) //nolint:gomnd
	clusterErrorStyle := lipgloss.NewStyle().PaddingLeft(4).Foreground(errorColor) //nolint:gomnd

	encounteredErrors := false

	for _, c := range clusters {
		outputKey := fmt.Sprintf("%s/%s", c.Namespace, c.Name)
		doc.WriteString(clusterStyle.Render(fmt.Sprintf("Cluster %s:", outputKey)) + "\n")

		out, err := ioutil.ReadAll(migrationOutput[outputKey])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading output for cluster %s: %v\n", outputKey, err)
		}

		if len(out) > 0 {
			doc.WriteString(outputHeadings.Render("Output:") + "\n")
			doc.WriteString(clusterOutputStyle.Render(string(out)) + "\n")
		}

		if err, ok := migrationErrors[outputKey]; ok {
			encounteredErrors = true

			doc.WriteString(clusterErrorStyle.Render(fmt.Sprintf("Error: %s", err.Error())) + "\n\n")
		}
	}

	if physicalWidth > 0 {
		docStyle = docStyle.MaxWidth(physicalWidth)
	} else if physicalWidth < 0 {
		docStyle = docStyle.MaxWidth(80) //nolint:gomnd
	}

	fmt.Println(docStyle.Render(doc.String())) //nolint:forbidigo

	return encounteredErrors
}

func migrateWorkloadCluster(
	ctx context.Context,
	clusterKey client.ObjectKey,
	mgmtClient client.Client,
	stdout io.Writer,
) error {
	c, err := remote.NewClusterClient(ctx, mgmtClient, clusterKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	nodeList := &corev1.NodeList{} //nolint:exhaustivestruct
	if err := c.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodeList.Items {
		// TODO: should this stop at first error, or attempt to continue?
		// TODO: should probably give some additional safety to users since this will be
		// deleting and re-creating Node resources
		if err := migrateNode(ctx, node, c, stdout); err != nil {
			return err
		}
	}

	return nil
}

func migrateNode(ctx context.Context, node corev1.Node, workloadClient client.Client, stdout io.Writer) error {
	if strings.HasPrefix(node.Spec.ProviderID, "equinixmetal") {
		fmt.Fprintf(stdout, "✔ Node %s already has the updated providerID\n", node.Name)

		return nil
	}

	if err := workloadClient.Delete(ctx, &node); err != nil {
		return fmt.Errorf("failed to delete existing node resource: %w", err)
	}

	node.SetResourceVersion("")
	node.Spec.ProviderID = strings.Replace(node.Spec.ProviderID, "packet", "equinixmetal", 1)

	if err := retry.OnError(
		retry.DefaultRetry,
		func(err error) bool {
			return true
		},
		func() error {
			return workloadClient.Create(ctx, &node) //nolint:wrapcheck
		},
	); err != nil {
		return fmt.Errorf("failed to create replacement node resource: %w", err)
	}

	fmt.Fprintf(stdout, "✅ Node %s has been successfully migrated\n", node.Name)

	return nil
}

func init() { //nolint:gochecknoinits
	migrationCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
}

func main() {
	if err := migrationCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
