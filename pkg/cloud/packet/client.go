package packet

import (
	"fmt"
	"os"
	"strings"

	"github.com/packethost/packngo"
)

const (
	apiTokenVarName = "PACKET_API_TOKEN"
)

type PacketClient struct {
	*packngo.Client
}

// NewClient creates a new Client for the given Packet credentials
func NewClient(packetAPIKey string) *PacketClient {
	token := strings.TrimSpace(packetAPIKey)

	if token != "" {
		return &PacketClient{packngo.NewClientWithAuth("gardener", token, nil)}
	}

	return nil
}

func GetClient() (*PacketClient, error) {
	token := os.Getenv(apiTokenVarName)
	if token == "" {
		return nil, fmt.Errorf("env var %s is required", apiTokenVarName)
	}
	return NewClient(token), nil
}
