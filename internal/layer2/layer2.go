package layer2

import (
	"bytes"
	"text/template"
)

// PortNetwork represents a network configuration for a Layer2 network
type PortNetwork struct {
	PortName  string
	Vxlan     int
	IPAddress string
	Netmask   string
	Gateway   string
	Routes    []RouteSpec
}

// RouteSpec represents a static route.
type RouteSpec struct {
    Destination string
    Gateway     string
}

type Config struct {
	// VLANs is a list of network configurations for the Layer2
	VLANs []PortNetwork
}

// NewConfig returns a new Config object
func NewConfig() *Config {
	return &Config{
		VLANs:  make([]PortNetwork, 0),
	}
}

func (c *Config) AddPortNetwork(portName string, vxlan int, ipAddress string, netmask string, routes []RouteSpec) {
	c.VLANs = append(c.VLANs, PortNetwork{
		PortName:  portName,
		Vxlan:     vxlan,
		IPAddress: ipAddress,
		Netmask:   netmask,
		Routes:    routes,
	})
}

func (c *Config) GetTemplate() (string, error) {
	tmpl, err := template.New("layer-2-user-data").Parse(configTemplate)
	if err != nil {
		return "", err
	}

	// execute the template and save the output to a buffer
	var output bytes.Buffer
	if err := tmpl.Execute(&output, c); err != nil {
		return "", err
	}
	return output.String(), nil
}
