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
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/packethost/packngo"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	AuthTokenEnvVar = "PACKET_API_KEY" // nolint:gosec
	ProjectIDEnvVar = "PROJECT_ID"
)

var ErrMissingRequiredEnvVar = errors.New("required environment variable not set")

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	rootCmd := &cobra.Command{ //nolint:exhaustivestruct
		Use:   "ci-clean",
		Short: "Clean up any stray resources in CI",
		RunE: func(cmd *cobra.Command, args []string) error {
			metalAuthToken := os.Getenv(AuthTokenEnvVar)
			if metalAuthToken == "" {
				return fmt.Errorf("%s: %w", AuthTokenEnvVar, ErrMissingRequiredEnvVar)
			}

			metalProjectID := os.Getenv(ProjectIDEnvVar)
			if metalProjectID == "" {
				return fmt.Errorf("%s: %w", ProjectIDEnvVar, ErrMissingRequiredEnvVar)
			}

			return cleanup(metalAuthToken, metalProjectID) //nolint:wrapcheck
		},
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func cleanup(metalAuthToken, metalProjectID string) error {
	metalClient := packngo.NewClientWithAuth("capp-e2e", metalAuthToken, nil)
	listOpts := &packngo.ListOptions{}

	devices, _, err := metalClient.Devices.List(metalProjectID, listOpts)
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	var errs []error

	if err := deleteDevices(metalClient, devices); err != nil {
		errs = append(errs, err)
	}

	ips, _, err := metalClient.ProjectIPs.List(metalProjectID, listOpts)
	if err != nil {
		return fmt.Errorf("failed to list ip addresses: %w", err)
	}

	if err := deleteIPs(metalClient, ips); err != nil {
		errs = append(errs, err)
	}

	return kerrors.NewAggregate(errs)
}

func deleteDevices(metalClient *packngo.Client, devices []packngo.Device) error {
	var errs []error

	for _, d := range devices {
		created, err := time.Parse(time.RFC3339, d.Created)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse creation time for device %q: %w", d.Hostname, err))
			continue
		}
		if time.Since(created) > 4*time.Hour {
			fmt.Printf("Deleting device: %s\n", d.Hostname)
			_, err := metalClient.Devices.Delete(d.ID, false)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to delete device %q: %w", d.Hostname, err))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}

func deleteIPs(metalClient *packngo.Client, ips []packngo.IPAddressReservation) error {
	var errs []error

	for _, ip := range ips {
		if len(ip.Tags) < 1 || len(ip.Tags) > 1 || !strings.HasPrefix(ip.Tags[0], "cluster-api-provider-packet:cluster-id:") {
			continue
		}

		created, err := time.Parse(time.RFC3339, ip.Created)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse creation time for ip address %q: %w", ip.Address, err))
			continue
		}

		if time.Since(created) > 4*time.Hour {
			fmt.Printf("Deleting IP: %s\n", ip.Address)
			_, err := metalClient.ProjectIPs.Remove(ip.ID)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to delete ip address %q: %w", ip.Address, err))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}
