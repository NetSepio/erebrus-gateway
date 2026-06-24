package config

import (
	"os"
	"testing"
	"time"
)

// TestEntitlementDefaults locks the S3 entitlement numbers: a 7-day general
// trial and a 30-day NFT-direct period. These are product decisions, not
// incidental — a change here should be deliberate.
func TestEntitlementDefaults(t *testing.T) {
	// Defaults only apply when the vars are absent (env-set-but-empty != default).
	unset(t, "TRIAL_PERIOD", "NFT_GATE_PERIOD", "PASETO_EXPIRATION")
	cfg := &Config{}
	mustParse(t, cfg)

	if cfg.TrialPeriod != 7*24*time.Hour {
		t.Errorf("TrialPeriod default = %s, want 168h (7d)", cfg.TrialPeriod)
	}
	if cfg.NFTGatePeriod != 30*24*time.Hour {
		t.Errorf("NFTGatePeriod default = %s, want 720h (30d)", cfg.NFTGatePeriod)
	}
	if cfg.PasetoExpiration != 24*time.Hour {
		t.Errorf("PasetoExpiration default = %s, want 24h", cfg.PasetoExpiration)
	}
}

func TestTrialPeriodOverride(t *testing.T) {
	t.Setenv("TRIAL_PERIOD", "48h")
	cfg := &Config{}
	mustParse(t, cfg)
	if cfg.TrialPeriod != 48*time.Hour {
		t.Errorf("TrialPeriod = %s, want 48h", cfg.TrialPeriod)
	}
}

// mustParse parses env into cfg without godotenv (so the test never reads a
// stray .env). Mirrors Load()'s env.Parse step.
func mustParse(t *testing.T, cfg *Config) {
	t.Helper()
	if err := parseEnv(cfg); err != nil {
		t.Fatalf("parse env: %v", err)
	}
}

// unset removes env vars for the duration of the test, restoring them after.
func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			os.Unsetenv(k)
			kk, vv := k, v
			t.Cleanup(func() { os.Setenv(kk, vv) })
		}
	}
}
