// Package config centralizes the v2 gateway configuration, parsed from the
// environment (see .env.example). It replaces the scattered v1 load/* config.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/joho/godotenv"
)

// Config is the full gateway configuration.
type Config struct {
	// app
	AppName        string `env:"APP_NAME" envDefault:"erebrus-gateway"`
	AppPort        string `env:"APP_PORT" envDefault:"8080"`
	GinMode        string `env:"GIN_MODE" envDefault:"release"`
	AllowedOrigin  string `env:"ALLOWED_ORIGIN" envDefault:"http://localhost:3000"`
	TrustedProxies string `env:"TRUSTED_PROXIES"` // CSV of reverse-proxy IPs/CIDRs; empty = trust none
	Version        string `env:"VERSION" envDefault:"2.0.0"`

	// rate limiting (per-IP, per minute; <=0 disables). Redis-backed, fail-open.
	RateLimitAuthPerMin     int `env:"RATE_LIMIT_AUTH_PER_MIN" envDefault:"30"`
	RateLimitRegisterPerMin int `env:"RATE_LIMIT_REGISTER_PER_MIN" envDefault:"10"`

	// auth
	Mnemonic            string        `env:"MNEMONIC"` // derives PASETO key when PASETO_PRIVATE_KEY is empty
	PasetoPrivateKey    string        `env:"PASETO_PRIVATE_KEY"`
	PasetoExpiration    time.Duration `env:"PASETO_EXPIRATION" envDefault:"24h"`
	PasetoSignedBy      string        `env:"PASETO_SIGNED_BY" envDefault:"Erebrus"`
	AuthEULA            string        `env:"AUTH_EULA" envDefault:"I accept the Erebrus Terms of Service https://erebrus.network/terms."`
	MagicLinkExpiration time.Duration `env:"MAGIC_LINK_EXPIRATION" envDefault:"15m"` // email OTP TTL
	AdminWalletAddress  string        `env:"ADMIN_WALLET_ADDRESS"`

	// database
	DBHost     string `env:"DB_HOST" envDefault:"localhost"`
	DBPort     string `env:"DB_PORT" envDefault:"5432"`
	DBUsername string `env:"DB_USERNAME" envDefault:"erebrus"`
	DBPassword string `env:"DB_PASSWORD"`
	DBName     string `env:"DB_NAME" envDefault:"erebrus"`
	DBSSLMode  string `env:"DB_SSLMODE" envDefault:"require"`

	// redis
	RedisHost     string `env:"REDIS_HOST" envDefault:"localhost:6379"`
	RedisPassword string `env:"REDIS_PASSWORD"`

	// entitlement / NFT gating (no money in v2.0 — trial + NFT ownership only).
	// General free trial: 7 days, one per user (idx_subs_one_trial).
	TrialPeriod time.Duration `env:"TRIAL_PERIOD" envDefault:"168h"` // 7d
	// v2.0 targets Solana Metaplex Core; NFT_GATE_CONTRACT is the collection
	// address (Solana) or ERC-721 contract (EVM, future). NFT holders get 30 days
	// directly (NFT_GATE_PERIOD), upgrading any active trial.
	NFTGateChain    string        `env:"NFT_GATE_CHAIN" envDefault:"solana"`
	NFTGateContract string        `env:"NFT_GATE_CONTRACT"`                 // collection address; empty = NFT gating disabled
	NFTGateRPCURL   string        `env:"NFT_GATE_RPC_URL"`                  // Solana: a DAS-capable endpoint (e.g. Helius)
	NFTGatePeriod   time.Duration `env:"NFT_GATE_PERIOD" envDefault:"720h"` // re-verify window (30d)
	NFTGatePlanID   string        `env:"NFT_GATE_PLAN_ID" envDefault:"pro"`

	// operator node metrics time-series retention (S4)
	NodeMetricsRetention time.Duration `env:"NODE_METRICS_RETENTION" envDefault:"720h"` // 30d

	// XP weights + tiers (tunable; §5b).
	XPReferrerPoints int64   `env:"XP_REFERRER_POINTS" envDefault:"100"` // referee's first trial
	XPRefereePoints  int64   `env:"XP_REFEREE_POINTS" envDefault:"25"`
	XPEmailVerified  int64   `env:"XP_EMAIL_VERIFIED" envDefault:"25"` // once
	XPNFTHeld        int64   `env:"XP_NFT_HELD" envDefault:"50"`       // monthly while held
	XPUptimeDay      int64   `env:"XP_UPTIME_DAY" envDefault:"20"`     // per healthy node, per UTC day
	XPTierThresholds []int64 `env:"XP_TIER_THRESHOLDS" envSeparator:"," envDefault:"0,100,500,2000,10000"`

	XPSocialVerified int64 `env:"XP_SOCIAL_VERIFIED" envDefault:"75"` // once per provider (X/Telegram)

	// Rank reward: spend XP for free entitlement days (source='rank').
	XPFreeDaysCost  int64 `env:"XP_FREE_DAYS_COST" envDefault:"500"`
	XPFreeDaysGrant int   `env:"XP_FREE_DAYS_GRANT" envDefault:"7"`

	// social verification (S7). Tokens are never stored.
	TelegramBotToken string `env:"TELEGRAM_BOT_TOKEN"` // empty disables /social/telegram (503)
	XAPIBaseURL      string `env:"X_API_BASE_URL" envDefault:"https://api.twitter.com"`

	// email (optional — links a verified email to a wallet account via Resend OTP)
	ResendAPIKey string `env:"RESEND_API_KEY"`
	ResendFrom   string `env:"RESEND_FROM" envDefault:"Erebrus <no-reply@erebrus.network>"`

	// p2p
	P2PListenPort string `env:"P2P_LISTEN_PORT" envDefault:"9001"`
}

// Load reads .env (best-effort) then parses the environment.
func Load() (*Config, error) {
	_ = godotenv.Load()
	cfg := &Config{}
	if err := parseEnv(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseEnv parses the process environment into cfg (no .env side-load), so
// callers/tests can exercise defaults deterministically.
func parseEnv(cfg *Config) error {
	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return nil
}

// DSN builds the Postgres connection string.
func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUsername, c.DBPassword, c.DBName, c.DBSSLMode)
}
