package packet

import (
	"github.com/packethost/packngo"
	"strings"
)

type packetClient struct {
	packet *packngo.Client
}

// NewClient creates a new Client for the given Packet credentials
func NewClient(packetAPIKey string) *packetClient {
	token := strings.TrimSpace(packetAPIKey)

	if token != "" {
		return &packetClient{packngo.NewClientWithAuth("gardener", token, nil)}
	}

	return nil
}
