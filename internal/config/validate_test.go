package config

import (
	"testing"
)

func TestValidateReleaseRequiresSecrets(t *testing.T) {
	cfg := &Config{GinMode: "release"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without MNEMONIC and DB_PASSWORD")
	}

	cfg.Mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without DB_PASSWORD")
	}

	cfg.DBPassword = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateDebugOptional(t *testing.T) {
	cfg := &Config{GinMode: "debug"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("debug should not require secrets: %v", err)
	}
}