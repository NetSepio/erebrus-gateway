// Package config centralizes the v2 gateway configuration, parsed from the
// environment (see .env.example). Product tunables live in platform_settings (DB).
package config

import (
	"fmt"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/joho/godotenv"
)

// Config holds deployment essentials: infra, secrets, and identity.
type Config struct {
	// app
	AppPort        string `env:"APP_PORT" envDefault:"8080"`
	GinMode        string `env:"GIN_MODE" envDefault:"release"`
	Environment    string `env:"ENVIRONMENT" envDefault:"dev"`
	// Webapp origins: prod https://erebrus.io, dev https://dev.erebrus.io
	AllowedOrigin  string `env:"ALLOWED_ORIGIN" envDefault:"https://erebrus.io,https://dev.erebrus.io,http://localhost:3000"`
	TrustedProxies string `env:"TRUSTED_PROXIES"` // CSV of reverse-proxy IPs/CIDRs; empty = trust none

	// auth / identity — PASETO signer derived from MNEMONIC at m/44'/501'/1'/0'
	Mnemonic           string `env:"MNEMONIC"`
	AdminWalletAddress string `env:"ADMIN_WALLET_ADDRESS"`

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

	// NFT gating — collection addresses live in nft_gate_contracts (DB); RPC here.
	SolanaRPCURL  string `env:"SOLANA_RPC_URL" envDefault:"https://api.mainnet-beta.solana.com"`
	EVMRPCURL     string `env:"EVM_RPC_URL"`
	// Legacy single-contract env (dev fallback when DB has no rows).
	NFTGateChain    string `env:"NFT_GATE_CHAIN" envDefault:"solana"`
	NFTGateContract string `env:"NFT_GATE_CONTRACT"`
	NFTGateRPCURL   string `env:"NFT_GATE_RPC_URL"`

	// optional integrations (secrets)
	TelegramBotToken string `env:"TELEGRAM_BOT_TOKEN"`
	ResendAPIKey     string `env:"RESEND_API_KEY"`
	ResendFrom       string `env:"RESEND_FROM" envDefault:"Erebrus <no-reply@info.erebrus.io>"`
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

// ResolveSolanaRPC returns the Solana JSON-RPC URL for NFT gating (DAS-capable in prod).
func (c *Config) ResolveSolanaRPC() string {
	if strings.TrimSpace(c.SolanaRPCURL) != "" {
		return strings.TrimSpace(c.SolanaRPCURL)
	}
	return strings.TrimSpace(c.NFTGateRPCURL)
}