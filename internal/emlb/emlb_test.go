// Package emlb manages authentication to the Equinix Metal Load Balancer service.
package emlb

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

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
			got, err := getExternalIPv4Target(tt.args.deviceAddr)
			if (err != nil) != tt.wantErr {
				t.Errorf("getExternalIPv4Target() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getExternalIPv4Target() = %v, want %v", got, tt.want)
			}
		})
	}
}
