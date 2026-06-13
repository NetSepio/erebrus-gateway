// Command gateway is the Erebrus v2 gateway: wallet auth, node discovery +
// control plane (WebSocket hub), VPN client provisioning, USDC subscriptions,
// and admin. It replaces the v1 app.Init bootstrap.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NetSepio/gateway/internal/gw/api"
	"github.com/NetSepio/gateway/internal/gw/cache"
	"github.com/NetSepio/gateway/internal/gw/config"
	"github.com/NetSepio/gateway/internal/gw/nodehub"
	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/NetSepio/gateway/internal/gw/token"
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Postgres + migrations.
	st, err := store.Open(ctx, cfg.DSN())
	if err != nil {
		return err
	}
	defer st.Close()
	log.Info("database ready")

	// PASETO signer. Generate an ephemeral key in dev if none is configured.
	pasetoKey := cfg.PasetoPrivateKey
	if pasetoKey == "" {
		_, sk, _ := ed25519.GenerateKey(rand.Reader)
		pasetoKey = hex.EncodeToString(sk)
		log.Warn("PASETO_PRIVATE_KEY not set — generated an ephemeral key; tokens will not survive restart")
	}
	tokens, err := token.New(pasetoKey, cfg.PasetoSignedBy, cfg.PasetoExpiration)
	if err != nil {
		return err
	}

	// Redis cache (best-effort).
	c, err := cache.New(ctx, cfg.RedisHost, cfg.RedisPassword)
	if err != nil {
		log.Warn("redis unavailable — running without discovery cache", "err", err)
	}
	defer c.Close()

	// Node control-plane hub.
	hub := nodehub.New(st, log)

	// Background maintenance: flip stale nodes offline, purge expired challenges.
	go maintenance(ctx, st, log)

	// HTTP server.
	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           api.New(cfg, st, tokens, hub, c).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("gateway listening", "addr", srv.Addr, "version", cfg.Version)
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

// maintenance periodically marks unresponsive nodes offline (3 missed
// heartbeats = 90s) and purges expired login challenges.
func maintenance(ctx context.Context, st *store.Store, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if n, err := st.MarkStaleNodesOffline(mctx, 95*time.Second); err == nil && n > 0 {
				log.Info("marked stale nodes offline", "count", n)
			}
			_ = st.PurgeExpiredFlowIDs(mctx)
			cancel()
		}
	}
}
