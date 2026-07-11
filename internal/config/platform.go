package config

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PlatformValues holds product tunables loaded from platform_settings (DB).
// Defaults match migration 0009 seeds.
type PlatformValues struct {
	TrialPeriod          time.Duration
	NFTGatePeriod        time.Duration
	NFTGatePlanID        string
	NodeMetricsRetention time.Duration

	XPReferrerPoints int64
	XPRefereePoints  int64
	XPEmailVerified  int64
	XPNFTHeld        int64
	XPUptimeDay      int64
	XPTierThresholds []int64
	XPSocialVerified int64
	XPFreeDaysCost   int64
	XPFreeDaysGrant  int

	RateLimitAuthPerMin     int
	RateLimitRegisterPerMin int
	// Drop rate limits: write covers upload create/content; read covers file and
	// public content retrieval. Per (user or IP) per minute.
	RateLimitDropWritePerMin int
	RateLimitDropReadPerMin  int

	// Node capacity gate thresholds.
	NodeCPUMax            float64
	NodeCPUSoft           float64
	NodePeerRatioMax      float64
	NodePeerConnectedSoft int

	PasetoExpiration    time.Duration
	PasetoSignedBy      string
	AuthEULA            string
	MagicLinkExpiration time.Duration
	XAPIBaseURL         string
}

// PlatformSettings is a mutex-protected live copy (shared by API + maintenance).
type PlatformSettings struct {
	mu sync.RWMutex
	v  PlatformValues
}

// DefaultPlatformValues returns the migration 0009 seed values.
func DefaultPlatformValues() PlatformValues {
	return PlatformValues{
		TrialPeriod:          168 * time.Hour,
		NFTGatePeriod:        720 * time.Hour,
		NFTGatePlanID:        "pro",
		NodeMetricsRetention: 720 * time.Hour,

		XPReferrerPoints: 100,
		XPRefereePoints:  25,
		XPEmailVerified:  25,
		XPNFTHeld:        50,
		XPUptimeDay:      20,
		XPTierThresholds: []int64{0, 100, 500, 2000, 10000},
		XPSocialVerified: 75,
		XPFreeDaysCost:   500,
		XPFreeDaysGrant:  7,

		RateLimitAuthPerMin:      30,
		RateLimitRegisterPerMin:  10,
		RateLimitDropWritePerMin: 60,
		RateLimitDropReadPerMin:  120,

		NodeCPUMax:            80.0,
		NodeCPUSoft:           60.0,
		NodePeerRatioMax:      0.9,
		NodePeerConnectedSoft: 80,

		PasetoExpiration:    24 * time.Hour,
		PasetoSignedBy:      "Erebrus",
		AuthEULA:            "I accept the Erebrus Terms of Service https://erebrus.io/terms.",
		MagicLinkExpiration: 15 * time.Minute,
		XAPIBaseURL:         "https://api.twitter.com",
	}
}

// Snapshot returns a copy safe for readers outside the mutex.
func (p *PlatformSettings) Snapshot() PlatformValues {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return clonePlatformValues(p.v)
}

// Replace swaps all fields (used after DB load or admin reload).
func (p *PlatformSettings) Replace(v PlatformValues) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.v = clonePlatformValues(v)
}

func clonePlatformValues(v PlatformValues) PlatformValues {
	cp := v
	cp.XPTierThresholds = append([]int64(nil), v.XPTierThresholds...)
	return cp
}

