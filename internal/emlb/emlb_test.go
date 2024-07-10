/*
Copyright 2024 The Kubernetes Authors.

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

// Package emlb manages authentication to the Equinix Metal Load Balancer service.
package emlb

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
)

func Test_getResourceName(t *testing.T) {
	g := NewWithT(t)
	loadBalancerName := "my-loadbalancer"
	resourceType := "pool"
	want := "my-loadbalancer-pool"

	got := getResourceName(loadBalancerName, resourceType)

	// assert name is correct
	g.Expect(got).To(Equal(want))
}

func Test_checkDebugEnabled(t *testing.T) {
	g := NewWithT(t)
	// Set the PACKNGO_DEBUG environment variable to enable debug mode
	if err := os.Setenv("PACKNGO_DEBUG", "true"); err != nil {
		t.Errorf("Error testing checkDebugEnabled: %v", err)
	}

	// Call the checkDebugEnabled function
	debugEnabled := checkDebugEnabled()

	// Check if debugEnabled is true
	g.Expect(debugEnabled).To(BeTrue())

	// Unset the PACKNGO_DEBUG environment variable
	os.Unsetenv("PACKNGO_DEBUG")

	// Call the checkDebugEnabled function again
	debugEnabled = checkDebugEnabled()

	// Check if debugEnabled is false
	g.Expect(debugEnabled).To(BeFalse())
}

func Test_convertToTarget(t *testing.T) {
	type args struct {
		devaddr corev1.NodeAddress
	}
	tests := []struct {
		name string
		args args
		want *Target
	}{
		{
			name: "Internal IP",
			args: args{
				corev1.NodeAddress{
					Type:    "InternalIP",
					Address: "10.2.1.5",
				},
			},
			want: &Target{
				IP:   "10.2.1.5",
				Port: loadBalancerVIPPort,
			},
		},
		{
			name: "External IP",
			args: args{
				corev1.NodeAddress{
					Type:    "ExternalIP",
					Address: "1.2.3.4",
				},
			},
			want: &Target{
				IP:   "1.2.3.4",
				Port: loadBalancerVIPPort,
			},
		},
		{
			name: "Empty IP",
			args: args{
				corev1.NodeAddress{
					Type:    "ExternalIP",
					Address: "",
				},
			},
			want: &Target{
				IP:   "",
				Port: loadBalancerVIPPort,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := convertToTarget(tt.args.devaddr)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_getExternalIPv4Target(t *testing.T) {
	type args struct {
		deviceAddr []corev1.NodeAddress
	}
	tests := []struct {
		name    string
		args    args
		want    *Target
		wantErr bool
	}{
		{
			name: "Single Valid External Address",
			args: args{
				[]corev1.NodeAddress{
					{
						Type:    "InternalIP",
						Address: "10.2.1.5",
					},
					{
						Type:    "ExternalIP",
						Address: "",
					},
					{
						Type:    "ExternalIP",
						Address: "1.2.3.4",
					},
				},
			},
			want: &Target{
				IP:   "1.2.3.4",
				Port: loadBalancerVIPPort,
			},
		},
		{
			name: "Single Invalid External Address",
			args: args{
				[]corev1.NodeAddress{{
					Type:    "ExternalIP",
					Address: "ffff::0",
				}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := getExternalIPv4Target(tt.args.deviceAddr)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}
func TestNewEMLB(t *testing.T) {
	g := NewWithT(t)
	metalAPIKey := "metal-api-key" //nolint:gosec
	projectID := "project-id"
	metro := "am"

	emlb := NewEMLB(metalAPIKey, projectID, metro)

	// assert client is not nil
	g.Expect(emlb.client).ToNot(BeNil())

	// assert tokenExchanger is not nil
	g.Expect(emlb.TokenExchanger).ToNot(BeNil())

	// assert project ID is correct
	g.Expect(emlb.projectID).To(Equal(projectID))

	// assert metro is correct
	g.Expect(emlb.metro).To(Equal(metro))
}
