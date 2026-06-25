package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// OrgEnrollmentPrefix is the human-visible prefix for org enrollment secrets.
const OrgEnrollmentPrefix = "ere_org_"

// NewOrgEnrollmentSecret mints a retrievable org enrollment credential.
func NewOrgEnrollmentSecret() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return OrgEnrollmentPrefix + hex.EncodeToString(raw), nil
}

// NewNodeKey mints a per-node bearer for gateway→node private API calls.
func NewNodeKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return "ere_node_" + hex.EncodeToString(raw), nil
}