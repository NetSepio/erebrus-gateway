// Package api is the gateway's HTTP surface (Gin) under /api/v2: auth, account,
// node discovery + control plane, VPN client provisioning, entitlements, and admin.
package api

import (
	"github.com/NetSepio/gateway/internal/cache"
	"github.com/NetSepio/gateway/internal/config"
	"github.com/NetSepio/gateway/internal/dropclient"
	"github.com/NetSepio/gateway/internal/mailer"
	"github.com/NetSepio/gateway/internal/middleware"
	"github.com/NetSepio/gateway/internal/nftgate"
	"github.com/NetSepio/gateway/internal/nodeclient"
	"github.com/NetSepio/gateway/internal/nodehub"
	"github.com/NetSepio/gateway/internal/oauth"
	"github.com/NetSepio/gateway/internal/secretbox"
	"github.com/NetSepio/gateway/internal/socialverify"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server holds the gateway's API dependencies.
type Server struct {
	cfg      *config.Config
	platform *config.PlatformSettings
	store    *store.Store
	tokens   *token.Manager
	hub      *nodehub.Hub
	cache    *cache.Cache
	nodes    *nodeclient.Client
	drop     *dropclient.Client
	nft      nftgate.Checker
	mailer   *mailer.Mailer
	xverify  *socialverify.XVerifier
	google   *oauth.Verifier
	apple    *oauth.Verifier
	crypt    *secretbox.Box
}

// New builds the API server. platform is the live DB-backed settings object
// (shared with maintenance); when nil, defaults are used.
func New(cfg *config.Config, platform *config.PlatformSettings, st *store.Store, tm *token.Manager, hub *nodehub.Hub, c *cache.Cache, nft nftgate.Checker, ml *mailer.Mailer) *Server {
	if platform == nil {
		platform = &config.PlatformSettings{}
		platform.Replace(config.DefaultPlatformValues())
	}
	p := platform.Snapshot()
	return &Server{
		cfg: cfg, platform: platform, store: st, tokens: tm, hub: hub, cache: c, nodes: nodeclient.New(), drop: dropclient.New(),
		nft: nft, mailer: ml, xverify: socialverify.NewXVerifier(p.XAPIBaseURL),
		google: oauth.NewGoogle(splitCSVRaw(cfg.GoogleClientIDs)),
		apple:  oauth.NewApple(splitCSVRaw(cfg.AppleClientIDs)),
		crypt:  secretbox.New(cfg.Mnemonic),
	}
}

// Router wires all routes.
func (s *Server) Router() *gin.Engine {
	if s.cfg.GinMode != "" {
		gin.SetMode(s.cfg.GinMode)
	}
	r := gin.New()
	// Trust only the configured reverse proxies so ClientIP (rate limiting +
	// activity log) reflects the real client, not a spoofable X-Forwarded-For.
	// Empty => trust none (ClientIP = direct peer).
	_ = r.SetTrustedProxies(splitCSVRaw(s.cfg.TrustedProxies))
	r.Use(gin.Recovery())
	r.Use(middleware.Metrics(s.cfg.Environment))

	corsCfg := cors.DefaultConfig()
	corsCfg.AllowOrigins = splitCSV(s.cfg.AllowedOrigin)
	corsCfg.AllowHeaders = []string{"Origin", "Content-Type", "Authorization", "X-Api-Key", "X-Erebrus-Client"}
	corsCfg.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	r.Use(cors.New(corsCfg))

	r.GET("/healthz", s.handleHealthz)
	r.GET("/readyz", s.handleReadyz)
	r.GET("/version", s.handleVersion)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.POST("/telemetry/event", s.handleTelemetryEvent)

	v2 := r.Group("/api/v2")

	// auth (public) — GET challenge, POST signed response. Per-IP rate limited.
	auth := v2.Group("/auth")
	auth.Use(s.rateLimit("auth", s.platform.Snapshot().RateLimitAuthPerMin))
	{
		auth.GET("", s.handleAuthChallenge)
		auth.POST("", s.handleAuthComplete)
		// which login methods are configured (so clients hide the rest cleanly)
		auth.GET("/methods", s.handleAuthMethods)
		// optional email linking (authenticated wallet session; verified OTP)
		auth.POST("/email", s.authUser(), s.handleEmailOTPStart)
		auth.POST("/email/verify", s.authUser(), s.handleEmailOTPVerify)
		// passwordless / OIDC login (public; resolve-or-create by verified identity)
		auth.POST("/email/login/start", s.handleEmailLoginStart)
		auth.POST("/email/login/verify", s.handleEmailLoginVerify)
		auth.POST("/google", s.handleGoogleAuth)
		auth.POST("/apple", s.handleAppleAuth)
	}

	// node discovery (public) + control plane
	v2.GET("/nodes", s.handleListNodes)
	v2.POST("/nodes/register", s.rateLimit("register", s.platform.Snapshot().RateLimitRegisterPerMin), s.handleNodeRegister)
	v2.POST("/nodes/token/refresh", s.handleNodeTokenRefresh)
	v2.GET("/nodes/ws", s.handleNodeWS) // auth handled inside (node PASETO)

	// subscriptions: plans are public
	v2.GET("/subscriptions/plans", s.handlePlans)

	// public org profiles
	v2.GET("/public/orgs/:slug", s.handlePublicOrgBySlug)
	v2.GET("/public/orgs/:slug/invite", s.handleOrgInviteBySlug)

	// Drop: opaque public share (no auth; visibility + status enforced). Rate
	// limited per IP. The file id is an unguessable opaque handle, never the CID.
	// The bare route returns metadata (incl. node gateway_url(s) for direct
	// retrieval); /content streams bytes through the gateway as a fallback.
	v2.GET("/drop/public/:fileId", s.rateLimit("drop_public", s.platform.Snapshot().RateLimitDropReadPerMin), s.handleDropPublicGet)
	v2.GET("/drop/public/:fileId/content", s.rateLimit("drop_public_content", s.platform.Snapshot().RateLimitDropReadPerMin), s.handleDropPublicContent)
	// Drop: short-lived same-origin WebUI proxy. The session id is validated
	// inside; the raw node/Kubo address is never exposed to the caller.
	v2.Any("/drop/webui/:sessionId/*proxyPath", s.handleDropWebUIProxy)

	// node heartbeat (node PASETO)
	v2.POST("/nodes/:nodeId/heartbeat", s.handleNodeHeartbeat)
	// node reports its Shield (AdGuard) admin credential (node PASETO)
	v2.POST("/nodes/:nodeId/firewall/credentials", s.handleNodeReportFirewallCredentials)

	// authenticated user routes (audit-logged on successful mutations)
	user := v2.Group("")
	user.Use(s.authUser(), s.activityLog())
	{
		user.GET("/account/profile", s.handleGetProfile)
		user.PATCH("/account/profile", s.handlePatchProfile)
		user.GET("/account/org-invites", s.handleListAccountOrgInvites)
		user.GET("/account/org-invites/:orgId", s.handleGetAccountOrgInvite)
		user.POST("/account/org-invites/:orgId/accept", s.handleAcceptAccountOrgInvite)
		user.POST("/account/org-invites/:orgId/decline", s.handleDeclineAccountOrgInvite)
		user.GET("/account/activity", s.handleAccountActivity)
		user.POST("/account/wallet", s.handleLinkWallet)

		user.GET("/vpn/clients", s.handleListClients)
		user.POST("/vpn/clients", s.handleProvisionClient)
		user.DELETE("/vpn/clients/:id", s.handleDeleteClient)
		user.GET("/vpn/clients/:id/config", s.handleClientConfig)

		// operator: my nodes (owned + via org) + per-node metric charts
		user.GET("/operator/nodes", s.handleOperatorNodes)
		user.PATCH("/operator/nodes/:id", s.handlePatchOperatorNode)
		user.GET("/operator/nodes/:id/metrics", s.handleOperatorNodeMetrics)

		// referrals (social layer): my code, who referred me, recent referees
		user.GET("/referrals/me", s.handleReferralsMe)
		user.POST("/referrals/redeem", s.handleReferralRedeem)

		// rank: my XP, tier, claimable balance, breakdown; leaderboard; claim
		user.GET("/rank/me", s.handleRankMe)
		user.POST("/rank/claim", s.handleRankClaim)
		user.GET("/leaderboard", s.handleLeaderboard)

		// social verification (X / Telegram; email links via /auth/email)
		user.GET("/social/accounts", s.handleSocialAccounts)
		user.POST("/social/telegram", s.handleVerifyTelegram)
		user.POST("/social/x", s.handleVerifyX)
		user.POST("/social/google", s.handleLinkGoogle)
		user.POST("/social/apple", s.handleLinkApple)

		// perks: catalog (tier-annotated) + my granted perks
		user.GET("/perks", s.handleListPerks)
		user.GET("/perks/me", s.handleMyPerks)

		// Legacy compatibility routes. Responses are organization-derived; trial
		// and NFT refreshes never grant product access.
		user.GET("/subscriptions", s.handleMySubscription)
		user.POST("/subscriptions/trial", s.handleStartTrial)
		user.POST("/subscriptions/nft/refresh", s.handleNFTRefresh)

		// organizations (owner/member, user-authed management)
		user.POST("/orgs", s.handleCreateOrg)
		user.GET("/orgs", s.handleListOrgs)
		user.GET("/orgs/:id", s.handleGetOrg)
		user.PATCH("/orgs/:id", s.handlePatchOrg)
		user.DELETE("/orgs/:id", s.handleDeleteOrg)
		user.GET("/orgs/:id/entitlements", s.handleGetOrgEntitlements)
		user.GET("/orgs/:id/profile", s.handleGetOrgProfile)
		user.PATCH("/orgs/:id/profile", s.handlePatchOrgProfile)
		user.GET("/orgs/:id/profile/public", s.handleGetOrgPublicProfile)
		user.PATCH("/orgs/:id/profile/public", s.handlePatchOrgPublicProfile)
		user.GET("/orgs/:id/seats", s.handleListSeats)
		user.POST("/orgs/:id/seats/assign", s.handleAssignSeat)
		user.POST("/orgs/:id/seats/revoke", s.handleRevokeSeat)
		user.GET("/orgs/:id/members", s.handleListMembers)
		user.GET("/orgs/:id/invites", s.handleListOrgInvites)
		user.DELETE("/orgs/:id/invites/:inviteId", s.handleRevokeOrgInvite)
		user.POST("/orgs/:id/members/invite", s.handleInviteMember)
		user.POST("/orgs/:id/members", s.handleAddMember)
		user.PATCH("/orgs/:id/members/:memberId", s.handlePatchMember)
		user.DELETE("/orgs/:id/members/:memberId", s.handleRemoveMember)
		user.POST("/orgs/:id/transfer-ownership", s.handleTransferOwnership)
		user.GET("/orgs/:id/nodes", s.handleListOrgNodes)
		user.POST("/orgs/:id/nodes/register", s.handleOrgNodeRegister)
		user.GET("/orgs/:id/nodes/:nodeId", s.handleGetOrgNode)
		user.PATCH("/orgs/:id/nodes/:nodeId", s.handlePatchOrgNode)
		user.POST("/orgs/:id/node-registration-tokens", s.handleCreateNodeRegistrationToken)
		user.GET("/orgs/:id/nodes/:nodeId/services", s.handleListOrgNodeServices)
		user.POST("/orgs/:id/nodes/:nodeId/services", s.handleAttachOrgNodeService)
		user.PATCH("/orgs/:id/nodes/:nodeId/services/:serviceId", s.handlePatchOrgNodeService)
		user.DELETE("/orgs/:id/nodes/:nodeId/services/:serviceId", s.handleDeleteOrgNodeService)
		user.GET("/orgs/:id/nodes/:nodeId/firewall", s.handleGetFirewall)
		user.GET("/orgs/:id/nodes/:nodeId/firewall/status", s.handleGetFirewallStatus)
		user.POST("/orgs/:id/nodes/:nodeId/firewall/restart", s.handleFirewallRestart)
		user.POST("/orgs/:id/nodes/:nodeId/firewall/sync", s.handleFirewallSync)
		user.POST("/orgs/:id/nodes/:nodeId/firewall/reset-credentials", s.handleFirewallResetCredentials)
		user.GET("/orgs/:id/nodes/:nodeId/firewall/credentials", s.handleGetFirewallCredentials)
		user.POST("/orgs/:id/nodes/:nodeId/firewall/credentials", s.handleUpdateFirewallCredentials)
		user.GET("/orgs/:id/nodes/:nodeId/firewall/rules", s.handleListFirewallRules)
		user.POST("/orgs/:id/nodes/:nodeId/firewall/rules", s.handleCreateFirewallRule)
		user.PATCH("/orgs/:id/nodes/:nodeId/firewall/rules/:ruleId", s.handlePatchFirewallRule)
		user.DELETE("/orgs/:id/nodes/:nodeId/firewall/rules/:ruleId", s.handleDeleteFirewallRule)
		user.GET("/orgs/:id/runtime-nodes", s.handleOrgNodes)
		user.GET("/orgs/:id/apikeys", s.handleListAPIKeys)
		user.POST("/orgs/:id/apikeys", s.handleCreateAPIKey)
		user.DELETE("/orgs/:id/apikeys/:keyId", s.handleRevokeAPIKey)
		user.GET("/orgs/:id/usage", s.handleOrgUsage)
		user.GET("/orgs/:id/clients", s.handleOrgClients)
		user.POST("/orgs/:id/vpn/clients", s.handleUserOrgProvisionClient)

		// Drop: private org file metadata/usage + WebUI session (org-scoped)
		user.GET("/orgs/:id/drop/files", s.handleOrgDropFiles)
		user.GET("/orgs/:id/drop/usage", s.handleOrgDropUsage)
		user.POST("/orgs/:id/nodes/:nodeId/drop/webui/session", s.handleDropWebUISession)

		// Drop: authenticated node discovery, uploads, files, usage, vault.
		user.GET("/drop/nodes", s.handleDropNodes)
		user.POST("/drop/uploads", s.dropRateLimit("drop_create", true), s.handleDropReserveUpload)
		user.PUT("/drop/uploads/:uploadId/content", s.dropRateLimit("drop_content", true), s.handleDropUploadContent)
		user.GET("/drop/uploads/:uploadId", s.handleDropUploadStatus)
		user.GET("/drop/files", s.handleDropListFiles)
		user.GET("/drop/files/:fileId", s.handleDropGetFile)
		user.GET("/drop/files/:fileId/content", s.dropRateLimit("drop_read", false), s.handleDropFileContent)
		user.PATCH("/drop/files/:fileId", s.handleDropPatchFile)
		user.DELETE("/drop/files/:fileId", s.handleDropDeleteFile)
		user.GET("/drop/usage", s.handleDropUsage)
		user.GET("/drop/crypto/vault", s.handleDropGetVault)
		user.PUT("/drop/crypto/vault", s.handleDropPutVault)
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
	admin.Use(s.authUser(), s.requireAdmin(), s.activityLog())
	{
		admin.GET("/stats", s.handleAdminStats)
		admin.GET("/activity", s.handleAdminActivity)
		admin.GET("/nodes", s.handleAdminNodes)
		admin.GET("/users", s.handleAdminUsers)
		admin.GET("/subscriptions", s.handleAdminSubscriptions)
		admin.GET("/orgs", s.handleAdminOrgs)
		admin.PATCH("/orgs/:id", s.handleAdminPatchOrg)
		admin.GET("/orgs/:id/usage", s.handleAdminOrgUsage)
		admin.GET("/nodes/:id/metrics", s.handleAdminNodeMetrics)
		admin.POST("/nodes/:id/command", s.handleAdminNodeCommand)
		admin.POST("/nodes/:id/min_tier", s.handleAdminSetNodeMinTier)
		admin.POST("/perks", s.handleAdminUpsertPerk)
		admin.POST("/perks/:id/grant", s.handleAdminGrantPerk)
		admin.GET("/settings", s.handleAdminListSettings)
		admin.PATCH("/settings", s.handleAdminPatchSettings)
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

// splitCSVRaw is splitCSV without the "*" fallback: empty input yields nil (used
// for trusted proxies, where nil means "trust none").
func splitCSVRaw(s string) []string {
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
		return nil
	}
	return out
}