// ParsePlatformSettings builds values from a key→value map (DB rows).
func ParsePlatformSettings(raw map[string]string) (PlatformValues, error) {
	def := DefaultPlatformValues()
	if len(raw) == 0 {
		return def, nil
	}

	var err error
	s := def

	if v, ok := raw["trial_period"]; ok {
		s.TrialPeriod, err = time.ParseDuration(v)
		if err != nil {
			return s, fmt.Errorf("trial_period: %w", err)
		}
	}
	if v, ok := raw["nft_gate_period"]; ok {
		s.NFTGatePeriod, err = time.ParseDuration(v)
		if err != nil {
			return s, fmt.Errorf("nft_gate_period: %w", err)
		}
	}
	if v, ok := raw["nft_gate_plan_id"]; ok && v != "" {
		s.NFTGatePlanID = v
	}
	if v, ok := raw["node_metrics_retention"]; ok {
		s.NodeMetricsRetention, err = time.ParseDuration(v)
		if err != nil {
			return s, fmt.Errorf("node_metrics_retention: %w", err)
		}
	}

	if v, ok := raw["xp_referrer_points"]; ok {
		s.XPReferrerPoints, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_referrer_points: %w", err)
		}
	}
	if v, ok := raw["xp_referee_points"]; ok {
		s.XPRefereePoints, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_referee_points: %w", err)
		}
	}
	if v, ok := raw["xp_email_verified"]; ok {
		s.XPEmailVerified, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_email_verified: %w", err)
		}
	}
	if v, ok := raw["xp_nft_held"]; ok {
		s.XPNFTHeld, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_nft_held: %w", err)
		}
	}
	if v, ok := raw["xp_uptime_day"]; ok {
		s.XPUptimeDay, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_uptime_day: %w", err)
		}
	}
	if v, ok := raw["xp_tier_thresholds"]; ok {
		s.XPTierThresholds, err = parseInt64CSV(v)
		if err != nil {
			return s, fmt.Errorf("xp_tier_thresholds: %w", err)
		}
	}
	if v, ok := raw["xp_social_verified"]; ok {
		s.XPSocialVerified, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_social_verified: %w", err)
		}
	}
	if v, ok := raw["xp_free_days_cost"]; ok {
		s.XPFreeDaysCost, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return s, fmt.Errorf("xp_free_days_cost: %w", err)
		}
	}
	if v, ok := raw["xp_free_days_grant"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return s, fmt.Errorf("xp_free_days_grant: %w", err)
		}
		s.XPFreeDaysGrant = n
	}

	if v, ok := raw["rate_limit_auth_per_min"]; ok {
		s.RateLimitAuthPerMin, err = strconv.Atoi(v)
		if err != nil {
			return s, fmt.Errorf("rate_limit_auth_per_min: %w", err)
		}
	}
	if v, ok := raw["rate_limit_register_per_min"]; ok {
		s.RateLimitRegisterPerMin, err = strconv.Atoi(v)
		if err != nil {
			return s, fmt.Errorf("rate_limit_register_per_min: %w", err)
		}
	}

	if v, ok := raw["paseto_expiration"]; ok {
		s.PasetoExpiration, err = time.ParseDuration(v)
		if err != nil {
			return s, fmt.Errorf("paseto_expiration: %w", err)
		}
	}
	if v, ok := raw["paseto_signed_by"]; ok && v != "" {
		s.PasetoSignedBy = v
	}
	if v, ok := raw["auth_eula"]; ok && v != "" {
		s.AuthEULA = v
	}
	if v, ok := raw["magic_link_expiration"]; ok {
		s.MagicLinkExpiration, err = time.ParseDuration(v)
		if err != nil {
			return s, fmt.Errorf("magic_link_expiration: %w", err)
		}
	}
	if v, ok := raw["x_api_base_url"]; ok && v != "" {
		s.XAPIBaseURL = v
	}

	if v, ok := raw["node_cpu_max"]; ok {
		s.NodeCPUMax, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return s, fmt.Errorf("node_cpu_max: %w", err)
		}
	}
	if v, ok := raw["node_cpu_soft"]; ok {
		s.NodeCPUSoft, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return s, fmt.Errorf("node_cpu_soft: %w", err)
		}
	}
	if v, ok := raw["node_peer_ratio_max"]; ok {
		s.NodePeerRatioMax, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return s, fmt.Errorf("node_peer_ratio_max: %w", err)
		}
	}
	if v, ok := raw["node_peer_connected_soft"]; ok {
		s.NodePeerConnectedSoft, err = strconv.Atoi(v)
		if err != nil {
			return s, fmt.Errorf("node_peer_connected_soft: %w", err)
		}
	}

	return s, nil
}

// PlatformSettingMeta describes a tunable for admin UIs.
type PlatformSettingMeta struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// KnownPlatformKeys lists valid admin-patchable setting keys.
var KnownPlatformKeys = []string{
	"trial_period", "nft_gate_period", "nft_gate_plan_id", "node_metrics_retention",
	"xp_referrer_points", "xp_referee_points", "xp_email_verified", "xp_nft_held",
	"xp_uptime_day", "xp_tier_thresholds", "xp_social_verified",
	"xp_free_days_cost", "xp_free_days_grant",
	"rate_limit_auth_per_min", "rate_limit_register_per_min",
	"node_cpu_max", "node_cpu_soft", "node_peer_ratio_max", "node_peer_connected_soft",
	"paseto_expiration", "paseto_signed_by", "auth_eula",
	"magic_link_expiration", "x_api_base_url",
}

func parseInt64CSV(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty list")
	}
	return out, nil
}