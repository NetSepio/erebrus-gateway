package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NodeRegistrationPrefix is the human-visible prefix for scoped registration tokens.
const NodeRegistrationPrefix = "ere_reg_"

// NewNodeRegistrationToken mints a one-time-capable org registration credential.
func NewNodeRegistrationToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return NodeRegistrationPrefix + hex.EncodeToString(raw), nil
}

// NewNodeKey mints a per-node bearer for gateway→node private API calls.
func NewNodeKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return "ere_node_" + hex.EncodeToString(raw), nil
}