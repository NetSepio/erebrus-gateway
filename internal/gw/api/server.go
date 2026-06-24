// Package api is the gateway's HTTP surface (Gin) under /api/v2: auth, account,
// node discovery + control plane, VPN client provisioning, subscriptions,
// payments, and admin. It implements docs/v2/gateway-api.openapi.yaml.
package api

import (
	"github.com/NetSepio/gateway/internal/gw/cache"
	"github.com/NetSepio/gateway/internal/gw/config"
	"github.com/NetSepio/gateway/internal/gw/mailer"
	"github.com/NetSepio/gateway/internal/gw/nftgate"
	"github.com/NetSepio/gateway/internal/gw/nodeclient"
	"github.com/NetSepio/gateway/internal/gw/nodehub"
	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/NetSepio/gateway/internal/gw/token"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Server holds the gateway's API dependencies.
type Server struct {
	cfg    *config.Config
	store  *store.Store
	tokens *token.Manager
	hub    *nodehub.Hub
	cache  *cache.Cache
	nodes  *nodeclient.Client
	nft    nftgate.Checker
	mailer *mailer.Mailer
}

// New builds the API server.
func New(cfg *config.Config, st *store.Store, tm *token.Manager, hub *nodehub.Hub, c *cache.Cache, nft nftgate.Checker, ml *mailer.Mailer) *Server {
	return &Server{cfg: cfg, store: st, tokens: tm, hub: hub, cache: c, nodes: nodeclient.New(), nft: nft, mailer: ml}
}

// Router wires all routes.
func (s *Server) Router() *gin.Engine {
	if s.cfg.GinMode != "" {
		gin.SetMode(s.cfg.GinMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())

	corsCfg := cors.DefaultConfig()
	corsCfg.AllowOrigins = splitCSV(s.cfg.AllowedOrigin)
	corsCfg.AllowHeaders = []string{"Origin", "Content-Type", "Authorization", "X-Api-Key"}
	corsCfg.AllowMethods = []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"}
	r.Use(cors.New(corsCfg))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "version": s.cfg.Version}) })

	v2 := r.Group("/api/v2")

	// auth (public) — GET challenge, POST signed response
	auth := v2.Group("/auth")
	{
		auth.GET("", s.handleFlowID)
		auth.POST("", s.handleAuthenticate)
		// deprecated v2.0 paths (existing clients)
		auth.GET("/flowid", s.handleFlowID)
		auth.POST("/authenticate", s.handleAuthenticate)
		// optional email linking (authenticated wallet session; verified OTP)
		auth.POST("/email", s.authUser(), s.handleEmailOTPStart)
		auth.POST("/email/verify", s.authUser(), s.handleEmailOTPVerify)
	}

	// node discovery (public) + control plane
	v2.GET("/nodes", s.handleListNodes)
	v2.POST("/nodes/register", s.handleNodeRegister)
	v2.GET("/nodes/ws", s.handleNodeWS) // auth handled inside (node PASETO)

	// subscriptions: plans are public
	v2.GET("/subscriptions/plans", s.handlePlans)

	// authenticated user routes
	user := v2.Group("")
	user.Use(s.authUser())
	{
		user.GET("/account/profile", s.handleGetProfile)
		user.PATCH("/account/profile", s.handlePatchProfile)

		user.GET("/vpn/clients", s.handleListClients)
		user.POST("/vpn/clients", s.handleProvisionClient)
		user.DELETE("/vpn/clients/:id", s.handleDeleteClient)
		user.GET("/vpn/clients/:id/config", s.handleClientConfig)

		// operator: my nodes (owned + via org) + per-node metric charts
		user.GET("/operator/nodes", s.handleOperatorNodes)
		user.GET("/operator/nodes/:id/metrics", s.handleOperatorNodeMetrics)

		// referrals (social layer): my code, who referred me, recent referees
		user.GET("/referrals/me", s.handleReferralsMe)

		// rank: my XP, tier, claimable balance, breakdown
		user.GET("/rank/me", s.handleRankMe)

		// entitlement: trial + NFT gating only (no money in v2.0)
		user.GET("/subscriptions", s.handleMySubscription)
		user.POST("/subscriptions/trial", s.handleStartTrial)
		user.POST("/subscriptions/nft/refresh", s.handleNFTRefresh)

		// organizations (owner/member, user-authed management)
		user.POST("/orgs", s.handleCreateOrg)
		user.GET("/orgs", s.handleListOrgs)
		user.GET("/orgs/:id", s.handleGetOrg)
		user.GET("/orgs/:id/members", s.handleListMembers)
		user.POST("/orgs/:id/members", s.handleAddMember)
		user.GET("/orgs/:id/apikeys", s.handleListAPIKeys)
		user.POST("/orgs/:id/apikeys", s.handleCreateAPIKey)
		user.DELETE("/orgs/:id/apikeys/:keyId", s.handleRevokeAPIKey)
		user.GET("/orgs/:id/usage", s.handleOrgUsage)
		user.GET("/orgs/:id/clients", s.handleOrgClients)
	}

	// org programmatic access (X-Api-Key) — scoped to the key's org
	orgapi := v2.Group("/org")
	orgapi.Use(s.authAPIKey())
	{
		orgapi.POST("/vpn/clients", s.handleOrgProvisionClient)
		orgapi.GET("/vpn/clients", s.handleOrgListClients)
		orgapi.GET("/usage", s.handleOrgSelfUsage)
	}

	// admin routes
	admin := v2.Group("/admin")
	admin.Use(s.authUser(), s.requireAdmin())
	{
		admin.GET("/stats", s.handleAdminStats)
		admin.GET("/nodes", s.handleAdminNodes)
		admin.GET("/users", s.handleAdminUsers)
		admin.GET("/subscriptions", s.handleAdminSubscriptions)
		admin.GET("/orgs", s.handleAdminOrgs)
		admin.GET("/orgs/:id/usage", s.handleAdminOrgUsage)
		admin.GET("/nodes/:id/metrics", s.handleAdminNodeMetrics)
		admin.POST("/nodes/:id/command", s.handleAdminNodeCommand)
	}

	return r
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, ch := range s {
		if ch == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		if ch == ' ' {
			continue
		}
		cur += string(ch)
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		out = []string{"*"}
	}
	return out
}
