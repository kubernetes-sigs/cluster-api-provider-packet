package layer2

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	cloudConfigHeader = "#cloud-config"
	jinjaHeader      = "## template: jinja"
)

// CloudConfigMerger handles the merging of cloud configs
type CloudConfigMerger struct {
}

// NewCloudConfigMerger creates a new instance of CloudConfigMerger
func NewCloudConfigMerger() *CloudConfigMerger {
	return &CloudConfigMerger{}
}

// configHeaders represents the headers found in a cloud-config file
type configHeaders struct {
	hasJinja      bool
	hasCloudConfig bool
}

// stripHeaders removes the template and cloud-config headers and returns the remaining content
func stripHeaders(data string) (string, configHeaders) {
	headers := configHeaders{}
	lines := strings.Split(strings.TrimSpace(data), "\n")
	startIndex := 0

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		switch trimmedLine {
		case jinjaHeader:
			headers.hasJinja = true
			startIndex = i + 1
		case cloudConfigHeader:
			headers.hasCloudConfig = true
			startIndex = i + 1
		default:
			if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
				return strings.Join(lines[startIndex:], "\n"), headers
			}
		}
	}
	return "", headers
}

// deepMerge recursively merges two maps
func (m *CloudConfigMerger) deepMerge(base, overlay map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base map
	for k, v := range base {
		result[k] = v
	}

	// Merge overlay
	for k, v := range overlay {
		if baseVal, exists := result[k]; exists {
			// If both values are maps, merge them recursively
			if baseMap, ok := baseVal.(map[string]interface{}); ok {
				if overlayMap, ok := v.(map[string]interface{}); ok {
					result[k] = m.deepMerge(baseMap, overlayMap)
					continue
				}
			}

			// If either value is a slice or both values are different, create/extend a slice
			baseSlice, baseIsSlice := baseVal.([]interface{})
			overlaySlice, overlayIsSlice := v.([]interface{})

			if baseIsSlice && overlayIsSlice {
				result[k] = append(baseSlice, overlaySlice...)
			} else if baseIsSlice {
				result[k] = append(baseSlice, v)
			} else if overlayIsSlice {
				result[k] = append([]interface{}{baseVal}, overlaySlice...)
			} else {
				result[k] = []interface{}{baseVal, v}
			}
		} else {
			// Key doesn't exist in base, so add it
			result[k] = v
		}
	}

	return result
}

// buildHeader constructs the appropriate header based on the input configurations
func buildHeader(bootstrapHeaders, layer2Headers configHeaders) string {
	var headers []string

	// If either input has the Jinja header, include it in the output
	if bootstrapHeaders.hasJinja || layer2Headers.hasJinja {
		headers = append(headers, jinjaHeader)
	}

	// Always include the cloud-config header
	headers = append(headers, cloudConfigHeader)

	return strings.Join(headers, "\n")
}

// MergeConfigs combines bootstrap data with layer2 config
func (m *CloudConfigMerger) MergeConfigs(bootstrapData string, layer2UserData string) (string, error) {
	// Strip headers and get header info
	bootstrapStripped, bootstrapHeaders := stripHeaders(bootstrapData)
	layer2Stripped, layer2Headers := stripHeaders(layer2UserData)

	// Validate that at least one input has the cloud-config header
	if !bootstrapHeaders.hasCloudConfig && !layer2Headers.hasCloudConfig {
		return "", fmt.Errorf("neither input contains #cloud-config header")
	}

	var bootstrapConfig, layer2UserDataConfig map[string]interface{}
	
	if bootstrapStripped != "" {
		if err := yaml.Unmarshal([]byte(bootstrapStripped), &bootstrapConfig); err != nil {
			return "", fmt.Errorf("error parsing bootstrap YAML: %v", err)
		}
	} else {
		bootstrapConfig = make(map[string]interface{})
	}

	if layer2Stripped != "" {
		if err := yaml.Unmarshal([]byte(layer2Stripped), &layer2UserDataConfig); err != nil {
			return "", fmt.Errorf("error parsing layer2 YAML: %v", err)
		}
	} else {
		layer2UserDataConfig = make(map[string]interface{})
	}

	// Merge configurations
	mergedConfig := m.deepMerge(layer2UserDataConfig, bootstrapConfig)

	// Convert merged config back to YAML
	result, err := yaml.Marshal(mergedConfig)
	if err != nil {
		return "", fmt.Errorf("error marshaling merged config: %v", err)
	}

	// Build appropriate headers and combine with content
	header := buildHeader(bootstrapHeaders, layer2Headers)
	return fmt.Sprintf("%s\n%s", header, string(result)), nil
}
