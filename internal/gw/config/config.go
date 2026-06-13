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
	AppName       string `env:"APP_NAME" envDefault:"erebrus-gateway"`
	AppPort       string `env:"APP_PORT" envDefault:"8080"`
	GinMode       string `env:"GIN_MODE" envDefault:"release"`
	AllowedOrigin string `env:"ALLOWED_ORIGIN" envDefault:"http://localhost:3000"`
	Version       string `env:"VERSION" envDefault:"2.0.0"`

	// auth
	PasetoPrivateKey    string        `env:"PASETO_PRIVATE_KEY"`
	PasetoExpiration    time.Duration `env:"PASETO_EXPIRATION" envDefault:"24h"`
	PasetoSignedBy      string        `env:"PASETO_SIGNED_BY" envDefault:"Erebrus"`
	AuthEULA            string        `env:"AUTH_EULA" envDefault:"I accept the Erebrus Terms of Service https://erebrus.network/terms."`
	MagicLinkExpiration time.Duration `env:"MAGIC_LINK_EXPIRATION" envDefault:"15m"`
	GoogleAudience      string        `env:"GOOGLE_AUDIENCE"`
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

	// entitlement / NFT gating (no money in v2.0 — trial + NFT ownership only)
	NFTGateChain    string        `env:"NFT_GATE_CHAIN" envDefault:"evm"`
	NFTGateContract string        `env:"NFT_GATE_CONTRACT"` // ERC-721 collection; empty = NFT gating disabled
	NFTGateRPCURL   string        `env:"NFT_GATE_RPC_URL"`
	NFTGatePeriod   time.Duration `env:"NFT_GATE_PERIOD" envDefault:"720h"` // re-verify window (30d)
	NFTGatePlanID   string        `env:"NFT_GATE_PLAN_ID" envDefault:"pro"`

	// email
	ResendAPIKey string `env:"RESEND_API_KEY"`

	// p2p
	P2PListenPort string `env:"P2P_LISTEN_PORT" envDefault:"9001"`
}

// Load reads .env (best-effort) then parses the environment.
func Load() (*Config, error) {
	_ = godotenv.Load()
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// DSN builds the Postgres connection string.
func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUsername, c.DBPassword, c.DBName, c.DBSSLMode)
}
