package config

import (
	"testing"
	"time"
)

func TestEnvEssentialsDefaults(t *testing.T) {
	cfg := &Config{}
	mustParse(t, cfg)

	if cfg.AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", cfg.AppPort)
	}
	if cfg.NFTGateChain != "solana" {
		t.Errorf("NFTGateChain = %q, want solana", cfg.NFTGateChain)
	}
}

func TestPlatformDefaults(t *testing.T) {
	s := DefaultPlatformValues()

	if s.TrialPeriod != 7*24*time.Hour {
		t.Errorf("TrialPeriod = %s, want 168h", s.TrialPeriod)
	}
	if s.NFTGatePeriod != 30*24*time.Hour {
		t.Errorf("NFTGatePeriod = %s, want 720h", s.NFTGatePeriod)
	}
	if s.PasetoExpiration != 24*time.Hour {
		t.Errorf("PasetoExpiration = %s, want 24h", s.PasetoExpiration)
	}

	want := []int64{0, 100, 500, 2000, 10000}
	if len(s.XPTierThresholds) != len(want) {
		t.Fatalf("XPTierThresholds = %v, want %v", s.XPTierThresholds, want)
	}
	for i := range want {
		if s.XPTierThresholds[i] != want[i] {
			t.Fatalf("XPTierThresholds[%d] = %d, want %d", i, s.XPTierThresholds[i], want[i])
		}
	}
	if s.XPReferrerPoints != 100 || s.XPRefereePoints != 25 {
		t.Errorf("referral XP = %d/%d, want 100/25", s.XPReferrerPoints, s.XPRefereePoints)
	}
}

func TestParsePlatformSettingsOverride(t *testing.T) {
	raw := map[string]string{
		"trial_period": "48h",
		"xp_referrer_points": "200",
	}
	s, err := ParsePlatformSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.TrialPeriod != 48*time.Hour {
		t.Errorf("TrialPeriod = %s, want 48h", s.TrialPeriod)
	}
	if s.XPReferrerPoints != 200 {
		t.Errorf("XPReferrerPoints = %d, want 200", s.XPReferrerPoints)
	}
	// Unspecified keys fall back to defaults.
	if s.XPRefereePoints != 25 {
		t.Errorf("XPRefereePoints = %d, want default 25", s.XPRefereePoints)
	}
}

func mustParse(t *testing.T, cfg *Config) {
	t.Helper()
	if err := parseEnv(cfg); err != nil {
		t.Fatalf("parse env: %v", err)
	}
}