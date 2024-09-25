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
}

type Config struct {
	// Ports is a list of network configurations for the Layer2
	Ports []PortNetwork
}

// NewConfig returns a new Config object
func NewConfig() *Config {
	return &Config{
		Ports: make([]PortNetwork, 0),
	}
}

func (c *Config) AddPortNetwork(portName string, vxlan int, ipAddress string, netmask string) {
	c.Ports = append(c.Ports, PortNetwork{
		PortName:  portName,
		Vxlan:     vxlan,
		IPAddress: ipAddress,
		Netmask:   netmask,
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