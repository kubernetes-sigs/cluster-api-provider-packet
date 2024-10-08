package layer2

import (
	"testing"
	"gopkg.in/yaml.v3"
	"strings"
	"reflect"
)

func TestStripHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		headers  configHeaders
	}{
		{
			name:     "Both headers present",
			input:    "## template: jinja\n#cloud-config\ncontent: here",
			expected: "content: here",
			headers:  configHeaders{hasJinja: true, hasCloudConfig: true},
		},
		{
			name:     "Only cloud-config header",
			input:    "#cloud-config\ncontent: here",
			expected: "content: here",
			headers:  configHeaders{hasJinja: false, hasCloudConfig: true},
		},
		{
			name:     "No headers",
			input:    "content: here",
			expected: "content: here",
			headers:  configHeaders{hasJinja: false, hasCloudConfig: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, headers := stripHeaders(tt.input)
			if content != tt.expected {
				t.Errorf("Expected content %q, got %q", tt.expected, content)
			}
			if headers != tt.headers {
				t.Errorf("Expected headers %v, got %v", tt.headers, headers)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	merger := NewCloudConfigMerger()

	tests := []struct {
		name     string
		base     map[string]interface{}
		overlay  map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "Merge simple maps with slice creation",
			base: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
			overlay: map[string]interface{}{
				"b": 3,
				"c": 4,
			},
			expected: map[string]interface{}{
				"a": 1,
				"b": []interface{}{2, 3},
				"c": 4,
			},
		},
		{
			name: "Merge nested maps",
			base: map[string]interface{}{
				"a": map[string]interface{}{
					"x": 1,
					"y": 2,
				},
			},
			overlay: map[string]interface{}{
				"a": map[string]interface{}{
					"y": 3,
					"z": 4,
				},
			},
			expected: map[string]interface{}{
				"a": map[string]interface{}{
					"x": 1,
					"y": []interface{}{2, 3},
					"z": 4,
				},
			},
		},
		{
			name: "Merge existing slices",
			base: map[string]interface{}{
				"a": []interface{}{1, 2},
			},
			overlay: map[string]interface{}{
				"a": []interface{}{3, 4},
			},
			expected: map[string]interface{}{
				"a": []interface{}{1, 2, 3, 4},
			},
		},
		{
			name: "Merge slice with non-slice",
			base: map[string]interface{}{
				"a": []interface{}{1, 2},
			},
			overlay: map[string]interface{}{
				"a": 3,
			},
			expected: map[string]interface{}{
				"a": []interface{}{1, 2, 3},
			},
		},
		{
			name: "two cloud configs",
			base: map[string]interface{}{
				"write_files": []interface{}{
					map[string]interface{}{
						"path":        "/etc/hostname",
						"permissions": "0644",
						"owner":       "root",
						"content":     "node-{{ node_index }}",
					},
				},
			},
			overlay: map[string]interface{}{
				"write_files": []interface{}{
					map[string]interface{}{
						"path":        "/var/lib/cloud/instance/hostname",
						"permissions": "0644",
						"owner":       "root",
						"content":     "node-{{ node_index }}",
					},
					map[string]interface{}{
						"path":        "/etc/hosts",
						"permissions": "0644",
						"owner":       "root",
						"content":     "xyz",
					},
				},
			},
			expected: map[string]interface{}{
				"write_files": []interface{}{
					map[string]interface{}{
						"path":        "/etc/hostname",
						"permissions": "0644",
						"owner":       "root",
						"content":     "node-{{ node_index }}",
					},
					map[string]interface{}{
						"path":        "/var/lib/cloud/instance/hostname",
						"permissions": "0644",
						"owner":       "root",
						"content":     "node-{{ node_index }}",
					},
					map[string]interface{}{
						"path":        "/etc/hosts",
						"permissions": "0644",
						"owner":       "root",
						"content":     "xyz",
					},
				},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := merger.deepMerge(tt.base, tt.overlay)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMergeConfigs(t *testing.T) {
	merger := NewCloudConfigMerger()

	tests := []struct {
		name          string
		bootstrap     string
		layer2        string
		expectedYAML  map[string]interface{}
		expectedError bool
	}{
		{
			name:      "Merge valid configs",
			bootstrap: "#cloud-config\nbootstrap: data",
			layer2:    "#cloud-config\nlayer2: data",
			expectedYAML: map[string]interface{}{
				"bootstrap": "data",
				"layer2":    "data",
			},
			expectedError: false,
		},
		{
			name:          "No cloud-config header",
			bootstrap:     "bootstrap: data",
			layer2:        "layer2: data",
			expectedYAML:  nil,
			expectedError: true,
		},
		{
			name:      "Merge with Jinja header",
			bootstrap: "## template: jinja\n#cloud-config\nbootstrap: data",
			layer2:    "#cloud-config\nlayer2: data",
			expectedYAML: map[string]interface{}{
				"bootstrap": "data",
				"layer2":    "data",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := merger.MergeConfigs(tt.bootstrap, tt.layer2)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected an error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Parse the result YAML
			var resultYAML map[string]interface{}
			err = yaml.Unmarshal([]byte(result), &resultYAML)
			if err != nil {
				t.Errorf("Error parsing result YAML: %v", err)
				return
			}

			// Compare the parsed YAML
			if !reflect.DeepEqual(resultYAML, tt.expectedYAML) {
				t.Errorf("Expected YAML %v, got %v", tt.expectedYAML, resultYAML)
			}

			// Check for correct headers
			lines := strings.Split(result, "\n")
			if tt.bootstrap == "## template: jinja\n#cloud-config\nbootstrap: data" {
				if lines[0] != "## template: jinja" || lines[1] != "#cloud-config" {
					t.Errorf("Expected Jinja and cloud-config headers, got %v", lines[:2])
				}
			} else {
				if lines[0] != "#cloud-config" {
					t.Errorf("Expected cloud-config header, got %v", lines[0])
				}
			}
		})
	}
}