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
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	metal "github.com/equinix-labs/metal-go/metal/v1"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/cluster-api-provider-packet/pkg/cloud/packet"
)

const (
	AuthTokenEnvVar = "PACKET_API_KEY" //nolint:gosec
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

			return cleanup(context.Background(), metalAuthToken, metalProjectID) //nolint:wrapcheck
		},
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func cleanup(ctx context.Context, metalAuthToken, metalProjectID string) error {
	metalClient := packet.NewClient(metalAuthToken)
	var errs []error

	devices, _, err := metalClient.DevicesApi.FindProjectDevices(ctx, metalProjectID).Execute()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	if err := deleteDevices(ctx, metalClient, *devices); err != nil {
		errs = append(errs, err)
	}

	ips, _, err := metalClient.IPAddressesApi.FindIPReservations(ctx, metalProjectID).Execute()
	if err != nil {
		return fmt.Errorf("failed to list ip addresses: %w", err)
	}

	if err := deleteIPs(ctx, metalClient, *ips); err != nil {
		errs = append(errs, err)
	}

	keys, _, err := metalClient.SSHKeysApi.FindProjectSSHKeys(ctx, metalProjectID).Execute()
	if err != nil {
		return fmt.Errorf("failed to list ssh keys: %w", err)
	}

	if err := deleteKeys(ctx, metalClient, *keys); err != nil {
		errs = append(errs, err)
	}

	return kerrors.NewAggregate(errs)
}

func deleteDevices(ctx context.Context, metalClient *packet.Client, devices metal.DeviceList) error {
	var errs []error

	for _, d := range devices.Devices {
		if time.Since(d.GetCreatedAt()) > 4*time.Hour {
			hostname := d.GetHostname()
			fmt.Printf("Deleting device: %s\n", hostname)
			_, err := metalClient.DevicesApi.DeleteDevice(ctx, d.GetId()).ForceDelete(false).Execute()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to delete device %q: %w", hostname, err))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}

func deleteIPs(ctx context.Context, metalClient *packet.Client, ips metal.IPReservationList) error {
	var errs []error

	for _, reservation := range ips.IpAddresses {
		// TODO: per the spec, `reservation` could be an `IPReservation` or a `VrfIpReservation`
		// maybe metal-go could define and we could move the if block to function that takes
		// that interface as an argument
		ip := reservation.IPReservation
		if ip != nil && time.Since(ip.GetCreatedAt()) > 4*time.Hour {
			for _, tag := range ip.Tags {
				if strings.HasPrefix(tag, "cluster-api-provider-packet:cluster-id:") || strings.HasPrefix(tag, "usage=cloud-provider-equinix-metal-auto") {
					fmt.Printf("Deleting IP: %s\n", ip.GetAddress())

					if _, err := metalClient.IPAddressesApi.DeleteIPAddress(ctx, ip.GetId()).Execute(); err != nil {
						errs = append(errs, fmt.Errorf("failed to delete ip address %q: %w", ip.GetAddress(), err))
					}

					break
				}
			}
		}
	}

	return kerrors.NewAggregate(errs)
}

func deleteKeys(ctx context.Context, metalClient *packet.Client, keys metal.SSHKeyList) error {
	var errs []error

	for _, k := range keys.SshKeys {
		if time.Since(k.GetCreatedAt()) > 4*time.Hour {
			fmt.Printf("Deleting SSH Key: %s\n", k.GetLabel())
			_, err := metalClient.SSHKeysApi.DeleteSSHKey(ctx, k.GetId()).Execute()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to delete SSH Key %q: %w", k.GetLabel(), err))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}
