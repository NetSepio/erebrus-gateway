// Command gateway is the Erebrus v2 gateway: wallet auth, node discovery +
// control plane (WebSocket hub), VPN client provisioning, entitlements, and admin.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NetSepio/gateway/internal/api"
	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/NetSepio/gateway/internal/cache"
	"github.com/NetSepio/gateway/internal/config"
	"github.com/NetSepio/gateway/internal/identity"
	"github.com/NetSepio/gateway/internal/mailer"
	"github.com/NetSepio/gateway/internal/nftgate"
	"github.com/NetSepio/gateway/internal/nodehub"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/NetSepio/gateway/internal/version"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("gateway exited with error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Postgres + migrations.
	st, err := store.Open(ctx, cfg.DSN())
	if err != nil {
		return err
	}
	defer st.Close()

	platform, err := st.LoadPlatformSettings(ctx)
	if err != nil {
		return fmt.Errorf("platform settings: %w", err)
	}
	st.SetTierThresholds(platform.XPTierThresholds)
	platformLive := &config.PlatformSettings{}
	platformLive.Replace(platform)
	log.Info("database ready")

	pasetoKey, err := resolvePasetoKey(cfg, log)
	if err != nil {
		return err
	}
	plat := platformLive.Snapshot()
	tokens, err := token.New(pasetoKey, plat.PasetoSignedBy, plat.PasetoExpiration)
	if err != nil {
		return err
	}

	// Redis cache (best-effort).
	c, err := cache.New(ctx, cfg.RedisHost, cfg.RedisPassword)
	if err != nil {
		log.Warn("redis unavailable — running without discovery cache", "err", err)
	}
	defer c.Close()

	metrics.Register()
	metrics.SetGatewayInfo(cfg.Environment, version.Version, version.Tag)

	// Node control-plane hub.
	hub := nodehub.New(st, c, log, cfg.Environment)

	// NFT entitlement gate — contracts from DB; Solana RPC from env.
	nftContracts, err := st.ListNFTGateContracts(ctx)
	if err != nil {
		return fmt.Errorf("nft gate contracts: %w", err)
	}
	var nftChecks []nftgate.Contract
	for _, c := range nftContracts {
		nftChecks = append(nftChecks, nftgate.Contract{Chain: c.Chain, Address: c.Address})
	}
	nft := nftgate.NewFromContracts(cfg.ResolveSolanaRPC(), cfg.EVMRPCURL, nftChecks)
	if !nft.Enabled() && cfg.NFTGateContract != "" {
		nft = nftgate.New(cfg.NFTGateChain, cfg.ResolveSolanaRPC(), cfg.NFTGateContract)
	}
	if nft.Enabled() {
		log.Info("NFT gating enabled", "contracts", len(nftChecks), "solana_rpc", cfg.ResolveSolanaRPC() != "")
	}

	// Email (Resend) — optional; only enables verified email linking when keyed.
	ml := mailer.New(cfg.ResendAPIKey, cfg.ResendFrom)
	if ml.Enabled() {
		log.Info("email (Resend) enabled", "from", cfg.ResendFrom)
	} else {
		log.Info("email (Resend) disabled — RESEND_API_KEY unset; email linking unavailable")
	}

	// Background maintenance: flip stale nodes offline, purge expired challenges.
	go maintenance(ctx, st, platformLive, log)

	// HTTP server.
	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           api.New(cfg, platformLive, st, tokens, hub, c, nft, ml).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("gateway listening", "addr", srv.Addr, "version", version.Version, "tag", version.Tag, "environment", cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}

// resolvePasetoKey derives the gateway PASETO signer from MNEMONIC. Validate()
// already requires MNEMONIC in release; debug may use an ephemeral key.
func resolvePasetoKey(cfg *config.Config, log *slog.Logger) (string, error) {
	if key, err := identity.PasetoKeyFromMnemonic(cfg.Mnemonic); err == nil {
		log.Info("PASETO key derived from MNEMONIC", "path", identity.PasetoDerivationPath)
		return key, nil
	} else if cfg.Mnemonic != "" {
		return "", fmt.Errorf("paseto key from mnemonic: %w", err)
	}
	_, sk, _ := ed25519.GenerateKey(rand.Reader)
	log.Warn("MNEMONIC not set — generated an ephemeral PASETO key; tokens will not survive restart")
	return hex.EncodeToString(sk), nil
}

// maintenance periodically marks unresponsive nodes offline, purges stale data,
// and awards operator-uptime XP (idempotent per node per UTC day).
func maintenance(ctx context.Context, st *store.Store, platform *config.PlatformSettings, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			plat := platform.Snapshot()
			mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if n, err := st.MarkStaleNodesOffline(mctx, 95*time.Second); err == nil && n > 0 {
				log.Info("marked stale nodes offline", "count", n)
			}
			_ = st.PurgeExpiredFlowIDs(mctx)
			_ = st.PurgeExpiredEmailOTPs(mctx)
			_ = st.PurgeOldNodeMetrics(mctx, plat.NodeMetricsRetention)
			_ = st.AwardOperatorUptimeXP(mctx, plat.XPUptimeDay)
			// Drop reconciliation: release TTL-expired upload reservations so
			// their quota/node holds are returned. Idempotent per reservation.
			if n, err := st.ExpireDropReservations(mctx, 200); err == nil && n > 0 {
				metrics.DropReconciliationJobsTotal.WithLabelValues("expire_reservations", "ok").Inc()
				log.Info("expired drop reservations", "count", n)
			} else if err != nil {
				metrics.DropReconciliationJobsTotal.WithLabelValues("expire_reservations", "failed").Inc()
			}
			cancel()
		}
	}
}