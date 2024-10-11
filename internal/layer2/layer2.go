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
	Gateway   string // Added Gateway field to match template
}

// RouteSpec represents a static route.
type RouteSpec struct {
    Destination string
    Gateway     string
}

type Config struct {
	// VLANs is a list of network configurations for the Layer2
	VLANs []PortNetwork
	// Routes is a list of static routes.
	Routes []RouteSpec
}

// NewConfig returns a new Config object
func NewConfig() *Config {
	return &Config{
		VLANs:  make([]PortNetwork, 0),
		Routes: make([]RouteSpec, 0),
	}
}

func (c *Config) AddPortNetwork(portName string, vxlan int, ipAddress string, netmask string) {
	c.VLANs = append(c.VLANs, PortNetwork{
		PortName:  portName,
		Vxlan:     vxlan,
		IPAddress: ipAddress,
		Netmask:   netmask,
	})
}

func (c *Config) AddRoute(destination, gateway string) {
	c.Routes = append(c.Routes, RouteSpec{
		Destination: destination,
		Gateway:     gateway,
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
