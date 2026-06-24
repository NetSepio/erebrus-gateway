package config

import (
	"fmt"
	"strings"
)

// Validate checks required deployment config. In release mode MNEMONIC and
// DB_PASSWORD are mandatory; validation runs before the server binds a port.
func (c *Config) Validate() error {
	if c.GinMode != "release" {
		return nil
	}
	if strings.TrimSpace(c.Mnemonic) == "" {
		return fmt.Errorf("MNEMONIC is required when GIN_MODE=release")
	}
	if c.DBPassword == "" {
		return fmt.Errorf("DB_PASSWORD is required when GIN_MODE=release")
	}
	return nil
}